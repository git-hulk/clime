package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestRepoToCloneURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"owner/repo", "https://github.com/owner/repo.git"},
		{"https://github.com/owner/repo.git", "https://github.com/owner/repo.git"},
		{"https://gitlab.com/group/repo.git", "https://gitlab.com/group/repo.git"},
		{"git@github.com:owner/repo.git", "git@github.com:owner/repo.git"},
		{"/tmp/local-repo", "/tmp/local-repo"},
		{"./relative-repo", "./relative-repo"},
		{"../parent-repo", "../parent-repo"},
	}

	for _, tt := range tests {
		got := repoToCloneURL(tt.input)
		if got != tt.want {
			t.Errorf("repoToCloneURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseRepoManifestYAML(t *testing.T) {
	t.Parallel()

	yamlContent := `skills:
  - name: docker-helper
    description: Docker management skill
    path: skills/docker-helper
    tags:
      - devops
  - name: git-wizard
    description: Git workflow automation
    path: skills/git-wizard
`
	var manifest RepoManifest
	if err := yaml.Unmarshal([]byte(yamlContent), &manifest); err != nil {
		t.Fatalf("failed to parse yaml: %v", err)
	}

	if len(manifest.Skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(manifest.Skills))
	}
	if manifest.Skills[0].Name != "docker-helper" {
		t.Fatalf("expected first skill name docker-helper, got %s", manifest.Skills[0].Name)
	}
	if manifest.Skills[0].Path != "skills/docker-helper" {
		t.Fatalf("expected path skills/docker-helper, got %s", manifest.Skills[0].Path)
	}
	if manifest.Skills[1].Name != "git-wizard" {
		t.Fatalf("expected second skill name git-wizard, got %s", manifest.Skills[1].Name)
	}
	if len(manifest.Skills[0].Tags) != 1 || manifest.Skills[0].Tags[0] != "devops" {
		t.Fatalf("unexpected tags: %v", manifest.Skills[0].Tags)
	}
}

func TestCloneAndReadSkillFilesFromLocal(t *testing.T) {
	t.Parallel()

	// Create a fake skill directory to simulate a cloned repo.
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my-skill")
	agentsDir := filepath.Join(skillDir, "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Skill"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentsDir, "openai.yaml"), []byte("model: gpt-4"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Directly test the file-walking logic by calling filepath.Walk
	// (CloneAndReadSkillFiles wraps cloneRepo + walk; we test the walk here).
	files := make(map[string][]byte)
	root := skillDir
	err := filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[rel] = data
		return nil
	})
	if err != nil {
		t.Fatalf("walk failed: %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	if string(files["SKILL.md"]) != "# Skill" {
		t.Fatalf("unexpected SKILL.md content: %q", files["SKILL.md"])
	}
	if string(files[filepath.Join("agents", "openai.yaml")]) != "model: gpt-4" {
		t.Fatalf("unexpected agents/openai.yaml content: %q", files[filepath.Join("agents", "openai.yaml")])
	}
}

func TestLocalRepoDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	got, ok, err := LocalRepoDir(dir)
	if err != nil {
		t.Fatalf("LocalRepoDir() error = %v", err)
	}
	if !ok {
		t.Fatal("LocalRepoDir() did not detect local repo")
	}

	want, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("filepath.Abs() error = %v", err)
	}
	if got != want {
		t.Fatalf("LocalRepoDir() = %q, want %q", got, want)
	}
}

func TestFetchRepoManifestFromLocalRepoWithoutGit(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills", "alpha")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skills.yaml"), []byte("skills:\n  - name: alpha\n    path: skills/alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "SKILL.md"), []byte("# Alpha"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", "")

	manifest, err := FetchRepoManifest(dir)
	if err != nil {
		t.Fatalf("FetchRepoManifest() error = %v", err)
	}
	if len(manifest.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(manifest.Skills))
	}
	if manifest.Skills[0].Name != "alpha" {
		t.Fatalf("skill name = %q, want %q", manifest.Skills[0].Name, "alpha")
	}
}

func TestCloneAndReadSkillFilesFromLocalRepoWithoutGit(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills", "alpha")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Alpha"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "extra.txt"), []byte("extra"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", "")

	files, err := CloneAndReadSkillFiles(dir, "skills/alpha")
	if err != nil {
		t.Fatalf("CloneAndReadSkillFiles() error = %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	if string(files["extra.txt"]) != "extra" {
		t.Fatalf("extra.txt = %q, want %q", files["extra.txt"], "extra")
	}
}

func TestSourceRepoDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input   string
		wantEnd string
	}{
		{"owner/repo", filepath.Join(".clime", "sources", "owner", "repo")},
		{"https://github.com/owner/repo.git", filepath.Join(".clime", "sources", "github.com", "owner", "repo")},
		{"git@github.com:owner/repo.git", filepath.Join(".clime", "sources", "github.com", "owner", "repo")},
		{"http://example.com/foo/bar.git", filepath.Join(".clime", "sources", "example.com", "foo", "bar")},
	}

	for _, tt := range tests {
		got, err := sourceRepoDir(tt.input)
		if err != nil {
			t.Errorf("sourceRepoDir(%q) error = %v", tt.input, err)
			continue
		}
		if !filepath.IsAbs(got) {
			t.Errorf("sourceRepoDir(%q) = %q, want absolute path", tt.input, got)
		}
		if !strings.HasSuffix(got, tt.wantEnd) {
			t.Errorf("sourceRepoDir(%q) = %q, want suffix %q", tt.input, got, tt.wantEnd)
		}
	}
}

func TestPrepareRepoDirUsesLocalRepo(t *testing.T) {
	dir := t.TempDir()

	got, cleanup, err := PrepareRepoDir(dir)
	if err != nil {
		t.Fatalf("PrepareRepoDir() error = %v", err)
	}
	defer cleanup()

	want, _ := filepath.Abs(dir)
	if got != want {
		t.Fatalf("PrepareRepoDir() = %q, want %q", got, want)
	}
}

func TestRemoveSourceDir(t *testing.T) {
	// Create a fake source dir.
	srcDir, err := sourceRepoDir("test-owner/test-repo")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "test.txt"), []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := RemoveSourceDir("test-owner/test-repo"); err != nil {
		t.Fatalf("RemoveSourceDir() error = %v", err)
	}

	if _, err := os.Stat(srcDir); !os.IsNotExist(err) {
		t.Fatalf("source dir still exists after RemoveSourceDir")
	}

	// Cleanup parent dirs.
	home, _ := os.UserHomeDir()
	os.RemoveAll(filepath.Join(home, ".clime", "sources", "test-owner"))
}
