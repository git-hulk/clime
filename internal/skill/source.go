package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Source identifies where skills come from: a remote git repository
// (owner/repo shorthand, full URL, or SSH) or an existing local directory,
// together with an optional version query. Parse raw input once with
// ParseSource at the boundary; every other API takes a Source value.
type Source struct {
	// Repo is the repository identity: owner/repo, a clone URL, or a
	// local path. It never carries a version query.
	Repo string
	// Query is the version query: "", "latest", a semver line such as
	// "v1" or "v1.2", a tag, a branch, or a commit SHA prefix.
	Query string
}

// ParseSource validates and splits a source such as "owner/repo@v1.2.3",
// a clone URL, or a local path. The version separator is the last "@"
// after the last "/", so the user in an SSH URL like
// "git@github.com:owner/repo.git" is never treated as a version. A source
// naming an existing local directory is kept whole, even when it contains
// an "@".
func ParseSource(raw string) (Source, error) {
	repo, query := splitSource(raw)
	if repo == "" {
		return Source{}, fmt.Errorf("invalid source: expected owner/repo or local path, got %q", raw)
	}
	if isRemoteURL(repo) {
		return Source{Repo: repo, Query: query}, nil
	}

	info, err := os.Stat(repo)
	switch {
	case err == nil:
		if !info.IsDir() {
			return Source{}, fmt.Errorf("local source path %q is not a directory", repo)
		}
		return Source{Repo: repo, Query: query}, nil
	case !os.IsNotExist(err):
		return Source{}, fmt.Errorf("failed to inspect local source path %q: %w", repo, err)
	}

	if looksLikeLocalPath(repo) {
		return Source{}, fmt.Errorf("local source path %q does not exist", repo)
	}
	if !strings.Contains(repo, "/") {
		return Source{}, fmt.Errorf("invalid source: expected owner/repo or local path, got %q", raw)
	}
	return Source{Repo: repo, Query: query}, nil
}

// splitSource separates a raw source into repository and version query.
// An existing local directory is returned whole so a directory name
// containing "@" is never split.
func splitSource(raw string) (repo, query string) {
	if info, err := os.Stat(raw); err == nil && info.IsDir() {
		return raw, ""
	}
	return splitQuery(raw)
}

// splitQuery splits "repo@query" at the last "@" appearing after the last
// "/". A trailing "@" is kept as part of the repository.
func splitQuery(text string) (repo, query string) {
	separator := strings.LastIndex(text, "@")
	if separator <= strings.LastIndex(text, "/") {
		return text, ""
	}
	if query := text[separator+1:]; query != "" {
		return text[:separator], query
	}
	return text, ""
}

func isRemoteURL(repo string) bool {
	return strings.Contains(repo, "://") || strings.HasPrefix(repo, "git@")
}

func looksLikeLocalPath(repo string) bool {
	return filepath.IsAbs(repo) || repo == "." || repo == ".." ||
		strings.HasPrefix(repo, "./") || strings.HasPrefix(repo, "../")
}

// IsLocal reports whether the source names an existing local directory.
func (source Source) IsLocal() bool {
	if isRemoteURL(source.Repo) {
		return false
	}
	info, err := os.Stat(source.Repo)
	return err == nil && info.IsDir()
}

// Dir returns the absolute path of a local source.
func (source Source) Dir() (string, error) {
	dir, err := filepath.Abs(source.Repo)
	if err != nil {
		return "", fmt.Errorf("failed to resolve local source path %q: %w", source.Repo, err)
	}
	return dir, nil
}

// CloneURL converts an "owner/repo" shorthand to a git clone URL. Full
// URLs (any scheme, or SSH git@) and local paths are returned as-is.
func (source Source) CloneURL() string {
	if isRemoteURL(source.Repo) || looksLikeLocalPath(source.Repo) {
		return source.Repo
	}
	return fmt.Sprintf("https://github.com/%s.git", source.Repo)
}

// Equal reports whether two sources refer to the same repository.
// Version queries are per-install state, not part of a source's identity.
func (source Source) Equal(other Source) bool {
	return sameRepo(source.Repo, other.Repo)
}

// sameRepo is the one definition of source identity: repository names are
// case-insensitive on GitHub and other major hosts.
func sameRepo(left, right string) bool {
	return strings.EqualFold(left, right)
}

// WithQuery returns the source pinned to the given version query.
func (source Source) WithQuery(query string) Source {
	return Source{Repo: source.Repo, Query: query}
}

// String renders the source as "repo" or "repo@query".
func (source Source) String() string {
	if source.Query == "" {
		return source.Repo
	}
	return source.Repo + "@" + source.Query
}

// DisplayVersion shortens a full commit SHA for display; tags are shown
// as-is and an unknown version renders as a dash.
func DisplayVersion(version string) string {
	if version == "" {
		return "—"
	}
	if len(version) == 40 && isHex(version) {
		return version[:12]
	}
	return version
}

func isHex(text string) bool {
	for _, character := range text {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') && (character < 'A' || character > 'F') {
			return false
		}
	}
	return true
}
