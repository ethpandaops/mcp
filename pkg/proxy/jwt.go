package proxy

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sirupsen/logrus"
)

// JWTValidator validates JWTs using JWKS fetched from an OIDC provider (e.g., Dex).
type JWTValidator interface {
	// Validate validates a JWT and returns the claims if valid.
	Validate(ctx context.Context, tokenString string) (*JWTClaims, error)

	// Start starts the JWKS refresh background goroutine.
	Start(ctx context.Context) error

	// Stop stops the JWKS refresh background goroutine.
	Stop() error
}

// JWTClaims contains the validated claims from a JWT.
type JWTClaims struct {
	Subject     string   // User ID (sub claim)
	Email       string   // User email
	Groups      []string // Groups/organizations the user belongs to
	Issuer      string   // Token issuer
	Audience    []string // Token audience
	ExpiresAt   time.Time
	IssuedAt    time.Time
	GitHubLogin string // GitHub username (if available)
	GitHubID    int64  // GitHub user ID (if available)
}

// JWTValidatorConfig configures the JWT validator.
type JWTValidatorConfig struct {
	// JWKSURL is the URL to fetch JWKS from (e.g., https://dex.example.com/keys).
	JWKSURL string `yaml:"jwks_url"`

	// Issuer is the expected token issuer (e.g., https://dex.example.com).
	Issuer string `yaml:"issuer"`

	// Audience is the expected token audience (optional).
	Audience string `yaml:"audience,omitempty"`

	// AllowedOrgs is the list of allowed GitHub organizations.
	// If empty, org membership is not checked.
	AllowedOrgs []string `yaml:"allowed_orgs,omitempty"`

	// RefreshInterval is how often to refresh the JWKS cache.
	RefreshInterval time.Duration `yaml:"refresh_interval,omitempty"`
}

// ApplyDefaults sets default values for the JWT validator config.
func (c *JWTValidatorConfig) ApplyDefaults() {
	if c.RefreshInterval == 0 {
		c.RefreshInterval = 1 * time.Hour
	}
}

// jwtValidator implements JWTValidator using JWKS from an OIDC provider.
type jwtValidator struct {
	log    logrus.FieldLogger
	cfg    JWTValidatorConfig
	client *http.Client

	mu   sync.RWMutex
	keys map[string]*rsa.PublicKey

	stopCh  chan struct{}
	stopped bool
}

// Compile-time interface check.
var _ JWTValidator = (*jwtValidator)(nil)

// NewJWTValidator creates a new JWT validator.
func NewJWTValidator(log logrus.FieldLogger, cfg JWTValidatorConfig) JWTValidator {
	cfg.ApplyDefaults()

	return &jwtValidator{
		log:    log.WithField("component", "jwt-validator"),
		cfg:    cfg,
		client: &http.Client{Timeout: 30 * time.Second},
		keys:   make(map[string]*rsa.PublicKey, 4),
		stopCh: make(chan struct{}),
	}
}

// Start starts the JWKS refresh background goroutine.
func (v *jwtValidator) Start(ctx context.Context) error {
	// Fetch JWKS immediately on startup.
	if err := v.refreshJWKS(ctx); err != nil {
		return fmt.Errorf("initial JWKS fetch: %w", err)
	}

	// Start background refresh goroutine.
	go v.refreshLoop()

	v.log.WithField("jwks_url", v.cfg.JWKSURL).Info("JWT validator started")

	return nil
}

// Stop stops the JWKS refresh background goroutine.
func (v *jwtValidator) Stop() error {
	v.mu.Lock()
	if v.stopped {
		v.mu.Unlock()
		return nil
	}

	v.stopped = true
	v.mu.Unlock()

	close(v.stopCh)
	v.log.Info("JWT validator stopped")

	return nil
}

// Validate validates a JWT and returns the claims if valid.
func (v *jwtValidator) Validate(ctx context.Context, tokenString string) (*JWTClaims, error) {
	// Parse and validate the token.
	token, err := jwt.Parse(tokenString, v.keyFunc, jwt.WithValidMethods([]string{"RS256"}))
	if err != nil {
		return nil, fmt.Errorf("parsing token: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid claims type")
	}

	// Validate issuer.
	issuer, _ := claims["iss"].(string)
	if v.cfg.Issuer != "" && issuer != v.cfg.Issuer {
		return nil, fmt.Errorf("invalid issuer: got %q, expected %q", issuer, v.cfg.Issuer)
	}

	// Validate audience if configured.
	if v.cfg.Audience != "" {
		aud := extractAudience(claims)
		if !containsAudience(aud, v.cfg.Audience) {
			return nil, fmt.Errorf("invalid audience: %v does not contain %q", aud, v.cfg.Audience)
		}
	}

	// Extract standard claims.
	jwtClaims := &JWTClaims{
		Subject:  getString(claims, "sub"),
		Email:    getString(claims, "email"),
		Issuer:   issuer,
		Audience: extractAudience(claims),
	}

	// Extract groups/organizations.
	if groups, ok := claims["groups"].([]any); ok {
		for _, g := range groups {
			if s, ok := g.(string); ok {
				jwtClaims.Groups = append(jwtClaims.Groups, s)
			}
		}
	}

	// Extract GitHub-specific claims if available.
	jwtClaims.GitHubLogin = getString(claims, "github_login")

	if githubID, ok := claims["github_id"].(float64); ok {
		jwtClaims.GitHubID = int64(githubID)
	}

	// Extract timestamps.
	if exp, err := claims.GetExpirationTime(); err == nil && exp != nil {
		jwtClaims.ExpiresAt = exp.Time
	}

	if iat, err := claims.GetIssuedAt(); err == nil && iat != nil {
		jwtClaims.IssuedAt = iat.Time
	}

	// Validate org membership if configured.
	if len(v.cfg.AllowedOrgs) > 0 {
		if !hasAllowedOrg(jwtClaims.Groups, v.cfg.AllowedOrgs) {
			return nil, fmt.Errorf("user not in allowed organizations")
		}
	}

	return jwtClaims, nil
}

// keyFunc returns the public key for JWT validation.
func (v *jwtValidator) keyFunc(token *jwt.Token) (any, error) {
	// Get key ID from token header.
	kid, ok := token.Header["kid"].(string)
	if !ok {
		return nil, fmt.Errorf("missing kid in token header")
	}

	v.mu.RLock()
	key, ok := v.keys[kid]
	v.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("key not found for kid: %s", kid)
	}

	return key, nil
}

// refreshLoop periodically refreshes the JWKS cache.
func (v *jwtValidator) refreshLoop() {
	ticker := time.NewTicker(v.cfg.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-v.stopCh:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

			if err := v.refreshJWKS(ctx); err != nil {
				v.log.WithError(err).Warn("Failed to refresh JWKS")
			}

			cancel()
		}
	}
}

// refreshJWKS fetches the JWKS from the configured URL and updates the cache.
func (v *jwtValidator) refreshJWKS(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.cfg.JWKSURL, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetching JWKS: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("JWKS endpoint returned status %d", resp.StatusCode)
	}

	var jwks jwksResponse
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return fmt.Errorf("decoding JWKS: %w", err)
	}

	// Parse and cache keys.
	newKeys := make(map[string]*rsa.PublicKey, len(jwks.Keys))

	for _, key := range jwks.Keys {
		if key.Kty != "RSA" || key.Use != "sig" {
			continue
		}

		pubKey, err := parseRSAPublicKey(key)
		if err != nil {
			v.log.WithError(err).WithField("kid", key.Kid).Warn("Failed to parse RSA key")

			continue
		}

		newKeys[key.Kid] = pubKey
	}

	v.mu.Lock()
	v.keys = newKeys
	v.mu.Unlock()

	v.log.WithField("key_count", len(newKeys)).Debug("Refreshed JWKS cache")

	return nil
}

// jwksResponse is the response from a JWKS endpoint.
type jwksResponse struct {
	Keys []jwkKey `json:"keys"`
}

// jwkKey represents a single JWK.
type jwkKey struct {
	Kty string `json:"kty"` // Key type (RSA)
	Use string `json:"use"` // Usage (sig)
	Kid string `json:"kid"` // Key ID
	Alg string `json:"alg"` // Algorithm (RS256)
	N   string `json:"n"`   // Modulus
	E   string `json:"e"`   // Exponent
}

// parseRSAPublicKey parses an RSA public key from a JWK.
func parseRSAPublicKey(key jwkKey) (*rsa.PublicKey, error) {
	// Decode modulus (n).
	nBytes, err := base64.RawURLEncoding.DecodeString(key.N)
	if err != nil {
		return nil, fmt.Errorf("decoding modulus: %w", err)
	}

	n := new(big.Int).SetBytes(nBytes)

	// Decode exponent (e).
	eBytes, err := base64.RawURLEncoding.DecodeString(key.E)
	if err != nil {
		return nil, fmt.Errorf("decoding exponent: %w", err)
	}

	// Convert exponent bytes to int.
	var e int
	for _, b := range eBytes {
		e = e<<8 | int(b)
	}

	return &rsa.PublicKey{N: n, E: e}, nil
}

// Helper functions.

func getString(claims jwt.MapClaims, key string) string {
	if v, ok := claims[key].(string); ok {
		return v
	}

	return ""
}

func extractAudience(claims jwt.MapClaims) []string {
	switch aud := claims["aud"].(type) {
	case string:
		return []string{aud}
	case []any:
		var result []string
		for _, a := range aud {
			if s, ok := a.(string); ok {
				result = append(result, s)
			}
		}

		return result
	default:
		return nil
	}
}

func containsAudience(audiences []string, expected string) bool {
	return slices.Contains(audiences, expected)
}

func hasAllowedOrg(groups, allowedOrgs []string) bool {
	for _, org := range allowedOrgs {
		if slices.Contains(groups, org) {
			return true
		}
	}

	return false
}
