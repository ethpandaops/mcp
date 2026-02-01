package lab

// Config holds the Lab plugin configuration.
// Lab is enabled by default since it's a public service and
// requires no credentials.
type Config struct {
	// Enabled controls whether the Lab plugin is active.
	// Defaults to true.
	Enabled *bool `yaml:"enabled,omitempty"`

	// RoutesURL is the URL to fetch the lab routes.json from.
	// Defaults to https://raw.githubusercontent.com/ethpandaops/lab/main/routes.json
	RoutesURL string `yaml:"routes_url,omitempty"`

	// SkillURL is the URL to fetch the lab SKILL.md from.
	// Defaults to https://raw.githubusercontent.com/ethpandaops/lab/main/SKILL.md
	SkillURL string `yaml:"skill_url,omitempty"`

	// Networks allows specifying custom Lab URLs for networks.
	// If not specified, default URLs will be used for known networks.
	Networks map[string]string `yaml:"networks,omitempty"`
}

// IsEnabled returns true if the plugin is enabled (default: true).
func (c *Config) IsEnabled() bool {
	if c.Enabled == nil {
		return true
	}

	return *c.Enabled
}

// GetRoutesURL returns the routes.json URL with default.
func (c *Config) GetRoutesURL() string {
	if c.RoutesURL == "" {
		return "https://raw.githubusercontent.com/ethpandaops/lab/main/routes.json"
	}
	return c.RoutesURL
}

// GetSkillURL returns the SKILL.md URL with default.
func (c *Config) GetSkillURL() string {
	if c.SkillURL == "" {
		return "https://raw.githubusercontent.com/ethpandaops/lab/main/SKILL.md"
	}
	return c.SkillURL
}
