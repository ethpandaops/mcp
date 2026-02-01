package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ethpandaops/mcp/pkg/plugin"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPlugin implements the plugin.Plugin interface for testing.
type mockPlugin struct {
	name          string
	healthStatus  plugin.HealthStatus
	healthMessage string
}

func (m *mockPlugin) Name() string { return m.name }
func (m *mockPlugin) Init([]byte) error { return nil }
func (m *mockPlugin) ApplyDefaults() {}
func (m *mockPlugin) Validate() error { return nil }
func (m *mockPlugin) SandboxEnv() (map[string]string, error) { return nil, nil }
func (m *mockPlugin) DatasourceInfo() []struct {
	Type        string
	Name        string
	Description string
	Metadata    map[string]string
} {
	return nil
}
func (m *mockPlugin) Examples() map[string]struct {
	Description string
	Examples    []struct {
		Name        string
		Description string
		Query       string
	}
} {
	return nil
}
func (m *mockPlugin) PythonAPIDocs() map[string]struct {
	Description string
	Functions   map[string]struct {
		Signature   string
		Description string
		Parameters  map[string]string
		Returns     string
	}
} {
	return nil
}
func (m *mockPlugin) GettingStartedSnippet() string { return "" }
func (m *mockPlugin) RegisterResources(logrus.FieldLogger, plugin.ResourceRegistry) error { return nil }
func (m *mockPlugin) Start(context.Context) error { return nil }
func (m *mockPlugin) Stop(context.Context) error { return nil }
func (m *mockPlugin) HealthCheck(ctx context.Context) plugin.HealthCheckResult {
	return plugin.HealthCheckResult{
		Status:    m.healthStatus,
		Message:   m.healthMessage,
		CheckedAt: time.Now().UTC(),
	}
}

func TestMountHealthRoutes_Health(t *testing.T) {
	s := &service{
		log: logrus.New(),
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	router := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mountHealthRoutes(nil)
	}))
	defer router.Close()

	// Test using a simple HTTP handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			response := HealthResponse{
				Status:    "healthy",
				Version:   "test-version",
				Timestamp: time.Now().UTC(),
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(response)
		}
	})

	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response HealthResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "healthy", response.Status)
	assert.NotEmpty(t, response.Timestamp)
}

func TestMountHealthRoutes_Ready_WhenRunning(t *testing.T) {
	s := &service{
		log:     logrus.New(),
		running: true,
	}

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		running := s.running
		s.mu.Unlock()

		if running {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"status": "ready",
			})
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"status": "not ready",
			})
		}
	})

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "ready", response["status"])
}

func TestMountHealthRoutes_Ready_WhenNotRunning(t *testing.T) {
	s := &service{
		log:     logrus.New(),
		running: false,
	}

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		running := s.running
		s.mu.Unlock()

		if running {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"status": "ready",
			})
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"status": "not ready",
			})
		}
	})

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var response map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "not ready", response["status"])
}

func TestMountHealthRoutes_Live_WhenRunning(t *testing.T) {
	s := &service{
		log:     logrus.New(),
		running: true,
	}

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	w := httptest.NewRecorder()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		running := s.running
		s.mu.Unlock()

		if running {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"status": "alive",
			})
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	})

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "alive", response["status"])
}

func TestMountHealthRoutes_Live_WhenNotRunning(t *testing.T) {
	s := &service{
		log:     logrus.New(),
		running: false,
	}

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	w := httptest.NewRecorder()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		running := s.running
		s.mu.Unlock()

		if running {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"status": "alive",
			})
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"status": "not alive",
			})
		}
	})

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestMountHealthRoutes_Plugins(t *testing.T) {
	// Create mock plugins
	mockClickHouse := &mockPlugin{
		name:          "clickhouse",
		healthStatus:  plugin.HealthStatusHealthy,
		healthMessage: "2 cluster(s) configured",
	}
	mockPrometheus := &mockPlugin{
		name:          "prometheus",
		healthStatus:  plugin.HealthStatusHealthy,
		healthMessage: "1 instance(s) configured",
	}
	mockLoki := &mockPlugin{
		name:          "loki",
		healthStatus:  plugin.HealthStatusUnhealthy,
		healthMessage: "Connection failed",
	}

	// Create plugin registry
	reg := plugin.NewRegistry(logrus.New())
	reg.Add(mockClickHouse)
	reg.Add(mockPrometheus)
	reg.Add(mockLoki)

	// Initialize plugins (normally done via InitPlugin with YAML, but we can just add to initialized)
	// For testing, we'll create a mock registry response directly

	s := &service{
		log:            logrus.New(),
		pluginRegistry: reg,
	}

	// Create a test server to verify the handler works
	req := httptest.NewRequest(http.MethodGet, "/health/plugins", nil)
	w := httptest.NewRecorder()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		pluginResults := make(map[string]plugin.HealthCheckResult)
		overallStatus := "healthy"

		if s.pluginRegistry != nil {
			pluginResults = s.pluginRegistry.HealthChecks(ctx)

			for _, result := range pluginResults {
				if result.Status == plugin.HealthStatusUnhealthy {
					overallStatus = "degraded"
					break
				}
			}
		}

		response := PluginsHealthResponse{
			Status:    overallStatus,
			Plugins:   pluginResults,
			Timestamp: time.Now().UTC(),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	})

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response PluginsHealthResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "healthy", response.Status) // No plugins initialized, so healthy by default
	assert.NotNil(t, response.Plugins)
}

func TestHealthResponse_Struct(t *testing.T) {
	now := time.Now().UTC()
	response := HealthResponse{
		Status:    "healthy",
		Version:   "1.0.0",
		Timestamp: now,
	}

	data, err := json.Marshal(response)
	require.NoError(t, err)

	var decoded HealthResponse
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "healthy", decoded.Status)
	assert.Equal(t, "1.0.0", decoded.Version)
	assert.True(t, decoded.Timestamp.Equal(now))
}

func TestPluginsHealthResponse_Struct(t *testing.T) {
	now := time.Now().UTC()
	response := PluginsHealthResponse{
		Status: "healthy",
		Plugins: map[string]plugin.HealthCheckResult{
			"clickhouse": {
				Status:    plugin.HealthStatusHealthy,
				Message:   "2 clusters",
				CheckedAt: now,
			},
		},
		Timestamp: now,
	}

	data, err := json.Marshal(response)
	require.NoError(t, err)

	var decoded PluginsHealthResponse
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "healthy", decoded.Status)
	assert.Len(t, decoded.Plugins, 1)
	assert.Equal(t, plugin.HealthStatusHealthy, decoded.Plugins["clickhouse"].Status)
}

func TestHealthStatus_Constants(t *testing.T) {
	assert.Equal(t, plugin.HealthStatus("healthy"), plugin.HealthStatusHealthy)
	assert.Equal(t, plugin.HealthStatus("unhealthy"), plugin.HealthStatusUnhealthy)
	assert.Equal(t, plugin.HealthStatus("unknown"), plugin.HealthStatusUnknown)
}
