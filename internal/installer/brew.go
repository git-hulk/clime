package installer

import (
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"

	"github.com/git-hulk/clime/internal/plugin"
)

// BrewInstaller installs plugins from Homebrew formulae.
type BrewInstaller struct {
	Formula          string
	runBrewInstall   func(formula string) error
	runBrewUpdate    func(formula string) error
	runBrewUninstall func(formula string) error
	brewBinDir       func() (string, error)
	formulaBinDir    func(formula string) (string, error)
	pluginBinDir     func() (string, error)
	getVersion       func(formula string) (string, error)
	lookPath         func(name string) (string, error)
}

// NewBrewInstaller returns a BrewInstaller for the given formula.
func NewBrewInstaller(formula string) *BrewInstaller {
	return &BrewInstaller{
		Formula:          formula,
		runBrewInstall:   runBrewInstall,
		runBrewUpdate:    runBrewUpdate,
		runBrewUninstall: runBrewUninstall,
		brewBinDir:       brewBinDir,
		formulaBinDir:    brewFormulaBinDir,
		pluginBinDir:     plugin.PluginBinDir,
		getVersion:       getBrewInstalledVersion,
		lookPath:         osexec.LookPath,
	}
}

func (b *BrewInstaller) Install(name string) (string, error) {
	if _, err := b.lookPath("brew"); err != nil {
		return "", fmt.Errorf("homebrew is not installed or not on PATH: %w", err)
	}

	installErr := b.runBrewInstall(b.Formula)
	binaryPath, resolveErr := b.resolveInstalledBinary(name)
	if installErr != nil {
		// If brew install fails but an executable is already available, link it anyway.
		if resolveErr != nil {
			return "", fmt.Errorf("installing formula %q: %w", b.Formula, installErr)
		}
	} else if resolveErr != nil {
		return "", resolveErr
	}

	installDir, err := b.pluginBinDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return "", err
	}

	linkPath := filepath.Join(installDir, plugin.BinPrefix+name)
	os.Remove(linkPath)
	if err := os.Symlink(binaryPath, linkPath); err != nil {
		return "", fmt.Errorf("failed to create symlink: %w", err)
	}

	version, err := b.getVersion(b.Formula)
	if err != nil {
		version = plugin.VersionLatest
	}

	return version, nil
}

func (b *BrewInstaller) Update(name string, current plugin.ManifestEntry) (*UpdateResult, error) {
	if _, err := b.lookPath("brew"); err != nil {
		return nil, fmt.Errorf("homebrew is not installed or not on PATH: %w", err)
	}

	if err := b.runBrewUpdate(b.Formula); err != nil {
		return nil, fmt.Errorf("failed to update brew plugin %q: %w", b.Formula, err)
	}

	version, err := b.getVersion(b.Formula)
	if err != nil {
		version = plugin.VersionLatest
	}

	installDir, err := b.pluginBinDir()
	if err != nil {
		return nil, err
	}

	// Re-resolve the binary and update the symlink, since brew upgrade
	// may change the Cellar path the binary lives under.
	binaryPath, err := b.resolveInstalledBinary(name)
	if err == nil {
		linkPath := filepath.Join(installDir, plugin.BinPrefix+name)
		os.Remove(linkPath)
		_ = os.Symlink(binaryPath, linkPath)
	}

	updated := true
	if current.Version != "" && semverRe.MatchString(version) &&
		normalizeVersion(current.Version) == normalizeVersion(version) {
		updated = false
	}

	return &UpdateResult{
		Name:           name,
		Source:         b.Formula,
		CurrentVersion: current.Version,
		LatestVersion:  version,
		Updated:        updated,
		Path:           filepath.Join(installDir, plugin.BinPrefix+name),
	}, nil
}

func (b *BrewInstaller) Uninstall(name string, entry plugin.ManifestEntry) error {
	if _, err := b.lookPath("brew"); err != nil {
		fmt.Fprintf(os.Stderr, "warning: homebrew not found, skipping brew uninstall for %s\n", b.Formula)
	} else if err := b.runBrewUninstall(b.Formula); err != nil {
		fmt.Fprintf(os.Stderr, "warning: brew uninstall %s failed: %v\n", b.Formula, err)
	}
	return removePluginBinary(name)
}

func (b *BrewInstaller) DetectVersion(name string) string {
	version, err := b.getVersion(b.Formula)
	if err != nil {
		return plugin.VersionLatest
	}
	return version
}

func (b *BrewInstaller) PluginType() string { return plugin.SourceTypeBrew }
func (b *BrewInstaller) Source() string     { return b.Formula }

func (b *BrewInstaller) resolveInstalledBinary(name string) (string, error) {
	candidates := preferredBinaryNames(name, b.Formula)
	binName := candidates[0]

	if b.brewBinDir != nil {
		if binDir, err := b.brewBinDir(); err == nil {
			if path, ok := findFirstExistingBinary(binDir, candidates); ok {
				return path, nil
			}
		}
	}

	lookPath := b.lookPath
	if lookPath == nil {
		lookPath = osexec.LookPath
	}

	for _, candidate := range candidates {
		if found, err := lookPath(candidate); err == nil {
			return found, nil
		}
	}

	formulaBinDir := b.formulaBinDir
	if formulaBinDir == nil {
		formulaBinDir = brewFormulaBinDir
	}
	if binDir, err := formulaBinDir(b.Formula); err == nil {
		if path, ok := findFirstExistingBinary(binDir, candidates); ok {
			return path, nil
		}
		if path, ok := findLikelyBinaryInDir(binDir, candidates); ok {
			return path, nil
		}
	}

	return "", fmt.Errorf("binary %q or %q not found after brew install %q", binName, name, b.Formula)
}

func preferredBinaryNames(name, formula string) []string {
	var candidates []string
	addCandidateWithCLIVariants(&candidates, plugin.BinPrefix+name)
	addCandidateWithCLIVariants(&candidates, name)

	formulaName := filepath.Base(strings.TrimSpace(formula))
	if formulaName != "" {
		addCandidateWithCLIVariants(&candidates, formulaName)
		if strings.HasPrefix(formulaName, plugin.BinPrefix) {
			addCandidateWithCLIVariants(&candidates, strings.TrimPrefix(formulaName, plugin.BinPrefix))
		} else {
			addCandidateWithCLIVariants(&candidates, plugin.BinPrefix+formulaName)
		}
	}

	deduped := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if _, exists := seen[candidate]; exists {
			continue
		}
		seen[candidate] = struct{}{}
		deduped = append(deduped, candidate)
	}
	return deduped
}

func addCandidateWithCLIVariants(candidates *[]string, name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	*candidates = append(*candidates, name)

	if strings.HasSuffix(name, "-cli") {
		trimmed := strings.TrimSuffix(name, "-cli")
		trimmed = strings.TrimSuffix(trimmed, "-")
		if trimmed != "" {
			*candidates = append(*candidates, trimmed)
		}
	}
}

func findFirstExistingBinary(dir string, names []string) (string, bool) {
	for _, name := range names {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
	}
	return "", false
}

func findLikelyBinaryInDir(dir string, preferred []string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}

	executables := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
			continue
		}
		executables = append(executables, filepath.Join(dir, entry.Name()))
	}

	if len(executables) == 1 {
		return executables[0], true
	}

	matches := make([]string, 0, len(executables))
	for _, path := range executables {
		base := filepath.Base(path)
		for _, token := range preferred {
			if strings.Contains(base, token) {
				matches = append(matches, path)
				break
			}
		}
	}
	if len(matches) == 1 {
		return matches[0], true
	}
	return "", false
}

// brew helper functions

func runBrewInstall(formula string) error {
	cmd := brewInstallOrUpgradeCmd("install", formula)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("brew install failed: %w\n%s", err, string(output))
	}
	return nil
}

func runBrewUpdate(formula string) error {
	cmd := brewInstallOrUpgradeCmd("upgrade", formula)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// brew upgrade exits non-zero when the formula is already at the latest version.
		if strings.Contains(string(output), "already installed") {
			return nil
		}
		return fmt.Errorf("brew upgrade failed: %w\n%s", err, string(output))
	}
	return nil
}

func runBrewUninstall(formula string) error {
	cmd := osexec.Command("brew", "uninstall", formula)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("brew uninstall failed: %w\n%s", err, string(output))
	}
	return nil
}

func brewInstallOrUpgradeCmd(action, formula string) *osexec.Cmd {
	cmd := osexec.Command("brew", action, formula)
	cmd.Env = append(os.Environ(), "HOMEBREW_NO_INSTALL_CLEANUP=1")
	return cmd
}

func brewBinDir() (string, error) {
	out, err := osexec.Command("brew", "--prefix").Output()
	if err != nil {
		return "", fmt.Errorf("failed to get brew prefix: %w", err)
	}
	return filepath.Join(strings.TrimSpace(string(out)), "bin"), nil
}

func brewFormulaBinDir(formula string) (string, error) {
	out, err := osexec.Command("brew", "--prefix", formula).Output()
	if err != nil {
		return "", fmt.Errorf("failed to get brew formula prefix: %w", err)
	}
	return filepath.Join(strings.TrimSpace(string(out)), "bin"), nil
}

// getBrewInstalledVersion returns the latest listed installed version for a formula.
func getBrewInstalledVersion(formula string) (string, error) {
	out, err := osexec.Command("brew", "list", "--versions", formula).Output()
	if err != nil {
		return "", fmt.Errorf("failed to get brew formula version: %w", err)
	}

	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) < 2 {
		return "", fmt.Errorf("formula %s is not installed", formula)
	}

	return fields[len(fields)-1], nil
}
