package plugin

import (
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"

	"github.com/git-hulk/clime/internal/githubrelease"
)

// UpdateOptions configures a plugin update run.
type UpdateOptions struct {
	Name  string
	Repo  string
	Force bool
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

// Updater updates plugin binaries from their configured sources.
type Updater struct {
	fetchLatest         func(repo string) (*githubrelease.Release, error)
	downloadBinary      func(url, binaryName string) ([]byte, error)
	pluginBinDir        func() (string, error)
	loadManifest        func() (*Manifest, error)
	saveManifest        func(*Manifest) error
	writeBinary         func(destPath string, binaryContent []byte) error
	runScript           func(scriptURL string) error
	runNpmUpdate        func(pkg string) error
	detectScriptVersion func(name string) string
}

func NewUpdater() *Updater {
	return &Updater{
		fetchLatest:    githubrelease.FetchLatest,
		downloadBinary: githubrelease.DownloadTarGzBinary,
		pluginBinDir:   pluginBinDir,
		loadManifest:   LoadManifest,
		saveManifest: func(m *Manifest) error {
			return m.Save()
		},
		writeBinary:         writePluginBinary,
		runScript:           runInstallScript,
		runNpmUpdate:        runNpmGlobalUpdate,
		detectScriptVersion: detectScriptPluginVersion,
	}
}

// Update updates a managed plugin using the default updater.
func Update(opts UpdateOptions) (*UpdateResult, error) {
	return NewUpdater().Update(opts)
}

// Update updates a managed plugin from its configured source.
func (u *Updater) Update(opts UpdateOptions) (*UpdateResult, error) {
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		return nil, fmt.Errorf("plugin name is required")
	}

	manifest, err := u.loadManifest()
	if err != nil || manifest == nil {
		manifest = &Manifest{}
	}
	entry, _ := manifest.Get(name)

	// Determine source type and source value from manifest or flags.
	sourceType := entry.Type
	source := entry.Source
	if repo := strings.TrimSpace(opts.Repo); repo != "" {
		sourceType = SourceTypeGitHub
		source = repo
	}
	if sourceType == "" {
		sourceType = SourceTypeGitHub
	}
	if source == "" && sourceType == SourceTypeGitHub {
		return nil, fmt.Errorf("no repo configured for plugin %q; use --repo to specify one", name)
	}
	if sourceType == SourceTypeNpm {
		return u.updateFromNpm(manifest, name, entry, source)
	}
	if sourceType == SourceTypeScript {
		return u.updateFromScript(manifest, name, entry, source)
	}

	repo := source

	release, err := u.fetchLatest(repo)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch latest release for %s: %w", repo, err)
	}

	latest := release.Version()
	result := &UpdateResult{
		Name:           name,
		Source:         repo,
		CurrentVersion: entry.Version,
		LatestVersion:  latest,
	}

	if !opts.Force && entry.Version != "" && normalizeVersion(entry.Version) == normalizeVersion(latest) {
		return result, nil
	}

	repoName := repoBaseName(repo)
	asset, err := findAsset(release, repoName)
	if err != nil {
		return nil, err
	}

	binaryContent, err := u.downloadBinary(asset.BrowserDownloadURL, repoName)
	if err != nil {
		return nil, fmt.Errorf("failed to update plugin: %w", err)
	}

	installDir, err := u.pluginBinDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create plugin directory: %w", err)
	}

	destPath := filepath.Join(installDir, binPrefix+name)
	if err := u.writeBinary(destPath, binaryContent); err != nil {
		return nil, fmt.Errorf("failed to update plugin: %w", err)
	}

	manifest.Add(name, latest, SourceTypeGitHub, repo, "")
	if err := u.saveManifest(manifest); err != nil {
		return nil, fmt.Errorf("plugin updated but failed to update manifest: %w", err)
	}

	result.Path = destPath
	result.Updated = true
	return result, nil
}

func writePluginBinary(destPath string, binaryContent []byte) error {
	return os.WriteFile(destPath, binaryContent, 0755)
}

func runInstallScript(scriptURL string) error {
	cmd := osexec.Command("bash", "-c", fmt.Sprintf("curl -fsSL '%s' | bash", scriptURL))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("install script failed: %w\n%s", err, string(output))
	}
	return nil
}

func (u *Updater) updateFromScript(manifest *Manifest, name string, entry ManifestEntry, scriptURL string) (*UpdateResult, error) {
	runScript := u.runScript
	if runScript == nil {
		runScript = runInstallScript
	}
	if err := runScript(scriptURL); err != nil {
		return nil, fmt.Errorf("failed to update plugin from script source %q: %w", scriptURL, err)
	}

	// Detect version from the installed binary's "version" command
	detectVersion := u.detectScriptVersion
	if detectVersion == nil {
		detectVersion = detectScriptPluginVersion
	}
	version := detectVersion(name)

	manifest.Add(name, version, SourceTypeScript, scriptURL, entry.BinaryPath)
	if err := u.saveManifest(manifest); err != nil {
		return nil, fmt.Errorf("plugin updated but failed to update manifest: %w", err)
	}

	result := &UpdateResult{
		Name:           name,
		Source:         scriptURL,
		CurrentVersion: entry.Version,
		LatestVersion:  version,
		Updated:        true,
	}

	installDir, err := u.pluginBinDir()
	if err != nil {
		return nil, err
	}
	result.Path = filepath.Join(installDir, binPrefix+name)
	return result, nil
}

func runNpmGlobalUpdate(pkg string) error {
	cmd := osexec.Command("npm", "update", "-g", pkg)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("npm update failed: %w\n%s", err, string(output))
	}
	return nil
}

func (u *Updater) updateFromNpm(manifest *Manifest, name string, entry ManifestEntry, source string) (*UpdateResult, error) {
	pkg := source
	runNpm := u.runNpmUpdate
	if runNpm == nil {
		runNpm = runNpmGlobalUpdate
	}
	if err := runNpm(pkg); err != nil {
		return nil, fmt.Errorf("failed to update npm plugin %q: %w", pkg, err)
	}

	// Get actual installed version
	version, err := getNpmInstalledVersion(pkg)
	if err != nil {
		version = VersionLatest
	}

	manifest.Add(name, version, SourceTypeNpm, pkg, "")
	if err := u.saveManifest(manifest); err != nil {
		return nil, fmt.Errorf("plugin updated but failed to update manifest: %w", err)
	}

	installDir, err := u.pluginBinDir()
	if err != nil {
		return nil, err
	}

	return &UpdateResult{
		Name:           name,
		Source:         pkg,
		CurrentVersion: entry.Version,
		LatestVersion:  version,
		Updated:        true,
		Path:           filepath.Join(installDir, binPrefix+name),
	}, nil
}

func normalizeVersion(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}
