package skill

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"golang.org/x/mod/semver"
)

var (
	fullSHAPattern     = regexp.MustCompile(`^[0-9a-f]{40}$`)
	shortSHAPattern    = regexp.MustCompile(`^[0-9a-f]{7,39}$`)
	semverQueryPattern = regexp.MustCompile(`^v\d+(\.\d+)?$`)
)

// ResolveVersion resolves a version query against the remote's advertised
// refs, following the same rules as `go get`:
//
//   - "latest" (or an empty query) selects the highest stable semver tag,
//     falling back to the highest prerelease when no stable release exists,
//     and to the default branch's HEAD commit when the repo has no semver tags.
//   - "v1" or "v1.2" selects the highest semver tag in that major or
//     major.minor line.
//   - An existing tag resolves to itself.
//   - A branch resolves to its head commit SHA.
//   - A full commit SHA resolves to itself; a shorter SHA prefix resolves
//     when it uniquely matches an advertised ref.
//
// The result is always a concrete tag or full commit SHA, never a floating
// value.
func ResolveVersion(repo, query string) (string, error) {
	if fullSHAPattern.MatchString(query) {
		return query, nil
	}
	if query == "" || query == "latest" {
		return resolveLatest(repo)
	}

	tags, err := remoteTags(repo)
	if err != nil {
		return "", err
	}
	for _, tag := range tags {
		if tag == query {
			return tag, nil
		}
	}
	if semverQueryPattern.MatchString(query) {
		if v := maxSemverTag(tags, query); v != "" {
			return v, nil
		}
		return "", fmt.Errorf("no version of %s matches query %q", repo, query)
	}
	if sha, ok, err := remoteRefCommit(repo, "refs/heads/"+query); err != nil {
		return "", err
	} else if ok {
		return sha, nil
	}
	if shortSHAPattern.MatchString(query) {
		return expandShortSHA(repo, query)
	}
	return "", fmt.Errorf("unknown version %q for %s: no matching tag, branch, or commit", query, repo)
}

func resolveLatest(repo string) (string, error) {
	tags, err := remoteTags(repo)
	if err != nil {
		return "", err
	}
	if v := maxSemverTag(tags, ""); v != "" {
		return v, nil
	}
	sha, ok, err := remoteRefCommit(repo, "HEAD")
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("cannot resolve latest version of %s: no semver tags and no HEAD", repo)
	}
	return sha, nil
}

// maxSemverTag returns the highest semver tag, preferring stable releases
// over prereleases. A query of "v1" or "v1.2" narrows candidates to that
// major or major.minor line; an empty query considers every tag. Tags that
// are not valid semver (including those missing the "v" prefix) are ignored,
// as in Go modules.
func maxSemverTag(tags []string, query string) string {
	var best, bestPre string
	for _, tag := range tags {
		if !semver.IsValid(tag) {
			continue
		}
		switch strings.Count(query, ".") {
		case 0:
			if query != "" && semver.Major(tag) != query {
				continue
			}
		case 1:
			if semver.MajorMinor(tag) != query {
				continue
			}
		}
		if semver.Prerelease(tag) != "" {
			if bestPre == "" || semver.Compare(tag, bestPre) > 0 {
				bestPre = tag
			}
			continue
		}
		if best == "" || semver.Compare(tag, best) > 0 {
			best = tag
		}
	}
	if best != "" {
		return best
	}
	return bestPre
}

// lsRemote runs git ls-remote with the given arguments followed by the
// repo's URL and any ref patterns, returning non-empty output lines.
func lsRemote(repo string, flags []string, refs ...string) ([]string, error) {
	args := append([]string{"ls-remote"}, flags...)
	args = append(args, repoToCloneURL(repo))
	args = append(args, refs...)
	cmd := exec.Command("git", args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list remote refs of %s: %w", repo, err)
	}
	var lines []string
	for line := range strings.SplitSeq(string(out), "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines, nil
}

// remoteTags lists the remote's tag names, with annotated-tag peel entries
// ("^{}") folded into their tag.
func remoteTags(repo string) ([]string, error) {
	lines, err := lsRemote(repo, []string{"--tags"})
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	var tags []string
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(fields[1], "refs/tags/"), "^{}")
		if !seen[name] {
			seen[name] = true
			tags = append(tags, name)
		}
	}
	return tags, nil
}

// remoteRefCommit returns the commit SHA the given ref points to, and
// whether the remote advertises that ref.
func remoteRefCommit(repo, ref string) (string, bool, error) {
	lines, err := lsRemote(repo, nil, ref)
	if err != nil {
		return "", false, err
	}
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == ref {
			return fields[0], true, nil
		}
	}
	return "", false, nil
}

// expandShortSHA expands a commit SHA prefix to the full SHA when it
// uniquely matches one of the remote's advertised refs.
func expandShortSHA(repo, prefix string) (string, error) {
	lines, err := lsRemote(repo, nil)
	if err != nil {
		return "", err
	}
	matches := make(map[string]bool)
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.HasPrefix(fields[0], prefix) {
			matches[fields[0]] = true
		}
	}
	switch len(matches) {
	case 1:
		for sha := range matches {
			return sha, nil
		}
	case 0:
		return "", fmt.Errorf("commit %q does not match any advertised ref of %s; use the full 40-character SHA", prefix, repo)
	}
	return "", fmt.Errorf("commit %q is ambiguous in %s", prefix, repo)
}
