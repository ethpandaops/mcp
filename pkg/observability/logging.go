// Package observability provides logging capabilities for ethpandaops-mcp.
package observability

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// LogLevel represents the logging level.
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

// LogFormat represents the logging format.
type LogFormat string

const (
	LogFormatText LogFormat = "text"
	LogFormatJSON LogFormat = "json"
)

// contextKey is a type for context keys to avoid collisions.
type contextKey int

const (
	// RequestIDKey is the context key for request ID.
	RequestIDKey contextKey = iota
	// CorrelationIDKey is the context key for correlation ID.
	CorrelationIDKey
	// LoggerKey is the context key for the request-scoped logger.
	LoggerKey
)

// RequestIDHeader is the HTTP header for request ID propagation.
const RequestIDHeader = "X-Request-ID"

// CorrelationIDHeader is the HTTP header for correlation ID propagation.
const CorrelationIDHeader = "X-Correlation-ID"

// LoggerConfig holds configuration for the logger.
type LoggerConfig struct {
	Level      LogLevel  `yaml:"level"`
	Format     LogFormat `yaml:"format"`
	OutputPath string    `yaml:"output_path,omitempty"`
}

// Logger is the structured logger interface.
type Logger interface {
	logrus.FieldLogger
}

// DefaultLogger returns a new default logger.
func DefaultLogger() *logrus.Logger {
	logger := logrus.New()
	logger.SetOutput(os.Stdout)
	logger.SetLevel(logrus.InfoLevel)
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: time.RFC3339,
	})

	return logger
}

// ConfigureLogger configures the logger based on the provided config.
func ConfigureLogger(cfg LoggerConfig) (*logrus.Logger, error) {
	logger := logrus.New()

	// Set level.
	switch cfg.Level {
	case LogLevelDebug:
		logger.SetLevel(logrus.DebugLevel)
	case LogLevelInfo:
		logger.SetLevel(logrus.InfoLevel)
	case LogLevelWarn:
		logger.SetLevel(logrus.WarnLevel)
	case LogLevelError:
		logger.SetLevel(logrus.ErrorLevel)
	default:
		logger.SetLevel(logrus.InfoLevel)
	}

	// Set format.
	switch cfg.Format {
	case LogFormatJSON:
		logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339,
		})
	case LogFormatText, "":
		logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: time.RFC3339,
		})
	default:
		logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: time.RFC3339,
		})
	}

	// Set output.
	if cfg.OutputPath != "" {
		file, err := os.OpenFile(cfg.OutputPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o666)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file: %w", err)
		}
		logger.SetOutput(file)
	} else {
		logger.SetOutput(os.Stdout)
	}

	return logger, nil
}

// GenerateRequestID generates a new request ID.
func GenerateRequestID() string {
	return uuid.New().String()
}

// GetRequestID retrieves the request ID from context.
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(RequestIDKey).(string); ok {
		return id
	}
	return ""
}

// GetCorrelationID retrieves the correlation ID from context.
func GetCorrelationID(ctx context.Context) string {
	if id, ok := ctx.Value(CorrelationIDKey).(string); ok {
		return id
	}
	return ""
}

// WithRequestID adds a request ID to the context.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

// WithCorrelationID adds a correlation ID to the context.
func WithCorrelationID(ctx context.Context, correlationID string) context.Context {
	return context.WithValue(ctx, CorrelationIDKey, correlationID)
}

// WithLogger adds a logger to the context.
func WithLogger(ctx context.Context, logger logrus.FieldLogger) context.Context {
	return context.WithValue(ctx, LoggerKey, logger)
}

// GetLogger retrieves the logger from context, or returns a default logger.
func GetLogger(ctx context.Context) logrus.FieldLogger {
	if logger, ok := ctx.Value(LoggerKey).(logrus.FieldLogger); ok {
		return logger
	}
	return DefaultLogger()
}

// LoggingMiddleware creates an HTTP middleware that logs requests.
type LoggingMiddleware struct {
	logger *logrus.Logger
}

// NewLoggingMiddleware creates a new logging middleware.
func NewLoggingMiddleware(logger *logrus.Logger) *LoggingMiddleware {
	return &LoggingMiddleware{logger: logger}
}

// responseWriter is a wrapper around http.ResponseWriter to capture status code and response size.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	size, err := rw.ResponseWriter.Write(b)
	rw.size += size
	return size, err
}

// Middleware returns the HTTP middleware function.
func (m *LoggingMiddleware) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Extract or generate request ID.
			requestID := r.Header.Get(RequestIDHeader)
			if requestID == "" {
				requestID = GenerateRequestID()
			}

			// Extract correlation ID (can be empty).
			correlationID := r.Header.Get(CorrelationIDHeader)

			// Create request-scoped logger with fields.
			requestLogger := m.logger.WithFields(logrus.Fields{
				"request_id":     requestID,
				"correlation_id": correlationID,
			})

			// Add request ID and correlation ID to response headers.
			w.Header().Set(RequestIDHeader, requestID)
			if correlationID != "" {
				w.Header().Set(CorrelationIDHeader, correlationID)
			}

			// Create context with request info.
			ctx := r.Context()
			ctx = WithRequestID(ctx, requestID)
			ctx = WithCorrelationID(ctx, correlationID)
			ctx = WithLogger(ctx, requestLogger)

			// Wrap response writer to capture status code.
			rw := newResponseWriter(w)

			// Call next handler.
			next.ServeHTTP(rw, r.WithContext(ctx))

			// Calculate duration.
			duration := time.Since(start)

			// Build log entry.
			fields := logrus.Fields{
				"method":         r.Method,
				"path":           r.URL.Path,
				"status":         rw.statusCode,
				"duration_ms":    duration.Milliseconds(),
				"duration":       duration.String(),
				"request_id":     requestID,
				"correlation_id": correlationID,
				"user_agent":     r.UserAgent(),
				"remote_addr":    r.RemoteAddr,
				"size":           rw.size,
			}

			// Add query parameters if present (sanitized).
			if r.URL.RawQuery != "" {
				fields["query"] = sanitizeQuery(r.URL.Query())
			}

			// Log based on status code.
			switch {
			case rw.statusCode >= 500:
				requestLogger.WithFields(fields).Error("HTTP request error")
			case rw.statusCode >= 400:
				requestLogger.WithFields(fields).Warn("HTTP request warning")
			default:
				requestLogger.WithFields(fields).Info("HTTP request completed")
			}
		})
	}
}

// sanitizeQuery sanitizes query parameters by truncating long values.
func sanitizeQuery(values map[string][]string) map[string]string {
	result := make(map[string]string, len(values))
	for key, vals := range values {
		if len(vals) == 0 {
			continue
		}
		val := vals[0]
		if len(val) > 100 {
			val = val[:100] + "..."
		}
		result[key] = val
	}
	return result
}

// LoggingConfig holds logging configuration for the observability config.
type LoggingConfig struct {
	Level      LogLevel  `yaml:"level"`
	Format     LogFormat `yaml:"format"`
	OutputPath string    `yaml:"output_path,omitempty"`
}

// ApplyDefaults applies default values to logging config.
func (c *LoggingConfig) ApplyDefaults() {
	if c.Level == "" {
		c.Level = LogLevelInfo
	}
	if c.Format == "" {
		c.Format = LogFormatText
	}
}

// LoggerFromContext retrieves a logger from context or returns the default.
// This is a convenience function that matches the common pattern.
func LoggerFromContext(ctx context.Context, defaultLogger logrus.FieldLogger) logrus.FieldLogger {
	if ctx == nil {
		return defaultLogger
	}
	if logger, ok := ctx.Value(LoggerKey).(logrus.FieldLogger); ok {
		return logger
	}
	return defaultLogger
}

// RequestScopedLogger creates a logger with request context fields.
func RequestScopedLogger(base logrus.FieldLogger, ctx context.Context) logrus.FieldLogger {
	fields := logrus.Fields{}

	if requestID := GetRequestID(ctx); requestID != "" {
		fields["request_id"] = requestID
	}
	if correlationID := GetCorrelationID(ctx); correlationID != "" {
		fields["correlation_id"] = correlationID
	}

	if len(fields) > 0 {
		return base.WithFields(fields)
	}
	return base
}

// HTTPRequestInfo holds information about an HTTP request for logging.
type HTTPRequestInfo struct {
	Method       string        `json:"method"`
	Path         string        `json:"path"`
	Query        string        `json:"query,omitempty"`
	Status       int           `json:"status"`
	Duration     time.Duration `json:"duration"`
	DurationMs   int64         `json:"duration_ms"`
	RequestID    string        `json:"request_id"`
	Correlation  string        `json:"correlation_id,omitempty"`
	UserAgent    string        `json:"user_agent"`
	RemoteAddr   string        `json:"remote_addr"`
	ResponseSize int           `json:"size"`
}

// LogHTTPRequest logs an HTTP request with structured fields.
func LogHTTPRequest(logger logrus.FieldLogger, info HTTPRequestInfo) {
	fields := logrus.Fields{
		"method":      info.Method,
		"path":        info.Path,
		"status":      info.Status,
		"duration_ms": info.DurationMs,
		"duration":    info.Duration.String(),
		"request_id":  info.RequestID,
		"user_agent":  info.UserAgent,
		"remote_addr": info.RemoteAddr,
		"size":        info.ResponseSize,
	}

	if info.Query != "" {
		fields["query"] = info.Query
	}
	if info.Correlation != "" {
		fields["correlation_id"] = info.Correlation
	}

	switch {
	case info.Status >= 500:
		logger.WithFields(fields).Error("HTTP request error")
	case info.Status >= 400:
		logger.WithFields(fields).Warn("HTTP request warning")
	default:
		logger.WithFields(fields).Info("HTTP request completed")
	}
}

// IsValidLogLevel checks if a log level is valid.
func IsValidLogLevel(level string) bool {
	switch LogLevel(strings.ToLower(level)) {
	case LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError:
		return true
	default:
		return false
	}
}

// IsValidLogFormat checks if a log format is valid.
func IsValidLogFormat(format string) bool {
	switch LogFormat(strings.ToLower(format)) {
	case LogFormatText, LogFormatJSON:
		return true
	default:
		return false
	}
}
