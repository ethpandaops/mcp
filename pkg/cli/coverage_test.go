package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clickhousemodule "github.com/ethpandaops/panda/modules/clickhouse"
	"github.com/ethpandaops/panda/pkg/operations"
	"github.com/ethpandaops/panda/pkg/serverapi"
	"github.com/ethpandaops/panda/pkg/types"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type cliHarness struct {
	server *httptest.Server
}

func newCLIHarness(t *testing.T, handler http.Handler) *cliHarness {
	t.Helper()

	server := httptest.NewServer(handler)

	originalCfgFile := cfgFile
	originalServerHTTP := serverHTTP
	originalLog := log
	originalLogLevel := logLevel
	originalDatasourcesType := datasourcesType
	originalDatasourcesJSON := datasourcesJSON
	originalDocsJSON := docsJSON
	originalExecuteCode := executeCode
	originalExecuteFile := executeFile
	originalExecuteTimeout := executeTimeout
	originalExecuteSession := executeSession
	originalExecuteJSON := executeJSON
	originalClickHouseJSON := clickhouseJSON
	originalDoraJSON := doraJSON

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	config := strings.Join([]string{
		"server:",
		"  url: " + server.URL,
		"proxy:",
		"  url: https://proxy.example",
		"sandbox:",
		"  image: sandbox:test",
		"",
	}, "\n")
	require.NoError(t, os.WriteFile(configPath, []byte(config), 0o600))

	cfgFile = configPath
	serverHTTP = server.Client()
	log = logrus.New()
	logLevel = "info"
	datasourcesType = ""
	datasourcesJSON = false
	docsJSON = false
	executeCode = ""
	executeFile = ""
	executeTimeout = 0
	executeSession = ""
	executeJSON = false
	clickhouseJSON = false
	doraJSON = false

	t.Cleanup(func() {
		server.Close()
		cfgFile = originalCfgFile
		serverHTTP = originalServerHTTP
		log = originalLog
		logLevel = originalLogLevel
		datasourcesType = originalDatasourcesType
		datasourcesJSON = originalDatasourcesJSON
		docsJSON = originalDocsJSON
		executeCode = originalExecuteCode
		executeFile = originalExecuteFile
		executeTimeout = originalExecuteTimeout
		executeSession = originalExecuteSession
		executeJSON = originalExecuteJSON
		clickhouseJSON = originalClickHouseJSON
		doraJSON = originalDoraJSON
	})

	return &cliHarness{server: server}
}

func captureOutput(t *testing.T, fn func()) (string, string) {
	t.Helper()

	originalStdout := os.Stdout
	originalStderr := os.Stderr

	stdoutReader, stdoutWriter, err := os.Pipe()
	require.NoError(t, err)

	stderrReader, stderrWriter, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = stdoutWriter
	os.Stderr = stderrWriter

	t.Cleanup(func() {
		os.Stdout = originalStdout
		os.Stderr = originalStderr
	})

	fn()

	require.NoError(t, stdoutWriter.Close())
	require.NoError(t, stderrWriter.Close())

	stdoutBytes, err := io.ReadAll(stdoutReader)
	require.NoError(t, err)

	stderrBytes, err := io.ReadAll(stderrReader)
	require.NoError(t, err)

	os.Stdout = originalStdout
	os.Stderr = originalStderr

	return string(stdoutBytes), string(stderrBytes)
}

func withStdin(t *testing.T, input string, fn func()) {
	t.Helper()

	originalStdin := os.Stdin
	reader, writer, err := os.Pipe()
	require.NoError(t, err)

	_, err = writer.WriteString(input)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	os.Stdin = reader
	t.Cleanup(func() {
		os.Stdin = originalStdin
	})

	fn()

	os.Stdin = originalStdin
}

func writeJSONResponse(t *testing.T, w http.ResponseWriter, status int, payload any) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	require.NoError(t, json.NewEncoder(w).Encode(payload))
}

func TestRootCmdPersistentPreRunE(t *testing.T) {
	originalLog := log
	originalLogLevel := logLevel

	t.Cleanup(func() {
		log = originalLog
		logLevel = originalLogLevel
	})

	log = logrus.New()
	logLevel = "debug"

	require.NoError(t, rootCmd.PersistentPreRunE(rootCmd, nil))
	assert.Equal(t, logrus.DebugLevel, log.GetLevel())

	formatter, ok := log.Formatter.(*logrus.TextFormatter)
	require.True(t, ok)
	assert.True(t, formatter.FullTimestamp)

	logLevel = "definitely-not-a-level"
	require.Error(t, rootCmd.PersistentPreRunE(rootCmd, nil))
}

func TestDatasourcesAndCompletionHelpers(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/datasources", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)

		datasources := []types.DatasourceInfo{
			{Type: "clickhouse", Name: "xatu", Description: "Xatu warehouse"},
			{Type: "prometheus", Name: "metrics", Description: ""},
		}

		if filterType := r.URL.Query().Get("type"); filterType != "" {
			filtered := make([]types.DatasourceInfo, 0, len(datasources))
			for _, datasource := range datasources {
				if datasource.Type == filterType {
					filtered = append(filtered, datasource)
				}
			}

			datasources = filtered
		}

		writeJSONResponse(t, w, http.StatusOK, serverapi.DatasourcesResponse{Datasources: datasources})
	})
	mux.HandleFunc("/api/v1/sessions", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)

		writeJSONResponse(t, w, http.StatusOK, serverapi.ListSessionsResponse{
			Sessions: []serverapi.SessionResponse{
				{SessionID: "sess-a", CreatedAt: time.Unix(10, 0), TTLRemaining: "5m"},
				{SessionID: "sess-b", CreatedAt: time.Unix(20, 0), TTLRemaining: "4m"},
			},
			Total: 2,
		})
	})
	mux.HandleFunc("/api/v1/resources/read", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "clickhouse://tables", r.URL.Query().Get("uri"))

		writeJSONResponse(t, w, http.StatusOK, clickhousemodule.TablesListResponse{
			Clusters: map[string]*clickhousemodule.ClusterTablesSummary{
				"xatu": {
					Tables: []*clickhousemodule.TableSummary{
						{Name: "blocks"},
						{Name: "attestations"},
					},
				},
			},
		})
	})

	newCLIHarness(t, mux)

	stdout, _ := captureOutput(t, func() {
		require.NoError(t, runDatasources(datasourcesCmd, nil))
	})
	assert.Contains(t, stdout, "clickhouse")
	assert.Contains(t, stdout, "Xatu warehouse")
	assert.Contains(t, stdout, "metrics")

	datasourcesJSON = true

	stdout, _ = captureOutput(t, func() {
		require.NoError(t, runDatasources(datasourcesCmd, nil))
	})
	assert.Contains(t, stdout, `"datasources"`)
	assert.Contains(t, stdout, `"xatu"`)

	names, directive := completeDatasourceNames("clickhouse")(datasourcesCmd, nil, "")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	assert.Equal(t, []string{"xatu"}, names)

	names, directive = completeDatasourceNames("clickhouse")(datasourcesCmd, []string{"already"}, "")
	assert.Nil(t, names)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)

	sessionIDs, directive := completeSessionIDs(datasourcesCmd, nil, "")
	assert.Equal(t, []string{"sess-a", "sess-b"}, sessionIDs)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)

	tableNames, directive := completeTableNames(datasourcesCmd, nil, "")
	assert.Equal(t, []string{"blocks", "attestations"}, tableNames)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)

	noValues, directive := noCompletions(datasourcesCmd, nil, "")
	assert.Nil(t, noValues)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

func TestDocsAndDoraCommands(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/resources/read", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "python://ethpandaops", r.URL.Query().Get("uri"))

		writeJSONResponse(t, w, http.StatusOK, serverapi.APIDocResponse{
			Library: "ethpandaops",
			Modules: map[string]types.ModuleDoc{
				"clickhouse": {
					Description: "Query ClickHouse",
					Functions: map[string]types.FunctionDoc{
						"query": {
							Signature:   "query(sql: str) -> DataFrame",
							Description: "Execute SQL",
							Parameters:  map[string]string{"sql": "SQL query"},
							Returns:     "Rows",
							Example:     "query('SELECT 1')",
						},
					},
				},
				"dora": {
					Description: "Query Dora",
					Functions:   map[string]types.FunctionDoc{},
				},
			},
		})
	})
	mux.HandleFunc("/api/v1/operations/dora.list_networks", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResponse(t, w, http.StatusOK, operations.NewObjectResponse(operations.DoraNetworksPayload{
			Networks: []operations.DoraNetwork{
				{Name: "hoodi", DoraURL: "https://hoodi.example"},
				{Name: "mainnet", DoraURL: "https://mainnet.example"},
			},
		}, nil))
	})
	mux.HandleFunc("/api/v1/operations/dora.get_network_overview", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResponse(t, w, http.StatusOK, operations.NewObjectResponse(operations.DoraOverviewPayload{
			CurrentEpoch:          123,
			CurrentSlot:           456,
			Finalized:             true,
			ParticipationRate:     0.99,
			ActiveValidatorCount:  10,
			TotalValidatorCount:   12,
			PendingValidatorCount: 1,
			ExitedValidatorCount:  1,
		}, nil))
	})

	rawResponse := []byte(`{"ok":true,"name":"hoodi"}`)
	mux.HandleFunc("/api/v1/operations/dora.get_validator", func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write(rawResponse)
		require.NoError(t, err)
	})
	mux.HandleFunc("/api/v1/operations/dora.get_slot", func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write(rawResponse)
		require.NoError(t, err)
	})
	mux.HandleFunc("/api/v1/operations/dora.get_epoch", func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write(rawResponse)
		require.NoError(t, err)
	})

	newCLIHarness(t, mux)

	stdout, _ := captureOutput(t, func() {
		require.NoError(t, runDocs(docsCmd, nil))
	})
	assert.Contains(t, stdout, "Available modules:")
	assert.Contains(t, stdout, "clickhouse")

	stdout, _ = captureOutput(t, func() {
		require.NoError(t, runDocs(docsCmd, []string{"clickhouse"}))
	})
	assert.Contains(t, stdout, "Module: clickhouse")
	assert.Contains(t, stdout, "query(sql: str) -> DataFrame")

	docsJSON = true
	stdout, _ = captureOutput(t, func() {
		require.NoError(t, runDocs(docsCmd, []string{"clickhouse"}))
	})
	assert.Contains(t, stdout, `"clickhouse"`)
	assert.Contains(t, stdout, `"signature": "query(sql: str) -\u003e DataFrame"`)

	networkNames, directive := completeNetworkNames(docsCmd, nil, "")
	assert.Equal(t, []string{"hoodi", "mainnet"}, networkNames)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)

	doraJSON = false
	stdout, _ = captureOutput(t, func() {
		require.NoError(t, doraNetworksCmd.RunE(doraNetworksCmd, nil))
	})
	assert.Contains(t, stdout, "hoodi")
	assert.Contains(t, stdout, "https://hoodi.example")

	stdout, _ = captureOutput(t, func() {
		require.NoError(t, doraOverviewCmd.RunE(doraOverviewCmd, []string{"hoodi"}))
	})
	assert.Contains(t, stdout, "Current epoch:      123")
	assert.Contains(t, stdout, "Active validators:  10")

	stdout, _ = captureOutput(t, func() {
		require.NoError(t, doraValidatorCmd.RunE(doraValidatorCmd, []string{"hoodi", "123"}))
	})
	assert.Contains(t, stdout, `"ok": true`)

	stdout, _ = captureOutput(t, func() {
		require.NoError(t, doraSlotCmd.RunE(doraSlotCmd, []string{"hoodi", "456"}))
	})
	assert.Contains(t, stdout, `"name": "hoodi"`)

	stdout, _ = captureOutput(t, func() {
		require.NoError(t, doraEpochCmd.RunE(doraEpochCmd, []string{"hoodi", "7"}))
	})
	assert.Contains(t, stdout, `"ok": true`)
}

func TestExecuteHelpersAndCommand(t *testing.T) {
	t.Run("resolveCode prefers explicit sources", func(t *testing.T) {
		newCLIHarness(t, http.NewServeMux())

		executeCode = "print('inline')"
		code, err := resolveCode()
		require.NoError(t, err)
		assert.Equal(t, "print('inline')", code)

		executeCode = ""
		executeFile = filepath.Join(t.TempDir(), "script.py")
		require.NoError(t, os.WriteFile(executeFile, []byte("print('file')"), 0o600))

		code, err = resolveCode()
		require.NoError(t, err)
		assert.Equal(t, "print('file')", code)

		executeFile = ""
		withStdin(t, "print('stdin')", func() {
			code, err = resolveCode()
			require.NoError(t, err)
			assert.Equal(t, "print('stdin')", code)
		})
	})

	t.Run("runExecute prints result metadata", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/v1/execute", func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodPost, r.Method)

			var request serverapi.ExecuteRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
			assert.Equal(t, "print('hello')", request.Code)
			assert.Equal(t, 15, request.Timeout)
			assert.Equal(t, "sess-1", request.SessionID)

			writeJSONResponse(t, w, http.StatusOK, serverapi.ExecuteResponse{
				Stdout:              "hello\n",
				Stderr:              "warning\n",
				ExitCode:            0,
				ExecutionID:         "exec-1",
				OutputFiles:         []string{"plot.png"},
				SessionID:           "sess-1",
				SessionTTLRemaining: "9m",
			})
		})

		newCLIHarness(t, mux)
		executeCode = "print('hello')"
		executeTimeout = 15
		executeSession = "sess-1"

		stdout, stderr := captureOutput(t, func() {
			require.NoError(t, runExecute(executeCmd, nil))
		})
		assert.Equal(t, "hello\n", stdout)
		assert.Contains(t, stderr, "warning")
		assert.Contains(t, stderr, "[files] plot.png")
		assert.Contains(t, stderr, "[session] sess-1 (ttl: 9m)")
	})

	t.Run("runExecute returns exit status failures", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/v1/execute", func(w http.ResponseWriter, _ *http.Request) {
			writeJSONResponse(t, w, http.StatusOK, serverapi.ExecuteResponse{
				ExitCode:    7,
				ExecutionID: "exec-7",
			})
		})

		newCLIHarness(t, mux)
		executeCode = "print('bad')"

		err := runExecute(executeCmd, nil)
		require.Error(t, err)
		assert.EqualError(t, err, "exit code 7")
	})
}

func TestClickHouseCommands(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/operations/clickhouse.list_datasources", func(w http.ResponseWriter, _ *http.Request) {
		writeJSONResponse(t, w, http.StatusOK, operations.NewObjectResponse(operations.DatasourcesPayload{
			Datasources: []operations.Datasource{
				{Name: "xatu", Description: "Warehouse", Database: "default"},
			},
		}, nil))
	})

	tsv := []byte("name\tcount\nblocks\t5\n")
	mux.HandleFunc("/api/v1/operations/clickhouse.query", func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write(tsv)
		require.NoError(t, err)
	})
	mux.HandleFunc("/api/v1/operations/clickhouse.query_raw", func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write(tsv)
		require.NoError(t, err)
	})

	newCLIHarness(t, mux)

	stdout, _ := captureOutput(t, func() {
		require.NoError(t, clickhouseListDatasourcesCmd.RunE(clickhouseListDatasourcesCmd, nil))
	})
	assert.Contains(t, stdout, "xatu")
	assert.Contains(t, stdout, "database=default")

	stdout, _ = captureOutput(t, func() {
		require.NoError(t, runClickHouseQuery("xatu", "SELECT 1"))
	})
	assert.Equal(t, string(tsv), stdout)

	clickhouseJSON = true
	stdout, _ = captureOutput(t, func() {
		require.NoError(t, runClickHouseQuery("xatu", "SELECT 1"))
	})
	assert.Contains(t, stdout, `"columns": [`)
	assert.Contains(t, stdout, `"name": "blocks"`)

	stdout, _ = captureOutput(t, func() {
		require.NoError(t, runClickHouseRawQuery("xatu", "SELECT 1"))
	})
	assert.Contains(t, stdout, `"rows": [`)
	assert.Contains(t, stdout, `"blocks"`)

	columns, rows, err := parseClickHouseTSV([]byte("col_a\tcol_b\n1\t2\n"))
	require.NoError(t, err)
	assert.Equal(t, []string{"col_a", "col_b"}, columns)
	assert.Equal(t, [][]string{{"1", "2"}}, rows)

	columns, rows, err = parseClickHouseTSV([]byte("   \n"))
	require.NoError(t, err)
	assert.Nil(t, columns)
	assert.Nil(t, rows)
}

func TestServerHelperWrappers(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/inspect", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "bar", r.URL.Query().Get("foo"))
		assert.Equal(t, "value", r.Header.Get("X-Test"))

		w.Header().Set("X-Reply", "ok")
		_, err := w.Write([]byte("inspected"))
		require.NoError(t, err)
	})
	mux.HandleFunc("/inspect-json", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		writeJSONResponse(t, w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/inspect-bad-json", func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write([]byte("{"))
		require.NoError(t, err)
	})
	mux.HandleFunc("/inspect-post", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var payload map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		assert.Equal(t, "world", payload["hello"])

		writeJSONResponse(t, w, http.StatusOK, map[string]string{"status": "created"})
	})
	mux.HandleFunc("/inspect-post-error", func(w http.ResponseWriter, _ *http.Request) {
		writeJSONResponse(t, w, http.StatusBadRequest, map[string]string{"error": "nope"})
	})
	mux.HandleFunc("/api/v1/proxy/auth", func(w http.ResponseWriter, _ *http.Request) {
		writeJSONResponse(t, w, http.StatusOK, serverapi.ProxyAuthMetadataResponse{
			Enabled:   true,
			IssuerURL: "https://issuer.example",
			ClientID:  "panda",
			Resource:  "https://proxy.example",
		})
	})
	mux.HandleFunc("/api/v1/sessions", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSONResponse(t, w, http.StatusOK, serverapi.ListSessionsResponse{
				Sessions: []serverapi.SessionResponse{{SessionID: "session-1"}},
				Total:    1,
			})
		case http.MethodPost:
			writeJSONResponse(t, w, http.StatusOK, serverapi.CreateSessionResponse{
				SessionID:    "session-2",
				TTLRemaining: "10m",
			})
		default:
			t.Fatalf("unexpected sessions method: %s", r.Method)
		}
	})
	mux.HandleFunc("/api/v1/sessions/session%2F2", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodDelete, r.Method)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/api/v1/search/examples", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "validators", r.URL.Query().Get("category"))
		assert.Equal(t, "5", r.URL.Query().Get("limit"))

		writeJSONResponse(t, w, http.StatusOK, serverapi.SearchExamplesResponse{
			Query:        r.URL.Query().Get("query"),
			TotalMatches: 1,
			Results: []*serverapi.SearchExampleResult{{
				ExampleName: "validator count",
			}},
		})
	})
	mux.HandleFunc("/api/v1/search/runbooks", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "finality", r.URL.Query().Get("tag"))
		assert.Equal(t, "3", r.URL.Query().Get("limit"))

		writeJSONResponse(t, w, http.StatusOK, serverapi.SearchRunbooksResponse{
			Query:        r.URL.Query().Get("query"),
			TotalMatches: 1,
			Results: []*serverapi.SearchRunbookResult{{
				Name: "Network not finalizing",
			}},
		})
	})
	mux.HandleFunc("/api/v1/resources/read", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("uri") {
		case "clickhouse://tables/example_table":
			writeJSONResponse(t, w, http.StatusOK, clickhousemodule.TableDetailResponse{
				Cluster: "xatu",
				Table: &clickhousemodule.TableSchema{
					Name: "example_table",
				},
			})
		default:
			w.Header().Set("Content-Type", "text/plain")
			_, err := w.Write([]byte("plain text resource"))
			require.NoError(t, err)
		}
	})
	mux.HandleFunc("/api/v1/operations/", func(w http.ResponseWriter, r *http.Request) {
		op := strings.TrimPrefix(r.URL.Path, "/api/v1/operations/")

		switch op {
		case "prometheus.list_datasources":
			writeJSONResponse(t, w, http.StatusOK, operations.NewObjectResponse(operations.DatasourcesPayload{
				Datasources: []operations.Datasource{{Name: "prom"}},
			}, nil))
		case "loki.list_datasources":
			writeJSONResponse(t, w, http.StatusOK, operations.NewObjectResponse(operations.DatasourcesPayload{
				Datasources: []operations.Datasource{{Name: "logs"}},
			}, nil))
		case "ethnode.get_node_syncing":
			writeJSONResponse(t, w, http.StatusOK, operations.NewObjectResponse(map[string]any{
				"data": map[string]any{
					"head_slot":     "10",
					"sync_distance": "2",
					"is_syncing":    true,
					"is_optimistic": false,
					"el_offline":    false,
				},
			}, nil))
		case "ethnode.get_node_version":
			writeJSONResponse(t, w, http.StatusOK, operations.NewObjectResponse(map[string]any{
				"data": map[string]any{"version": "Lighthouse/v1"},
			}, nil))
		case "ethnode.web3_client_version":
			writeJSONResponse(t, w, http.StatusOK, operations.NewObjectResponse("reth/v1", nil))
		case "ethnode.get_node_health":
			writeJSONResponse(t, w, http.StatusOK, operations.NewObjectResponse(map[string]any{"status_code": 200}, nil))
		case "ethnode.get_peer_count":
			writeJSONResponse(t, w, http.StatusOK, operations.NewObjectResponse(map[string]any{
				"data": map[string]any{
					"connected":     "5",
					"disconnected":  "0",
					"connecting":    "1",
					"disconnecting": "0",
				},
			}, nil))
		case "ethnode.get_finality_checkpoints":
			writeJSONResponse(t, w, http.StatusOK, operations.NewObjectResponse(map[string]any{
				"data": map[string]any{
					"finalized":          map[string]any{"epoch": "100"},
					"current_justified":  map[string]any{"epoch": "101"},
					"previous_justified": map[string]any{"epoch": "99"},
				},
			}, nil))
		case "ethnode.get_beacon_headers":
			writeJSONResponse(t, w, http.StatusOK, operations.NewObjectResponse(map[string]any{
				"data": map[string]any{
					"root": "0xabc",
					"header": map[string]any{
						"message": map[string]any{
							"slot":           "123",
							"proposer_index": "1",
							"parent_root":    "0xdef",
							"state_root":     "0xghi",
							"body_root":      "0xjkl",
						},
					},
				},
			}, nil))
		case "ethnode.eth_block_number":
			writeJSONResponse(t, w, http.StatusOK, operations.NewObjectResponse(map[string]any{
				"hex":          "0x10",
				"block_number": 16,
			}, nil))
		case "prometheus.query", "prometheus.query_range", "prometheus.get_labels", "prometheus.get_label_values",
			"loki.query", "loki.query_instant", "loki.get_labels", "loki.get_label_values",
			"ethnode.beacon_get", "ethnode.execution_rpc":
			w.Header().Set("Content-Type", "application/json")
			_, err := w.Write([]byte(`{"ok":true}`))
			require.NoError(t, err)
		default:
			t.Fatalf("unexpected operation %q", op)
		}
	})

	harness := newCLIHarness(t, mux)

	baseURL, err := serverBaseURL()
	require.NoError(t, err)
	assert.Equal(t, harness.server.URL, baseURL)

	data, status, headers, err := serverDo(
		context.Background(),
		http.MethodGet,
		"/inspect",
		nil,
		url.Values{"foo": []string{"bar"}},
		map[string]string{"X-Test": "value"},
	)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "inspected", string(data))
	assert.Equal(t, "ok", headers.Get("X-Reply"))

	var generic map[string]string
	require.NoError(t, serverGetJSON(context.Background(), "/inspect-json", nil, &generic))
	assert.Equal(t, "ok", generic["status"])

	err = serverGetJSON(context.Background(), "/inspect-bad-json", nil, &generic)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decoding response")

	err = serverPostJSON(context.Background(), "/inspect-post", map[string]string{"hello": "world"}, nil)
	require.NoError(t, err)

	err = serverPostJSON(context.Background(), "/inspect-post-error", map[string]string{"hello": "world"}, nil)
	require.Error(t, err)
	assert.EqualError(t, err, "HTTP 400: nope")

	metadata, err := proxyAuthMetadata(context.Background())
	require.NoError(t, err)
	assert.True(t, metadata.Enabled)
	assert.Equal(t, "https://issuer.example", metadata.IssuerURL)

	created, err := createSession(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "session-2", created.SessionID)

	require.NoError(t, destroySession(context.Background(), "session/2"))

	exampleResults, err := searchExamples(context.Background(), "validator", "validators", 5)
	require.NoError(t, err)
	assert.Equal(t, "validator", exampleResults.Query)
	require.Len(t, exampleResults.Results, 1)

	runbookResults, err := searchRunbooks(context.Background(), "finality", "finality", 3)
	require.NoError(t, err)
	assert.Equal(t, "finality", runbookResults.Query)
	require.Len(t, runbookResults.Results, 1)

	resource, err := readResource(context.Background(), "plain://resource")
	require.NoError(t, err)
	assert.Equal(t, "text/plain", resource.MIMEType)
	assert.Equal(t, "plain text resource", resource.Content)

	tableDetail, err := readClickHouseTable(context.Background(), "example_table")
	require.NoError(t, err)
	require.NotNil(t, tableDetail.Table)
	assert.Equal(t, "example_table", tableDetail.Table.Name)

	prometheusDatasources, err := listPrometheusDatasources()
	require.NoError(t, err)
	assert.Equal(t, "prom", prometheusDatasources[0].Name)

	lokiDatasources, err := listLokiDatasources()
	require.NoError(t, err)
	assert.Equal(t, "logs", lokiDatasources[0].Name)

	rawResponse, err := prometheusQuery(operations.PrometheusQueryArgs{Datasource: "prom", Query: "up"})
	require.NoError(t, err)
	assert.Equal(t, `{"ok":true}`, string(rawResponse.Body))
	assert.Equal(t, "application/json", rawResponse.ContentType)

	_, err = prometheusQueryRange(operations.PrometheusRangeQueryArgs{Datasource: "prom", Query: "up", Start: "1", End: "2", Step: "3"})
	require.NoError(t, err)
	_, err = prometheusLabels(operations.DatasourceArgs{Datasource: "prom"})
	require.NoError(t, err)
	_, err = prometheusLabelValues(operations.DatasourceLabelArgs{Datasource: "prom", Label: "job"})
	require.NoError(t, err)
	_, err = lokiQuery(operations.LokiQueryArgs{Datasource: "logs", Query: "{job=\"panda\"}"})
	require.NoError(t, err)
	_, err = lokiInstantQuery(operations.LokiInstantQueryArgs{Datasource: "logs", Query: "{job=\"panda\"}"})
	require.NoError(t, err)
	_, err = lokiLabels(operations.LokiLabelsArgs{Datasource: "logs"})
	require.NoError(t, err)
	_, err = lokiLabelValues(operations.LokiLabelValuesArgs{Datasource: "logs", Label: "job"})
	require.NoError(t, err)

	syncing, err := ethNodeSyncing(operations.EthNodeNodeArgs{Network: "hoodi", Instance: "beacon"})
	require.NoError(t, err)
	assert.Equal(t, "10", syncing.Data.HeadSlot)

	version, err := ethNodeVersion(operations.EthNodeNodeArgs{Network: "hoodi", Instance: "beacon"})
	require.NoError(t, err)
	assert.Equal(t, "Lighthouse/v1", version.Data.Version)

	clientVersion, err := ethNodeExecutionClientVersion(operations.EthNodeNodeArgs{Network: "hoodi", Instance: "execution"})
	require.NoError(t, err)
	assert.Equal(t, "reth/v1", clientVersion)

	health, err := ethNodeHealth(operations.EthNodeNodeArgs{Network: "hoodi", Instance: "beacon"})
	require.NoError(t, err)
	assert.Equal(t, 200, health.StatusCode)

	peers, err := ethNodePeerCount(operations.EthNodeNodeArgs{Network: "hoodi", Instance: "beacon"})
	require.NoError(t, err)
	assert.Equal(t, "5", peers.Data.Connected)

	finality, err := ethNodeFinality(operations.EthNodeFinalityArgs{Network: "hoodi", Instance: "beacon"})
	require.NoError(t, err)
	assert.Equal(t, "100", finality.Data.Finalized.Epoch)

	headersPayload, err := ethNodeHeaders(operations.EthNodeBeaconHeadersArgs{Network: "hoodi", Instance: "beacon"})
	require.NoError(t, err)
	assert.Equal(t, "123", headersPayload.Data.Header.Message.Slot)

	blockNumber, err := ethNodeBlockNumber(operations.EthNodeNodeArgs{Network: "hoodi", Instance: "execution"})
	require.NoError(t, err)
	assert.EqualValues(t, 16, blockNumber.BlockNumber)

	_, err = ethNodeBeaconGet(operations.EthNodeBeaconGetArgs{Network: "hoodi", Instance: "beacon", Path: "/eth/v1/node/health"})
	require.NoError(t, err)
	_, err = ethNodeExecutionRPC(operations.EthNodeExecutionRPCArgs{Network: "hoodi", Instance: "execution", Method: "eth_chainId"})
	require.NoError(t, err)

	assert.EqualError(t, decodeAPIError(http.StatusBadRequest, []byte(`{"error":"bad request"}`)), "HTTP 400: bad request")
	assert.EqualError(t, decodeAPIError(http.StatusInternalServerError, []byte("server failed")), "HTTP 500: server failed")
}

func TestPrintJSON(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		require.NoError(t, printJSON(map[string]string{"hello": "world"}))
	})
	assert.JSONEq(t, `{"hello":"world"}`, strings.TrimSpace(stdout))
}

func TestPrintJSONBytesFallsBackToRawPayload(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		require.NoError(t, printJSONBytes([]byte("not-json")))
	})
	assert.Equal(t, "not-json\n", stdout)
}

func TestPrintClickHouseJSONMapsRows(t *testing.T) {
	var output bytes.Buffer
	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = writer
	t.Cleanup(func() { os.Stdout = originalStdout })

	require.NoError(t, printClickHouseJSON([]byte("name\tcount\nblocks\t2\n"), false))
	require.NoError(t, writer.Close())
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	output.Write(data)
	os.Stdout = originalStdout

	assert.Contains(t, output.String(), `"count": "2"`)
}
