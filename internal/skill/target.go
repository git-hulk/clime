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
	for _, target := range targetHomes {
		targets = append(targets, Target{Name: target.name, Dir: filepath.Join(home, target.dir)})
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
	for _, target := range all {
		if target.Name == "agents" || target.Exists() {
			detected = append(detected, target)
		}
	}
	return detected, nil
}

// Exists reports whether the target's base directory exists.
func (target Target) Exists() bool {
	info, err := os.Stat(target.Dir)
	return err == nil && info.IsDir()
}

func (target Target) skillDir(name string) string {
	return filepath.Join(target.Dir, "skills", name)
}

// Install writes the given skill files under <Dir>/skills/<name>/.
// The Claude target links to the shared copy, which must be installed first.
func (target Target) Install(name string, files map[string][]byte) error {
	dir := target.skillDir(name)
	if target.Name == "claude" {
		shared := filepath.Join(filepath.Dir(target.Dir), ".agents", "skills", name)
		if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
			return fmt.Errorf("failed to create skills directory: %w", err)
		}
		if _, err := target.Remove(name); err != nil {
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
func (target Target) Remove(name string) (bool, error) {
	dir := target.skillDir(name)
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
