// Package handlers provides reverse proxy handlers for each datasource type.
package handlers

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/sirupsen/logrus"
)

// CBTConfig holds CBT proxy configuration for a single instance.
type CBTConfig struct {
	Name     string
	URL      string
	Timeout  int
}

// CBTHandler handles requests to CBT instances.
type CBTHandler struct {
	log       logrus.FieldLogger
	instances map[string]*cbtInstance
}

type cbtInstance struct {
	cfg   CBTConfig
	proxy *httputil.ReverseProxy
}

// NewCBTHandler creates a new CBT handler.
func NewCBTHandler(log logrus.FieldLogger, configs []CBTConfig) *CBTHandler {
	h := &CBTHandler{
		log:       log.WithField("handler", "cbt"),
		instances: make(map[string]*cbtInstance, len(configs)),
	}

	for _, cfg := range configs {
		h.instances[cfg.Name] = h.createInstance(cfg)
	}

	return h
}

func (h *CBTHandler) createInstance(cfg CBTConfig) *cbtInstance {
	// Build target URL.
	targetURL, err := url.Parse(cfg.URL)
	if err != nil {
		// Log the error but continue - the proxy will fail at request time.
		h.log.WithError(err).WithField("instance", cfg.Name).Error("Failed to parse CBT URL")
		return &cbtInstance{cfg: cfg}
	}

	// Create reverse proxy.
	rp := httputil.NewSingleHostReverseProxy(targetURL)

	// Configure transport.
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90,
	}
	rp.Transport = transport

	// Customize the director to handle the path correctly.
	originalDirector := rp.Director
	rp.Director = func(req *http.Request) {
		originalDirector(req)

		// Remove the sandbox's Authorization header (Bearer token) before adding our own.
		req.Header.Del("Authorization")

		// Strip /cbt prefix from path, keep the rest for the upstream.
		path := strings.TrimPrefix(req.URL.Path, "/cbt")
		if path == "" {
			path = "/"
		}
		req.URL.Path = path

		// Set req.Host to the target host.
		req.Host = req.URL.Host
		req.Header.Del("Host")
	}

	// Error handler.
	rp.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		h.log.WithError(err).WithField("instance", cfg.Name).Error("CBT proxy error")
		http.Error(w, "cbt proxy error: "+err.Error(), http.StatusBadGateway)
	}

	return &cbtInstance{
		cfg:   cfg,
		proxy: rp,
	}
}

// ServeHTTP handles CBT requests. The instance is specified via X-Datasource header.
func (h *CBTHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Extract instance name from header.
	instanceName := r.Header.Get(DatasourceHeader)
	if instanceName == "" {
		http.Error(w, "missing "+DatasourceHeader+" header", http.StatusBadRequest)
		return
	}

	instance, ok := h.instances[instanceName]
	if !ok {
		http.Error(w, "unknown instance: "+instanceName, http.StatusNotFound)
		return
	}

	if instance.proxy == nil {
		http.Error(w, "CBT instance not properly configured: "+instanceName, http.StatusInternalServerError)
		return
	}

	h.log.WithFields(logrus.Fields{
		"instance": instanceName,
		"path":     r.URL.Path,
		"method":   r.Method,
	}).Debug("Proxying CBT request")

	instance.proxy.ServeHTTP(w, r)
}

// Instances returns the list of configured instance names.
func (h *CBTHandler) Instances() []string {
	names := make([]string, 0, len(h.instances))
	for name := range h.instances {
		names = append(names, name)
	}
	return names
}
