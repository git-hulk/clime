package skill

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRepoNormalization(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"acme/agent-skills", "github.com/acme/agent-skills"},
		{"github.com/acme/agent-skills", "github.com/acme/agent-skills"},
		{"github.company.com/platform/private-skills", "github.company.com/platform/private-skills"},
		{"https://github.com/acme/agent-skills", "github.com/acme/agent-skills"},
		{"https://github.com/acme/agent-skills.git", "github.com/acme/agent-skills"},
		{"https://user:secret@github.com/acme/agent-skills.git", "github.com/acme/agent-skills"},
		{"git@github.com:acme/agent-skills.git", "github.com/acme/agent-skills"},
		{"ssh://git@github.com/acme/agent-skills.git", "github.com/acme/agent-skills"},
		{"GITHUB.com/acme/agent-skills", "github.com/acme/agent-skills"},
	}
	for _, tt := range tests {
		id, err := ParseRepo(tt.input)
		require.NoError(t, err, "ParseRepo(%q)", tt.input)
		assert.Equal(t, tt.want, id.Canonical(), "ParseRepo(%q)", tt.input)
	}
}

func TestParseRepoHTTPSAndSSHNormalizeToSameIdentity(t *testing.T) {
	https, err := ParseRepo("https://github.com/acme/agent-skills.git")
	require.NoError(t, err)
	ssh, err := ParseRepo("git@github.com:acme/agent-skills.git")
	require.NoError(t, err)
	assert.Equal(t, https.Canonical(), ssh.Canonical(),
		"HTTPS and SSH inputs should normalize to the same identity")
}

func TestParseRepoRejectsLocalPaths(t *testing.T) {
	for _, input := range []string{".", "..", "/tmp/skills", "./skills", "../skills", "~/skills", "file:///tmp/skills"} {
		_, err := ParseRepo(input)
		assert.ErrorIs(t, err, ErrLocalPathUnsupported, "ParseRepo(%q)", input)
	}
}

func TestParseRepoRejectsInvalidInput(t *testing.T) {
	for _, input := range []string{"", "just-a-name", "noDot/owner/repo", "owner//repo", "a/../b"} {
		_, err := ParseRepo(input)
		assert.Error(t, err, "ParseRepo(%q)", input)
	}
}

func TestParseRepoDisplayKey(t *testing.T) {
	id, err := ParseRepo("https://github.com/acme/agent-skills")
	require.NoError(t, err)
	assert.Equal(t, "acme/agent-skills", id.DisplayKey())

	id, err = ParseRepo("github.company.com/platform/private-skills")
	require.NoError(t, err)
	assert.Equal(t, "github.company.com/platform/private-skills", id.DisplayKey())
}

func TestSplitRepoVersion(t *testing.T) {
	tests := []struct {
		arg         string
		wantRepo    string
		wantVersion string
	}{
		{"acme/agent-skills@v1.4.2", "acme/agent-skills", "v1.4.2"},
		{"acme/agent-skills@latest", "acme/agent-skills", "latest"},
		{"acme/agent-skills", "acme/agent-skills", ""},
		// The SSH user's @ is not a version separator.
		{"git@github.com:acme/agent-skills.git@v1.4.2", "git@github.com:acme/agent-skills.git", "v1.4.2"},
		{"git@github.com:acme/agent-skills.git", "git@github.com:acme/agent-skills.git", ""},
		{"ssh://git@github.com/acme/agent-skills.git", "ssh://git@github.com/acme/agent-skills.git", ""},
		{"acme/agent-skills@", "acme/agent-skills@", ""},
		{"acme/agent-skills@8f9f4e0", "acme/agent-skills", "8f9f4e0"},
	}
	for _, tt := range tests {
		repo, version := SplitRepoVersion(tt.arg)
		assert.Equal(t, tt.wantRepo, repo, "SplitRepoVersion(%q) repo", tt.arg)
		assert.Equal(t, tt.wantVersion, version, "SplitRepoVersion(%q) version", tt.arg)
	}
}

func TestSanitizeCredentials(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://user:secret@github.com/a/b.git", "https://github.com/a/b.git"},
		{"https://x-access-token-abc123@github.com/a/b.git", "https://github.com/a/b.git"},
		{"fatal: could not read from https://alice:hunter2@example.com/x/y", "fatal: could not read from https://example.com/x/y"},
		{"https://github.com/a/b.git", "https://github.com/a/b.git"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, SanitizeCredentials(tt.input), "SanitizeCredentials(%q)", tt.input)
	}
}

func TestSanitizeCredentialsNoSecretSubstring(t *testing.T) {
	out := SanitizeCredentials("remote error at https://token:ghp_supersecret@github.com/acme/agent-skills.git")
	assert.NotContains(t, out, "ghp_supersecret")
	assert.NotContains(t, out, "token")
}

func TestIsFullCommit(t *testing.T) {
	full := strings.Repeat("8f9f4e0b67", 4)
	assert.True(t, IsFullCommit(full), "IsFullCommit(%q)", full)
	for _, v := range []string{"8f9f4e0", "v1.2.3", "", strings.Repeat("z", 40)} {
		assert.False(t, IsFullCommit(v), "IsFullCommit(%q)", v)
	}
}
