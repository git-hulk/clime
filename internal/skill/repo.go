package skill

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// RepoID identifies a Git repository independent of the transport used to
// reach it. Host is lowercase and Path is the credential-free "owner/repo"
// path without a trailing ".git".
type RepoID struct {
	Host string
	Path string
}

// Canonical returns the normalized "host/owner/repo" identity used for
// lookups, conflict detection, and cache addressing.
func (id RepoID) Canonical() string {
	return id.Host + "/" + id.Path
}

// DisplayKey returns the preferred manifest spelling: the "owner/repo"
// shorthand for github.com and "host/owner/repo" for every other host.
func (id RepoID) DisplayKey() string {
	if id.Host == "github.com" {
		return id.Path
	}
	return id.Canonical()
}

// CloneURL returns the HTTPS transport URL. Local Git configuration
// (insteadOf rules, credential helpers) chooses the effective transport.
func (id RepoID) CloneURL() string {
	return "https://" + id.Host + "/" + id.Path + ".git"
}

// IsZero reports whether the identity is unset.
func (id RepoID) IsZero() bool {
	return id.Host == "" && id.Path == ""
}

var (
	scpLikeRe    = regexp.MustCompile(`^([A-Za-z0-9._-]+)@([A-Za-z0-9._-]+):(.+)$`)
	fullCommitRe = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)
	userinfoRe   = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^/@\s]+@`)
)

// ErrLocalPathUnsupported is returned when a repository argument points at a
// local directory, which has no immutable version identity.
var ErrLocalPathUnsupported = fmt.Errorf(
	"local directories are not supported: publish the skill in a Git repository and install it as owner/repo or host/owner/repo")

func isLocalPathInput(s string) bool {
	if s == "." || s == ".." || s == "~" {
		return true
	}
	for _, prefix := range []string{"/", "./", "../", "~/", "file://"} {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

// ParseRepo normalizes a repository argument or manifest key to a RepoID.
// Accepted forms: "owner/repo", "host/owner/repo", HTTPS/SSH URLs, and
// scp-like "git@host:owner/repo.git". Userinfo in URLs is discarded so the
// identity is always credential-free. Local directory paths are rejected.
func ParseRepo(input string) (RepoID, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return RepoID{}, fmt.Errorf("repository is empty")
	}
	if isLocalPathInput(s) {
		return RepoID{}, ErrLocalPathUnsupported
	}

	var host, path string
	switch {
	case strings.HasPrefix(s, "https://"), strings.HasPrefix(s, "http://"), strings.HasPrefix(s, "ssh://"):
		u, err := url.Parse(s)
		if err != nil {
			return RepoID{}, fmt.Errorf("invalid repository URL %q", SanitizeCredentials(s))
		}
		host = u.Hostname()
		if u.Port() != "" {
			host += ":" + u.Port()
		}
		path = strings.Trim(u.Path, "/")
	default:
		if m := scpLikeRe.FindStringSubmatch(s); m != nil {
			host = m[2]
			path = strings.Trim(m[3], "/")
		} else {
			segments := strings.Split(strings.Trim(s, "/"), "/")
			switch {
			case len(segments) == 2:
				host = "github.com"
				path = segments[0] + "/" + segments[1]
			case len(segments) >= 3:
				host = segments[0]
				path = strings.Join(segments[1:], "/")
				if !strings.Contains(host, ".") && !strings.Contains(host, ":") && host != "localhost" {
					return RepoID{}, fmt.Errorf("invalid repository %q: host %q must be a domain name", s, host)
				}
			default:
				return RepoID{}, fmt.Errorf("invalid repository %q: expected owner/repo or host/owner/repo", s)
			}
		}
	}

	host = strings.ToLower(strings.TrimSpace(host))
	path = strings.TrimSuffix(path, ".git")
	if host == "" {
		return RepoID{}, fmt.Errorf("invalid repository %q: missing host", SanitizeCredentials(s))
	}
	segments := strings.Split(path, "/")
	if len(segments) < 2 {
		return RepoID{}, fmt.Errorf("invalid repository %q: expected owner/repo or host/owner/repo", SanitizeCredentials(s))
	}
	for _, seg := range segments {
		if seg == "" || seg == "." || seg == ".." || strings.ContainsAny(seg, " \t") {
			return RepoID{}, fmt.Errorf("invalid repository %q: bad path segment %q", SanitizeCredentials(s), seg)
		}
	}
	return RepoID{Host: host, Path: path}, nil
}

// SplitRepoVersion splits a "repo@version" argument into its repository and
// version parts. The version suffix is the text after the last "@" only when
// it contains neither "/" nor ":", so the "@" of an SSH user (git@host:...)
// is never treated as a version separator.
func SplitRepoVersion(arg string) (repo, version string) {
	idx := strings.LastIndex(arg, "@")
	if idx <= 0 || idx == len(arg)-1 {
		return arg, ""
	}
	suffix := arg[idx+1:]
	if strings.ContainsAny(suffix, "/:") {
		return arg, ""
	}
	return arg[:idx], suffix
}

// IsFullCommit reports whether v is a full 40- or 64-character hex commit SHA.
func IsFullCommit(v string) bool {
	return fullCommitRe.MatchString(v)
}

// SanitizeCredentials removes URL userinfo (user:password@, token@) from s so
// errors and output never disclose credentials.
func SanitizeCredentials(s string) string {
	return userinfoRe.ReplaceAllString(s, "$1")
}
