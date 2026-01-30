package proxy

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/mcp/pkg/auth/client"
	"github.com/ethpandaops/mcp/pkg/auth/store"
)

// RemoteService is a proxy service adapter for remote proxy deployments.
// Instead of running a local proxy, it uses a remote proxy URL and retrieves
// the user's JWT from the local credential store for authentication.
type RemoteService interface {
	Service
}

// RemoteConfig configures the remote proxy adapter.
type RemoteConfig struct {
	// URL is the base URL of the remote proxy (e.g., https://proxy.ethpandaops.io).
	URL string

	// IssuerURL is the OIDC issuer URL for authentication.
	IssuerURL string

	// ClientID is the OAuth client ID for authentication.
	ClientID string
}

// remoteService implements RemoteService.
type remoteService struct {
	log         logrus.FieldLogger
	cfg         RemoteConfig
	authClient  client.Client
	credStore   store.Store
	datasources *remoteDatasources
}

// remoteDatasources holds the datasource names for a remote proxy.
// Since we don't have access to the remote proxy's config, these are
// passed from the MCP config or discovered via the /datasources endpoint.
type remoteDatasources struct {
	ClickHouse []string
	Prometheus []string
	Loki       []string
	S3Bucket   string
}

// Compile-time interface check.
var _ RemoteService = (*remoteService)(nil)

// NewRemote creates a new remote proxy service adapter.
func NewRemote(log logrus.FieldLogger, cfg RemoteConfig) RemoteService {
	authClient := client.New(log, client.Config{
		IssuerURL: cfg.IssuerURL,
		ClientID:  cfg.ClientID,
	})

	credStore := store.New(log, store.Config{
		AuthClient: authClient,
	})

	return &remoteService{
		log:        log.WithField("component", "remote-proxy"),
		cfg:        cfg,
		authClient: authClient,
		credStore:  credStore,
		datasources: &remoteDatasources{
			// These will be populated from config or discovered
			ClickHouse: []string{},
			Prometheus: []string{},
			Loki:       []string{},
		},
	}
}

// Start is a no-op for remote proxy - nothing to start locally.
func (r *remoteService) Start(_ context.Context) error {
	r.log.WithField("url", r.cfg.URL).Info("Using remote proxy")

	return nil
}

// Stop is a no-op for remote proxy - nothing to stop locally.
func (r *remoteService) Stop(_ context.Context) error {
	return nil
}

// URL returns the remote proxy URL.
func (r *remoteService) URL() string {
	return r.cfg.URL
}

// RegisterToken returns the user's JWT from the local credential store.
// Unlike the local proxy which generates per-execution tokens, the remote proxy
// uses the user's persistent JWT for all requests.
//
// The executionID is not used for remote proxy - the user's identity comes from
// the JWT itself.
func (r *remoteService) RegisterToken(_ string) string {
	token, err := r.credStore.GetAccessToken()
	if err != nil {
		r.log.WithError(err).Error("Failed to get access token from credential store")

		return ""
	}

	return token
}

// RevokeToken is a no-op for remote proxy - JWTs expire naturally.
func (r *remoteService) RevokeToken(_ string) {
	// No-op: JWTs are managed by the OIDC provider, not revoked per-execution
}

// ClickHouseDatasources returns the list of ClickHouse datasource names.
func (r *remoteService) ClickHouseDatasources() []string {
	return r.datasources.ClickHouse
}

// PrometheusDatasources returns the list of Prometheus datasource names.
func (r *remoteService) PrometheusDatasources() []string {
	return r.datasources.Prometheus
}

// LokiDatasources returns the list of Loki datasource names.
func (r *remoteService) LokiDatasources() []string {
	return r.datasources.Loki
}

// S3Bucket returns the configured S3 bucket name.
func (r *remoteService) S3Bucket() string {
	return r.datasources.S3Bucket
}

// SetDatasources configures the known datasources for the remote proxy.
// This is called by the builder when plugin configs are available.
func (r *remoteService) SetDatasources(ch, prom, loki []string, s3Bucket string) {
	r.datasources = &remoteDatasources{
		ClickHouse: ch,
		Prometheus: prom,
		Loki:       loki,
		S3Bucket:   s3Bucket,
	}
}

// EnsureAuthenticated checks if the user has valid credentials and prompts
// login if needed. This should be called during MCP server startup.
func (r *remoteService) EnsureAuthenticated(ctx context.Context) error {
	if r.credStore.IsAuthenticated() {
		return nil
	}

	return fmt.Errorf(
		"not authenticated to remote proxy. Run 'mcp auth login --issuer %s --client-id %s' first",
		r.cfg.IssuerURL,
		r.cfg.ClientID,
	)
}
