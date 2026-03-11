package client

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildSuccessPage_Default(t *testing.T) {
	t.Parallel()

	page := buildSuccessPage(callbackUser{
		Login:     "testuser",
		AvatarURL: "https://example.com/avatar.png",
	})

	assert.Contains(t, page, "testuser")
	assert.Contains(t, page, "Authenticated")
	assert.Contains(t, page, "logged in to panda")
	assert.Contains(t, page, "avatar.png")
	assert.Contains(t, page, "panda datasources")
	assert.Contains(t, page, "You can close this window and return to your terminal.")
	assert.NotContains(t, page, "<pre class=\"ascii-art\"")
	assert.NotContains(t, page, `<div class="media-frame">`)
}

func TestBuildSuccessPage_WithOrg(t *testing.T) {
	t.Parallel()

	page := buildSuccessPage(callbackUser{
		Login:     "pandafan",
		AvatarURL: "https://example.com/avatar.png",
		Orgs:      []string{"ethpandaops"},
	})

	assert.Contains(t, page, "pandafan")
	assert.Contains(t, page, "ethpandaops")
	assert.Contains(t, page, "logged in to ethpandaops/panda")
}

func TestBuildSuccessPage_GIFMedia(t *testing.T) {
	t.Parallel()

	page := buildSuccessPage(callbackUser{
		Login:     "pandafan",
		AvatarURL: "https://example.com/avatar.png",
		Orgs:      []string{"ethpandaops"},
		Tagline:   "Enjoy debugging your devnet champ",
		MediaType: "gif",
		MediaURL:  "https://example.com/cool.gif",
	})

	assert.Contains(t, page, "Enjoy debugging your devnet champ")
	assert.Contains(t, page, "cool.gif")
	assert.Contains(t, page, `<div class="media-frame">`)
	assert.NotContains(t, page, "<pre class=\"ascii-art\"")
}

func TestBuildSuccessPage_ASCIIMedia(t *testing.T) {
	t.Parallel()

	art := "  /\\_/\\\n ( o.o )\n  > ^ <"
	artB64 := base64.StdEncoding.EncodeToString([]byte(art))

	page := buildSuccessPage(callbackUser{
		Login:         "samcm",
		AvatarURL:     "https://example.com/avatar.png",
		Orgs:          []string{"ethpandaops"},
		Tagline:       "Enjoy debugging your devnet champ",
		MediaType:     "ascii",
		MediaASCIIB64: artB64,
	})

	assert.Contains(t, page, "Enjoy debugging your devnet champ")
	assert.Contains(t, page, "<pre class=\"ascii-art\"")
	assert.Contains(t, page, "o.o")
	assert.NotContains(t, page, `<div class="media-frame">`)
}

func TestBuildSuccessPage_InvalidASCIIBase64(t *testing.T) {
	t.Parallel()

	page := buildSuccessPage(callbackUser{
		Login:         "someone",
		MediaType:     "ascii",
		MediaASCIIB64: "not-valid-base64!!!",
	})

	// Should gracefully skip the media block.
	assert.NotContains(t, page, "<pre")
}

func TestBuildSuccessPage_CustomTagline(t *testing.T) {
	t.Parallel()

	page := buildSuccessPage(callbackUser{
		Login:   "someone",
		Tagline: "Welcome aboard!",
	})

	assert.Contains(t, page, "Welcome aboard!")
	assert.NotContains(t, page, "You can close this window and return to your terminal.")
}

func TestBuildSuccessPage_NoAvatar(t *testing.T) {
	t.Parallel()

	page := buildSuccessPage(callbackUser{
		Login: "noavatar",
	})

	assert.Contains(t, page, "avatar-fallback")
	assert.Contains(t, page, ">N<")
}

func TestBuildSuccessPage_EmptyLogin(t *testing.T) {
	t.Parallel()

	page := buildSuccessPage(callbackUser{})

	assert.Contains(t, page, "user")
	assert.Contains(t, page, "Authenticated")
}

func TestBuildSuccessPage_CaseInsensitiveOrgBadge(t *testing.T) {
	t.Parallel()

	page := buildSuccessPage(callbackUser{
		Login: "someone",
		Orgs:  []string{"EthPandaOps"},
	})

	assert.Contains(t, page, "ethpandaops")
	assert.Contains(t, page, "org-badge")
}

func TestHasOrg(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		orgs   []string
		target string
		want   bool
	}{
		{"exact match", []string{"ethpandaops"}, "ethpandaops", true},
		{"case insensitive", []string{"EthPandaOps"}, "ethpandaops", true},
		{"no match", []string{"other-org"}, "ethpandaops", false},
		{"empty orgs", []string{}, "ethpandaops", false},
		{"multiple orgs", []string{"foo", "ethpandaops", "bar"}, "ethpandaops", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, hasOrg(tt.orgs, tt.target))
		})
	}
}

// TestBuildSuccessPage_WritePreview writes HTML files to /tmp/panda-auth-preview/
// for visual inspection. Run with:
//
//	go test -run TestBuildSuccessPage_WritePreview -v ./pkg/auth/client/
//
// Then open the printed file paths in your browser.
func TestBuildSuccessPage_WritePreview(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(os.TempDir(), "panda-auth-preview")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	art := "  /\\_/\\\n ( o.o )\n  > ^ <"
	artB64 := base64.StdEncoding.EncodeToString([]byte(art))

	cases := map[string]callbackUser{
		"default.html": {
			Login:     "randomdev",
			AvatarURL: "https://avatars.githubusercontent.com/u/1?v=4",
		},
		"org_gif.html": {
			Login:     "pandafan",
			AvatarURL: "https://avatars.githubusercontent.com/u/1?v=4",
			Orgs:      []string{"ethpandaops"},
			Tagline:   "Enjoy debugging your devnet champ",
			MediaType: "gif",
			MediaURL:  "https://media1.tenor.com/m/92A2K1kvoHcAAAAd/casino-royale-bond.gif",
		},
		"ascii_art.html": {
			Login:         "samcm",
			AvatarURL:     "https://avatars.githubusercontent.com/u/1?v=4",
			Orgs:          []string{"ethpandaops"},
			Tagline:       "Enjoy debugging your devnet champ",
			MediaType:     "ascii",
			MediaASCIIB64: artB64,
		},
	}

	var paths []string

	for name, user := range cases {
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, []byte(buildSuccessPage(user)), 0o644))

		paths = append(paths, p)
	}

	t.Logf("\n\nPreview files written. Open in browser:\n\n  %s\n", strings.Join(paths, "\n  "))
}
