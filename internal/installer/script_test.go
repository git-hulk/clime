package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/git-hulk/clime/internal/plugin"
)

func TestScriptInstallerUpdate(t *testing.T) {
	t.Parallel()

	var ranScript bool
	s := &ScriptInstaller{
		ScriptURL:  "https://example.com/install.sh",
		BinaryPath: "/usr/local/bin/account",
		runScript: func(scriptURL string) error {
			ranScript = true
			if scriptURL != "https://example.com/install.sh" {
				t.Fatalf("scriptURL = %q, want %q", scriptURL, "https://example.com/install.sh")
			}
			return nil
		},
		pluginBinDir: func() (string, error) {
			return "/tmp/clime-plugin-test", nil
		},
		findPlugin: func(name string) (string, bool) {
			return "/usr/local/bin/account", true
		},
		runVersion: func(binPath string) (string, error) {
			return "2.1.0", nil
		},
	}

	entry := plugin.ManifestEntry{
		Name:    "account",
		Version: plugin.VersionLatest,
		Type:    plugin.SourceTypeScript,
		Source:  "https://example.com/install.sh",
	}
	result, err := s.Update("account", entry)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if !ranScript {
		t.Fatal("script install should run for script source")
	}
	if !result.Updated {
		t.Fatal("Update() should mark updated for script source")
	}
	if result.LatestVersion != "2.1.0" {
		t.Fatalf("LatestVersion = %q, want %q", result.LatestVersion, "2.1.0")
	}
}

func TestScriptInstallerUpdateUpToDate(t *testing.T) {
	t.Parallel()

	s := &ScriptInstaller{
		ScriptURL:  "https://example.com/install.sh",
		BinaryPath: "/usr/local/bin/account",
		runScript: func(scriptURL string) error {
			return nil
		},
		pluginBinDir: func() (string, error) {
			return "/tmp/clime-plugin-test", nil
		},
		findPlugin: func(name string) (string, bool) {
			return "/usr/local/bin/account", true
		},
		runVersion: func(binPath string) (string, error) {
			return "2.1.0", nil
		},
	}

	entry := plugin.ManifestEntry{
		Name:    "account",
		Version: "2.1.0",
		Type:    plugin.SourceTypeScript,
		Source:  "https://example.com/install.sh",
	}
	result, err := s.Update("account", entry)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if result.Updated {
		t.Fatal("Update() should not mark updated when semver version is unchanged")
	}
}

func TestScriptInstallerUpdateFallsBackToLatest(t *testing.T) {
	t.Parallel()

	s := &ScriptInstaller{
		ScriptURL: "https://example.com/install.sh",
		runScript: func(scriptURL string) error {
			return nil
		},
		pluginBinDir: func() (string, error) {
			return "/tmp/clime-plugin-test", nil
		},
		findPlugin: func(name string) (string, bool) {
			return "", false
		},
		runVersion: func(binPath string) (string, error) {
			return "", nil
		},
	}

	entry := plugin.ManifestEntry{
		Name:    "tool",
		Version: "1.0.0",
		Type:    plugin.SourceTypeScript,
		Source:  "https://example.com/install.sh",
	}
	result, err := s.Update("tool", entry)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if result.LatestVersion != plugin.VersionLatest {
		t.Fatalf("LatestVersion = %q, want %q", result.LatestVersion, plugin.VersionLatest)
	}
}

func TestScriptInstallerInstallAutoDetectBinary(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	s := &ScriptInstaller{
		ScriptURL: "https://bun.sh/install",
		runScript: func(scriptURL string) error {
			return nil
		},
		pluginBinDir: func() (string, error) {
			return tmpDir, nil
		},
		findPlugin: func(name string) (string, bool) {
			return "/usr/local/bin/bun", true
		},
		runVersion: func(binPath string) (string, error) {
			return "1.2.0", nil
		},
		lookPath: func(name string) (string, error) {
			if name != "bun" {
				t.Fatalf("lookPath name = %q, want %q", name, "bun")
			}
			return "/usr/local/bin/bun", nil
		},
	}

	version, err := s.Install("bun")
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if version != "1.2.0" {
		t.Fatalf("version = %q, want %q", version, "1.2.0")
	}

	// Verify symlink was created
	linkPath := filepath.Join(tmpDir, "clime-bun")
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("symlink not created: %v", err)
	}
	if target != "/usr/local/bin/bun" {
		t.Fatalf("symlink target = %q, want %q", target, "/usr/local/bin/bun")
	}
}

func TestScriptInstallerInstallAutoDetectNotFound(t *testing.T) {
	t.Parallel()

	s := &ScriptInstaller{
		ScriptURL: "https://example.com/install.sh",
		runScript: func(scriptURL string) error {
			return nil
		},
		lookPath: func(name string) (string, error) {
			return "", fmt.Errorf("not found")
		},
	}

	_, err := s.Install("missing")
	if err == nil {
		t.Fatal("Install() should fail when binary not found on PATH")
	}
	if !strings.Contains(err.Error(), "not found on PATH") {
		t.Fatalf("error = %q, want message about PATH", err.Error())
	}
}

func TestRunPluginVersionCmdFallback(t *testing.T) {
	t.Parallel()

	// Create a script that only responds to -V
	tmpDir := t.TempDir()
	script := filepath.Join(tmpDir, "fakecli")
	if err := os.WriteFile(script, []byte(`#!/bin/sh
case "$1" in
  -V) echo "3.2.1" ;;
  *)  exit 1 ;;
esac
`), 0755); err != nil {
		t.Fatal(err)
	}

	out, err := runPluginVersionCmd(script)
	if err != nil {
		t.Fatalf("runPluginVersionCmd() error = %v", err)
	}
	if !strings.Contains(out, "3.2.1") {
		t.Fatalf("output = %q, want to contain %q", out, "3.2.1")
	}
}

func TestRunPluginVersionCmdFirstMatch(t *testing.T) {
	t.Parallel()

	// Create a script that responds to -v (first in the list)
	tmpDir := t.TempDir()
	script := filepath.Join(tmpDir, "fakecli")
	if err := os.WriteFile(script, []byte(`#!/bin/sh
case "$1" in
  -v) echo "1.0.0" ;;
  version) echo "1.0.0-full" ;;
  -V) echo "1.0.0-caps" ;;
  *)  exit 1 ;;
esac
`), 0755); err != nil {
		t.Fatal(err)
	}

	out, err := runPluginVersionCmd(script)
	if err != nil {
		t.Fatalf("runPluginVersionCmd() error = %v", err)
	}
	if !strings.Contains(out, "1.0.0\n") {
		t.Fatalf("output = %q, want first match (-v)", out)
	}
}

func TestRunPluginVersionCmdAllFail(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	script := filepath.Join(tmpDir, "fakecli")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}

	_, err := runPluginVersionCmd(script)
	if err == nil {
		t.Fatal("runPluginVersionCmd() should fail when all commands fail")
	}
}

func TestScriptInstallerUninstallRemovesSymlinkTarget(t *testing.T) {
	t.Parallel()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}
	pluginsDir := filepath.Join(home, ".clime", "plugins")
	if err := os.MkdirAll(pluginsDir, 0755); err != nil {
		t.Fatalf("create plugins dir: %v", err)
	}

	targetBin := filepath.Join(t.TempDir(), "account")
	if err := os.WriteFile(targetBin, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("write target binary: %v", err)
	}

	name := fmt.Sprintf("script-rmtest-%d", time.Now().UnixNano())
	linkPath := filepath.Join(pluginsDir, "clime-"+name)
	if err := os.Symlink(targetBin, linkPath); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(linkPath)
		_ = os.Remove(targetBin)
	})

	s := NewScriptInstaller("https://example.com/install.sh", "")
	entry := plugin.ManifestEntry{
		Name:   name,
		Type:   plugin.SourceTypeScript,
		Source: "https://example.com/install.sh",
	}
	if err := s.Uninstall(name, entry); err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}

	if _, err := os.Lstat(linkPath); !os.IsNotExist(err) {
		t.Fatal("plugin symlink should have been removed")
	}
	if _, err := os.Stat(targetBin); !os.IsNotExist(err) {
		t.Fatal("resolved target binary should have been removed")
	}
}

func TestScriptInstallerUninstallRemovesRelativeSymlinkTarget(t *testing.T) {
	t.Parallel()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}
	pluginsDir := filepath.Join(home, ".clime", "plugins")
	if err := os.MkdirAll(pluginsDir, 0755); err != nil {
		t.Fatalf("create plugins dir: %v", err)
	}

	targetDir := filepath.Join(pluginsDir, fmt.Sprintf(".script-target-%d", time.Now().UnixNano()))
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatalf("create target dir: %v", err)
	}
	targetBin := filepath.Join(targetDir, "tool")
	if err := os.WriteFile(targetBin, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("write target binary: %v", err)
	}

	name := fmt.Sprintf("script-rmtest-rel-%d", time.Now().UnixNano())
	linkPath := filepath.Join(pluginsDir, "clime-"+name)
	relTarget, err := filepath.Rel(pluginsDir, targetBin)
	if err != nil {
		t.Fatalf("compute relative target path: %v", err)
	}
	if err := os.Symlink(relTarget, linkPath); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(linkPath)
		_ = os.RemoveAll(targetDir)
	})

	s := NewScriptInstaller("https://example.com/install.sh", "")
	entry := plugin.ManifestEntry{
		Name:   name,
		Type:   plugin.SourceTypeScript,
		Source: "https://example.com/install.sh",
	}
	if err := s.Uninstall(name, entry); err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}

	if _, err := os.Lstat(linkPath); !os.IsNotExist(err) {
		t.Fatal("plugin symlink should have been removed")
	}
	if _, err := os.Stat(targetBin); !os.IsNotExist(err) {
		t.Fatal("resolved relative target binary should have been removed")
	}
}

func TestScriptInstallerPluginType(t *testing.T) {
	t.Parallel()
	s := NewScriptInstaller("https://example.com/install.sh", "/usr/local/bin/foo")
	if s.PluginType() != plugin.SourceTypeScript {
		t.Fatalf("PluginType() = %q, want %q", s.PluginType(), plugin.SourceTypeScript)
	}
	if s.Source() != "https://example.com/install.sh" {
		t.Fatalf("Source() = %q, want %q", s.Source(), "https://example.com/install.sh")
	}
}
