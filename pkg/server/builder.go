package server

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/xatu-mcp/pkg/auth"
	"github.com/ethpandaops/xatu-mcp/pkg/clickhouse"
	"github.com/ethpandaops/xatu-mcp/pkg/config"
	"github.com/ethpandaops/xatu-mcp/pkg/resource"
	"github.com/ethpandaops/xatu-mcp/pkg/sandbox"
	"github.com/ethpandaops/xatu-mcp/pkg/tool"
)

// Dependencies contains all the services required to run the MCP server.
type Dependencies struct {
	Logger           logrus.FieldLogger
	Config           *config.Config
	ToolRegistry     tool.Registry
	ResourceRegistry resource.Registry
	Sandbox          sandbox.Service
	ClickHouse       clickhouse.Client
}

// Builder constructs and wires all dependencies for the MCP server.
type Builder struct {
	log logrus.FieldLogger
	cfg *config.Config
}

// NewBuilder creates a new server builder.
func NewBuilder(log logrus.FieldLogger, cfg *config.Config) *Builder {
	return &Builder{
		log: log.WithField("component", "builder"),
		cfg: cfg,
	}
}

// Build constructs all dependencies and returns the server service.
func (b *Builder) Build(ctx context.Context) (Service, error) {
	b.log.Info("Building xatu-mcp server dependencies")

	// Create sandbox service
	sandboxSvc, err := b.buildSandbox()
	if err != nil {
		return nil, fmt.Errorf("building sandbox: %w", err)
	}

	// Start the sandbox service to initialize Docker client
	if err := sandboxSvc.Start(ctx); err != nil {
		return nil, fmt.Errorf("starting sandbox: %w", err)
	}

	b.log.WithField("backend", sandboxSvc.Name()).Info("Sandbox service started")

	// Create ClickHouse client
	chClient, err := b.buildClickHouse()
	if err != nil {
		// Clean up sandbox on failure
		_ = sandboxSvc.Stop(ctx)

		return nil, fmt.Errorf("building clickhouse client: %w", err)
	}

	// Start the ClickHouse client to initialize the HTTP client
	if err := chClient.Start(ctx); err != nil {
		// Clean up sandbox on failure
		_ = sandboxSvc.Stop(ctx)

		return nil, fmt.Errorf("starting clickhouse client: %w", err)
	}

	b.log.Info("ClickHouse client started")

	// Create tool registry and register tools
	toolReg := b.buildToolRegistry(sandboxSvc, b.cfg.Storage)

	// Create resource registry and register resources
	resourceReg := b.buildResourceRegistry(chClient)

	// Create auth service
	authSvc, err := b.buildAuth()
	if err != nil {
		_ = chClient.Stop()
		_ = sandboxSvc.Stop(ctx)

		return nil, fmt.Errorf("building auth service: %w", err)
	}

	// Create and return the server service (sandbox and clickhouse are passed for lifecycle management)
	return NewService(
		b.log,
		b.cfg.Server,
		b.cfg.Auth,
		toolReg,
		resourceReg,
		sandboxSvc,
		chClient,
		authSvc,
	), nil
}

// buildSandbox creates the sandbox service.
func (b *Builder) buildSandbox() (sandbox.Service, error) {
	return sandbox.New(b.cfg.Sandbox, b.log)
}

// buildAuth creates the auth service.
func (b *Builder) buildAuth() (auth.SimpleService, error) {
	// Build the base URL for auth.
	baseURL := b.cfg.Server.BaseURL
	if baseURL == "" {
		baseURL = fmt.Sprintf("http://%s:%d", b.cfg.Server.Host, b.cfg.Server.Port)
	}

	return auth.NewSimpleService(b.log, b.cfg.Auth, baseURL)
}

// buildClickHouse creates the ClickHouse client.
func (b *Builder) buildClickHouse() (clickhouse.Client, error) {
	clusters := b.cfg.ClickHouse.ToClusters()

	return clickhouse.NewClient(b.log, clusters), nil
}

// buildToolRegistry creates and populates the tool registry.
func (b *Builder) buildToolRegistry(
	sandboxSvc sandbox.Service,
	storageCfg *config.StorageConfig,
) tool.Registry {
	reg := tool.NewRegistry(b.log)

	// Register execute_python tool
	reg.Register(tool.NewExecutePythonTool(b.log, sandboxSvc, b.cfg))

	// Register file tools
	reg.Register(tool.NewListOutputFilesTool(b.log))
	reg.Register(tool.NewGetOutputFileTool(b.log))

	// Register get_image tool if storage is configured
	if storageCfg != nil && storageCfg.PublicURLPrefix != "" {
		reg.Register(tool.NewGetImageTool(b.log, tool.GetImageConfig{
			PublicURLPrefix:   storageCfg.PublicURLPrefix,
			InternalURLPrefix: storageCfg.InternalURLPrefix,
		}))

		b.log.WithFields(logrus.Fields{
			"public_url_prefix":   storageCfg.PublicURLPrefix,
			"internal_url_prefix": storageCfg.InternalURLPrefix,
		}).Debug("Registered get_image tool")
	}

	b.log.WithField("tool_count", len(reg.List())).Info("Tool registry built")

	return reg
}

// buildResourceRegistry creates and populates the resource registry.
func (b *Builder) buildResourceRegistry(chClient clickhouse.Client) resource.Registry {
	reg := resource.NewRegistry(b.log)

	// Create cluster provider adapter
	provider := newClusterProviderAdapter(chClient)

	// Create schema client factory
	clientFactory := func(clusterName string) (resource.SchemaClient, error) {
		return newSchemaClientAdapter(chClient, clusterName), nil
	}

	// Register schema resources
	resource.RegisterSchemaResources(b.log, reg, provider, clientFactory)

	// Register examples resources
	resource.RegisterExamplesResources(b.log, reg)

	// Register networks resources
	resource.RegisterNetworksResources(b.log, reg)

	// Register API resources
	resource.RegisterAPIResources(b.log, reg)

	staticCount := len(reg.ListStatic())
	templateCount := len(reg.ListTemplates())
	b.log.WithFields(logrus.Fields{
		"static_count":   staticCount,
		"template_count": templateCount,
	}).Info("Resource registry built")

	return reg
}

// clusterProviderAdapter adapts clickhouse.Client to resource.ClusterProvider.
type clusterProviderAdapter struct {
	client clickhouse.Client
}

func newClusterProviderAdapter(client clickhouse.Client) resource.ClusterProvider {
	return &clusterProviderAdapter{client: client}
}

func (a *clusterProviderAdapter) GetCluster(name string) (*clickhouse.ClusterInfo, bool) {
	return a.client.GetCluster(name)
}

func (a *clusterProviderAdapter) ListClusters() []*clickhouse.ClusterInfo {
	infos := a.client.ListClusters()
	result := make([]*clickhouse.ClusterInfo, len(infos))

	for i := range infos {
		result[i] = &infos[i]
	}

	return result
}

// schemaClientAdapter adapts clickhouse.Client to resource.SchemaClient.
type schemaClientAdapter struct {
	client      clickhouse.Client
	clusterName string
}

func newSchemaClientAdapter(client clickhouse.Client, clusterName string) resource.SchemaClient {
	return &schemaClientAdapter{
		client:      client,
		clusterName: clusterName,
	}
}

func (a *schemaClientAdapter) ListTables(ctx context.Context) ([]clickhouse.TableInfo, error) {
	return a.client.ListTables(ctx, a.clusterName)
}

func (a *schemaClientAdapter) GetTableInfo(ctx context.Context, tableName string) (*clickhouse.TableInfo, error) {
	return a.client.GetTableInfo(ctx, a.clusterName, tableName)
}

func (a *schemaClientAdapter) GetTableSchema(ctx context.Context, tableName string) ([]clickhouse.ColumnInfo, error) {
	return a.client.GetTableSchema(ctx, a.clusterName, tableName)
}
