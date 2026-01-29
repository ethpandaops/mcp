package runbooks

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	runbooks, err := Load()
	require.NoError(t, err)
	require.NotEmpty(t, runbooks, "expected at least one runbook to be loaded")

	// Verify all runbooks have required fields.
	for _, rb := range runbooks {
		t.Run(rb.FilePath, func(t *testing.T) {
			require.NotEmpty(t, rb.Name, "runbook must have a name")
			require.NotEmpty(t, rb.Description, "runbook must have a description")
			require.NotEmpty(t, rb.Content, "runbook must have content")
			require.NotEmpty(t, rb.FilePath, "runbook must have file path")
		})
	}
}

func TestSplitFrontmatter(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantFM      string
		wantBody    string
		expectError bool
	}{
		{
			name:     "valid frontmatter",
			input:    "---\nname: Test\n---\nBody content",
			wantFM:   "name: Test",
			wantBody: "Body content",
		},
		{
			name:        "missing opening delimiter",
			input:       "name: Test\n---\nBody content",
			expectError: true,
		},
		{
			name:        "missing closing delimiter",
			input:       "---\nname: Test\nBody content",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm, body, err := splitFrontmatter([]byte(tt.input))
			if tt.expectError {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantFM, string(fm))
			require.Equal(t, tt.wantBody, string(body))
		})
	}
}
