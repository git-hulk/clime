package skill

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// gitIn runs a git command in dir and returns its trimmed output.
func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
	return strings.TrimSpace(string(out))
}

// createTestGitRepo creates a temporary git repo with files under skillPath.
func createTestGitRepo(t *testing.T, skillPath string, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()

	gitIn(t, dir, "init")
	gitIn(t, dir, "config", "user.email", "test@test.com")
	gitIn(t, dir, "config", "user.name", "Test")

	for relPath, content := range files {
		fullPath := filepath.Join(dir, skillPath, relPath)
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o755))
		require.NoError(t, os.WriteFile(fullPath, []byte(content), 0o644))
	}

	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-m", "init")

	return dir
}

// checkoutVersion returns the version a checkout directory holds: a tag
// pointing exactly at HEAD, or the full commit SHA.
func checkoutVersion(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "describe", "--tags", "--exact-match", "HEAD")
	cmd.Dir = dir
	if out, err := cmd.Output(); err == nil {
		return strings.TrimSpace(string(out))
	}
	cmd = exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}
