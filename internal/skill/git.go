package skill

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/mod/semver"
)

// runGit executes git with prompts disabled and returns stdout. Stderr is
// credential-sanitized before it is attached to an error. It is a variable so
// tests can observe or disable network access.
var runGit = func(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s failed: %w: %s",
			args[0], err, strings.TrimSpace(SanitizeCredentials(stderr.String())))
	}
	return stdout.String(), nil
}

type remoteRefs struct {
	headSHA       string
	defaultBranch string
	branches      map[string]string
	tags          map[string]string
}

// lsRemote queries the remote's refs and default branch over Git transport.
func lsRemote(id RepoID) (*remoteRefs, error) {
	out, err := runGit("", "ls-remote", "--symref", id.CloneURL())
	if err != nil {
		return nil, fmt.Errorf("failed to query %s: %w", id.Canonical(), err)
	}
	refs := &remoteRefs{branches: map[string]string{}, tags: map[string]string{}}
	for line := range strings.SplitSeq(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if fields[0] == "ref:" && fields[len(fields)-1] == "HEAD" {
			refs.defaultBranch = strings.TrimPrefix(fields[1], "refs/heads/")
			continue
		}
		sha, ref := fields[0], fields[1]
		switch {
		case ref == "HEAD":
			refs.headSHA = sha
		case strings.HasPrefix(ref, "refs/heads/"):
			refs.branches[strings.TrimPrefix(ref, "refs/heads/")] = sha
		case strings.HasPrefix(ref, "refs/tags/"):
			name := strings.TrimPrefix(ref, "refs/tags/")
			// A peeled entry (tag^{}) points at the commit; prefer it.
			if peeled, ok := strings.CutSuffix(name, "^{}"); ok {
				refs.tags[peeled] = sha
			} else if _, ok := refs.tags[name]; !ok {
				refs.tags[name] = sha
			}
		}
	}
	return refs, nil
}

// latestStableTag returns the highest stable SemVer tag name, or "" when the
// repository has no stable SemVer tags. Tags with and without a leading "v"
// are both recognized; prereleases and build-metadata tags are skipped.
func latestStableTag(tags map[string]string) string {
	var best, bestCanon string
	for name := range tags {
		canon := name
		if !strings.HasPrefix(canon, "v") {
			canon = "v" + canon
		}
		if !semver.IsValid(canon) || semver.Prerelease(canon) != "" || semver.Build(canon) != "" {
			continue
		}
		if best == "" || semver.Compare(canon, bestCanon) > 0 {
			best, bestCanon = name, canon
		}
	}
	return best
}

// ResolveVersion resolves a user-supplied version spec against the remote to
// a locked version: a tag name kept verbatim, or a full commit SHA. An empty
// spec or "latest" selects the highest stable SemVer tag, falling back to the
// default branch's HEAD commit when the repository has no stable SemVer tags.
func ResolveVersion(id RepoID, spec string) (string, error) {
	if IsFullCommit(spec) {
		return spec, nil
	}
	refs, err := lsRemote(id)
	if err != nil {
		return "", err
	}
	if spec == "" || spec == "latest" {
		if tag := latestStableTag(refs.tags); tag != "" {
			return tag, nil
		}
		if refs.headSHA == "" {
			return "", fmt.Errorf("repository %s has no stable SemVer tag and no HEAD commit", id.Canonical())
		}
		return refs.headSHA, nil
	}
	if _, ok := refs.tags[spec]; ok {
		return spec, nil
	}
	if sha, ok := refs.branches[spec]; ok {
		return sha, nil
	}
	if looksLikeShortCommit(spec) {
		return expandShortCommit(id, spec)
	}
	return "", fmt.Errorf("version %q not found in %s: no matching tag, branch, or commit", spec, id.Canonical())
}

// expandShortCommit resolves a short commit hash to a full SHA by fetching
// history metadata into a throwaway repository.
func expandShortCommit(id RepoID, short string) (string, error) {
	tmp, err := os.MkdirTemp("", "clime-resolve-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)

	if _, err := runGit(tmp, "init", "-q", "--bare"); err != nil {
		return "", err
	}
	if _, err := runGit(tmp, "remote", "add", "origin", id.CloneURL()); err != nil {
		return "", err
	}
	refspecs := []string{"+refs/heads/*:refs/heads/*", "+refs/tags/*:refs/tags/*"}
	// A treeless fetch keeps this cheap; fall back for hosts without
	// partial-clone support.
	if _, err := runGit(tmp, append([]string{"fetch", "-q", "--filter=tree:0", "origin"}, refspecs...)...); err != nil {
		if _, err := runGit(tmp, append([]string{"fetch", "-q", "origin"}, refspecs...)...); err != nil {
			return "", fmt.Errorf("failed to resolve commit %q in %s: %w", short, id.Canonical(), err)
		}
	}
	out, err := runGit(tmp, "rev-parse", "--verify", short+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("commit %q not found in %s: %w", short, id.Canonical(), err)
	}
	return strings.TrimSpace(out), nil
}

func initRemote(dir string, id RepoID) error {
	if _, err := runGit(dir, "init", "-q"); err != nil {
		return err
	}
	_, err := runGit(dir, "remote", "add", "origin", id.CloneURL())
	return err
}

// fetchSnapshot checks out the given locked version (tag or full commit SHA)
// into dir and strips Git metadata, returning the resolved commit SHA.
func fetchSnapshot(id RepoID, version, dir string) (string, error) {
	if err := initRemote(dir, id); err != nil {
		return "", err
	}
	if IsFullCommit(version) {
		if _, err := runGit(dir, "fetch", "-q", "--depth", "1", "origin", version); err != nil {
			// Some hosts refuse fetching by SHA; fall back to a full
			// branch fetch and check out the commit from history.
			if _, ferr := runGit(dir, "fetch", "-q", "origin", "+refs/heads/*:refs/remotes/origin/*"); ferr != nil {
				return "", fmt.Errorf("failed to fetch %s@%s: %w", id.Canonical(), version, err)
			}
			if _, cerr := runGit(dir, "checkout", "-q", "--detach", version); cerr != nil {
				return "", fmt.Errorf("failed to fetch %s@%s: %w", id.Canonical(), version, err)
			}
			return finishSnapshot(dir)
		}
	} else {
		if _, err := runGit(dir, "fetch", "-q", "--depth", "1", "origin", "refs/tags/"+version); err != nil {
			return "", fmt.Errorf("failed to fetch %s@%s: %w", id.Canonical(), version, err)
		}
	}
	if _, err := runGit(dir, "checkout", "-q", "--detach", "FETCH_HEAD"); err != nil {
		return "", fmt.Errorf("failed to check out %s@%s: %w", id.Canonical(), version, err)
	}
	return finishSnapshot(dir)
}

func finishSnapshot(dir string) (string, error) {
	out, err := runGit(dir, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	if err := os.RemoveAll(filepath.Join(dir, ".git")); err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}
