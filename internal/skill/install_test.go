package skill

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// createTestGitRepo creates a temporary git repo with files under skillPath.
func createTestGitRepo(t *testing.T, skillPath string, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")

	for relPath, content := range files {
		fullPath := filepath.Join(dir, skillPath, relPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	run("add", "-A")
	run("commit", "-m", "init")

	return dir
}

func TestInstallWritesFiles(t *testing.T) {
	repoDir := createTestGitRepo(t, "skills/test-skill", map[string]string{
		"SKILL.md":       "---\nname: test-skill\ndescription: A test skill\n---\n# Test Skill",
		"helper.sh":      "#!/bin/bash\necho hello",
		"sub/nested.txt": "nested content",
	})

	home := t.TempDir()
	claudeDir := filepath.Join(home, ".claude")
	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	installed, err := Install("test-skill", repoDir, "skills/test-skill")
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if len(installed) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(installed))
	}

	for _, dir := range []string{claudeDir, codexDir} {
		skillDir := filepath.Join(dir, "skills", "test-skill")

		data, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
		if err != nil {
			t.Fatalf("failed to read SKILL.md from %s: %v", dir, err)
		}
		if string(data) != "---\nname: test-skill\ndescription: A test skill\n---\n# Test Skill" {
			t.Fatalf("unexpected SKILL.md content: %q", data)
		}

		data, err = os.ReadFile(filepath.Join(skillDir, "helper.sh"))
		if err != nil {
			t.Fatalf("failed to read helper.sh from %s: %v", dir, err)
		}
		if string(data) != "#!/bin/bash\necho hello" {
			t.Fatalf("unexpected helper.sh content: %q", data)
		}

		data, err = os.ReadFile(filepath.Join(skillDir, "sub", "nested.txt"))
		if err != nil {
			t.Fatalf("failed to read sub/nested.txt from %s: %v", dir, err)
		}
		if string(data) != "nested content" {
			t.Fatalf("unexpected nested.txt content: %q", data)
		}
	}
}

func TestInstallMissingSKILLMd(t *testing.T) {
	repoDir := createTestGitRepo(t, "skills/no-skill-md", map[string]string{
		"README.md": "# Not a skill",
	})

	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	_, err := Install("no-skill-md", repoDir, "skills/no-skill-md")
	if err == nil {
		t.Fatal("expected error for missing SKILL.md")
	}
}

func TestInstallNoTargetDirs(t *testing.T) {
	repoDir := createTestGitRepo(t, "skills/test-skill", map[string]string{
		"SKILL.md": "# Skill",
	})

	home := t.TempDir()
	t.Setenv("HOME", home)

	installed, err := Install("test-skill", repoDir, "skills/test-skill")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(installed) != 0 {
		t.Fatalf("expected 0 targets, got %d", len(installed))
	}
}

func TestInstallOnlyClaudeDir(t *testing.T) {
	repoDir := createTestGitRepo(t, "skills/test-skill", map[string]string{
		"SKILL.md": "# Skill",
	})

	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	installed, err := Install("test-skill", repoDir, "skills/test-skill")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(installed) != 1 {
		t.Fatalf("expected 1 target, got %d", len(installed))
	}
}

func TestInstallInvalidSkillPath(t *testing.T) {
	repoDir := createTestGitRepo(t, "skills/test-skill", map[string]string{
		"SKILL.md": "# Skill",
	})

	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	_, err := Install("test-skill", repoDir, "skills/nonexistent")
	if err == nil {
		t.Fatal("expected error for invalid skill path")
	}
}

func TestUninstallRemovesFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	for _, dir := range []string{".claude", ".codex"} {
		skillDir := filepath.Join(home, dir, "skills", "test-skill")
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Skill"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	removed, err := Uninstall("test-skill")
	if err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}
	if len(removed) != 2 {
		t.Fatalf("expected 2 removals, got %d", len(removed))
	}

	for _, dir := range []string{".claude", ".codex"} {
		skillDir := filepath.Join(home, dir, "skills", "test-skill")
		if _, err := os.Stat(skillDir); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed", skillDir)
		}
	}
}

func TestUninstallNotFound(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	removed, err := Uninstall("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(removed) != 0 {
		t.Fatalf("expected 0 removals, got %d", len(removed))
	}
}

func TestFetchRepoManifestFromSkillsYAML(t *testing.T) {
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")

	skillsYAML := `skills:
  - name: my-skill
    description: A test skill
    path: skills/my-skill
    tags:
      - test
`
	if err := os.WriteFile(filepath.Join(dir, "skills.yaml"), []byte(skillsYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	skillDir := filepath.Join(dir, "skills", "my-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# My Skill"), 0o644); err != nil {
		t.Fatal(err)
	}

	run("add", "-A")
	run("commit", "-m", "init")

	manifest, err := FetchRepoManifest(dir)
	if err != nil {
		t.Fatalf("FetchRepoManifest failed: %v", err)
	}
	if len(manifest.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(manifest.Skills))
	}
	if manifest.Skills[0].Name != "my-skill" {
		t.Fatalf("expected skill name my-skill, got %s", manifest.Skills[0].Name)
	}
}

func TestFetchRepoManifestFromMarketplaceJSON(t *testing.T) {
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")

	mpDir := filepath.Join(dir, ".claude-plugin")
	if err := os.MkdirAll(mpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mpJSON := `{
  "plugins": [
    {
      "name": "Test Plugin",
      "description": "A test plugin",
      "skills": ["./skills/skill-a", "./skills/skill-b"]
    }
  ]
}`
	if err := os.WriteFile(filepath.Join(mpDir, "marketplace.json"), []byte(mpJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"skill-a", "skill-b"} {
		skillDir := filepath.Join(dir, "skills", name)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatal(err)
		}
		content := "---\nname: " + name + "\ndescription: " + name + " desc\n---\n# " + name
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	run("add", "-A")
	run("commit", "-m", "init")

	manifest, err := FetchRepoManifest(dir)
	if err != nil {
		t.Fatalf("FetchRepoManifest failed: %v", err)
	}
	if len(manifest.Skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(manifest.Skills))
	}
	if manifest.Skills[0].Name != "skill-a" {
		t.Fatalf("expected skill-a, got %s", manifest.Skills[0].Name)
	}
	if manifest.Skills[0].Path != "skills/skill-a" {
		t.Fatalf("expected path skills/skill-a, got %s", manifest.Skills[0].Path)
	}
	if manifest.Skills[1].Name != "skill-b" {
		t.Fatalf("expected skill-b, got %s", manifest.Skills[1].Name)
	}
}

func TestFetchRepoManifestFromPluginJSON(t *testing.T) {
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")

	// Create .claude-plugin/plugin.json pointing to a skills directory.
	pluginDir := filepath.Join(dir, ".claude-plugin")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	pluginJSON := `{"name": "test-plugin", "skills": "./.claude/skills"}`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(pluginJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create two skill directories with SKILL.md files.
	for _, name := range []string{"skill-x", "skill-y"} {
		skillDir := filepath.Join(dir, ".claude", "skills", name)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatal(err)
		}
		content := "---\nname: " + name + "\ndescription: " + name + " desc\n---\n# " + name
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	run("add", "-A")
	run("commit", "-m", "init")

	manifest, err := FetchRepoManifest(dir)
	if err != nil {
		t.Fatalf("FetchRepoManifest failed: %v", err)
	}
	if len(manifest.Skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(manifest.Skills))
	}

	// Skills are returned in directory order; check both exist.
	names := map[string]bool{}
	for _, s := range manifest.Skills {
		names[s.Name] = true
	}
	if !names["skill-x"] || !names["skill-y"] {
		t.Fatalf("expected skill-x and skill-y, got %v", names)
	}
}

func TestFetchRepoManifestPluginJSONFallbackFromEmptyMarketplace(t *testing.T) {
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")

	pluginDir := filepath.Join(dir, ".claude-plugin")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// marketplace.json with plugins that have no skills arrays (like pbakaus/impeccable).
	mpJSON := `{"plugins": [{"name": "test", "description": "test plugin", "source": "./"}]}`
	if err := os.WriteFile(filepath.Join(pluginDir, "marketplace.json"), []byte(mpJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	// plugin.json points to the skills directory.
	pluginJSON := `{"name": "test", "skills": "./.claude/skills"}`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(pluginJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a skill directory.
	skillDir := filepath.Join(dir, ".claude", "skills", "my-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: my-skill\ndescription: A skill\n---\n# My Skill"), 0o644); err != nil {
		t.Fatal(err)
	}

	run("add", "-A")
	run("commit", "-m", "init")

	manifest, err := FetchRepoManifest(dir)
	if err != nil {
		t.Fatalf("FetchRepoManifest failed: %v", err)
	}
	if len(manifest.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(manifest.Skills))
	}
	if manifest.Skills[0].Name != "my-skill" {
		t.Fatalf("expected my-skill, got %s", manifest.Skills[0].Name)
	}
}

func TestFetchRepoManifestEmptyNameFallback(t *testing.T) {
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")

	mpDir := filepath.Join(dir, ".claude-plugin")
	if err := os.MkdirAll(mpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Skill with empty name in frontmatter
	mpJSON := `{"plugins": [{"name": "P", "skills": ["./skills/my-skill"]}]}`
	if err := os.WriteFile(filepath.Join(mpDir, "marketplace.json"), []byte(mpJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	skillDir := filepath.Join(dir, "skills", "my-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Frontmatter with no name field
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\ndescription: has desc but no name\n---\n# Skill"), 0o644); err != nil {
		t.Fatal(err)
	}

	run("add", "-A")
	run("commit", "-m", "init")

	manifest, err := FetchRepoManifest(dir)
	if err != nil {
		t.Fatalf("FetchRepoManifest failed: %v", err)
	}
	if len(manifest.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(manifest.Skills))
	}
	// Should fall back to directory basename "my-skill"
	if manifest.Skills[0].Name != "my-skill" {
		t.Fatalf("expected name my-skill (from directory), got %q", manifest.Skills[0].Name)
	}
	if manifest.Skills[0].Description != "has desc but no name" {
		t.Fatalf("expected description preserved, got %q", manifest.Skills[0].Description)
	}
}

func TestEndToEndInstallFromLocalRepo(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")

	mpDir := filepath.Join(dir, ".claude-plugin")
	if err := os.MkdirAll(mpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mpJSON := `{"plugins": [{"name": "P", "skills": ["./skills/alpha"]}]}`
	if err := os.WriteFile(filepath.Join(mpDir, "marketplace.json"), []byte(mpJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	skillDir := filepath.Join(dir, "skills", "alpha")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: alpha\ndescription: Alpha skill\n---\n# Alpha"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "extra.txt"), []byte("extra"), 0o644); err != nil {
		t.Fatal(err)
	}

	run("add", "-A")
	run("commit", "-m", "init")

	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Step 1: Fetch repo manifest
	manifest, err := FetchRepoManifest(dir)
	if err != nil {
		t.Fatalf("FetchRepoManifest failed: %v", err)
	}
	if len(manifest.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(manifest.Skills))
	}

	entry := manifest.Skills[0]
	if entry.Name != "alpha" {
		t.Fatalf("expected name alpha, got %s", entry.Name)
	}
	if entry.Path != "skills/alpha" {
		t.Fatalf("expected path skills/alpha, got %s", entry.Path)
	}

	// Step 2: Install the skill
	installed, err := Install(entry.Name, dir, entry.Path)
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if len(installed) != 1 {
		t.Fatalf("expected 1 target, got %d", len(installed))
	}

	// Step 3: Verify files
	installedSkillDir := filepath.Join(home, ".claude", "skills", "alpha")
	data, err := os.ReadFile(filepath.Join(installedSkillDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("failed to read SKILL.md: %v", err)
	}
	if string(data) != "---\nname: alpha\ndescription: Alpha skill\n---\n# Alpha" {
		t.Fatalf("unexpected SKILL.md content: %q", data)
	}

	data, err = os.ReadFile(filepath.Join(installedSkillDir, "extra.txt"))
	if err != nil {
		t.Fatalf("failed to read extra.txt: %v", err)
	}
	if string(data) != "extra" {
		t.Fatalf("unexpected extra.txt content: %q", data)
	}

	// Step 4: Uninstall
	removed, err := Uninstall("alpha")
	if err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}
	if len(removed) != 1 {
		t.Fatalf("expected 1 removal, got %d", len(removed))
	}

	if _, err := os.Stat(installedSkillDir); !os.IsNotExist(err) {
		t.Fatal("expected skill directory to be removed")
	}
}

func TestInstallFromDirSkipsClone(t *testing.T) {
	// Create a local directory (not a git repo) with skill files.
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills", "test-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: test-skill\ndescription: A test\n---\n# Test"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "helper.sh"), []byte("#!/bin/bash\necho hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	// InstallFromDir should work without cloning.
	installed, err := InstallFromDir("test-skill", dir, "skills/test-skill")
	if err != nil {
		t.Fatalf("InstallFromDir failed: %v", err)
	}
	if len(installed) != 1 || installed[0] != "claude" {
		t.Fatalf("expected [claude], got %v", installed)
	}

	data, err := os.ReadFile(filepath.Join(home, ".claude", "skills", "test-skill", "SKILL.md"))
	if err != nil {
		t.Fatalf("failed to read SKILL.md: %v", err)
	}
	if string(data) != "---\nname: test-skill\ndescription: A test\n---\n# Test" {
		t.Fatalf("unexpected SKILL.md content: %q", data)
	}

	data, err = os.ReadFile(filepath.Join(home, ".claude", "skills", "test-skill", "helper.sh"))
	if err != nil {
		t.Fatalf("failed to read helper.sh: %v", err)
	}
	if string(data) != "#!/bin/bash\necho hello" {
		t.Fatalf("unexpected helper.sh content: %q", data)
	}
}

func TestReadSkillFilesFromDir(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my-skill")
	subDir := filepath.Join(skillDir, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Skill"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "nested.txt"), []byte("nested"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := ReadSkillFilesFromDir(dir, "my-skill")
	if err != nil {
		t.Fatalf("ReadSkillFilesFromDir failed: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	if string(files["SKILL.md"]) != "# Skill" {
		t.Fatalf("unexpected SKILL.md: %q", files["SKILL.md"])
	}
	if string(files[filepath.Join("sub", "nested.txt")]) != "nested" {
		t.Fatalf("unexpected nested.txt: %q", files[filepath.Join("sub", "nested.txt")])
	}
}
