package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectTargets(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	targets, err := DetectTargets()
	if err != nil {
		t.Fatalf("DetectTargets() error = %v", err)
	}
	if len(targets) != 1 || targets[0].Name != "agents" {
		t.Fatalf("targets = %v, want [agents] in an empty home", targets)
	}

	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	targets, err = DetectTargets()
	if err != nil {
		t.Fatalf("DetectTargets() error = %v", err)
	}
	if len(targets) != 2 || targets[0].Name != "agents" || targets[1].Name != "claude" {
		t.Fatalf("targets = %v, want [agents claude]", targets)
	}

	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	targets, err = DetectTargets()
	if err != nil {
		t.Fatalf("DetectTargets() error = %v", err)
	}
	if len(targets) != 2 || targets[0].Name != "agents" || targets[1].Name != "claude" {
		t.Fatalf("targets = %v, want [agents claude] even with Codex present", targets)
	}
}

func TestTargetInstallAndRemove(t *testing.T) {
	target := Target{Name: "agents", Dir: filepath.Join(t.TempDir(), ".agents")}
	if err := os.MkdirAll(target.Dir, 0o755); err != nil {
		t.Fatal(err)
	}

	files := map[string][]byte{
		"SKILL.md":                       []byte("# Skill"),
		"helper.sh":                      []byte("#!/bin/bash\necho hello"),
		filepath.Join("sub", "nest.txt"): []byte("nested"),
	}
	if err := target.Install("test-skill", files); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	for rel, want := range files {
		got, err := os.ReadFile(filepath.Join(target.Dir, "skills", "test-skill", rel))
		if err != nil {
			t.Fatalf("reading %s: %v", rel, err)
		}
		if string(got) != string(want) {
			t.Fatalf("%s = %q, want %q", rel, got, want)
		}
	}

	removed, err := target.Remove("test-skill")
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if !removed {
		t.Fatal("Remove() = false, want true for an installed skill")
	}
	if _, err := os.Stat(filepath.Join(target.Dir, "skills", "test-skill")); !os.IsNotExist(err) {
		t.Fatal("skill directory still exists after Remove")
	}

	removed, err = target.Remove("test-skill")
	if err != nil {
		t.Fatalf("Remove() second call error = %v", err)
	}
	if removed {
		t.Fatal("Remove() = true for a skill that is not installed")
	}
}

func TestClaudeInstallLinksSharedSkill(t *testing.T) {
	for _, existing := range []string{"absent", "directory", "shared link", "dangling link", "other link"} {
		t.Run(existing, func(t *testing.T) {
			home := t.TempDir()
			shared := Target{Name: "agents", Dir: filepath.Join(home, ".agents")}
			claude := Target{Name: "claude", Dir: filepath.Join(home, ".claude")}
			files := map[string][]byte{"SKILL.md": []byte("# Shared")}
			if err := shared.Install("test-skill", files); err != nil {
				t.Fatal(err)
			}
			link := claude.skillDir("test-skill")
			if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
				t.Fatal(err)
			}
			other := filepath.Join(home, "other")
			switch existing {
			case "directory":
				writeFile(t, filepath.Join(link, "SKILL.md"), "# Old")
			case "shared link", "dangling link", "other link":
				dest := shared.skillDir("test-skill")
				if existing != "shared link" {
					dest = other
				}
				if existing == "other link" {
					writeFile(t, filepath.Join(other, "SKILL.md"), "# Other")
				}
				if err := os.Symlink(dest, link); err != nil {
					t.Fatal(err)
				}
			}

			if err := claude.Install("test-skill", files); err != nil {
				t.Fatalf("Install() error = %v", err)
			}
			if got, err := os.Readlink(link); err != nil || got != shared.skillDir("test-skill") {
				t.Fatalf("Claude link = %q, %v", got, err)
			}
			files["SKILL.md"] = []byte("# Updated")
			if err := shared.Install("test-skill", files); err != nil {
				t.Fatal(err)
			}
			if got := readInstalledSkill(t, home, "test-skill"); got != "# Updated" {
				t.Fatalf("Claude content = %q, want updated shared content", got)
			}
			if existing == "other link" {
				data, err := os.ReadFile(filepath.Join(other, "SKILL.md"))
				if err != nil || string(data) != "# Other" {
					t.Fatalf("previous link target changed: %q, %v", data, err)
				}
			}

			if removed, err := claude.Remove("test-skill"); err != nil || !removed {
				t.Fatalf("Remove() = %v, %v", removed, err)
			}
			if _, err := os.Stat(shared.skillDir("test-skill")); err != nil {
				t.Fatalf("removing Claude link removed shared files: %v", err)
			}
		})
	}
}
