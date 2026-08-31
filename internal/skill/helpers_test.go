package skill

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// gitCmd runs git in dir for building test fixtures.
func gitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed:\n%s", args, out)
	return strings.TrimSpace(string(out))
}

// initSkillFixture creates a Git repository containing a skills.yaml catalog
// with the given skill names and returns its directory.
func initSkillFixture(t *testing.T, skills ...string) string {
	t.Helper()
	dir := t.TempDir()
	gitCmd(t, dir, "init", "-q", "-b", "main")
	gitCmd(t, dir, "config", "uploadpack.allowAnySHA1InWant", "true")
	writeSkillFixtureContent(t, dir, "v1", skills...)
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-q", "-m", "initial")
	return dir
}

// writeSkillFixtureContent (re)writes the catalog and skill directories.
func writeSkillFixtureContent(t *testing.T, dir, marker string, skills ...string) {
	t.Helper()
	var catalog strings.Builder
	catalog.WriteString("skills:\n")
	for _, name := range skills {
		fmt.Fprintf(&catalog, "  - name: %s\n    description: Test skill %s\n    path: skills/%s\n", name, name, name)
		skillDir := filepath.Join(dir, "skills", name)
		require.NoError(t, os.RemoveAll(skillDir))
		require.NoError(t, os.MkdirAll(skillDir, 0o755))
		md := fmt.Sprintf("---\nname: %s\ndescription: Test skill %s\n---\nContent %s\n", name, name, marker)
		require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(md), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(skillDir, marker+".txt"), []byte(marker), 0o644))
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "skills.yaml"), []byte(catalog.String()), 0o644))
}

// routeRepos points the canonical HTTPS clone URLs of the given repositories
// at local fixture directories through Git insteadOf rules, and isolates the
// tests from the user's real Git configuration.
func routeRepos(t *testing.T, routes map[string]string) {
	t.Helper()
	var cfg strings.Builder
	for repo, dir := range routes {
		id, err := ParseRepo(repo)
		require.NoError(t, err)
		fmt.Fprintf(&cfg, "[url \"%s\"]\n\tinsteadOf = %s\n", dir, id.CloneURL())
	}
	cfgPath := filepath.Join(t.TempDir(), "gitconfig")
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfg.String()), 0o644))
	t.Setenv("GIT_CONFIG_GLOBAL", cfgPath)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
}

// setTempHome points HOME at a fresh directory and creates the agent target
// base directories.
func setTempHome(t *testing.T, agentDirs ...string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, dir := range agentDirs {
		require.NoError(t, os.MkdirAll(filepath.Join(home, dir), 0o755))
	}
	return home
}

// disableGit makes every Git invocation fail, proving an operation is offline.
func disableGit(t *testing.T) {
	t.Helper()
	orig := runGit
	runGit = func(dir string, args ...string) (string, error) {
		return "", errors.New("network disabled by test")
	}
	t.Cleanup(func() { runGit = orig })
}
