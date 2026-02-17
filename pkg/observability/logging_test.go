package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateRequestID(t *testing.T) {
	id1 := GenerateRequestID()
	id2 := GenerateRequestID()

	assert.NotEmpty(t, id1)
	assert.NotEmpty(t, id2)
	assert.NotEqual(t, id1, id2, "Request IDs should be unique")
	assert.Len(t, id1, 36, "Request ID should be a UUID (36 characters)")
}

func TestConfigureLogger(t *testing.T) {
	tests := []struct {
		name      string
		config    LoggerConfig
		wantLevel logrus.Level
	}{
		{
			name: "debug level",
			config: LoggerConfig{
				Level:  LogLevelDebug,
				Format: LogFormatText,
			},
			wantLevel: logrus.DebugLevel,
		},
		{
			name: "info level",
			config: LoggerConfig{
				Level:  LogLevelInfo,
				Format: LogFormatText,
			},
			wantLevel: logrus.InfoLevel,
		},
		{
			name: "warn level",
			config: LoggerConfig{
				Level:  LogLevelWarn,
				Format: LogFormatJSON,
			},
			wantLevel: logrus.WarnLevel,
		},
		{
			name: "error level",
			config: LoggerConfig{
				Level:  LogLevelError,
				Format: LogFormatJSON,
			},
			wantLevel: logrus.ErrorLevel,
		},
		{
			name: "default level for invalid",
			config: LoggerConfig{
				Level:  "invalid",
				Format: LogFormatText,
			},
			wantLevel: logrus.InfoLevel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, err := ConfigureLogger(tt.config)
			require.NoError(t, err)
			assert.Equal(t, tt.wantLevel, logger.Level)
		})
	}
}

func TestConfigureLoggerWithFileOutput(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-log-*.log")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	config := LoggerConfig{
		Level:      LogLevelInfo,
		Format:     LogFormatText,
		OutputPath: tmpFile.Name(),
	}

	logger, err := ConfigureLogger(config)
	require.NoError(t, err)

	logger.Info("test message")

	content, err := os.ReadFile(tmpFile.Name())
	require.NoError(t, err)
	assert.Contains(t, string(content), "test message")
}

func TestContextWithRequestID(t *testing.T) {
	ctx := context.Background()

	// Test with request ID
	ctx = WithRequestID(ctx, "test-request-id")
	assert.Equal(t, "test-request-id", GetRequestID(ctx))

	// Test with correlation ID
	ctx = WithCorrelationID(ctx, "test-correlation-id")
	assert.Equal(t, "test-correlation-id", GetCorrelationID(ctx))
}

func TestContextWithLogger(t *testing.T) {
	logger := logrus.New()
	ctx := context.Background()

	// Test setting and getting logger
	ctx = WithLogger(ctx, logger.WithField("test", "value"))
	retrievedLogger := GetLogger(ctx)
	assert.NotNil(t, retrievedLogger)
}

func TestLoggingMiddleware(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.NewFile(0, os.DevNull)) // Suppress output

	middleware := NewLoggingMiddleware(logger)

	handler := middleware.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))

		// Verify context has request ID
		requestID := GetRequestID(r.Context())
		assert.NotEmpty(t, requestID)

		// Verify context has logger
		ctxLogger := GetLogger(r.Context())
		assert.NotNil(t, ctxLogger)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test-path", nil)
	req.Header.Set("User-Agent", "test-agent")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.NotEmpty(t, rr.Header().Get(RequestIDHeader))
}

func TestLoggingMiddlewareWithExistingRequestID(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.NewFile(0, os.DevNull))

	middleware := NewLoggingMiddleware(logger)

	handler := middleware.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(RequestIDHeader, "existing-request-id")
	req.Header.Set(CorrelationIDHeader, "existing-correlation-id")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Verify existing IDs are preserved
	assert.Equal(t, "existing-request-id", rr.Header().Get(RequestIDHeader))
	assert.Equal(t, "existing-correlation-id", rr.Header().Get(CorrelationIDHeader))
}

func TestLoggingMiddlewareErrorStatus(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.NewFile(0, os.DevNull))

	middleware := NewLoggingMiddleware(logger)

	handler := middleware.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	req := httptest.NewRequest(http.MethodGet, "/error", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestResponseWriter(t *testing.T) {
	recorder := httptest.NewRecorder()
	rw := newResponseWriter(recorder)

	assert.Equal(t, http.StatusOK, rw.statusCode)

	rw.WriteHeader(http.StatusCreated)
	assert.Equal(t, http.StatusCreated, rw.statusCode)

	n, err := rw.Write([]byte("hello"))
	assert.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, 5, rw.size)
}

func TestSanitizeQuery(t *testing.T) {
	input := map[string][]string{
		"short":   {"value"},
		"long":    {strings.Repeat("a", 200)},
		"empty":   {},
		"special": {"hello world"},
	}

	result := sanitizeQuery(input)

	assert.Equal(t, "value", result["short"])
	assert.Equal(t, 103, len(result["long"])) // 100 + "..."
	assert.NotContains(t, result, "empty")
	assert.Equal(t, "hello world", result["special"])
}

func TestIsValidLogLevel(t *testing.T) {
	assert.True(t, IsValidLogLevel("debug"))
	assert.True(t, IsValidLogLevel("info"))
	assert.True(t, IsValidLogLevel("warn"))
	assert.True(t, IsValidLogLevel("error"))
	assert.True(t, IsValidLogLevel("DEBUG"))
	assert.False(t, IsValidLogLevel("invalid"))
	assert.False(t, IsValidLogLevel(""))
}

func TestIsValidLogFormat(t *testing.T) {
	assert.True(t, IsValidLogFormat("text"))
	assert.True(t, IsValidLogFormat("json"))
	assert.True(t, IsValidLogFormat("TEXT"))
	assert.False(t, IsValidLogFormat("invalid"))
	assert.False(t, IsValidLogFormat(""))
}

func TestRequestScopedLogger(t *testing.T) {
	baseLogger := logrus.New()
	ctx := context.Background()

	// Without context values
	logger := RequestScopedLogger(baseLogger, ctx)
	assert.NotNil(t, logger)

	// With request ID
	ctx = WithRequestID(ctx, "req-123")
	logger = RequestScopedLogger(baseLogger, ctx)
	assert.NotNil(t, logger)

	// With correlation ID
	ctx = WithCorrelationID(ctx, "corr-456")
	logger = RequestScopedLogger(baseLogger, ctx)
	assert.NotNil(t, logger)
}

func TestLogHTTPRequest(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.NewFile(0, os.DevNull))

	info := HTTPRequestInfo{
		Method:       http.MethodGet,
		Path:         "/test",
		Status:       http.StatusOK,
		Duration:     100 * time.Millisecond,
		DurationMs:   100,
		RequestID:    "req-123",
		Correlation:  "corr-456",
		UserAgent:    "test-agent",
		RemoteAddr:   "127.0.0.1",
		ResponseSize: 1024,
	}

	// Should not panic
	LogHTTPRequest(logger, info)

	// Test with error status
	info.Status = http.StatusInternalServerError
	LogHTTPRequest(logger, info)

	// Test with warning status
	info.Status = http.StatusBadRequest
	LogHTTPRequest(logger, info)
}

func TestLoggerFromContext(t *testing.T) {
	defaultLogger := logrus.New()

	// With nil context
	logger := LoggerFromContext(nil, defaultLogger)
	assert.Equal(t, defaultLogger, logger)

	// With empty context
	ctx := context.Background()
	logger = LoggerFromContext(ctx, defaultLogger)
	assert.Equal(t, defaultLogger, logger)

	// With logger in context
	ctxLogger := logrus.New()
	ctx = WithLogger(ctx, ctxLogger)
	logger = LoggerFromContext(ctx, defaultLogger)
	assert.Equal(t, ctxLogger, logger)
}

func TestDefaultLogger(t *testing.T) {
	logger := DefaultLogger()
	assert.NotNil(t, logger)
	assert.Equal(t, logrus.InfoLevel, logger.Level)
}

func TestLoggingConfigApplyDefaults(t *testing.T) {
	config := LoggingConfig{}
	config.ApplyDefaults()

	assert.Equal(t, LogLevelInfo, config.Level)
	assert.Equal(t, LogFormatText, config.Format)
}
