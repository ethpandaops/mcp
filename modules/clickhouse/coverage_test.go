package clickhouse

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	modulepkg "github.com/ethpandaops/panda/pkg/module"
	"github.com/ethpandaops/panda/pkg/types"
)

func TestSchemaDiscoveryConfigIsEnabled(t *testing.T) {
	t.Run("defaults to enabled", func(t *testing.T) {
		var cfg SchemaDiscoveryConfig
		assert.True(t, cfg.IsEnabled())
	})

	t.Run("honors explicit values", func(t *testing.T) {
		disabled := false
		enabled := true

		assert.False(t, (&SchemaDiscoveryConfig{Enabled: &disabled}).IsEnabled())
		assert.True(t, (&SchemaDiscoveryConfig{Enabled: &enabled}).IsEnabled())
	})
}

func TestLoadExamplesReturnsIndependentCatalogs(t *testing.T) {
	first, err := loadExamples()
	require.NoError(t, err)

	second, err := loadExamples()
	require.NoError(t, err)

	require.NotEmpty(t, first)

	var categoryName string
	for name := range first {
		categoryName = name
		break
	}

	category := first[categoryName]
	require.NotEmpty(t, category.Examples)

	category.Examples[0].Name = "mutated"
	first[categoryName] = category

	assert.NotEqual(t, "mutated", second[categoryName].Examples[0].Name)
}

func TestResourceHandlersExposeSchemaData(t *testing.T) {
	client := &stubSchemaClient{
		clusters: map[string]*ClusterTables{
			"xatu": {
				ClusterName: "xatu",
				LastUpdated: time.Unix(100, 0).UTC(),
				Tables: map[string]*TableSchema{
					"z_table": {
						Name:          "z_table",
						Columns:       []TableColumn{{Name: "slot", Type: "UInt64"}},
						HasNetworkCol: true,
					},
					"a_table": {
						Name:    "a_table",
						Columns: []TableColumn{{Name: "root", Type: "String"}},
					},
				},
			},
		},
	}

	listHandler := createTablesListHandler(client)
	payload, err := listHandler(context.Background(), "clickhouse://tables")
	require.NoError(t, err)

	var response TablesListResponse
	require.NoError(t, json.Unmarshal([]byte(payload), &response))
	require.Contains(t, response.Clusters, "xatu")
	assert.Equal(t, 2, response.Clusters["xatu"].TableCount)
	require.Len(t, response.Clusters["xatu"].Tables, 2)
	assert.Equal(t, "a_table", response.Clusters["xatu"].Tables[0].Name)
	assert.Equal(t, "z_table", response.Clusters["xatu"].Tables[1].Name)

	detailHandler := createTableDetailHandler(logrus.New(), client)
	payload, err = detailHandler(context.Background(), "clickhouse://tables/A_TABLE")
	require.NoError(t, err)

	var detail TableDetailResponse
	require.NoError(t, json.Unmarshal([]byte(payload), &detail))
	require.NotNil(t, detail.Table)
	assert.Equal(t, "a_table", detail.Table.Name)
	assert.Equal(t, "xatu", detail.Cluster)
}

func TestTableDetailHandlerListsAvailableTablesOnMissingMatch(t *testing.T) {
	client := &stubSchemaClient{
		clusters: map[string]*ClusterTables{
			"xatu": {
				ClusterName: "xatu",
				Tables: map[string]*TableSchema{
					"blocks": {Name: "blocks"},
					"slots":  {Name: "slots"},
				},
			},
			"xatu-cbt": {
				ClusterName: "xatu-cbt",
				Tables: map[string]*TableSchema{
					"blocks": {Name: "blocks"},
				},
			},
		},
	}

	_, err := createTableDetailHandler(logrus.New(), client)(context.Background(), "clickhouse://tables/missing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `table "missing" not found`)
	assert.Contains(t, err.Error(), "blocks, slots")
}

func TestModuleConfigAndDiscoveryBehavior(t *testing.T) {
	t.Run("init drops nameless schema discovery entries and applies defaults", func(t *testing.T) {
		module := New()
		raw := []byte(`
schema_discovery:
  datasources:
    - name: xatu
    - cluster: ignored
`)

		require.NoError(t, module.Init(raw))
		module.ApplyDefaults()

		require.Len(t, module.cfg.SchemaDiscovery.Datasources, 1)
		assert.Equal(t, "xatu", module.cfg.SchemaDiscovery.Datasources[0].Name)
		assert.Equal(t, 15*time.Minute, module.cfg.SchemaDiscovery.RefreshInterval)
	})

	t.Run("discovery filters non-clickhouse datasources", func(t *testing.T) {
		module := New()

		err := module.InitFromDiscovery([]types.DatasourceInfo{
			{Type: "loki", Name: "logs"},
			{Type: "clickhouse", Name: "xatu"},
			{Type: "clickhouse", Name: "xatu-cbt"},
		})
		require.NoError(t, err)
		require.Len(t, module.datasources, 2)
		assert.Equal(t, []types.DatasourceInfo{
			{Type: "clickhouse", Name: "xatu"},
			{Type: "clickhouse", Name: "xatu-cbt"},
		}, module.datasources)
	})

	t.Run("returns no valid config when discovery finds nothing relevant", func(t *testing.T) {
		module := New()
		err := module.InitFromDiscovery([]types.DatasourceInfo{{Type: "loki", Name: "logs"}})
		assert.ErrorIs(t, err, modulepkg.ErrNoValidConfig)
	})

	t.Run("registers resources when schema client is available", func(t *testing.T) {
		module := New()
		module.schemaClient = &stubSchemaClient{clusters: map[string]*ClusterTables{}}
		registry := &stubResourceRegistry{}

		require.NoError(t, module.RegisterResources(logrus.New(), registry))
		assert.Len(t, registry.staticResources, 1)
		assert.Len(t, registry.templateResources, 1)
	})

	t.Run("start skips disabled or missing datasources safely", func(t *testing.T) {
		disabled := false
		module := New()
		module.cfg.SchemaDiscovery.Enabled = &disabled

		require.NoError(t, module.Start(context.Background()))
		assert.Nil(t, module.schemaClient)

		module = New()
		module.BindRuntimeDependencies(modulepkg.RuntimeDependencies{
			ProxySchemaAccess: &stubProxySchemaAccess{datasources: nil},
		})
		require.NoError(t, module.Start(context.Background()))
		assert.Nil(t, module.schemaClient)
	})

	t.Run("start requires proxy service when enabled", func(t *testing.T) {
		module := New()
		err := module.Start(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "proxy service is required")
	})

	t.Run("documentation surfaces stay populated", func(t *testing.T) {
		assert.NotEmpty(t, New().PythonAPIDocs())
		assert.NotEmpty(t, New().GettingStartedSnippet())
	})
}

func TestSchemaHelpersAndQueries(t *testing.T) {
	t.Run("parses create table statements and helper utilities", func(t *testing.T) {
		schema, err := parseCreateTable("blocks", ""+
			"CREATE TABLE blocks (\n"+
			"  `slot` UInt64,\n"+
			"  `meta_network_name` LowCardinality(String),\n"+
			"  `root` String DEFAULT lower(hex(root)),\n"+
			"  INDEX idx_root root TYPE bloom_filter,\n"+
			"  PROJECTION by_slot (SELECT slot)\n"+
			") ENGINE = MergeTree COMMENT 'Beacon blocks'")
		require.NoError(t, err)
		require.Len(t, schema.Columns, 3)
		assert.Equal(t, "MergeTree", schema.Engine)
		assert.Equal(t, "Beacon blocks", schema.Comment)
		assert.True(t, schema.HasNetworkCol)

		schemaWithComments, err := parseCreateTable("blocks", ""+
			"CREATE TABLE blocks (\n"+
			"  `root` String DEFAULT lower(hex(root)) COMMENT 'root value'\n"+
			") ENGINE = MergeTree")
		require.NoError(t, err)
		require.Len(t, schemaWithComments.Columns, 1)
		assert.Equal(t, "DEFAULT", schemaWithComments.Columns[0].DefaultType)
		assert.Equal(t, "lower(hex(root))", schemaWithComments.Columns[0].DefaultValue)
		assert.Equal(t, "root value", schemaWithComments.Columns[0].Comment)

		assert.Equal(t, "meta_network_name", pickColumn([]clickhouseJSONMeta{{Name: "meta_network_name"}}, "meta_network_name"))
		assert.Equal(t, "fallback", pickColumn([]clickhouseJSONMeta{{Name: "fallback"}}, "missing"))
		assert.Equal(t, "", pickColumn(nil, "missing"))
		assert.Equal(t, "42", asString(42))
		assert.Equal(t, "Array(String)", cleanColumnType("Array(String) COMMENT 'x'"))
		require.NoError(t, validateIdentifier("valid_name_1"))
		require.Error(t, validateIdentifier("invalid-name"))
	})

	t.Run("initializes datasource mappings and copies cached tables", func(t *testing.T) {
		proxyAccess := &stubProxySchemaAccess{}
		client := NewClickHouseSchemaClient(logrus.New(), ClickHouseSchemaConfig{
			Datasources: []SchemaDiscoveryDatasource{
				{Name: "xatu", Cluster: "main"},
				{Name: "other", Cluster: "main"},
			},
		}, proxyAccess).(*clickhouseSchemaClient)

		require.NoError(t, client.initDatasources())
		assert.Equal(t, map[string]string{"main": "xatu"}, client.datasources)

		client.clusters["xatu"] = &ClusterTables{
			ClusterName: "xatu",
			Tables: map[string]*TableSchema{
				"blocks": {Name: "blocks"},
			},
		}

		copied := client.GetAllTables()
		copied["xatu"].Tables["shadow"] = &TableSchema{Name: "shadow"}
		assert.NotContains(t, client.clusters["xatu"].Tables, "shadow")

		table, cluster, ok := client.GetTable("blocks")
		require.True(t, ok)
		assert.Equal(t, "xatu", cluster)
		assert.Equal(t, "blocks", table.Name)
	})

	t.Run("queries proxy-backed schema endpoints", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "/clickhouse/", r.URL.Path)
			assert.Equal(t, "xatu", r.Header.Get("X-Datasource"))
			assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
			assert.Equal(t, "JSON", r.URL.Query().Get("default_format"))

			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)

			switch string(body) {
			case "SHOW TABLES":
				require.NoError(t, json.NewEncoder(w).Encode(clickhouseJSONResponse{
					Meta: []clickhouseJSONMeta{{Name: "name"}},
					Data: []map[string]any{
						{"name": "blocks"},
						{"name": "blocks_local"},
					},
				}))
			case "SHOW CREATE TABLE `blocks`":
				require.NoError(t, json.NewEncoder(w).Encode(clickhouseJSONResponse{
					Meta: []clickhouseJSONMeta{{Name: "statement"}},
					Data: []map[string]any{{
						"statement": "CREATE TABLE blocks (`meta_network_name` String, `slot` UInt64) ENGINE = MergeTree",
					}},
				}))
			case "SELECT DISTINCT meta_network_name FROM `blocks` WHERE meta_network_name IS NOT NULL AND meta_network_name != '' LIMIT 1000":
				require.NoError(t, json.NewEncoder(w).Encode(clickhouseJSONResponse{
					Meta: []clickhouseJSONMeta{{Name: "meta_network_name"}},
					Data: []map[string]any{
						{"meta_network_name": "mainnet"},
						{"meta_network_name": "hoodi"},
					},
				}))
			default:
				t.Fatalf("unexpected SQL: %q", string(body))
			}
		}))
		defer server.Close()

		proxyAccess := &stubProxySchemaAccess{baseURL: server.URL}
		client := NewClickHouseSchemaClient(logrus.New(), ClickHouseSchemaConfig{
			QueryTimeout: time.Second,
		}, proxyAccess).(*clickhouseSchemaClient)
		client.httpClient = server.Client()

		tables, err := client.fetchTableList(context.Background(), "xatu")
		require.NoError(t, err)
		assert.Equal(t, []string{"blocks"}, tables)

		schema, err := client.fetchTableSchema(context.Background(), "xatu", "blocks")
		require.NoError(t, err)
		assert.True(t, schema.HasNetworkCol)

		networks, err := client.fetchTableNetworks(context.Background(), "xatu", "blocks")
		require.NoError(t, err)
		assert.Equal(t, []string{"hoodi", "mainnet"}, networks)
		assert.Equal(t, 3, proxyAccess.authCalls)
	})

	t.Run("surfaces query errors", func(t *testing.T) {
		client := &clickhouseSchemaClient{
			log: logrus.New(),
			cfg: ClickHouseSchemaConfig{QueryTimeout: time.Second},
			proxySvc: &stubProxySchemaAccess{
				baseURL:       "",
				useConfigured: true,
			},
			httpClient: &http.Client{},
		}

		_, err := client.queryJSON(context.Background(), "", "SHOW TABLES")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "datasource name is required")

		_, err = client.queryJSON(context.Background(), "xatu", "SHOW TABLES")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "proxy URL is empty")
	})
}

type stubSchemaClient struct {
	clusters map[string]*ClusterTables
}

func (s *stubSchemaClient) Start(context.Context) error             { return nil }
func (s *stubSchemaClient) Stop() error                             { return nil }
func (s *stubSchemaClient) WaitForReady(context.Context) error      { return nil }
func (s *stubSchemaClient) GetAllTables() map[string]*ClusterTables { return s.clusters }
func (s *stubSchemaClient) GetTable(tableName string) (*TableSchema, string, bool) {
	for clusterName, cluster := range s.clusters {
		if table, ok := cluster.Tables[tableName]; ok {
			return table, clusterName, true
		}
	}

	return nil, "", false
}

type stubProxySchemaAccess struct {
	datasources   []string
	baseURL       string
	authCalls     int
	useConfigured bool
}

func (s *stubProxySchemaAccess) URL() string {
	if s.useConfigured || s.baseURL != "" {
		return s.baseURL
	}

	return "http://proxy.example"
}

func (s *stubProxySchemaAccess) AuthorizeRequest(req *http.Request) error {
	s.authCalls++
	req.Header.Set("Authorization", "Bearer test-token")

	return nil
}

func (s *stubProxySchemaAccess) ClickHouseDatasources() []string { return s.datasources }

type stubResourceRegistry struct {
	staticResources   []types.StaticResource
	templateResources []types.TemplateResource
}

func (s *stubResourceRegistry) RegisterStatic(res types.StaticResource) {
	s.staticResources = append(s.staticResources, res)
}

func (s *stubResourceRegistry) RegisterTemplate(res types.TemplateResource) {
	s.templateResources = append(s.templateResources, res)
}
