package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"golang.org/x/term"

	"github.com/ethpandaops/mcp/pkg/config"
	"github.com/ethpandaops/mcp/pkg/defaults"
	"github.com/ethpandaops/mcp/pkg/plugin"
	"github.com/ethpandaops/mcp/pkg/proxy"
	"github.com/ethpandaops/mcp/pkg/sandbox"
	"github.com/ethpandaops/mcp/pkg/tool"

	clickhouseplugin "github.com/ethpandaops/mcp/plugins/clickhouse"
	lokiplugin "github.com/ethpandaops/mcp/plugins/loki"
	prometheusplugin "github.com/ethpandaops/mcp/plugins/prometheus"
)

// buildPluginRegistry creates a plugin registry with all compiled-in plugins
// and initializes those with config.
func buildPluginRegistry(cfg *config.Config) (*plugin.Registry, error) {
	reg := plugin.NewRegistry(log)

	reg.Add(clickhouseplugin.New())
	reg.Add(prometheusplugin.New())
	reg.Add(lokiplugin.New())

	for _, name := range reg.All() {
		rawYAML, err := cfg.PluginConfigYAML(name)
		if err != nil {
			return nil, fmt.Errorf("getting config for plugin %q: %w", name, err)
		}

		if rawYAML == nil {
			continue
		}

		if err := reg.InitPlugin(name, rawYAML); err != nil {
			return nil, fmt.Errorf("initializing plugin %q: %w", name, err)
		}
	}

	return reg, nil
}

// buildProxyClient creates and starts a proxy client with the given parameters.
// For CLI usage, background refresh is disabled (one-shot discovery).
func buildProxyClient(
	ctx context.Context,
	proxyURL, issuerURL, clientID string,
) (proxy.Client, error) {
	cfg := proxy.ClientConfig{
		URL:               proxyURL,
		IssuerURL:         issuerURL,
		ClientID:          clientID,
		DiscoveryInterval: 0, // No background refresh for CLI
	}

	client := proxy.NewClient(log, cfg)

	if err := client.EnsureAuthenticated(ctx); err != nil {
		return nil, err
	}

	if err := client.Start(ctx); err != nil {
		return nil, fmt.Errorf("starting proxy client: %w", err)
	}

	return client, nil
}

// injectProxyClient passes the proxy client to plugins that need it.
func injectProxyClient(pluginReg *plugin.Registry, client proxy.Service) {
	for _, p := range pluginReg.Initialized() {
		if aware, ok := p.(plugin.ProxyAware); ok {
			aware.SetProxyClient(client)
		}
	}
}

// buildSandboxService creates and starts a sandbox service.
func buildSandboxService(ctx context.Context, cfg *config.Config) (sandbox.Service, error) {
	svc, err := sandbox.New(cfg.Sandbox, log)
	if err != nil {
		return nil, fmt.Errorf("creating sandbox: %w", err)
	}

	if err := svc.Start(ctx); err != nil {
		return nil, fmt.Errorf("starting sandbox: %w", err)
	}

	return svc, nil
}

// buildSandboxEnv creates credential-free environment variables for the sandbox.
// This replicates the logic from pkg/tool/execute_python.go's buildSandboxEnv.
func buildSandboxEnv(
	pluginReg *plugin.Registry,
	proxySvc proxy.Service,
) (map[string]string, error) {
	return tool.BuildSandboxEnv(pluginReg, proxySvc)
}

// loadConfigOrDefaults loads config from file if provided, otherwise returns
// a minimal config with production defaults suitable for CLI usage.
func loadConfigOrDefaults(cfgPath string) (*config.Config, error) {
	if cfgPath != "" {
		return config.Load(cfgPath)
	}

	// Check if CONFIG_PATH env var is set.
	if envPath := os.Getenv("CONFIG_PATH"); envPath != "" {
		return config.Load(envPath)
	}

	// Check if default config.yaml exists.
	if _, err := os.Stat("config.yaml"); err == nil {
		return config.Load("config.yaml")
	}

	// Return minimal config with production defaults.
	return &config.Config{
		Sandbox: config.SandboxConfig{
			Backend: "docker",
			Image:   defaults.SandboxImage,
			Timeout: defaults.SandboxTimeout,
		},
		Proxy: config.ProxyConfig{
			URL: defaults.ProxyURL,
		},
	}, nil
}

// outputJSON marshals a value to JSON and prints it to stdout.
func outputJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}

	fmt.Println(string(data))

	return nil
}

// isTerminal returns true if stdout is a terminal (TTY).
func isTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// suppressLogs sets log output to discard when not in verbose mode.
// CLI commands should be quiet by default, only showing output.
func suppressLogs() {
	if log.GetLevel() < logrus.DebugLevel {
		log.SetOutput(os.Stderr)
		log.SetLevel(logrus.WarnLevel)
	}
}
