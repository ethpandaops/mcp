package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sirupsen/logrus"
)

const testIssuerURL = "https://proxy.example.com"

func TestHandleTokenIssuesAccessTokenWithoutRefreshToken(t *testing.T) {
	t.Parallel()

	svc := newTestSimpleService(t)
	verifier := "verifier-123"
	challenge := sha256.Sum256([]byte(verifier))
	svc.codes["auth-code"] = &issuedCode{
		Code:          "auth-code",
		ClientID:      "panda",
		RedirectURI:   "http://localhost:8085/callback",
		Resource:      testIssuerURL,
		CodeChallenge: base64.RawURLEncoding.EncodeToString(challenge[:]),
		GitHubLogin:   "sam",
		GitHubID:      42,
		CreatedAt:     time.Now(),
	}

	resp := exchangeToken(t, svc, "http://internal-proxy/auth/token", url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {"auth-code"},
		"redirect_uri":  {"http://localhost:8085/callback"},
		"client_id":     {"panda"},
		"code_verifier": {verifier},
		"resource":      {testIssuerURL},
	})

	if resp.AccessToken == "" {
		t.Fatal("expected access token in authorization_code response")
	}

	if resp.RefreshToken != "" {
		t.Fatalf("expected no refresh token, got %q", resp.RefreshToken)
	}

	claims := &tokenClaims{}
	token, err := jwt.ParseWithClaims(resp.AccessToken, claims, func(t *jwt.Token) (any, error) {
		return []byte("test-secret"), nil
	})
	if err != nil || !token.Valid {
		t.Fatalf("failed to parse access token: %v", err)
	}

	if claims.Issuer != testIssuerURL {
		t.Fatalf("expected issuer %q, got %q", testIssuerURL, claims.Issuer)
	}
	if len(claims.Audience) != 1 || claims.Audience[0] != testIssuerURL {
		t.Fatalf("expected audience %q, got %#v", testIssuerURL, claims.Audience)
	}
}

func TestMiddlewareUsesConfiguredIssuerURLInsteadOfRequestHost(t *testing.T) {
	t.Parallel()

	svc := newTestSimpleService(t)
	token, err := svc.issueAccessToken(testIssuerURL, testIssuerURL, "sam", 42, nil)
	if err != nil {
		t.Fatalf("issueAccessToken failed: %v", err)
	}

	handler := svc.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://internal-proxy/clickhouse/query", nil)
	req.Host = "internal-proxy"
	req.Header.Set("X-Forwarded-Host", "attacker.example.com")
	req.Header.Set("X-Forwarded-Proto", "http")
	req.Header.Set("Authorization", "Bearer "+token)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d: %s", http.StatusNoContent, rec.Code, rec.Body.String())
	}
}

func TestHandleTokenRejectsRefreshGrant(t *testing.T) {
	t.Parallel()

	svc := newTestSimpleService(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://internal-proxy/auth/token", strings.NewReader(url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {"stale-token"},
		"client_id":     {"panda"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	svc.handleToken(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "unsupported_grant_type") {
		t.Fatalf("expected unsupported_grant_type error, got %s", rec.Body.String())
	}
}

type tokenResponseBody struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

func exchangeToken(t *testing.T, svc *simpleService, targetURL string, values url.Values) tokenResponseBody {
	t.Helper()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, targetURL, strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	svc.handleToken(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", rec.Code, rec.Body.String())
	}

	var body tokenResponseBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	return body
}

func newTestSimpleService(t *testing.T) *simpleService {
	t.Helper()

	service, err := NewSimpleService(logrus.New(), Config{
		Enabled:   true,
		IssuerURL: testIssuerURL,
		GitHub: &GitHubConfig{
			ClientID:     "github-client",
			ClientSecret: "github-secret",
		},
		Tokens: TokensConfig{SecretKey: "test-secret"},
	})
	if err != nil {
		t.Fatalf("NewSimpleService failed: %v", err)
	}

	return service.(*simpleService)
}
