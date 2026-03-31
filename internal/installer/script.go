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
	}
}

func (s *ScriptInstaller) Install(name string) (string, error) {
	if err := s.runScript(s.ScriptURL); err != nil {
		return "", fmt.Errorf("install script failed: %w", err)
	}

	if s.BinaryPath != "" {
		binaryPath := s.BinaryPath
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

		installDir, err := s.pluginBinDir()
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
	}

	version := s.DetectVersion(name)
	return version, nil
}

func (s *ScriptInstaller) Update(name string, current plugin.ManifestEntry) (*UpdateResult, error) {
	if err := s.runScript(s.ScriptURL); err != nil {
		return nil, fmt.Errorf("failed to update plugin from script source %q: %w", s.ScriptURL, err)
	}

	version := s.DetectVersion(name)

	installDir, err := s.pluginBinDir()
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
		Source:         s.ScriptURL,
		CurrentVersion: current.Version,
		LatestVersion:  version,
		Updated:        updated,
		Path:           filepath.Join(installDir, plugin.BinPrefix+name),
	}, nil
}

func (s *ScriptInstaller) Uninstall(name string, entry plugin.ManifestEntry) error {
	return removePluginBinary(name)
}

func (s *ScriptInstaller) DetectVersion(name string) string {
	binPath, ok := s.findPlugin(name)
	if !ok {
		return plugin.VersionLatest
	}

	output, err := s.runVersion(binPath)
	if err != nil {
		return plugin.VersionLatest
	}

	return parseVersionOutput(output)
}

func (s *ScriptInstaller) PluginType() string { return plugin.SourceTypeScript }
func (s *ScriptInstaller) Source() string     { return s.ScriptURL }

// script helper functions

func runInstallScript(scriptURL string) error {
	cmd := osexec.Command("bash", "-c", fmt.Sprintf("curl -fsSL '%s' | bash", scriptURL))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("install script failed: %w\n%s", err, string(output))
	}
	return nil
}

func runPluginVersionCmd(binPath string) (string, error) {
	out, err := osexec.Command(binPath, "version").CombinedOutput()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
