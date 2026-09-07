package installer

import (
	"encoding/json"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/git-hulk/clime/internal/plugin"
)

// NpmInstaller installs plugins from npm global packages.
type NpmInstaller struct {
	Package         string
	runNpmInstall   func(packageName string) error
	runNpmUpdate    func(packageName string) error
	runNpmUninstall func(packageName string) error
	npmGlobalBinDir func() (string, error)
	pluginBinDir    func() (string, error)
	getVersion      func(packageName string) (string, error)
}

// NewNpmInstaller returns an NpmInstaller for the given npm package.
// A bare "owner/repo" string is treated as a scoped package and rewritten to
// "@owner/repo"; npm would otherwise resolve it as a GitHub shorthand, which
// is rarely what callers want when they specify --npm.
func NewNpmInstaller(packageName string) *NpmInstaller {
	return &NpmInstaller{
		Package:         normalizeNpmPackageName(packageName),
		runNpmInstall:   runNpmGlobalInstall,
		runNpmUpdate:    runNpmGlobalUpdate,
		runNpmUninstall: runNpmGlobalUninstall,
		npmGlobalBinDir: npmGlobalBinDir,
		pluginBinDir:    plugin.PluginBinDir,
		getVersion:      getNpmInstalledVersion,
	}
}

// normalizeNpmPackageName prepends "@" to bare "owner/repo" strings so npm
// treats them as scoped registry packages rather than GitHub shorthand.
// Inputs that are already scoped, unscoped, URLs, protocol-prefixed
// (git+https://, github:, file:, etc.), or local paths are returned as-is.
func normalizeNpmPackageName(packageName string) string {
	packageName = strings.TrimSpace(packageName)
	if packageName == "" || strings.HasPrefix(packageName, "@") {
		return packageName
	}
	if strings.ContainsAny(packageName, ":\\") || strings.HasPrefix(packageName, ".") || strings.HasPrefix(packageName, "/") {
		return packageName
	}
	if strings.Count(packageName, "/") == 1 {
		return "@" + packageName
	}
	return packageName
}

func (installer *NpmInstaller) Install(name string) (string, error) {
	if _, err := osexec.LookPath("npm"); err != nil {
		return "", fmt.Errorf("npm is not installed or not on PATH: %w", err)
	}

	npmBinDir, err := installer.npmGlobalBinDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine npm global bin directory: %w", err)
	}
	before := snapshotDirEntries(npmBinDir)

	if err := installer.runNpmInstall(installer.Package); err != nil {
		return "", fmt.Errorf("npm install failed: %w", err)
	}

	binName := plugin.BinPrefix + name
	binaryPath, err := locateNpmInstalledBinary(npmBinDir, installer.Package, name, binName, before)
	if err != nil {
		return "", err
	}

	installDir, err := installer.pluginBinDir()
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

	version, err := installer.getVersion(installer.Package)
	if err != nil {
		version = plugin.VersionLatest
	}

	return version, nil
}

func (installer *NpmInstaller) Update(name string, current plugin.ManifestEntry) (*UpdateResult, error) {
	if err := installer.runNpmUpdate(installer.Package); err != nil {
		return nil, fmt.Errorf("failed to update npm plugin %q: %w", installer.Package, err)
	}

	version, err := installer.getVersion(installer.Package)
	if err != nil {
		version = plugin.VersionLatest
	}

	installDir, err := installer.pluginBinDir()
	if err != nil {
		return nil, err
	}

	updated := true
	if current.Version != "" && semverRe.MatchString(version) &&
		normalizeVersion(current.Version) == normalizeVersion(version) {
		updated = false
	}

	return &UpdateResult{
		Name:           name,
		Source:         installer.Package,
		CurrentVersion: current.Version,
		LatestVersion:  version,
		Updated:        updated,
		Path:           filepath.Join(installDir, plugin.BinPrefix+name),
	}, nil
}

func (installer *NpmInstaller) Uninstall(name string, entry plugin.ManifestEntry) error {
	cmd := osexec.Command("npm", "uninstall", "-g", installer.Package)
	if output, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: npm uninstall -g %s failed: %v\n%s", installer.Package, err, string(output))
	}
	return removePluginBinary(name)
}

func (installer *NpmInstaller) DetectVersion(name string) string {
	version, err := installer.getVersion(installer.Package)
	if err != nil {
		return plugin.VersionLatest
	}
	return version
}

func (installer *NpmInstaller) PluginType() string { return plugin.SourceTypeNpm }
func (installer *NpmInstaller) Source() string     { return installer.Package }

// npm helper functions

func runNpmGlobalInstall(packageName string) error {
	cmd := osexec.Command("npm", "install", "-g", packageName)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("npm install failed: %w\n%s", err, string(output))
	}
	return nil
}

func runNpmGlobalUpdate(packageName string) error {
	cmd := osexec.Command("npm", "update", "-g", packageName)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("npm update failed: %w\n%s", err, string(output))
	}
	return nil
}

func runNpmGlobalUninstall(packageName string) error {
	cmd := osexec.Command("npm", "uninstall", "-g", packageName)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("npm uninstall failed: %w\n%s", err, string(output))
	}
	return nil
}

func npmGlobalBinDir() (string, error) {
	output, err := osexec.Command("npm", "prefix", "-g").Output()
	if err != nil {
		return "", fmt.Errorf("failed to get npm global prefix: %w", err)
	}
	return filepath.Join(strings.TrimSpace(string(output)), "bin"), nil
}

// snapshotDirEntries returns the set of entry names directly under dir, or an
// empty set if the directory cannot be read.
func snapshotDirEntries(dir string) map[string]struct{} {
	set := make(map[string]struct{})
	entries, err := os.ReadDir(dir)
	if err != nil {
		return set
	}
	for _, entry := range entries {
		set[entry.Name()] = struct{}{}
	}
	return set
}

// locateNpmInstalledBinary picks the binary that an npm install created.
// It prefers clime-<name>, then <name>, then falls back to a single new entry
// added to npmBinDir during the install. The error explains why no candidate
// was found, including any unexpected new entries.
func locateNpmInstalledBinary(npmBinDir, packageName, name, binName string, before map[string]struct{}) (string, error) {
	if path := filepath.Join(npmBinDir, binName); fileExists(path) {
		return path, nil
	}
	if path := filepath.Join(npmBinDir, name); fileExists(path) {
		return path, nil
	}

	after := snapshotDirEntries(npmBinDir)
	var added []string
	for entry := range after {
		if _, existed := before[entry]; !existed {
			added = append(added, entry)
		}
	}
	sort.Strings(added)

	switch len(added) {
	case 0:
		return "", fmt.Errorf("npm install of %q did not create a binary in %q; the package may not provide a CLI (check that the package name is correct, e.g. a scoped package starting with \"@\")", packageName, npmBinDir)
	case 1:
		return filepath.Join(npmBinDir, added[0]), nil
	default:
		return "", fmt.Errorf("npm install of %q created multiple binaries in %q (%s); none matched %q or %q — rerun with a plugin name matching one of them", packageName, npmBinDir, strings.Join(added, ", "), binName, name)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// getNpmInstalledVersion returns the actual installed version of an npm package.
func getNpmInstalledVersion(packageName string) (string, error) {
	output, err := osexec.Command("npm", "list", "-g", packageName, "--json").Output()
	if err != nil {
		return "", fmt.Errorf("failed to get npm package version: %w", err)
	}

	var result struct {
		Dependencies map[string]struct {
			Version string `json:"version"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return "", fmt.Errorf("failed to parse npm list output: %w", err)
	}

	dependency, ok := result.Dependencies[packageName]
	if !ok {
		return "", fmt.Errorf("package %s not found in npm list output", packageName)
	}
	if dependency.Version == "" {
		return "", fmt.Errorf("version not found in npm list output")
	}
	return dependency.Version, nil
}
