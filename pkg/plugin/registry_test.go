package plugin

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPlugin is a mock implementation of the Plugin interface for testing.
type mockPlugin struct {
	name         string
	healthStatus HealthStatus
	healthMsg    string
}

func (m *mockPlugin) Name() string { return m.name }
func (m *mockPlugin) Init([]byte) error { return nil }
func (m *mockPlugin) ApplyDefaults() {}
func (m *mockPlugin) Validate() error { return nil }
func (m *mockPlugin) SandboxEnv() (map[string]string, error) { return nil, nil }
func (m *mockPlugin) DatasourceInfo() []struct {
	Type        string
	Name        string
	Description string
	Metadata    map[string]string
} {
	return nil
}
func (m *mockPlugin) Examples() map[string]struct {
	Description string
	Examples    []struct {
		Name        string
		Description string
		Query       string
	}
} {
	return nil
}
func (m *mockPlugin) PythonAPIDocs() map[string]struct {
	Description string
	Functions   map[string]struct {
		Signature   string
		Description string
		Parameters  map[string]string
		Returns     string
	}
} {
	return nil
}
func (m *mockPlugin) GettingStartedSnippet() string { return "" }
func (m *mockPlugin) RegisterResources(logrus.FieldLogger, ResourceRegistry) error { return nil }
func (m *mockPlugin) Start(context.Context) error { return nil }
func (m *mockPlugin) Stop(context.Context) error { return nil }
func (m *mockPlugin) HealthCheck(ctx context.Context) HealthCheckResult {
	return HealthCheckResult{
		Status:    m.healthStatus,
		Message:   m.healthMsg,
		CheckedAt: time.Now().UTC(),
	}
}

func TestRegistry_HealthChecks_Empty(t *testing.T) {
	log := logrus.New()
	reg := NewRegistry(log)

	ctx := context.Background()
	results := reg.HealthChecks(ctx)

	assert.NotNil(t, results)
	assert.Empty(t, results)
}

func TestRegistry_HealthChecks_WithPlugins(t *testing.T) {
	log := logrus.New()
	reg := NewRegistry(log)

	// Add mock plugins
	plugin1 := &mockPlugin{
		name:         "plugin1",
		healthStatus: HealthStatusHealthy,
		healthMsg:    "All good",
	}
	plugin2 := &mockPlugin{
		name:         "plugin2",
		healthStatus: HealthStatusUnhealthy,
		healthMsg:    "Connection failed",
	}
	plugin3 := &mockPlugin{
		name:         "plugin3",
		healthStatus: HealthStatusUnknown,
		healthMsg:    "Not checked",
	}

	reg.Add(plugin1)
	reg.Add(plugin2)
	reg.Add(plugin3)

	// Manually add to initialized slice for testing
	reg.initialized = append(reg.initialized, plugin1, plugin2, plugin3)

	ctx := context.Background()
	results := reg.HealthChecks(ctx)

	require.Len(t, results, 3)

	// Check plugin1
	assert.Equal(t, HealthStatusHealthy, results["plugin1"].Status)
	assert.Equal(t, "All good", results["plugin1"].Message)
	assert.False(t, results["plugin1"].CheckedAt.IsZero())

	// Check plugin2
	assert.Equal(t, HealthStatusUnhealthy, results["plugin2"].Status)
	assert.Equal(t, "Connection failed", results["plugin2"].Message)

	// Check plugin3
	assert.Equal(t, HealthStatusUnknown, results["plugin3"].Status)
	assert.Equal(t, "Not checked", results["plugin3"].Message)
}

func TestHealthCheckResult_Struct(t *testing.T) {
	now := time.Now().UTC()
	result := HealthCheckResult{
		Status:    HealthStatusHealthy,
		Message:   "Test message",
		CheckedAt: now,
	}

	assert.Equal(t, HealthStatusHealthy, result.Status)
	assert.Equal(t, "Test message", result.Message)
	assert.True(t, result.CheckedAt.Equal(now))
}

func TestHealthStatus_String(t *testing.T) {
	assert.Equal(t, "healthy", string(HealthStatusHealthy))
	assert.Equal(t, "unhealthy", string(HealthStatusUnhealthy))
	assert.Equal(t, "unknown", string(HealthStatusUnknown))
}
