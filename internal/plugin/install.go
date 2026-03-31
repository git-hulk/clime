package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/git-hulk/clime/internal/githubrelease"
)

var semverRe = regexp.MustCompile(`v?(\d+\.\d+\.\d+)`)

// InstallFromRepo downloads and installs a plugin from a specific GitHub repo.
func InstallFromRepo(name, repo string) (string, error) {
	// Fetch latest release
	release, err := githubrelease.FetchLatest(repo)
	if err != nil {
		return "", fmt.Errorf("failed to fetch latest release for %s: %w", repo, err)
	}

	// The binary name in the release asset is derived from the repo name,
	// which may differ from the plugin name.
	repoName := repoBaseName(repo)
	asset, err := findAsset(release, repoName)
	if err != nil {
		return "", err
	}

	// Download and extract
	installDir, err := pluginBinDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create plugin directory: %w", err)
	}

	destPath := filepath.Join(installDir, binPrefix+name)
	binaryContent, err := githubrelease.DownloadTarGzBinary(asset.BrowserDownloadURL, repoName)
	if err != nil {
		return "", fmt.Errorf("failed to install plugin: %w", err)
	}
	if err := os.WriteFile(destPath, binaryContent, 0755); err != nil {
		return "", fmt.Errorf("failed to install plugin: %w", err)
	}

	// Update manifest
	version := release.Version()
	manifest, err := LoadManifest()
	if err != nil {
		manifest = &Manifest{}
	}
	manifest.Add(name, version, SourceTypeGitHub, repo, "")
	if err := manifest.Save(); err != nil {
		return "", fmt.Errorf("plugin installed but failed to update manifest: %w", err)
	}

	return version, nil
}

// InstallFromScript runs a remote install script (curl | sh) and optionally
// creates a symlink so the installed binary is discoverable as clime-<name>.
// If binaryPath is empty, only the script is executed and no symlink is created.
func InstallFromScript(name, scriptURL, binaryPath string) error {
	// Run the install script (capture output to avoid interleaving with spinner)
	cmd := osexec.Command("bash", "-c", fmt.Sprintf("curl -fsSL '%s' | bash", scriptURL))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("install script failed: %w\n%s", err, string(output))
	}

	if binaryPath != "" {
		// Resolve the actual binary path (expand ~)
		if strings.HasPrefix(binaryPath, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			binaryPath = filepath.Join(home, binaryPath[2:])
		}

		// Verify the binary exists
		if _, err := os.Stat(binaryPath); err != nil {
			return fmt.Errorf("binary not found at %s after install: %w", binaryPath, err)
		}

		// Create symlink: ~/.clime/plugins/clime-<name> -> binaryPath
		installDir, err := pluginBinDir()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(installDir, 0755); err != nil {
			return err
		}

		linkPath := filepath.Join(installDir, binPrefix+name)
		// Remove existing symlink if any
		os.Remove(linkPath)
		if err := os.Symlink(binaryPath, linkPath); err != nil {
			return fmt.Errorf("failed to create symlink: %w", err)
		}
	}

	// Detect version from the installed binary's "version" command
	version := detectScriptPluginVersion(name)

	// Update manifest
	manifest, err := LoadManifest()
	if err != nil {
		manifest = &Manifest{}
	}
	manifest.Add(name, version, SourceTypeScript, scriptURL, binaryPath)
	if err := manifest.Save(); err != nil {
		return fmt.Errorf("plugin installed but failed to update manifest: %w", err)
	}

	return nil
}

// InstallFromNpm runs `npm install -g <package>` and creates a symlink so
// the installed binary is discoverable as clime-<name>.
func InstallFromNpm(name, npmPackage string) error {
	// Check npm is available
	if _, err := osexec.LookPath("npm"); err != nil {
		return fmt.Errorf("npm is not installed or not on PATH: %w", err)
	}

	// Run npm install -g (capture output to avoid interleaving with spinner)
	cmd := osexec.Command("npm", "install", "-g", npmPackage)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("npm install failed: %w\n%s", err, string(output))
	}

	// Discover the installed binary path
	binName := binPrefix + name
	npmBinDir, err := npmGlobalBinDir()
	if err != nil {
		return fmt.Errorf("failed to determine npm global bin directory: %w", err)
	}
	binaryPath := filepath.Join(npmBinDir, binName)

	// Verify the binary exists; fall back to the bare name (without clime- prefix)
	// since many npm packages install binaries without the prefix.
	if _, err := os.Stat(binaryPath); err != nil {
		binaryPath = filepath.Join(npmBinDir, name)
		if _, err := os.Stat(binaryPath); err != nil {
			return fmt.Errorf("binary %q or %q not found in npm global bin dir %q after install: %w", binName, name, npmBinDir, err)
		}
	}

	// Create symlink: ~/.clime/plugins/clime-<name> -> binaryPath
	installDir, err := pluginBinDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return err
	}

	linkPath := filepath.Join(installDir, binName)
	os.Remove(linkPath) // remove existing symlink if any
	if err := os.Symlink(binaryPath, linkPath); err != nil {
		return fmt.Errorf("failed to create symlink: %w", err)
	}

	// Update manifest with actual version
	version, err := getNpmInstalledVersion(npmPackage)
	if err != nil {
		// Fallback to "latest" if version detection fails
		version = VersionLatest
	}
	manifest, err := LoadManifest()
	if err != nil {
		manifest = &Manifest{}
	}
	manifest.Add(name, version, SourceTypeNpm, npmPackage, "")
	if err := manifest.Save(); err != nil {
		return fmt.Errorf("plugin installed but failed to update manifest: %w", err)
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

	// Parse the JSON output to extract version
	// npm list output format: {"dependencies": {"<pkg>": {"version": "x.y.z"}}}
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

// Uninstall removes a plugin binary and its manifest entry.
// For npm-sourced plugins, it also runs `npm uninstall -g`.
func Uninstall(name string) error {
	// Load manifest to check source type
	manifest, err := LoadManifest()
	if err != nil {
		manifest = &Manifest{}
	}

	// If npm source, run npm uninstall -g first
	if entry, ok := manifest.Get(name); ok && entry.Type == SourceTypeNpm {
		pkg := entry.Source
		cmd := osexec.Command("npm", "uninstall", "-g", pkg)
		if output, err := cmd.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: npm uninstall -g %s failed: %v\n%s", pkg, err, string(output))
		}
	}

	// Remove the binary/symlink
	binName := binPrefix + name
	installDir, err := pluginBinDir()
	if err != nil {
		return err
	}
	binPath := filepath.Join(installDir, binName)
	if err := os.Remove(binPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove plugin binary: %w", err)
	}

	manifest.Remove(name)
	return manifest.Save()
}

func pluginBinDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".clime", "plugins"), nil
}

func findAsset(release *githubrelease.Release, name string) (*githubrelease.Asset, error) {
	pattern := fmt.Sprintf("%s_", name)
	return release.FindTarGzAsset(pattern, runtime.GOOS, runtime.GOARCH)
}

// repoBaseName returns the last path segment of an "owner/name" repo string.
func repoBaseName(repo string) string {
	if i := strings.LastIndex(repo, "/"); i >= 0 {
		return repo[i+1:]
	}
	return repo
}

// detectScriptPluginVersion attempts to determine the version of a
// script-installed plugin by running its "version" command. It returns
// "latest" if the command fails or the output cannot be parsed.
func detectScriptPluginVersion(name string) string {
	binPath, ok := Find(name)
	if !ok {
		return VersionLatest
	}

	out, err := osexec.Command(binPath, "version").CombinedOutput()
	if err != nil {
		return VersionLatest
	}

	return parseVersionOutput(string(out))
}

// parseVersionOutput extracts a version string from command output.
// It tries to match semver (N.N.N or vN.N.N), then falls back to a
// single-word token (e.g. "dev"), and returns "latest" otherwise.
func parseVersionOutput(output string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return VersionLatest
	}

	// Try to match semver pattern (N.N.N or vN.N.N) anywhere in the output.
	if m := semverRe.FindStringSubmatch(output); m != nil {
		return m[1]
	}

	// If the output is a single word (e.g. "dev"), use it as the version.
	if !strings.ContainsAny(output, " \t\n") {
		return output
	}

	return VersionLatest
}
