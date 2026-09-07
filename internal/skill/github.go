package skill

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// githubRepo identifies github.com sources in shorthand, HTTPS, or SSH form.
func (s Source) githubRepo() string {
	raw := s.CloneURL()
	if strings.HasPrefix(raw, "git@github.com:") {
		raw = "ssh://git@github.com/" + strings.TrimPrefix(raw, "git@github.com:")
	}
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Hostname(), "github.com") {
		return ""
	}
	repo := strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git")
	parts := strings.Split(repo, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	return repo
}

type githubRef struct {
	name, sha string
}

// githubRefs reads every page, since the highest semver need not be on page one.
func githubRefs(repo, kind string) ([]githubRef, error) {
	cmd := exec.Command("gh", "api", "--hostname", "github.com",
		"repos/"+repo+"/"+kind+"?per_page=100", "--paginate",
		"--jq", ".[] | [.name, .commit.sha] | @tsv")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh api %s for %s: %w", kind, repo, err)
	}
	var refs []githubRef
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		name, sha, ok := strings.Cut(line, "\t")
		if !ok || !fullSHAPattern.MatchString(sha) {
			return nil, fmt.Errorf("invalid GitHub %s reference %q", kind, line)
		}
		refs = append(refs, githubRef{name: name, sha: sha})
	}
	return refs, nil
}

// githubSource uses the same version selection as Git with refs read through gh.
type githubSource struct {
	Source
	tags, branches []githubRef
}

func (s *githubSource) remoteTags() ([]string, error) {
	var err error
	s.tags, err = githubRefs(s.Repo, "tags")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, tag := range s.tags {
		names = append(names, tag.name)
	}
	return names, nil
}

func (s *githubSource) remoteRefCommit(ref string) (string, bool, error) {
	if ref == "HEAD" {
		out, err := exec.Command("gh", "api", "--hostname", "github.com",
			"repos/"+s.Repo+"/commits/HEAD", "--jq", ".sha").Output()
		if err != nil {
			return "", false, fmt.Errorf("gh api HEAD for %s: %w", s.Repo, err)
		}
		sha := strings.TrimSpace(string(out))
		if !fullSHAPattern.MatchString(sha) {
			return "", false, fmt.Errorf("invalid GitHub HEAD commit %q", sha)
		}
		return sha, true, nil
	}
	var err error
	s.branches, err = githubRefs(s.Repo, "branches")
	if err != nil {
		return "", false, err
	}
	for _, branch := range s.branches {
		if "refs/heads/"+branch.name == ref {
			return branch.sha, true, nil
		}
	}
	return "", false, nil
}

func (s *githubSource) expandShortSHA(prefix string) (string, error) {
	// Resolution has already loaded tags and branches before trying a prefix.
	matches := make(map[string]bool)
	for _, refs := range [][]githubRef{s.tags, s.branches} {
		for _, ref := range refs {
			if strings.HasPrefix(ref.sha, prefix) {
				matches[ref.sha] = true
			}
		}
	}
	if len(matches) == 1 {
		for sha := range matches {
			return sha, nil
		}
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("commit %q is ambiguous in %s", prefix, s.Repo)
	}
	return "", fmt.Errorf("commit %q does not match any advertised ref of %s; use the full 40-character SHA", prefix, s.Repo)
}

// downloadGitHubArchive publishes the snapshot only after download and
// extraction succeed. Archives need no Git checkout or Git credentials.
func downloadGitHubArchive(repo, version, dir string, report func(string)) error {
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(dir), ".clime-download-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	archive, err := os.Create(filepath.Join(tmp, "archive.tar.gz"))
	if err != nil {
		return err
	}
	defer archive.Close()
	if report != nil {
		report(fmt.Sprintf("Downloading %s@%s...", repo, DisplayVersion(version)))
	}
	output := &archiveProgress{Writer: archive, report: report}
	var stderr bytes.Buffer
	cmd := exec.Command("gh", "api", "--hostname", "github.com",
		"repos/"+repo+"/tarball/"+url.PathEscape(version))
	cmd.Stdout, cmd.Stderr = output, &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gh api archive: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if _, err := archive.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if report != nil {
		report(fmt.Sprintf("Extracting %s (%d KiB downloaded)...", repo, (output.total+1023)/1024))
	}
	snapshot := filepath.Join(tmp, "snapshot")
	if err := extractGitHubArchive(archive, snapshot); err != nil {
		return fmt.Errorf("extract GitHub archive: %w", err)
	}
	return os.Rename(snapshot, dir)
}

type archiveProgress struct {
	io.Writer
	total, reported int64
	report          func(string)
}

func (p *archiveProgress) Write(data []byte) (int, error) {
	n, err := p.Writer.Write(data)
	p.total += int64(n)
	if p.report != nil && (p.reported == 0 || p.total-p.reported >= 64*1024) {
		p.report(fmt.Sprintf("Downloading archive: %d KiB", (p.total+1023)/1024))
		p.reported = p.total
	}
	return n, err
}

func extractGitHubArchive(archive io.Reader, dir string) error {
	gz, err := gzip.NewReader(archive)
	if err != nil {
		return err
	}
	defer gz.Close()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			_, err = io.Copy(io.Discard, gz)
			return err
		}
		if err != nil {
			return err
		}
		if h.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		_, name, _ := strings.Cut(h.Name, "/")
		if name == "" && h.Typeflag == tar.TypeDir {
			continue
		}
		if !filepath.IsLocal(h.Name) || !filepath.IsLocal(name) {
			return fmt.Errorf("invalid archive path %q", h.Name)
		}
		if err := root.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			err = root.MkdirAll(name, 0o755)
		case tar.TypeReg:
			var file *os.File
			file, err = root.OpenFile(name, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(h.Mode)&0o777)
			if err == nil {
				_, err = io.Copy(file, tr)
				closeErr := file.Close()
				if err == nil {
					err = closeErr
				}
			}
		case tar.TypeSymlink:
			if filepath.IsAbs(h.Linkname) || !filepath.IsLocal(filepath.Join(filepath.Dir(name), h.Linkname)) {
				return fmt.Errorf("archive symlink %q escapes the snapshot", name)
			}
			err = root.Symlink(h.Linkname, name)
		default:
			return fmt.Errorf("unsupported archive entry %q", h.Name)
		}
		if err != nil {
			return err
		}
	}
}
