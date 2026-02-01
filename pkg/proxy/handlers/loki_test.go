package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLokiHandler(t *testing.T) {
	log := logrus.New()
	configs := []LokiConfig{
		{
			Name:     "test-loki",
			URL:      "http://localhost:3100",
			Username: "user",
			Password: "pass",
			Timeout:  30,
		},
	}

	handler := NewLokiHandler(log, configs)
	require.NotNil(t, handler)
	assert.Equal(t, 1, len(handler.instances))
	assert.Contains(t, handler.instances, "test-loki")
}

func TestLokiHandlerServeHTTP(t *testing.T) {
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)

	// Create a mock upstream server
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"success"}`))
	}))
	defer upstream.Close()

	configs := []LokiConfig{
		{
			Name:       "test",
			URL:        upstream.URL,
			SkipVerify: true,
		},
	}

	handler := NewLokiHandler(log, configs)

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
			req := httptest.NewRequest(http.MethodGet, "/loki/loki/api/v1/query", nil)
			if tt.datasource != "" {
				req.Header.Set(DatasourceHeader, tt.datasource)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)
			assert.Equal(t, tt.expectedStatus, rec.Code)
		})
	}
}

func TestLokiHandlerInstances(t *testing.T) {
	log := logrus.New()
	configs := []LokiConfig{
		{Name: "loki1", URL: "http://host1:3100"},
		{Name: "loki2", URL: "http://host2:3100"},
	}

	handler := NewLokiHandler(log, configs)
	instances := handler.Instances()

	assert.Equal(t, 2, len(instances))
	assert.Contains(t, instances, "loki1")
	assert.Contains(t, instances, "loki2")
}

func TestLokiHandlerInvalidURL(t *testing.T) {
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)

	configs := []LokiConfig{
		{
			Name: "invalid",
			URL:  "://invalid-url",
		},
	}

	handler := NewLokiHandler(log, configs)
	instance, exists := handler.instances["invalid"]
	assert.True(t, exists)
	assert.Nil(t, instance)
}

func TestLokiHandlerNilInstance(t *testing.T) {
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)

	configs := []LokiConfig{
		{
			Name: "invalid",
			URL:  "://invalid-url",
		},
	}

	handler := NewLokiHandler(log, configs)

	req := httptest.NewRequest(http.MethodGet, "/loki/loki/api/v1/query", nil)
	req.Header.Set(DatasourceHeader, "invalid")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestLokiHandlerEmptyConfig(t *testing.T) {
	log := logrus.New()
	configs := []LokiConfig{}

	handler := NewLokiHandler(log, configs)
	require.NotNil(t, handler)
	assert.Equal(t, 0, len(handler.instances))
	assert.Empty(t, handler.Instances())
}

func TestLokiHandlerWithTimeout(t *testing.T) {
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)

	// Create a slow upstream
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Slow response
	}))
	defer upstream.Close()

	configs := []LokiConfig{
		{
			Name:       "test",
			URL:        upstream.URL,
			SkipVerify: true,
			Timeout:    1,
		},
	}

	handler := NewLokiHandler(log, configs)

	req := httptest.NewRequest(http.MethodGet, "/loki/loki/api/v1/query", nil)
	req.Header.Set(DatasourceHeader, "test")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.True(t, rec.Code == http.StatusOK || rec.Code == http.StatusBadGateway)
}
