package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/mcp/pkg/proxy/handlers"
)

// StandaloneService is the standalone credential proxy service for K8s deployment.
type StandaloneService interface {
	// Start starts the proxy server.
	Start(ctx context.Context) error

	// Stop stops the proxy server.
	Stop(ctx context.Context) error

	// URL returns the proxy URL.
	URL() string

	// ClickHouseDatasources returns the list of ClickHouse datasource names.
	ClickHouseDatasources() []string

	// PrometheusDatasources returns the list of Prometheus datasource names.
	PrometheusDatasources() []string

	// LokiDatasources returns the list of Loki datasource names.
	LokiDatasources() []string

	// S3Bucket returns the configured S3 bucket name.
	S3Bucket() string
}

// standaloneService implements the StandaloneService interface.
type standaloneService struct {
	log    logrus.FieldLogger
	cfg    StandaloneConfig
	server *http.Server
	mux    *http.ServeMux

	authenticator Authenticator
	rateLimiter   *RateLimiter
	auditor       *Auditor

	clickhouseHandler *handlers.ClickHouseHandler
	prometheusHandler *handlers.PrometheusHandler
	lokiHandler       *handlers.LokiHandler
	s3Handler         *handlers.S3Handler

	mu      sync.RWMutex
	started bool
}

// Compile-time interface check.
var _ StandaloneService = (*standaloneService)(nil)

// NewStandalone creates a new standalone proxy service.
func NewStandalone(log logrus.FieldLogger, cfg StandaloneConfig) (StandaloneService, error) {
	s := &standaloneService{
		log: log.WithField("component", "proxy"),
		cfg: cfg,
		mux: http.NewServeMux(),
	}

	// Create authenticator based on mode.
	switch cfg.Auth.Mode {
	case AuthModeToken:
		tokens := NewTokenStore(cfg.Auth.TokenTTL)
		s.authenticator = NewTokenAuthenticator(log, tokens)
	case AuthModeJWT:
		if cfg.Auth.JWT == nil {
			return nil, fmt.Errorf("JWT config is required for JWT auth mode")
		}

		validator := NewJWTValidator(log, *cfg.Auth.JWT)
		s.authenticator = NewJWTAuthenticator(log, validator)
	default:
		return nil, fmt.Errorf("unsupported auth mode: %s", cfg.Auth.Mode)
	}

	// Create rate limiter if enabled.
	if cfg.RateLimiting.Enabled {
		s.rateLimiter = NewRateLimiter(log, RateLimiterConfig{
			RequestsPerMinute: cfg.RateLimiting.RequestsPerMinute,
			BurstSize:         cfg.RateLimiting.BurstSize,
		})
	}

	// Create auditor if enabled.
	if cfg.Audit.Enabled {
		s.auditor = NewAuditor(log, AuditorConfig{
			LogQueries:     cfg.Audit.LogQueries,
			MaxQueryLength: cfg.Audit.MaxQueryLength,
		})
	}

	// Create handlers from config.
	chConfigs, promConfigs, lokiConfigs, s3Config := cfg.ToHandlerConfigs()

	if len(chConfigs) > 0 {
		s.clickhouseHandler = handlers.NewClickHouseHandler(log, chConfigs)
	}

	if len(promConfigs) > 0 {
		s.prometheusHandler = handlers.NewPrometheusHandler(log, promConfigs)
	}

	if len(lokiConfigs) > 0 {
		s.lokiHandler = handlers.NewLokiHandler(log, lokiConfigs)
	}

	if s3Config != nil && s3Config.Endpoint != "" {
		s.s3Handler = handlers.NewS3Handler(log, s3Config)
	}

	// Register routes.
	s.registerRoutes()

	return s, nil
}

// registerRoutes sets up the HTTP routes.
func (s *standaloneService) registerRoutes() {
	// Health check endpoint (no auth required).
	s.mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Ready check endpoint (no auth required).
	s.mux.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.RLock()
		ready := s.started
		s.mu.RUnlock()

		if ready {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ready"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("not ready"))
		}
	})

	// Datasources info endpoint (for Python modules to discover available datasources).
	s.mux.HandleFunc("/datasources", s.handleDatasources)

	// Build middleware chain.
	chain := s.buildMiddlewareChain()

	// Authenticated routes.
	if s.clickhouseHandler != nil {
		s.mux.Handle("/clickhouse/", chain(s.clickhouseHandler))
	}

	if s.prometheusHandler != nil {
		s.mux.Handle("/prometheus/", chain(s.prometheusHandler))
	}

	if s.lokiHandler != nil {
		s.mux.Handle("/loki/", chain(s.lokiHandler))
	}

	if s.s3Handler != nil {
		s.mux.Handle("/s3/", chain(s.s3Handler))
	}
}

// buildMiddlewareChain builds the middleware chain for authenticated routes.
func (s *standaloneService) buildMiddlewareChain() func(http.Handler) http.Handler {
	return func(handler http.Handler) http.Handler {
		h := handler

		// Audit logging (innermost).
		if s.auditor != nil {
			h = s.auditor.Middleware()(h)
		}

		// Rate limiting.
		if s.rateLimiter != nil {
			h = s.rateLimiter.Middleware()(h)
		}

		// Authentication (outermost).
		h = s.authenticator.Middleware()(h)

		return h
	}
}

// handleDatasources returns the list of available datasources.
func (s *standaloneService) handleDatasources(w http.ResponseWriter, _ *http.Request) {
	info := struct {
		ClickHouse []string `json:"clickhouse"`
		Prometheus []string `json:"prometheus"`
		Loki       []string `json:"loki"`
		S3Bucket   string   `json:"s3_bucket,omitempty"`
	}{
		ClickHouse: s.ClickHouseDatasources(),
		Prometheus: s.PrometheusDatasources(),
		Loki:       s.LokiDatasources(),
		S3Bucket:   s.S3Bucket(),
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(info); err != nil {
		http.Error(w, fmt.Sprintf("failed to encode response: %v", err), http.StatusInternalServerError)
	}
}

// Start starts the proxy server.
func (s *standaloneService) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return fmt.Errorf("proxy already started")
	}

	// Start authenticator.
	if err := s.authenticator.Start(ctx); err != nil {
		return fmt.Errorf("starting authenticator: %w", err)
	}

	// Create listener first to detect port conflicts immediately.
	listener, err := net.Listen("tcp", s.cfg.Server.ListenAddr)
	if err != nil {
		return fmt.Errorf("binding to %s: %w", s.cfg.Server.ListenAddr, err)
	}

	s.server = &http.Server{
		Handler:           s.mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       s.cfg.Server.ReadTimeout,
		WriteTimeout:      s.cfg.Server.WriteTimeout,
		IdleTimeout:       s.cfg.Server.IdleTimeout,
		BaseContext:       func(_ net.Listener) context.Context { return ctx },
	}

	s.log.WithField("addr", s.cfg.Server.ListenAddr).Info("Starting standalone proxy server")

	// Start server in background with the already-bound listener.
	go func() {
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			s.log.WithError(err).Error("Proxy server error")
		}
	}()

	s.started = true

	return nil
}

// Stop stops the proxy server.
func (s *standaloneService) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return nil
	}

	// Stop authenticator.
	if err := s.authenticator.Stop(); err != nil {
		s.log.WithError(err).Warn("Error stopping authenticator")
	}

	// Stop rate limiter.
	if s.rateLimiter != nil {
		s.rateLimiter.Stop()
	}

	// Shutdown HTTP server.
	if s.server != nil {
		if err := s.server.Shutdown(ctx); err != nil {
			return fmt.Errorf("shutting down proxy server: %w", err)
		}
	}

	s.started = false
	s.log.Info("Standalone proxy server stopped")

	return nil
}

// URL returns the proxy URL.
func (s *standaloneService) URL() string {
	// Extract port from listen address.
	port := "18081"
	if _, p, err := net.SplitHostPort(s.cfg.Server.ListenAddr); err == nil && p != "" {
		port = p
	}

	return fmt.Sprintf("http://localhost:%s", port)
}

// ClickHouseDatasources returns the list of ClickHouse datasource names.
func (s *standaloneService) ClickHouseDatasources() []string {
	if s.clickhouseHandler == nil {
		return nil
	}

	return s.clickhouseHandler.Clusters()
}

// PrometheusDatasources returns the list of Prometheus datasource names.
func (s *standaloneService) PrometheusDatasources() []string {
	if s.prometheusHandler == nil {
		return nil
	}

	return s.prometheusHandler.Instances()
}

// LokiDatasources returns the list of Loki datasource names.
func (s *standaloneService) LokiDatasources() []string {
	if s.lokiHandler == nil {
		return nil
	}

	return s.lokiHandler.Instances()
}

// S3Bucket returns the configured S3 bucket name.
func (s *standaloneService) S3Bucket() string {
	if s.s3Handler == nil {
		return ""
	}

	return s.s3Handler.Bucket()
}
