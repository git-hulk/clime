package skill

import (
	"fmt"
	"os"
	"path/filepath"
)

// Target is one agent destination for installed skills, such as
// ~/.claude or ~/.codex.
type Target struct {
	Name string
	Dir  string
}

// targetHomes lists the known agent dot-directories and their display names.
var targetHomes = []struct{ name, dir string }{
	{"claude", ".claude"},
	{"codex", ".codex"},
}

// Targets returns every known target, whether or not its base directory
// exists.
func Targets() ([]Target, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	targets := make([]Target, 0, len(targetHomes))
	for _, t := range targetHomes {
		targets = append(targets, Target{Name: t.name, Dir: filepath.Join(home, t.dir)})
	}
	return targets, nil
}

// DetectTargets returns the targets whose base directory exists.
func DetectTargets() ([]Target, error) {
	all, err := Targets()
	if err != nil {
		return nil, err
	}
	var detected []Target
	for _, t := range all {
		if t.Exists() {
			detected = append(detected, t)
		}
	}
	return detected, nil
}

// Exists reports whether the target's base directory exists.
func (t Target) Exists() bool {
	info, err := os.Stat(t.Dir)
	return err == nil && info.IsDir()
}

func (t Target) skillDir(name string) string {
	return filepath.Join(t.Dir, "skills", name)
}

// Install writes the given skill files under <Dir>/skills/<name>/.
func (t Target) Install(name string, files map[string][]byte) error {
	dir := t.skillDir(name)
	for rel, content := range files {
		dest := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return fmt.Errorf("failed to create directory for %s: %w", dest, err)
		}
		if err := os.WriteFile(dest, content, 0o644); err != nil {
			return fmt.Errorf("failed to write %s: %w", dest, err)
		}
	}
	return nil
}

// Remove deletes <Dir>/skills/<name>/, reporting whether it existed.
func (t Target) Remove(name string) (bool, error) {
	dir := t.skillDir(name)
	if _, err := os.Stat(dir); err != nil {
		return false, nil
	}
	if err := os.RemoveAll(dir); err != nil {
		return false, fmt.Errorf("failed to remove %s: %w", dir, err)
	}
	return true, nil
}
