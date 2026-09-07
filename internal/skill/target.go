package skill

import (
	"fmt"
	"os"
	"path/filepath"
)

// Target is a destination for installed skills: ~/.agents or ~/.claude.
type Target struct {
	Name string
	Dir  string
}

// targetHomes lists the known agent dot-directories and their display names.
var targetHomes = []struct{ name, dir string }{
	{"agents", ".agents"},
	{"claude", ".claude"},
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

// DetectTargets always includes the shared skills target, followed by agent
// targets whose base directory exists.
func DetectTargets() ([]Target, error) {
	all, err := Targets()
	if err != nil {
		return nil, err
	}
	var detected []Target
	for _, t := range all {
		if t.Name == "agents" || t.Exists() {
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
// The Claude target links to the shared copy, which must be installed first.
func (t Target) Install(name string, files map[string][]byte) error {
	dir := t.skillDir(name)
	if t.Name == "claude" {
		shared := filepath.Join(filepath.Dir(t.Dir), ".agents", "skills", name)
		if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
			return fmt.Errorf("failed to create skills directory: %w", err)
		}
		if _, err := t.Remove(name); err != nil {
			return err
		}
		if err := os.Symlink(shared, dir); err != nil {
			return fmt.Errorf("failed to link %s to %s: %w", dir, shared, err)
		}
		return nil
	}
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
	if _, err := os.Lstat(dir); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to inspect %s: %w", dir, err)
	}
	if err := os.RemoveAll(dir); err != nil {
		return false, fmt.Errorf("failed to remove %s: %w", dir, err)
	}
	return true, nil
}
