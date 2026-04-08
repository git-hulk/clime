package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/git-hulk/clime/internal/plugin"
)

var semverRe = regexp.MustCompile(`v?(\d+\.\d+\.\d+)`)

// Installer defines the lifecycle operations for a plugin source type.
// Implementations handle the mechanical work only — callers manage the manifest.
type Installer interface {
	// Install downloads and installs the plugin, returning the installed version.
	Install(name string) (version string, err error)

	// Update updates an already-installed plugin, returning the result.
	Update(name string, current plugin.ManifestEntry) (*UpdateResult, error)

	// Uninstall removes the plugin binary/symlink and performs any
	// source-specific cleanup (e.g. npm uninstall -g).
	Uninstall(name string, entry plugin.ManifestEntry) error

	// DetectVersion returns the currently installed version of the plugin.
	DetectVersion(name string) string

	// PluginType returns the source type constant (e.g. plugin.SourceTypeGitHub).
	PluginType() string

	// Source returns the source identifier (repo path, npm package, script URL).
	Source() string
}

// UpdateResult reports the outcome of a plugin update.
type UpdateResult struct {
	Name           string
	Source         string
	CurrentVersion string
	LatestVersion  string
	Updated        bool
	Path           string
}

// FromPlugin creates an Installer from a Plugin config entry (used by init and install commands).
func FromPlugin(p plugin.Plugin) (Installer, error) {
	switch {
	case p.Npm != "":
		return NewNpmInstaller(p.Npm), nil
	case p.Brew != "":
		return NewBrewInstaller(p.Brew), nil
	case p.Script != "":
		return NewScriptInstaller(p.Script, p.BinaryPath), nil
	case p.Repo != "":
		return NewGitHubInstaller(p.Repo), nil
	default:
		return nil, fmt.Errorf("plugin %q has no install source configured (set --repo, --npm, --brew, or --script)", p.Name)
	}
}

// FromManifest creates an Installer from a ManifestEntry (used by update and uninstall).
func FromManifest(entry plugin.ManifestEntry) (Installer, error) {
	switch entry.Type {
	case plugin.SourceTypeGitHub:
		return NewGitHubInstaller(entry.Source), nil
	case plugin.SourceTypeNpm:
		return NewNpmInstaller(entry.Source), nil
	case plugin.SourceTypeBrew:
		return NewBrewInstaller(entry.Source), nil
	case plugin.SourceTypeScript:
		return NewScriptInstaller(entry.Source, entry.BinaryPath), nil
	default:
		return nil, fmt.Errorf("unknown source type %q for plugin %q", entry.Type, entry.Name)
	}
}

// removePluginBinary removes the plugin binary or symlink from the plugin directory.
func removePluginBinary(name string) error {
	return removePluginBinaryWithResolvedTarget(name, false)
}

// removePluginBinaryAndTarget removes the managed plugin binary/symlink and, if
// the managed path is a symlink, also removes the resolved target binary.
func removePluginBinaryAndTarget(name string) error {
	return removePluginBinaryWithResolvedTarget(name, true)
}

func removePluginBinaryWithResolvedTarget(name string, removeTarget bool) error {
	installDir, err := plugin.PluginBinDir()
	if err != nil {
		return err
	}

	binPath := filepath.Join(installDir, plugin.BinPrefix+name)
	targetPath := ""

	if removeTarget {
		info, err := os.Lstat(binPath)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			targetPath, err = os.Readlink(binPath)
			if err != nil {
				return fmt.Errorf("failed to resolve plugin binary symlink: %w", err)
			}
			if !filepath.IsAbs(targetPath) {
				targetPath = filepath.Join(filepath.Dir(binPath), targetPath)
			}
			targetPath = filepath.Clean(targetPath)
		} else if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to inspect plugin binary: %w", err)
		}
	}

	if err := os.Remove(binPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove plugin binary: %w", err)
	}

	if removeTarget && targetPath != "" {
		if err := os.Remove(targetPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove plugin target binary: %w", err)
		}
	}

	return nil
}

func writePluginBinary(destPath string, binaryContent []byte) error {
	return os.WriteFile(destPath, binaryContent, 0755)
}

func normalizeVersion(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

// parseVersionOutput extracts a version string from command output.
// It tries to match semver (N.N.N or vN.N.N), then falls back to a
// single-word token (e.g. "dev"), and returns "latest" otherwise.
func parseVersionOutput(output string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return plugin.VersionLatest
	}

	if m := semverRe.FindStringSubmatch(output); m != nil {
		return m[1]
	}

	if !strings.ContainsAny(output, " \t\n") {
		return output
	}

	return plugin.VersionLatest
}
