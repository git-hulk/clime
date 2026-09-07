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
	if len(targets) != 0 {
		t.Fatalf("targets = %v, want none in an empty home", targets)
	}

	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	targets, err = DetectTargets()
	if err != nil {
		t.Fatalf("DetectTargets() error = %v", err)
	}
	if len(targets) != 1 || targets[0].Name != "claude" {
		t.Fatalf("targets = %v, want [claude]", targets)
	}

	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	targets, err = DetectTargets()
	if err != nil {
		t.Fatalf("DetectTargets() error = %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("targets = %v, want claude and codex", targets)
	}
}

func TestTargetInstallAndRemove(t *testing.T) {
	target := Target{Name: "claude", Dir: filepath.Join(t.TempDir(), ".claude")}
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
