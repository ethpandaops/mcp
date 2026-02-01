package handlers

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewS3Handler(t *testing.T) {
	log := logrus.New()
	cfg := &S3Config{
		Endpoint:        "http://localhost:9000",
		AccessKey:       "test-key",
		SecretKey:       "test-secret",
		Bucket:          "test-bucket",
		Region:          "us-east-1",
		PublicURLPrefix: "https://cdn.example.com",
	}

	handler := NewS3Handler(log, cfg)
	require.NotNil(t, handler)
	assert.Equal(t, cfg, handler.cfg)
	assert.NotNil(t, handler.signer)
	assert.NotNil(t, handler.credentials)
	assert.NotNil(t, handler.httpClient)
	assert.Equal(t, "https://cdn.example.com", handler.publicURLPrefix)
}

func TestNewS3HandlerNilConfig(t *testing.T) {
	log := logrus.New()
	handler := NewS3Handler(log, nil)
	assert.Nil(t, handler)
}

func TestS3HandlerBucket(t *testing.T) {
	log := logrus.New()

	tests := []struct {
		name         string
		cfg          *S3Config
		expectedName string
	}{
		{
			name: "with bucket",
			cfg: &S3Config{
				Bucket: "my-bucket",
			},
			expectedName: "my-bucket",
		},
		{
			name:         "nil config",
			cfg:          nil,
			expectedName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewS3Handler(log, tt.cfg)
			if tt.cfg == nil {
				assert.Nil(t, handler)
			} else {
				assert.Equal(t, tt.expectedName, handler.Bucket())
			}
		})
	}
}

func TestS3HandlerPublicURLPrefix(t *testing.T) {
	log := logrus.New()

	cfg := &S3Config{
		PublicURLPrefix: "https://cdn.example.com",
	}

	handler := NewS3Handler(log, cfg)
	assert.Equal(t, "https://cdn.example.com", handler.PublicURLPrefix())
}

func TestS3HandlerGetPublicURL(t *testing.T) {
	log := logrus.New()

	tests := []struct {
		name         string
		prefix       string
		endpoint     string
		bucket       string
		key          string
		expectedURL  string
	}{
		{
			name:        "with public URL prefix",
			prefix:      "https://cdn.example.com",
			bucket:      "my-bucket",
			key:         "path/to/file.txt",
			expectedURL: "https://cdn.example.com/path/to/file.txt",
		},
		{
			name:         "without public URL prefix",
			prefix:       "",
			endpoint:     "http://localhost:9000",
			bucket:       "my-bucket",
			key:          "file.txt",
			expectedURL:  "http://localhost:9000/my-bucket/file.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &S3Config{
				Endpoint:        tt.endpoint,
				PublicURLPrefix: tt.prefix,
			}
			handler := NewS3Handler(log, cfg)
			url := handler.GetPublicURL(context.Background(), tt.bucket, tt.key)
			assert.Equal(t, tt.expectedURL, url)
		})
	}
}

func TestS3HandlerServeHTTP(t *testing.T) {
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)

	// Create a mock S3 server
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ETag":"\"abc123\""}`))
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("file content"))
		case http.MethodHead:
			w.Header().Set("Content-Length", "12")
			w.WriteHeader(http.StatusOK)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer upstream.Close()

	cfg := &S3Config{
		Endpoint:  upstream.URL,
		AccessKey: "test-key",
		SecretKey: "test-secret",
		Bucket:    "test-bucket",
		Region:    "us-east-1",
	}

	handler := NewS3Handler(log, cfg)

	tests := []struct {
		name           string
		method         string
		path           string
		body           []byte
		expectedStatus int
	}{
		{
			name:           "GET request",
			method:         http.MethodGet,
			path:           "/s3/test-bucket/file.txt",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "PUT request",
			method:         http.MethodPut,
			path:           "/s3/test-bucket/file.txt",
			body:           []byte("test content"),
			expectedStatus: http.StatusOK,
		},
		{
			name:           "HEAD request",
			method:         http.MethodHead,
			path:           "/s3/test-bucket/file.txt",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "DELETE request",
			method:         http.MethodDelete,
			path:           "/s3/test-bucket/file.txt",
			expectedStatus: http.StatusNoContent,
		},
		{
			name:           "missing bucket",
			method:         http.MethodGet,
			path:           "/s3/",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body io.Reader
			if tt.body != nil {
				body = bytes.NewReader(tt.body)
			}

			req := httptest.NewRequest(tt.method, tt.path, body)
			if tt.body != nil {
				req.Header.Set("Content-Type", "application/octet-stream")
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)
			assert.Equal(t, tt.expectedStatus, rec.Code)
		})
	}
}

func TestS3HandlerServeHTTPNilConfig(t *testing.T) {
	log := logrus.New()

	// Create a handler with nil config
	handler := &S3Handler{
		log: log,
		cfg: nil,
	}

	req := httptest.NewRequest(http.MethodGet, "/s3/test/file.txt", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}
