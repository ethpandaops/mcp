package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClickHouseHandler(t *testing.T) {
	log := logrus.New()
	configs := []ClickHouseConfig{
		{
			Name:     "test-cluster",
			Host:     "localhost",
			Port:     8123,
			Database: "testdb",
			Username: "user",
			Password: "pass",
			Secure:   false,
			Timeout:  30,
		},
	}

	handler := NewClickHouseHandler(log, configs)
	require.NotNil(t, handler)
	assert.Equal(t, 1, len(handler.clusters))
	assert.Contains(t, handler.clusters, "test-cluster")
}

func TestClickHouseHandlerServeHTTP(t *testing.T) {
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)

	// Create a mock upstream server
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer upstream.Close()

	configs := []ClickHouseConfig{
		{
			Name:       "test",
			Host:       upstream.Listener.Addr().String(),
			Port:       80,
			Database:   "testdb",
			Username:   "user",
			Password:   "pass",
			SkipVerify: true,
		},
	}

	handler := NewClickHouseHandler(log, configs)

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
			req := httptest.NewRequest(http.MethodGet, "/clickhouse/", nil)
			if tt.datasource != "" {
				req.Header.Set(DatasourceHeader, tt.datasource)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)
			assert.Equal(t, tt.expectedStatus, rec.Code)
		})
	}
}

func TestClickHouseHandlerClusters(t *testing.T) {
	log := logrus.New()
	configs := []ClickHouseConfig{
		{Name: "cluster1", Host: "host1", Port: 8123},
		{Name: "cluster2", Host: "host2", Port: 8123},
		{Name: "cluster3", Host: "host3", Port: 8123},
	}

	handler := NewClickHouseHandler(log, configs)
	clusters := handler.Clusters()

	assert.Equal(t, 3, len(clusters))
	assert.Contains(t, clusters, "cluster1")
	assert.Contains(t, clusters, "cluster2")
	assert.Contains(t, clusters, "cluster3")
}

func TestClickHouseHandlerEmptyConfig(t *testing.T) {
	log := logrus.New()
	configs := []ClickHouseConfig{}

	handler := NewClickHouseHandler(log, configs)
	require.NotNil(t, handler)
	assert.Equal(t, 0, len(handler.clusters))
	assert.Empty(t, handler.Clusters())
}

func TestClickHouseHandlerWithTimeout(t *testing.T) {
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)

	// Create a slow upstream that will timeout
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Don't respond, let it timeout
	}))
	defer upstream.Close()

	configs := []ClickHouseConfig{
		{
			Name:       "test",
			Host:       upstream.Listener.Addr().String(),
			Port:       80,
			Database:   "testdb",
			SkipVerify: true,
			Timeout:    1, // 1 second timeout
		},
	}

	handler := NewClickHouseHandler(log, configs)

	req := httptest.NewRequest(http.MethodGet, "/clickhouse/", nil)
	req.Header.Set(DatasourceHeader, "test")
	rec := httptest.NewRecorder()

	// This should either succeed or fail with a timeout/gateway error
	handler.ServeHTTP(rec, req)
	// We just verify the handler processes the request
	assert.True(t, rec.Code == http.StatusOK || rec.Code == http.StatusBadGateway)
}
