package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSuccessPageConfig_Resolve(t *testing.T) {
	t.Parallel()

	cfg := &SuccessPageConfig{
		Rules: []SuccessPageRule{
			{
				Match: SuccessPageMatch{
					Orgs:  []string{"ethpandaops"},
					Users: []string{"samcm", "mattevans"},
				},
				SuccessPageDisplay: SuccessPageDisplay{
					Tagline: "special user tagline",
					Media: &SuccessPageMedia{
						Type:           "ascii",
						ASCIIArtBase64: "dGVzdA==",
					},
				},
			},
			{
				Match: SuccessPageMatch{
					Orgs: []string{"ethpandaops"},
				},
				SuccessPageDisplay: SuccessPageDisplay{
					Tagline: "org tagline",
					Media: &SuccessPageMedia{
						Type: "gif",
						URL:  "https://example.com/cool.gif",
					},
				},
			},
		},
		Default: &SuccessPageDisplay{
			Tagline: "default tagline",
		},
	}

	tests := []struct {
		name           string
		login          string
		orgs           []string
		wantTagline    string
		wantMediaType  string
		wantMediaURL   string
		wantMediaASCII string
	}{
		{
			name:           "special user matches first rule",
			login:          "samcm",
			orgs:           []string{"ethpandaops"},
			wantTagline:    "special user tagline",
			wantMediaType:  "ascii",
			wantMediaASCII: "dGVzdA==",
		},
		{
			name:          "org member matches second rule",
			login:         "someone",
			orgs:          []string{"ethpandaops"},
			wantTagline:   "org tagline",
			wantMediaType: "gif",
			wantMediaURL:  "https://example.com/cool.gif",
		},
		{
			name:        "no match returns default",
			login:       "outsider",
			orgs:        []string{"other-org"},
			wantTagline: "default tagline",
		},
		{
			name:        "special user without matching org falls through",
			login:       "samcm",
			orgs:        []string{"other-org"},
			wantTagline: "default tagline",
		},
		{
			name:        "empty user returns default",
			login:       "",
			orgs:        nil,
			wantTagline: "default tagline",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			display := cfg.Resolve(tt.login, tt.orgs)

			assert.Equal(t, tt.wantTagline, display.Tagline)

			if tt.wantMediaType != "" {
				assert.NotNil(t, display.Media)
				assert.Equal(t, tt.wantMediaType, display.Media.Type)
				assert.Equal(t, tt.wantMediaURL, display.Media.URL)
				assert.Equal(t, tt.wantMediaASCII, display.Media.ASCIIArtBase64)
			} else {
				assert.Nil(t, display.Media)
			}
		})
	}
}

func TestSuccessPageConfig_Resolve_CaseInsensitive(t *testing.T) {
	t.Parallel()

	cfg := &SuccessPageConfig{
		Rules: []SuccessPageRule{
			{
				Match: SuccessPageMatch{
					Orgs:  []string{"EthPandaOps"},
					Users: []string{"SamCM"},
				},
				SuccessPageDisplay: SuccessPageDisplay{
					Tagline: "matched",
				},
			},
		},
	}

	display := cfg.Resolve("samcm", []string{"ethpandaops"})
	assert.Equal(t, "matched", display.Tagline)

	display = cfg.Resolve("SAMCM", []string{"ETHPANDAOPS"})
	assert.Equal(t, "matched", display.Tagline)
}

func TestSuccessPageConfig_Resolve_NilConfig(t *testing.T) {
	t.Parallel()

	var cfg *SuccessPageConfig
	display := cfg.Resolve("samcm", []string{"ethpandaops"})
	assert.Empty(t, display.Tagline)
	assert.Nil(t, display.Media)
}

func TestSuccessPageConfig_Resolve_EmptyRulesNoDefault(t *testing.T) {
	t.Parallel()

	cfg := &SuccessPageConfig{}
	display := cfg.Resolve("samcm", []string{"ethpandaops"})
	assert.Empty(t, display.Tagline)
	assert.Nil(t, display.Media)
}

func TestSuccessPageConfig_Resolve_UsersOnlyMatch(t *testing.T) {
	t.Parallel()

	cfg := &SuccessPageConfig{
		Rules: []SuccessPageRule{
			{
				Match: SuccessPageMatch{
					Users: []string{"admin"},
				},
				SuccessPageDisplay: SuccessPageDisplay{
					Tagline: "admin tagline",
				},
			},
		},
	}

	display := cfg.Resolve("admin", nil)
	assert.Equal(t, "admin tagline", display.Tagline)

	display = cfg.Resolve("other", nil)
	assert.Empty(t, display.Tagline)
}

func TestSuccessPageConfig_Resolve_OrgsOnlyMatch(t *testing.T) {
	t.Parallel()

	cfg := &SuccessPageConfig{
		Rules: []SuccessPageRule{
			{
				Match: SuccessPageMatch{
					Orgs: []string{"myorg", "otherorg"},
				},
				SuccessPageDisplay: SuccessPageDisplay{
					Tagline: "org member",
				},
			},
		},
	}

	display := cfg.Resolve("anyone", []string{"myorg"})
	assert.Equal(t, "org member", display.Tagline)

	display = cfg.Resolve("anyone", []string{"otherorg"})
	assert.Equal(t, "org member", display.Tagline)

	display = cfg.Resolve("anyone", []string{"nope"})
	assert.Empty(t, display.Tagline)
}
