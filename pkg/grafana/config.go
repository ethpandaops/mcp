// Package grafana provides a client for interacting with Grafana's datasource proxy.
package grafana

import (
	"errors"
	"time"
)

// Config holds Grafana connection configuration.
type Config struct {
	// URL is the base URL of the Grafana instance.
	URL string `yaml:"url"`

	// ServiceToken is the Grafana service account token for authentication.
	ServiceToken string `yaml:"service_token"`

	// Timeout is the HTTP request timeout in seconds. Defaults to 120.
	Timeout int `yaml:"timeout"`

	// DatasourceUIDs optionally restricts which datasources are available.
	// If empty, all discovered datasources of supported types are used.
	// Deprecated: Use Datasources instead for more control.
	DatasourceUIDs []string `yaml:"datasource_uids,omitempty"`

	// Datasources configures which datasources are available and their descriptions.
	// If specified, only these datasources are exposed (supersedes DatasourceUIDs).
	Datasources []DatasourceConfig `yaml:"datasources,omitempty"`
}

// DatasourceConfig configures a single datasource with optional description.
type DatasourceConfig struct {
	// UID is the Grafana datasource UID (required).
	UID string `yaml:"uid"`

	// Description provides context about this datasource for LLM consumption.
	Description string `yaml:"description,omitempty"`
}

// Validate validates the Grafana configuration.
func (c *Config) Validate() error {
	if c.URL == "" {
		return errors.New("grafana.url is required")
	}

	if c.ServiceToken == "" {
		return errors.New("grafana.service_token is required")
	}

	return nil
}

// GetTimeout returns the configured timeout or the default (120 seconds).
func (c *Config) GetTimeout() time.Duration {
	if c.Timeout <= 0 {
		return 120 * time.Second
	}

	return time.Duration(c.Timeout) * time.Second
}
