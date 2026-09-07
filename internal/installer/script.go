package installer

import (
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"

	"github.com/git-hulk/clime/internal/plugin"
)

// ScriptInstaller installs plugins via remote install scripts (curl | sh).
type ScriptInstaller struct {
	ScriptURL    string
	BinaryPath   string
	runScript    func(scriptURL string) error
	pluginBinDir func() (string, error)
	findPlugin   func(name string) (string, bool)
	runVersion   func(binPath string) (string, error)
	lookPath     func(name string) (string, error)
}

// NewScriptInstaller returns a ScriptInstaller for the given script URL and binary path.
func NewScriptInstaller(scriptURL, binaryPath string) *ScriptInstaller {
	return &ScriptInstaller{
		ScriptURL:    scriptURL,
		BinaryPath:   binaryPath,
		runScript:    runInstallScript,
		pluginBinDir: plugin.PluginBinDir,
		findPlugin:   plugin.Find,
		runVersion:   runPluginVersionCmd,
		lookPath:     osexec.LookPath,
	}
}

func (installer *ScriptInstaller) Install(name string) (string, error) {
	if err := installer.runScript(installer.ScriptURL); err != nil {
		return "", fmt.Errorf("install script failed: %w", err)
	}

	binaryPath := installer.BinaryPath
	if binaryPath != "" {
		if strings.HasPrefix(binaryPath, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			binaryPath = filepath.Join(home, binaryPath[2:])
		}
		if _, err := os.Stat(binaryPath); err != nil {
			return "", fmt.Errorf("binary not found at %s after install: %w", binaryPath, err)
		}
	} else {
		// Auto-detect binary on PATH by name
		found, err := installer.lookPath(name)
		if err != nil {
			return "", fmt.Errorf("binary %q not found on PATH after install; use --binary-path to specify its location", name)
		}
		binaryPath = found
	}

	installDir, err := installer.pluginBinDir()
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

	version := installer.DetectVersion(name)
	return version, nil
}

func (installer *ScriptInstaller) Update(name string, current plugin.ManifestEntry) (*UpdateResult, error) {
	if err := installer.runScript(installer.ScriptURL); err != nil {
		return nil, fmt.Errorf("failed to update plugin from script source %q: %w", installer.ScriptURL, err)
	}

	version := installer.DetectVersion(name)

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
		Source:         installer.ScriptURL,
		CurrentVersion: current.Version,
		LatestVersion:  version,
		Updated:        updated,
		Path:           filepath.Join(installDir, plugin.BinPrefix+name),
	}, nil
}

func (installer *ScriptInstaller) Uninstall(name string, entry plugin.ManifestEntry) error {
	return removePluginBinaryAndTarget(name)
}

func (installer *ScriptInstaller) DetectVersion(name string) string {
	binPath, ok := installer.findPlugin(name)
	if !ok {
		return plugin.VersionLatest
	}

	output, err := installer.runVersion(binPath)
	if err != nil {
		return plugin.VersionLatest
	}

	return parseVersionOutput(output)
}

func (installer *ScriptInstaller) PluginType() string { return plugin.SourceTypeScript }
func (installer *ScriptInstaller) Source() string     { return installer.ScriptURL }

// script helper functions

func runInstallScript(scriptURL string) error {
	cmd := osexec.Command("bash", "-c", fmt.Sprintf("curl -fsSL '%s' | bash", scriptURL))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("install script failed: %w\n%s", err, string(output))
	}
	return nil
}

func runPluginVersionCmd(binPath string) (string, error) {
	for _, arg := range []string{"-v", "version", "-V"} {
		output, err := osexec.Command(binPath, arg).CombinedOutput()
		if err == nil {
			return string(output), nil
		}
	}
	return "", fmt.Errorf("failed to detect version for %s", binPath)
}
