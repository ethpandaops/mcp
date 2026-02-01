package cbt

// Config holds the CBT (ClickHouse Build Tool) plugin configuration.
type Config struct {
	// Instances is a list of CBT instances to connect to.
	Instances []InstanceConfig `yaml:"instances"`
}

// InstanceConfig holds configuration for a CBT instance.
type InstanceConfig struct {
	// Name is the logical identifier for this instance (required).
	Name string `yaml:"name" json:"name"`

	// Description provides context about this instance for LLM consumption.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// URL is the base URL for the CBT API (required).
	// Example: http://localhost:8080
	URL string `yaml:"url" json:"url"`

	// UIURL is the base URL for the CBT UI (optional).
	// If not provided, defaults to URL.
	UIURL string `yaml:"ui_url,omitempty" json:"ui_url,omitempty"`

	// Timeout is the request timeout in seconds. Defaults to 60.
	Timeout int `yaml:"timeout,omitempty" json:"timeout,omitempty"`
}

// GetUIURL returns the UI URL, defaulting to the API URL if not set.
func (c *InstanceConfig) GetUIURL() string {
	if c.UIURL != "" {
		return c.UIURL
	}
	return c.URL
}
