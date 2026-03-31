package installer

import (
	"encoding/json"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"

	"github.com/git-hulk/clime/internal/plugin"
)

// NpmInstaller installs plugins from npm global packages.
type NpmInstaller struct {
	Package         string
	runNpmInstall   func(pkg string) error
	runNpmUpdate    func(pkg string) error
	runNpmUninstall func(pkg string) error
	npmGlobalBinDir func() (string, error)
	pluginBinDir    func() (string, error)
	getVersion      func(pkg string) (string, error)
}

// NewNpmInstaller returns an NpmInstaller for the given npm package.
func NewNpmInstaller(pkg string) *NpmInstaller {
	return &NpmInstaller{
		Package:         pkg,
		runNpmInstall:   runNpmGlobalInstall,
		runNpmUpdate:    runNpmGlobalUpdate,
		runNpmUninstall: runNpmGlobalUninstall,
		npmGlobalBinDir: npmGlobalBinDir,
		pluginBinDir:    plugin.PluginBinDir,
		getVersion:      getNpmInstalledVersion,
	}
}

func (n *NpmInstaller) Install(name string) (string, error) {
	if _, err := osexec.LookPath("npm"); err != nil {
		return "", fmt.Errorf("npm is not installed or not on PATH: %w", err)
	}

	if err := n.runNpmInstall(n.Package); err != nil {
		return "", fmt.Errorf("npm install failed: %w", err)
	}

	binName := plugin.BinPrefix + name
	npmBinDir, err := n.npmGlobalBinDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine npm global bin directory: %w", err)
	}
	binaryPath := filepath.Join(npmBinDir, binName)

	if _, err := os.Stat(binaryPath); err != nil {
		binaryPath = filepath.Join(npmBinDir, name)
		if _, err := os.Stat(binaryPath); err != nil {
			return "", fmt.Errorf("binary %q or %q not found in npm global bin dir %q after install: %w", binName, name, npmBinDir, err)
		}
	}

	installDir, err := n.pluginBinDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return "", err
	}

	linkPath := filepath.Join(installDir, binName)
	os.Remove(linkPath)
	if err := os.Symlink(binaryPath, linkPath); err != nil {
		return "", fmt.Errorf("failed to create symlink: %w", err)
	}

	version, err := n.getVersion(n.Package)
	if err != nil {
		version = plugin.VersionLatest
	}

	return version, nil
}

func (n *NpmInstaller) Update(name string, current plugin.ManifestEntry) (*UpdateResult, error) {
	if err := n.runNpmUpdate(n.Package); err != nil {
		return nil, fmt.Errorf("failed to update npm plugin %q: %w", n.Package, err)
	}

	version, err := n.getVersion(n.Package)
	if err != nil {
		version = plugin.VersionLatest
	}

	installDir, err := n.pluginBinDir()
	if err != nil {
		return nil, err
	}

	return &UpdateResult{
		Name:           name,
		Source:         n.Package,
		CurrentVersion: current.Version,
		LatestVersion:  version,
		Updated:        true,
		Path:           filepath.Join(installDir, plugin.BinPrefix+name),
	}, nil
}

func (n *NpmInstaller) Uninstall(name string, entry plugin.ManifestEntry) error {
	cmd := osexec.Command("npm", "uninstall", "-g", n.Package)
	if output, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: npm uninstall -g %s failed: %v\n%s", n.Package, err, string(output))
	}
	return removePluginBinary(name)
}

func (n *NpmInstaller) DetectVersion(name string) string {
	version, err := n.getVersion(n.Package)
	if err != nil {
		return plugin.VersionLatest
	}
	return version
}

func (n *NpmInstaller) PluginType() string { return plugin.SourceTypeNpm }
func (n *NpmInstaller) Source() string     { return n.Package }

// npm helper functions

func runNpmGlobalInstall(pkg string) error {
	cmd := osexec.Command("npm", "install", "-g", pkg)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("npm install failed: %w\n%s", err, string(output))
	}
	return nil
}

func runNpmGlobalUpdate(pkg string) error {
	cmd := osexec.Command("npm", "update", "-g", pkg)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("npm update failed: %w\n%s", err, string(output))
	}
	return nil
}

func runNpmGlobalUninstall(pkg string) error {
	cmd := osexec.Command("npm", "uninstall", "-g", pkg)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("npm uninstall failed: %w\n%s", err, string(output))
	}
	return nil
}

func npmGlobalBinDir() (string, error) {
	out, err := osexec.Command("npm", "prefix", "-g").Output()
	if err != nil {
		return "", fmt.Errorf("failed to get npm global prefix: %w", err)
	}
	return filepath.Join(strings.TrimSpace(string(out)), "bin"), nil
}

// getNpmInstalledVersion returns the actual installed version of an npm package.
func getNpmInstalledVersion(pkg string) (string, error) {
	out, err := osexec.Command("npm", "list", "-g", pkg, "--json").Output()
	if err != nil {
		return "", fmt.Errorf("failed to get npm package version: %w", err)
	}

	var result struct {
		Dependencies map[string]struct {
			Version string `json:"version"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return "", fmt.Errorf("failed to parse npm list output: %w", err)
	}

	dep, ok := result.Dependencies[pkg]
	if !ok {
		return "", fmt.Errorf("package %s not found in npm list output", pkg)
	}
	if dep.Version == "" {
		return "", fmt.Errorf("version not found in npm list output")
	}
	return dep.Version, nil
}
