package spamoor

// Config holds the Spamoor plugin configuration.
type Config struct {
	// Enabled controls whether the Spamoor plugin is active.
	// Defaults to false.
	Enabled *bool `yaml:"enabled,omitempty"`
}

// IsEnabled returns true if the plugin is enabled (default: false).
func (c *Config) IsEnabled() bool {
	if c.Enabled == nil {
		return false
	}

	return *c.Enabled
}
