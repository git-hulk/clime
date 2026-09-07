package skill

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/git-hulk/clime/internal/githubcli"
)

// Store owns the version cache under ~/.clime/sources/: one immutable
// directory per (source, resolved version), never mutated after fetching.
type Store struct {
	Root string
	// Progress receives download updates as they arrive. Nil keeps fetches silent.
	Progress   func(string)
	ghAuthOnce sync.Once
	ghAuthed   bool
}

// Snapshot is a Source materialized on disk at one concrete version.
type Snapshot struct {
	Source Source
	Dir    string
	// Version is the concrete tag or full commit SHA the directory holds.
	// Local sources have no version identity, so their version is empty.
	Version string
}

// OpenStore returns the store rooted at ~/.clime/sources.
func OpenStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	return &Store{Root: filepath.Join(home, ".clime", "sources")}, nil
}

// Snapshot materializes src at the version its query resolves to (latest
// when it carries none). A concrete version already in cache is returned
// without network access; floating queries (latest, a semver line, a
// branch) always resolve remotely because only resolved versions are
// cached. Local sources are used in place without cloning.
func (st *Store) Snapshot(src Source) (*Snapshot, error) {
	if src.IsLocal() {
		if src.Query != "" {
			return nil, fmt.Errorf("version %q is not supported for local path %q", src.Query, src.Repo)
		}
		dir, err := src.Dir()
		if err != nil {
			return nil, err
		}
		return &Snapshot{Source: src, Dir: dir}, nil
	}

	query := src.Query
	if query == "" {
		query = "latest"
	}
	base := st.repoDir(src)
	if dir := versionDir(base, query); dirExists(dir) {
		return &Snapshot{Source: src, Dir: dir, Version: query}, nil
	}
	repo := src.githubRepo()
	if repo != "" {
		st.ghAuthOnce.Do(func() { st.ghAuthed = githubcli.Authenticated() })
		if !st.ghAuthed {
			repo = ""
		}
	}
	var resolved string
	var err error
	if repo != "" {
		resolved, err = resolveVersion(&githubSource{Source: Source{Repo: repo}}, query)
	} else {
		resolved, err = resolveVersion(src, query)
	}
	if err != nil {
		return nil, err
	}
	dir := versionDir(base, resolved)
	if !dirExists(dir) {
		if repo != "" {
			err = downloadGitHubArchive(repo, resolved, dir, st.Progress)
		} else {
			err = cloneAtVersion(src, resolved, dir, st.Progress)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to fetch %s at %s: %w", src.Repo, resolved, err)
		}
	}
	return &Snapshot{Source: src, Dir: dir, Version: resolved}, nil
}

// Remove deletes every cached version of the source.
func (st *Store) Remove(src Source) error {
	return os.RemoveAll(st.repoDir(src))
}

// prune removes a source's cached snapshots except the installed version.
func (st *Store) prune(src Source, version string) error {
	base := st.repoDir(src)
	parent := filepath.Dir(base)
	entries, err := os.ReadDir(parent)
	if err != nil {
		return err
	}
	prefix := filepath.Base(base) + "@"
	keep := versionDir(base, version)
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		dir := filepath.Join(parent, entry.Name())
		if dir != keep {
			if err := os.RemoveAll(dir); err != nil {
				return err
			}
		}
	}
	return nil
}

// repoDir returns the base cache path for a source repository; version
// directories live beside it, keyed by versionDir.
func (st *Store) repoDir(src Source) string {
	name := src.Repo
	name = strings.TrimPrefix(name, "https://")
	name = strings.TrimPrefix(name, "http://")
	name = strings.TrimPrefix(name, "git@")
	name = strings.TrimSuffix(name, ".git")
	name = strings.ReplaceAll(name, ":", "/")
	return filepath.Join(st.Root, name)
}

// versionDir returns the immutable cache directory for one version of a
// source repository.
func versionDir(base, version string) string {
	return base + "@" + strings.ReplaceAll(version, "/", "-")
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// Catalog reads the skills this snapshot offers.
func (s *Snapshot) Catalog() (*Catalog, error) {
	catalog, err := ReadCatalog(s.Dir)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", s.Source.Repo, err)
	}
	return catalog, nil
}

// SkillFiles reads all files under one skill's path, keyed by path
// relative to it. A path naming a single file yields that file alone.
func (s *Snapshot) SkillFiles(path string) (map[string][]byte, error) {
	root := filepath.Join(s.Dir, path)
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("skill path %q not found: %w", path, err)
	}

	files := make(map[string][]byte)
	if !info.IsDir() {
		data, err := os.ReadFile(root)
		if err != nil {
			return nil, err
		}
		files[filepath.Base(root)] = data
		return files, nil
	}

	err = filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			if fi.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		files[rel] = data
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to read skill files: %w", err)
	}

	return files, nil
}

// cloneAtVersion clones a source checked out at the given version (a tag,
// branch, or commit SHA) into dir. Tags and branches are cloned directly;
// commit SHAs are fetched into an empty repository to avoid downloading
// the default branch first.
func cloneAtVersion(src Source, version, dir string, progress func(string)) error {
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	if !fullSHAPattern.MatchString(version) {
		args := []string{"clone", "--depth", "1", "--branch", version}
		if progress != nil {
			args = append(args, "--progress")
		}
		cmd := exec.Command("git", append(args, src.CloneURL(), dir)...)
		if out, err := runGitProgress(cmd, progress); err != nil {
			os.RemoveAll(dir)
			return fmt.Errorf("git clone failed: %w\n%s", err, out)
		}
		return nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create checkout directory: %w", err)
	}
	for _, args := range [][]string{
		{"init"},
		{"remote", "add", "origin", src.CloneURL()},
		{"fetch", "--depth", "1", "origin", version},
		{"checkout", "--detach", "FETCH_HEAD"},
	} {
		var report func(string)
		if args[0] == "fetch" && progress != nil {
			args = append([]string{"fetch", "--progress"}, args[1:]...)
			report = progress
		}
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := runGitProgress(cmd, report); err != nil {
			os.RemoveAll(dir)
			return fmt.Errorf("failed to check out version %q: %w\n%s", version, err, out)
		}
	}
	return nil
}

// gitProgress captures diagnostics while forwarding complete Git progress
// lines. Git uses carriage returns for progress and newlines for messages.
type gitProgress struct {
	output bytes.Buffer
	line   strings.Builder
	report func(string)
}

func (p *gitProgress) Write(data []byte) (int, error) {
	p.output.Write(data)
	if p.report != nil {
		for _, b := range data {
			if b == '\r' || b == '\n' {
				p.flush()
			} else {
				p.line.WriteByte(b)
			}
		}
	}
	return len(data), nil
}

func (p *gitProgress) flush() {
	if line := strings.TrimSpace(p.line.String()); line != "" {
		p.report(line)
	}
	p.line.Reset()
}

func runGitProgress(cmd *exec.Cmd, report func(string)) (string, error) {
	output := &gitProgress{report: report}
	cmd.Stdout = output
	cmd.Stderr = output
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	err := cmd.Run()
	output.flush()
	return output.output.String(), err
}
