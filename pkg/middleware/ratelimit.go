// Package middleware provides HTTP middleware for the MCP server.
package middleware

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"

	"github.com/ethpandaops/mcp/pkg/config"
)

const (
	// Rate limit header names.
	HeaderRateLimitLimit     = "X-RateLimit-Limit"
	HeaderRateLimitRemaining = "X-RateLimit-Remaining"
	HeaderRateLimitReset     = "X-RateLimit-Reset"
	HeaderRetryAfter         = "Retry-After"

	// Default cleanup interval for in-memory store.
	defaultCleanupInterval = 5 * time.Minute
)

// Limiter is the interface for rate limiter implementations.
// Both in-memory and Redis backends implement this interface.
type Limiter interface {
	// Allow checks if a request from the given key (IP + tool) is allowed.
	// Returns true if allowed, false if rate limited.
	Allow(key string, rule config.RateLimitRule) (bool, RateLimitInfo)
	// Close cleans up any resources used by the limiter.
	Close() error
}

// RateLimitInfo contains information about the current rate limit state.
type RateLimitInfo struct {
	// Limit is the maximum number of requests allowed per second.
	Limit float64
	// Remaining is the number of requests remaining in the current window.
	Remaining int
	// ResetAt is the Unix timestamp when the rate limit resets.
	ResetAt int64
}

// RateLimiter is the main rate limiting middleware.
type RateLimiter struct {
	log     logrus.FieldLogger
	cfg     config.RateLimitConfig
	limiter Limiter
}

// NewRateLimiter creates a new rate limiter middleware.
func NewRateLimiter(log logrus.FieldLogger, cfg config.RateLimitConfig) (*RateLimiter, error) {
	var limiter Limiter

	switch cfg.Backend {
	case "redis":
		// Redis backend would be implemented here with a proper Redis client.
		// For now, fall back to memory with a warning.
		log.Warn("Redis rate limit backend not yet implemented, falling back to memory")
		limiter = newInMemoryLimiter(log)
	case "memory", "":
		limiter = newInMemoryLimiter(log)
	default:
		return nil, fmt.Errorf("unknown rate limit backend: %s", cfg.Backend)
	}

	return &RateLimiter{
		log:     log.WithField("component", "rate-limiter"),
		cfg:     cfg,
		limiter: limiter,
	}, nil
}

// Middleware returns an HTTP middleware that enforces rate limiting.
// The toolName parameter is used to look up per-tool rate limits.
// For general endpoint limiting, pass an empty string.
func (rl *RateLimiter) Middleware(toolName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !rl.cfg.Enabled {
				next.ServeHTTP(w, r)
				return
			}

			// Get the effective rate limit rule for this tool/endpoint.
			rule := rl.getRule(toolName)

			// Get client IP.
			clientIP := rl.getClientIP(r)

			// Build the rate limit key: IP + tool name.
			key := rl.buildKey(clientIP, toolName)

			// Check rate limit.
			allowed, info := rl.limiter.Allow(key, rule)

			// Set rate limit headers.
			w.Header().Set(HeaderRateLimitLimit, fmt.Sprintf("%.2f", info.Limit))
			w.Header().Set(HeaderRateLimitRemaining, fmt.Sprintf("%d", info.Remaining))
			w.Header().Set(HeaderRateLimitReset, fmt.Sprintf("%d", info.ResetAt))

			if !allowed {
				rl.log.WithFields(logrus.Fields{
					"client_ip": clientIP,
					"tool":      toolName,
					"remaining": info.Remaining,
				}).Debug("Rate limit exceeded")

				// Calculate retry after based on block duration.
				retryAfter := int(rule.GetBlockDuration().Seconds())
				w.Header().Set(HeaderRetryAfter, fmt.Sprintf("%d", retryAfter))
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// getRule returns the rate limit rule for the given tool.
// Falls back to the default rule if no per-tool rule is configured.
func (rl *RateLimiter) getRule(toolName string) config.RateLimitRule {
	if toolName != "" {
		if rule, ok := rl.cfg.PerTool[toolName]; ok {
			return rule
		}
	}
	return rl.cfg.Default
}

// getClientIP extracts the client IP from the request.
// It considers X-Forwarded-For and X-Real-IP headers if the request comes from a trusted proxy.
func (rl *RateLimiter) getClientIP(r *http.Request) string {
	// Check if request comes from a trusted proxy.
	remoteIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remoteIP = r.RemoteAddr
	}

	// Check if remote IP is in trusted proxies list.
	isTrusted := rl.isTrustedProxy(remoteIP)

	if isTrusted {
		// Check X-Forwarded-For header.
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// X-Forwarded-For can contain multiple IPs, take the first one (client).
			ips := strings.Split(xff, ",")
			if len(ips) > 0 {
				clientIP := strings.TrimSpace(ips[0])
				if clientIP != "" {
					return clientIP
				}
			}
		}

		// Check X-Real-IP header.
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			return strings.TrimSpace(xri)
		}
	}

	return remoteIP
}

// isTrustedProxy checks if the given IP is in the trusted proxies list.
func (rl *RateLimiter) isTrustedProxy(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	for _, trusted := range rl.cfg.TrustedProxies {
		// Check if it's a CIDR range.
		if strings.Contains(trusted, "/") {
			_, ipNet, err := net.ParseCIDR(trusted)
			if err != nil {
				continue
			}
			if ipNet.Contains(parsedIP) {
				return true
			}
		} else {
			// Check for exact match.
			if trusted == ip {
				return true
			}
		}
	}

	return false
}

// buildKey builds the rate limit key from client IP and tool name.
func (rl *RateLimiter) buildKey(clientIP, toolName string) string {
	if toolName != "" {
		return fmt.Sprintf("%s:%s", clientIP, toolName)
	}
	return clientIP
}

// Close cleans up the rate limiter.
func (rl *RateLimiter) Close() error {
	return rl.limiter.Close()
}

// inMemoryLimiter is an in-memory implementation of the Limiter interface.
type inMemoryLimiter struct {
	log      logrus.FieldLogger
	limiters map[string]*inMemoryEntry
	mu       sync.RWMutex
	stopCh   chan struct{}
}

// inMemoryEntry holds a rate limiter and metadata for a key.
type inMemoryEntry struct {
	limiter    *rate.Limiter
	lastUsed   time.Time
	resetAt    int64
	burstSize  int
	ratePerSec float64
}

// newInMemoryLimiter creates a new in-memory rate limiter.
func newInMemoryLimiter(log logrus.FieldLogger) *inMemoryLimiter {
	iml := &inMemoryLimiter{
		log:      log.WithField("backend", "memory"),
		limiters: make(map[string]*inMemoryEntry, 64),
		stopCh:   make(chan struct{}),
	}

	// Start cleanup goroutine.
	go iml.cleanupLoop()

	return iml
}

// Allow checks if a request is allowed and returns rate limit info.
func (iml *inMemoryLimiter) Allow(key string, rule config.RateLimitRule) (bool, RateLimitInfo) {
	entry := iml.getOrCreateEntry(key, rule)

	// Calculate remaining tokens.
	remaining := int(entry.limiter.Tokens())
	if remaining < 0 {
		remaining = 0
	}

	info := RateLimitInfo{
		Limit:     entry.ratePerSec,
		Remaining: remaining,
		ResetAt:   entry.resetAt,
	}

	// Check if allowed.
	if !entry.limiter.Allow() {
		return false, info
	}

	return true, info
}

// getOrCreateEntry gets or creates a rate limiter entry for the given key.
func (iml *inMemoryLimiter) getOrCreateEntry(key string, rule config.RateLimitRule) *inMemoryEntry {
	iml.mu.RLock()
	entry, exists := iml.limiters[key]
	iml.mu.RUnlock()

	if exists {
		iml.mu.Lock()
		entry.lastUsed = time.Now()
		// Update reset time on each successful request.
		entry.resetAt = time.Now().Add(rule.GetBlockDuration()).Unix()
		iml.mu.Unlock()
		return entry
	}

	// Create new entry.
	iml.mu.Lock()
	defer iml.mu.Unlock()

	// Double-check after acquiring write lock.
	if entry, exists := iml.limiters[key]; exists {
		entry.lastUsed = time.Now()
		return entry
	}

	ratePerSec := rule.GetRequestsPerSecond()
	burstSize := rule.GetBurstSize()

	entry = &inMemoryEntry{
		limiter:    rate.NewLimiter(rate.Limit(ratePerSec), burstSize),
		lastUsed:   time.Now(),
		resetAt:    time.Now().Add(rule.GetBlockDuration()).Unix(),
		burstSize:  burstSize,
		ratePerSec: ratePerSec,
	}
	iml.limiters[key] = entry

	return entry
}

// cleanupLoop periodically removes stale rate limiters.
func (iml *inMemoryLimiter) cleanupLoop() {
	ticker := time.NewTicker(defaultCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-iml.stopCh:
			return
		case <-ticker.C:
			iml.cleanup()
		}
	}
}

// cleanup removes stale rate limiter entries.
// An entry is considered stale if it has been inactive for longer than the cleanup interval.
func (iml *inMemoryLimiter) cleanup() {
	iml.mu.Lock()
	defer iml.mu.Unlock()

	cutoff := time.Now().Add(-defaultCleanupInterval)
	removed := 0

	for key, entry := range iml.limiters {
		if entry.lastUsed.Before(cutoff) {
			delete(iml.limiters, key)
			removed++
		}
	}

	if removed > 0 {
		iml.log.WithFields(logrus.Fields{
			"removed":  removed,
			"remaining": len(iml.limiters),
		}).Debug("Rate limiter cleanup completed")
	}
}

// Close stops the cleanup goroutine and clears all entries.
func (iml *inMemoryLimiter) Close() error {
	close(iml.stopCh)

	iml.mu.Lock()
	iml.limiters = make(map[string]*inMemoryEntry)
	iml.mu.Unlock()

	return nil
}
