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
		pluginBinDir:     plugin.PluginBinDir,
		getVersion:       getBrewInstalledVersion,
		lookPath:         osexec.LookPath,
	}
}

func (b *BrewInstaller) Install(name string) (string, error) {
	if _, err := b.lookPath("brew"); err != nil {
		return "", fmt.Errorf("homebrew is not installed or not on PATH: %w", err)
	}

	if err := b.runBrewInstall(b.Formula); err != nil {
		return "", fmt.Errorf("installing formula %q: %w", b.Formula, err)
	}

	binaryPath, err := b.resolveInstalledBinary(name)
	if err != nil {
		return "", err
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
	binName := plugin.BinPrefix + name

	if binDir, err := b.brewBinDir(); err == nil {
		path := filepath.Join(binDir, binName)
		if _, statErr := os.Stat(path); statErr == nil {
			return path, nil
		}
		path = filepath.Join(binDir, name)
		if _, statErr := os.Stat(path); statErr == nil {
			return path, nil
		}
	}

	if found, err := b.lookPath(binName); err == nil {
		return found, nil
	}
	if found, err := b.lookPath(name); err == nil {
		return found, nil
	}

	return "", fmt.Errorf("binary %q or %q not found after brew install %q", binName, name, b.Formula)
}

// brew helper functions

func runBrewInstall(formula string) error {
	cmd := osexec.Command("brew", "install", formula)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("brew install failed: %w\n%s", err, string(output))
	}
	return nil
}

func runBrewUpdate(formula string) error {
	cmd := osexec.Command("brew", "upgrade", formula)
	if output, err := cmd.CombinedOutput(); err != nil {
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

func brewBinDir() (string, error) {
	out, err := osexec.Command("brew", "--prefix").Output()
	if err != nil {
		return "", fmt.Errorf("failed to get brew prefix: %w", err)
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
