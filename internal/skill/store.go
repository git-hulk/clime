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
	// Version is latest, a tag, branch name, or full commit SHA saved in the manifest.
	// Local sources have no version identity, so their version is empty.
	Version string
	// revision identifies the immutable cache directory, resolving branches to SHAs.
	revision string
}

// OpenStore returns the store rooted at ~/.clime/sources.
func OpenStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	return &Store{Root: filepath.Join(home, ".clime", "sources")}, nil
}

// Snapshot materializes source at the version its query resolves to (latest
// when it carries none). A concrete version already in cache is returned
// without network access; floating queries (latest, a semver line, a
// branch) always resolve remotely because only resolved versions are
// cached. Local sources are used in place without cloning.
func (store *Store) Snapshot(source Source) (*Snapshot, error) {
	if source.IsLocal() {
		if source.Query != "" {
			return nil, fmt.Errorf("version %q is not supported for local path %q", source.Query, source.Repo)
		}
		dir, err := source.Dir()
		if err != nil {
			return nil, err
		}
		return &Snapshot{Source: source, Dir: dir}, nil
	}

	query := source.Query
	if query == "" {
		query = "latest"
	}
	base := store.repoDir(source)
	if dir := versionDir(base, query); query != "latest" && dirExists(dir) {
		return &Snapshot{Source: source, Dir: dir, Version: query, revision: query}, nil
	}
	repo := source.githubRepo()
	if repo != "" {
		store.ghAuthOnce.Do(func() { store.ghAuthed = githubcli.Authenticated() })
		if !store.ghAuthed {
			repo = ""
		}
	}
	var resolved resolvedVersion
	var err error
	if repo != "" {
		resolved, err = resolveVersion(&githubSource{Source: Source{Repo: repo}}, query)
	} else {
		resolved, err = resolveVersion(source, query)
	}
	if err != nil {
		return nil, err
	}
	dir := versionDir(base, resolved.revision)
	if !dirExists(dir) {
		if repo != "" {
			err = downloadGitHubArchive(repo, resolved.revision, dir, store.Progress)
		} else {
			err = cloneAtVersion(source, resolved.revision, dir, store.Progress)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to fetch %s at %s: %w", source.Repo, resolved.version, err)
		}
	}
	return &Snapshot{Source: source, Dir: dir, Version: resolved.version, revision: resolved.revision}, nil
}

// Remove deletes every cached version of the source.
func (store *Store) Remove(source Source) error {
	return os.RemoveAll(store.repoDir(source))
}

// installedRevision reads the revision last applied successfully to the targets.
func (store *Store) installedRevision(source Source) (string, error) {
	data, err := os.ReadFile(filepath.Join(store.repoDir(source), "revision"))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to read installed revision of %s: %w", source.Repo, err)
	}
	return string(data), nil
}

// recordInstalledRevision records applied state separately from downloaded snapshots.
func (store *Store) recordInstalledRevision(source Source, revision string) error {
	dir := store.repoDir(source)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "revision"), []byte(revision), 0o644)
}

// prune removes a source's cached snapshots except the installed version.
func (store *Store) prune(source Source, version string) error {
	base := store.repoDir(source)
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
func (store *Store) repoDir(source Source) string {
	name := source.Repo
	name = strings.TrimPrefix(name, "https://")
	name = strings.TrimPrefix(name, "http://")
	name = strings.TrimPrefix(name, "git@")
	name = strings.TrimSuffix(name, ".git")
	name = strings.ReplaceAll(name, ":", "/")
	return filepath.Join(store.Root, name)
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
func (snapshot *Snapshot) Catalog() (*Catalog, error) {
	catalog, err := ReadCatalog(snapshot.Dir)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", snapshot.Source.Repo, err)
	}
	return catalog, nil
}

// SkillFiles reads all files under one skill's path, keyed by path
// relative to it. A path naming a single file yields that file alone.
func (snapshot *Snapshot) SkillFiles(path string) (map[string][]byte, error) {
	root := filepath.Join(snapshot.Dir, path)
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

	err = filepath.Walk(root, func(filePath string, fileInfo os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fileInfo.IsDir() {
			if fileInfo.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, filePath)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(filePath)
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
func cloneAtVersion(source Source, version, dir string, progress func(string)) error {
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	if !fullSHAPattern.MatchString(version) {
		args := []string{"clone", "--depth", "1", "--branch", version}
		if progress != nil {
			args = append(args, "--progress")
		}
		cmd := exec.Command("git", append(args, source.CloneURL(), dir)...)
		if output, err := runGitProgress(cmd, progress); err != nil {
			os.RemoveAll(dir)
			return fmt.Errorf("git clone failed: %w\n%s", err, output)
		}
		return nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create checkout directory: %w", err)
	}
	for _, args := range [][]string{
		{"init"},
		{"remote", "add", "origin", source.CloneURL()},
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
		if output, err := runGitProgress(cmd, report); err != nil {
			os.RemoveAll(dir)
			return fmt.Errorf("failed to check out version %q: %w\n%s", version, err, output)
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

func (progressWriter *gitProgress) Write(data []byte) (int, error) {
	progressWriter.output.Write(data)
	if progressWriter.report != nil {
		for _, character := range data {
			if character == '\r' || character == '\n' {
				progressWriter.flush()
			} else {
				progressWriter.line.WriteByte(character)
			}
		}
	}
	return len(data), nil
}

func (progressWriter *gitProgress) flush() {
	if line := strings.TrimSpace(progressWriter.line.String()); line != "" {
		progressWriter.report(line)
	}
	progressWriter.line.Reset()
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
