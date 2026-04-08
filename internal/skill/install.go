package skill

import (
	"fmt"
	"os"
	"path/filepath"
)

// targetName maps a dot-directory name to a display-friendly target name.
var targetName = map[string]string{
	".claude": "claude",
	".codex":  "codex",
}

// installFiles writes the given skill files to ~/.claude/skills/<name>/
// and ~/.codex/skills/<name>/.
// Returns the names of the targets installed to (e.g., ["claude", "codex"]).
func installFiles(name string, files map[string][]byte) ([]string, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("no files found for skill %q", name)
	}

	if _, ok := files["SKILL.md"]; !ok {
		return nil, fmt.Errorf("skill %q is missing required SKILL.md file", name)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	var installed []string
	targets := []string{".claude", ".codex"}
	for _, dir := range targets {
		base := filepath.Join(home, dir)
		info, err := os.Stat(base)
		if err != nil || !info.IsDir() {
			continue
		}

		skillDir := filepath.Join(base, "skills", name)
		for relPath, content := range files {
			destPath := filepath.Join(skillDir, relPath)
			if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
				return installed, fmt.Errorf("failed to create directory for %s: %w", destPath, err)
			}
			if err := os.WriteFile(destPath, content, 0o644); err != nil {
				return installed, fmt.Errorf("failed to write %s: %w", destPath, err)
			}
		}
		installed = append(installed, targetName[dir])
	}

	return installed, nil
}

// Install clones the repo, reads skill files, and writes them
// to ~/.claude/skills/<name>/ and ~/.codex/skills/<name>/.
// Returns the names of the targets installed to (e.g., ["claude", "codex"]).
func Install(name, repo, skillPath string) ([]string, error) {
	files, err := CloneAndReadSkillFiles(repo, skillPath)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch skill files: %w", err)
	}
	return installFiles(name, files)
}

// InstallFromDir reads skill files from a local directory and writes them
// to ~/.claude/skills/<name>/ and ~/.codex/skills/<name>/.
// Use this when the repo has already been cloned to avoid redundant clones.
func InstallFromDir(name, dir, skillPath string) ([]string, error) {
	files, err := ReadSkillFilesFromDir(dir, skillPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read skill files: %w", err)
	}
	return installFiles(name, files)
}

// Uninstall removes skill files from ~/.claude/skills/<name>/ and ~/.codex/skills/<name>/.
// Returns the names of the targets removed from (e.g., ["claude", "codex"]).
func Uninstall(name string) ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	var removed []string
	targets := []string{".claude", ".codex"}
	for _, dir := range targets {
		skillDir := filepath.Join(home, dir, "skills", name)
		if _, err := os.Stat(skillDir); err != nil {
			continue
		}
		if err := os.RemoveAll(skillDir); err != nil {
			return removed, fmt.Errorf("failed to remove %s: %w", skillDir, err)
		}
		removed = append(removed, targetName[dir])
	}

	return removed, nil
}
