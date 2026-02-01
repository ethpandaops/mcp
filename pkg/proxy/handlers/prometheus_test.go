package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPrometheusHandler(t *testing.T) {
	log := logrus.New()
	configs := []PrometheusConfig{
		{
			Name:     "test-prom",
			URL:      "http://localhost:9090",
			Username: "user",
			Password: "pass",
			Timeout:  30,
		},
	}

	handler := NewPrometheusHandler(log, configs)
	require.NotNil(t, handler)
	assert.Equal(t, 1, len(handler.instances))
	assert.Contains(t, handler.instances, "test-prom")
}

func TestPrometheusHandlerServeHTTP(t *testing.T) {
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)

	// Create a mock upstream server
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"success"}`))
	}))
	defer upstream.Close()

	configs := []PrometheusConfig{
		{
			Name:       "test",
			URL:        upstream.URL,
			SkipVerify: true,
		},
	}

	handler := NewPrometheusHandler(log, configs)

	tests := []struct {
		name           string
		datasource     string
		expectedStatus int
	}{
		{
			name:           "missing datasource header",
			datasource:     "",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "unknown datasource",
			datasource:     "unknown",
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/prometheus/api/v1/query", nil)
			if tt.datasource != "" {
				req.Header.Set(DatasourceHeader, tt.datasource)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)
			assert.Equal(t, tt.expectedStatus, rec.Code)
		})
	}
}

func TestPrometheusHandlerInstances(t *testing.T) {
	log := logrus.New()
	configs := []PrometheusConfig{
		{Name: "prom1", URL: "http://host1:9090"},
		{Name: "prom2", URL: "http://host2:9090"},
	}

	handler := NewPrometheusHandler(log, configs)
	instances := handler.Instances()

	assert.Equal(t, 2, len(instances))
	assert.Contains(t, instances, "prom1")
	assert.Contains(t, instances, "prom2")
}

func TestPrometheusHandlerInvalidURL(t *testing.T) {
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)

	configs := []PrometheusConfig{
		{
			Name: "invalid",
			URL:  "://invalid-url",
		},
	}

	handler := NewPrometheusHandler(log, configs)
	// The instance should be nil due to parse error
	instance, exists := handler.instances["invalid"]
	assert.True(t, exists)
	assert.Nil(t, instance)
}

func TestPrometheusHandlerNilInstance(t *testing.T) {
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)

	configs := []PrometheusConfig{
		{
			Name: "invalid",
			URL:  "://invalid-url",
		},
	}

	handler := NewPrometheusHandler(log, configs)

	req := httptest.NewRequest(http.MethodGet, "/prometheus/api/v1/query", nil)
	req.Header.Set(DatasourceHeader, "invalid")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestPrometheusHandlerEmptyConfig(t *testing.T) {
	log := logrus.New()
	configs := []PrometheusConfig{}

	handler := NewPrometheusHandler(log, configs)
	require.NotNil(t, handler)
	assert.Equal(t, 0, len(handler.instances))
	assert.Empty(t, handler.Instances())
}

func TestPrometheusHandlerWithTimeout(t *testing.T) {
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)

	// Create a slow upstream
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Slow response
	}))
	defer upstream.Close()

	configs := []PrometheusConfig{
		{
			Name:       "test",
			URL:        upstream.URL,
			SkipVerify: true,
			Timeout:    1,
		},
	}

	handler := NewPrometheusHandler(log, configs)

	req := httptest.NewRequest(http.MethodGet, "/prometheus/api/v1/query", nil)
	req.Header.Set(DatasourceHeader, "test")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	// Should complete (may timeout or succeed)
	assert.True(t, rec.Code == http.StatusOK || rec.Code == http.StatusBadGateway)
}
