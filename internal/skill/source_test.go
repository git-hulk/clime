package skill

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw       string
		wantRepo  string
		wantQuery string
	}{
		{"owner/repo", "owner/repo", ""},
		{"owner/repo@v1.2.3", "owner/repo", "v1.2.3"},
		{"owner/repo@8f9f4e0b67b9f6c627e93ab4e56ee48d623aa095", "owner/repo", "8f9f4e0b67b9f6c627e93ab4e56ee48d623aa095"},
		{"owner/repo@", "owner/repo@", ""},
		{"https://github.com/owner/repo.git@v1.2.3", "https://github.com/owner/repo.git", "v1.2.3"},
		{"git@github.com:owner/repo.git", "git@github.com:owner/repo.git", ""},
		{"git@github.com:owner/repo.git@v1.4.2", "git@github.com:owner/repo.git", "v1.4.2"},
	}

	for _, tt := range tests {
		src, err := ParseSource(tt.raw)
		if !assert.NoError(t, err, "ParseSource(%q)", tt.raw) {
			continue
		}
		assert.Equal(t, tt.wantRepo, src.Repo, "ParseSource(%q).Repo", tt.raw)
		assert.Equal(t, tt.wantQuery, src.Query, "ParseSource(%q).Query", tt.raw)
		assert.Equal(t, tt.raw, src.String(), "source must round-trip")
	}
}

func TestParseSourceRejectsInvalid(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{"", "noslash", "./does-not-exist", "../does-not-exist"} {
		_, err := ParseSource(raw)
		assert.Error(t, err)
	}
}

func TestParseSourceLocalDirectories(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "skills@v2")
	require.NoError(t, os.Mkdir(dir, 0o755))
	for _, path := range []string{".", t.TempDir(), dir} {
		t.Run(path, func(t *testing.T) {
			src, err := ParseSource(path)
			require.NoError(t, err)
			require.Equal(t, Source{Repo: path}, src)
			require.True(t, src.IsLocal())
			abs, err := src.Dir()
			require.NoError(t, err)
			want, err := filepath.Abs(path)
			require.NoError(t, err)
			require.Equal(t, want, abs)
		})
	}
}

func TestCloneURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		repo string
		want string
	}{
		{"owner/repo", "https://github.com/owner/repo.git"},
		{"https://github.com/owner/repo.git", "https://github.com/owner/repo.git"},
		{"https://gitlab.com/group/repo.git", "https://gitlab.com/group/repo.git"},
		{"git@github.com:owner/repo.git", "git@github.com:owner/repo.git"},
		{"file:///tmp/local-repo", "file:///tmp/local-repo"},
		{"/tmp/local-repo", "/tmp/local-repo"},
		{"./relative-repo", "./relative-repo"},
		{"../parent-repo", "../parent-repo"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, (Source{Repo: tt.repo}).CloneURL())
	}
}

func TestSourceEqual(t *testing.T) {
	t.Parallel()

	require.True(t, (Source{Repo: "owner/repo", Query: "v1.0.0"}).Equal(Source{Repo: "Owner/Repo", Query: "latest"}), "Equal should ignore case and version queries")
	require.False(t, (Source{Repo: "owner/repo"}).Equal(Source{Repo: "owner/other"}), "Equal should not match different repos")
}

func TestDisplayVersion(t *testing.T) {
	t.Parallel()

	sha := "8f9f4e0b67b9f6c627e93ab4e56ee48d623aa095"
	tests := []struct{ in, want string }{
		{"", "—"},
		{"v1.2.3", "v1.2.3"},
		{sha, sha[:12]},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, DisplayVersion(tt.in))
	}
}
