package proxy

import (
	"net/http"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
)

// RateLimiter provides per-user rate limiting for the proxy.
type RateLimiter struct {
	log      logrus.FieldLogger
	cfg      RateLimiterConfig
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	stopCh   chan struct{}
	stopped  bool
}

// RateLimiterConfig configures the rate limiter.
type RateLimiterConfig struct {
	// RequestsPerMinute is the maximum requests per minute per user.
	RequestsPerMinute int

	// BurstSize is the maximum burst size.
	BurstSize int
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(log logrus.FieldLogger, cfg RateLimiterConfig) *RateLimiter {
	rl := &RateLimiter{
		log:      log.WithField("component", "rate-limiter"),
		cfg:      cfg,
		limiters: make(map[string]*rate.Limiter, 64),
		stopCh:   make(chan struct{}),
	}

	// Start cleanup goroutine.
	go rl.cleanupLoop()

	return rl
}

// getLimiter returns the rate limiter for the given user ID.
func (rl *RateLimiter) getLimiter(userID string) *rate.Limiter {
	rl.mu.RLock()
	limiter, ok := rl.limiters[userID]
	rl.mu.RUnlock()

	if ok {
		return limiter
	}

	// Create new limiter.
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Double-check after acquiring write lock.
	if limiter, ok := rl.limiters[userID]; ok {
		return limiter
	}

	// Calculate rate: requests per minute -> requests per second.
	ratePerSecond := rate.Limit(float64(rl.cfg.RequestsPerMinute) / 60.0)
	limiter = rate.NewLimiter(ratePerSecond, rl.cfg.BurstSize)
	rl.limiters[userID] = limiter

	return limiter
}

// Allow checks if a request is allowed for the given user ID.
func (rl *RateLimiter) Allow(userID string) bool {
	return rl.getLimiter(userID).Allow()
}

// Middleware returns an HTTP middleware that enforces rate limiting.
func (rl *RateLimiter) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := GetUserID(r.Context())
			if userID == "" {
				// No user ID, allow request (auth middleware should have rejected).
				next.ServeHTTP(w, r)

				return
			}

			if !rl.Allow(userID) {
				rl.log.WithField("user_id", userID).Debug("Rate limit exceeded")

				w.Header().Set("Retry-After", "60")
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)

				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Stop stops the rate limiter cleanup goroutine.
func (rl *RateLimiter) Stop() {
	rl.mu.Lock()
	if rl.stopped {
		rl.mu.Unlock()

		return
	}

	rl.stopped = true
	rl.mu.Unlock()

	close(rl.stopCh)
}

// cleanupLoop periodically removes inactive rate limiters.
func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-rl.stopCh:
			return
		case <-ticker.C:
			rl.cleanup()
		}
	}
}

// cleanup removes rate limiters that have been inactive.
// A limiter is considered inactive if it has recovered to full burst capacity.
func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	for userID, limiter := range rl.limiters {
		// If the limiter has recovered to full burst, remove it.
		// This is a heuristic - if tokens == burst, user hasn't used it recently.
		if limiter.Tokens() >= float64(rl.cfg.BurstSize) {
			delete(rl.limiters, userID)
		}
	}

	rl.log.WithField("active_limiters", len(rl.limiters)).Debug("Rate limiter cleanup complete")
}

// AuditEntry represents a single audit log entry.
type AuditEntry struct {
	UserID     string `json:"user_id"`
	Email      string `json:"email,omitempty"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	Datasource string `json:"datasource"`
	Query      string `json:"query,omitempty"`
	StatusCode int    `json:"status_code"`
	Duration   string `json:"duration"`
}

// Auditor logs audit entries for proxy requests.
type Auditor struct {
	log logrus.FieldLogger
	cfg AuditorConfig
}

// AuditorConfig configures the auditor.
type AuditorConfig struct {
	// LogQueries controls whether to log query content.
	LogQueries bool

	// MaxQueryLength is the maximum length of query to log.
	MaxQueryLength int
}

// NewAuditor creates a new auditor.
func NewAuditor(log logrus.FieldLogger, cfg AuditorConfig) *Auditor {
	return &Auditor{
		log: log.WithField("component", "auditor"),
		cfg: cfg,
	}
}

// Middleware returns an HTTP middleware that logs audit entries.
func (a *Auditor) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap response writer to capture status code.
			wrapped := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}

			// Call next handler.
			next.ServeHTTP(wrapped, r)

			// Build audit entry.
			entry := AuditEntry{
				UserID:     GetUserID(r.Context()),
				Method:     r.Method,
				Path:       r.URL.Path,
				Datasource: extractDatasource(r.URL.Path),
				StatusCode: wrapped.statusCode,
				Duration:   time.Since(start).String(),
			}

			// Add email if available from JWT claims.
			if claims := GetJWTClaims(r.Context()); claims != nil {
				entry.Email = claims.Email
			}

			// Add query if configured.
			if a.cfg.LogQueries {
				query := extractQuery(r)
				if len(query) > a.cfg.MaxQueryLength {
					query = query[:a.cfg.MaxQueryLength] + "..."
				}

				entry.Query = query
			}

			// Log the audit entry.
			a.log.WithFields(logrus.Fields{
				"user_id":    entry.UserID,
				"email":      entry.Email,
				"method":     entry.Method,
				"path":       entry.Path,
				"datasource": entry.Datasource,
				"query":      entry.Query,
				"status":     entry.StatusCode,
				"duration":   entry.Duration,
			}).Info("Audit")
		})
	}
}

// responseRecorder wraps http.ResponseWriter to capture the status code.
type responseRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (rr *responseRecorder) WriteHeader(code int) {
	rr.statusCode = code
	rr.ResponseWriter.WriteHeader(code)
}

// extractDatasource extracts the datasource type from the path.
func extractDatasource(path string) string {
	switch {
	case len(path) > 11 && path[:12] == "/clickhouse/":
		return "clickhouse"
	case len(path) > 12 && path[:13] == "/prometheus/":
		return "prometheus"
	case len(path) > 5 && path[:6] == "/loki/":
		return "loki"
	case len(path) > 3 && path[:4] == "/s3/":
		return "s3"
	default:
		return "unknown"
	}
}

// extractQuery extracts query content from the request.
func extractQuery(r *http.Request) string {
	// Try URL query parameter first (ClickHouse uses this).
	if q := r.URL.Query().Get("query"); q != "" {
		return q
	}

	// For Prometheus/Loki, check the query parameter.
	if q := r.URL.Query().Get("query"); q != "" {
		return q
	}

	// No query found.
	return ""
}
