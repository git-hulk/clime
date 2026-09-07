package skill

import (
	"os"
	"path/filepath"
	"testing"
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
		if err != nil {
			t.Errorf("ParseSource(%q) error = %v", tt.raw, err)
			continue
		}
		if src.Repo != tt.wantRepo || src.Query != tt.wantQuery {
			t.Errorf("ParseSource(%q) = (%q, %q), want (%q, %q)",
				tt.raw, src.Repo, src.Query, tt.wantRepo, tt.wantQuery)
		}
	}
}

func TestParseSourceRejectsInvalid(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{"", "noslash", "./does-not-exist", "../does-not-exist"} {
		if src, err := ParseSource(raw); err == nil {
			t.Errorf("ParseSource(%q) = %+v, want error", raw, src)
		}
	}
}

func TestParseSourceAllowsCurrentDir(t *testing.T) {
	src, err := ParseSource(".")
	if err != nil {
		t.Fatalf("ParseSource(.) error = %v", err)
	}
	if !src.IsLocal() {
		t.Fatal("ParseSource(.) should be local")
	}
}

func TestParseSourceLocalDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src, err := ParseSource(dir)
	if err != nil {
		t.Fatalf("ParseSource(%q) error = %v", dir, err)
	}
	if src.Repo != dir || src.Query != "" {
		t.Fatalf("ParseSource(%q) = (%q, %q), want the path with no query", dir, src.Repo, src.Query)
	}
	if !src.IsLocal() {
		t.Fatal("existing directory should be local")
	}

	abs, err := src.Dir()
	if err != nil {
		t.Fatalf("Dir() error = %v", err)
	}
	want, _ := filepath.Abs(dir)
	if abs != want {
		t.Fatalf("Dir() = %q, want %q", abs, want)
	}
}

func TestParseSourceKeepsLocalDirWithAt(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	dir := filepath.Join(base, "skills@v2")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	src, err := ParseSource(dir)
	if err != nil {
		t.Fatalf("ParseSource(%q) error = %v", dir, err)
	}
	if src.Repo != dir || src.Query != "" {
		t.Fatalf("ParseSource(%q) = (%q, %q), want the existing path unchanged", dir, src.Repo, src.Query)
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
		if got := (Source{Repo: tt.repo}).CloneURL(); got != tt.want {
			t.Errorf("CloneURL(%q) = %q, want %q", tt.repo, got, tt.want)
		}
	}
}

func TestSourceEqual(t *testing.T) {
	t.Parallel()

	if !(Source{Repo: "owner/repo", Query: "v1.0.0"}).Equal(Source{Repo: "Owner/Repo", Query: "latest"}) {
		t.Fatal("Equal should ignore case and version queries")
	}
	if (Source{Repo: "owner/repo"}).Equal(Source{Repo: "owner/other"}) {
		t.Fatal("Equal should not match different repos")
	}
}

func TestSourceString(t *testing.T) {
	t.Parallel()

	if got := (Source{Repo: "owner/repo"}).String(); got != "owner/repo" {
		t.Fatalf("String() = %q", got)
	}
	if got := (Source{Repo: "owner/repo"}).WithQuery("v1.2.3").String(); got != "owner/repo@v1.2.3" {
		t.Fatalf("String() = %q", got)
	}
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
		if got := DisplayVersion(tt.in); got != tt.want {
			t.Errorf("DisplayVersion(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
