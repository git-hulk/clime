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
	Repo           string
	CurrentVersion string
	LatestVersion  string
	Updated        bool
	Path           string
}

// Updater updates plugin binaries from their configured sources.
type Updater struct {
	fetchLatest    func(repo string) (*githubrelease.Release, error)
	downloadBinary func(url, binaryName string) ([]byte, error)
	pluginBinDir   func() (string, error)
	loadManifest   func() (*Manifest, error)
	saveManifest   func(*Manifest) error
	writeBinary    func(destPath string, binaryContent []byte) error
	runScript      func(scriptURL string) error
	runNpmUpdate   func(pkg string) error
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
		writeBinary: writePluginBinary,
		runScript:      runInstallScript,
		runNpmUpdate:   runNpmGlobalUpdate,
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
	entry, hasEntry := manifest.Get(name)

	repo := strings.TrimSpace(opts.Repo)
	if repo == "" {
		if hasEntry && strings.TrimSpace(entry.Repo) != "" {
			repo = strings.TrimSpace(entry.Repo)
		} else {
			repo = defaultPluginRepo(name)
		}
	}
	if isNpmSource(repo) {
		return u.updateFromNpm(manifest, name, entry, repo)
	}
	if isScriptSource(repo) {
		return u.updateFromScript(manifest, name, entry, repo)
	}

	release, err := u.fetchLatest(repo)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch latest release for %s: %w", repo, err)
	}

	latest := release.Version()
	result := &UpdateResult{
		Name:           name,
		Repo:           repo,
		CurrentVersion: entry.Version,
		LatestVersion:  latest,
	}

	if !opts.Force && entry.Version != "" && normalizeVersion(entry.Version) == normalizeVersion(latest) {
		return result, nil
	}

	asset, err := findAsset(release, name)
	if err != nil {
		return nil, err
	}

	binName := binPrefix + name
	binaryContent, err := u.downloadBinary(asset.BrowserDownloadURL, binName)
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

	destPath := filepath.Join(installDir, binName)
	if err := u.writeBinary(destPath, binaryContent); err != nil {
		return nil, fmt.Errorf("failed to update plugin: %w", err)
	}

	manifest.Add(name, latest, repo, "")
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

	manifest.Add(name, "latest", scriptURL, entry.BinaryPath)
	if err := u.saveManifest(manifest); err != nil {
		return nil, fmt.Errorf("plugin updated but failed to update manifest: %w", err)
	}

	result := &UpdateResult{
		Name:           name,
		Repo:           scriptURL,
		CurrentVersion: entry.Version,
		LatestVersion:  "latest",
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
	pkg := npmPackageName(source)
	runNpm := u.runNpmUpdate
	if runNpm == nil {
		runNpm = runNpmGlobalUpdate
	}
	if err := runNpm(pkg); err != nil {
		return nil, fmt.Errorf("failed to update npm plugin %q: %w", pkg, err)
	}

	manifest.Add(name, "latest", source, "")
	if err := u.saveManifest(manifest); err != nil {
		return nil, fmt.Errorf("plugin updated but failed to update manifest: %w", err)
	}

	installDir, err := u.pluginBinDir()
	if err != nil {
		return nil, err
	}

	return &UpdateResult{
		Name:           name,
		Repo:           source,
		CurrentVersion: entry.Version,
		LatestVersion:  "latest",
		Updated:        true,
		Path:           filepath.Join(installDir, binPrefix+name),
	}, nil
}

func defaultPluginRepo(name string) string {
	return fmt.Sprintf("%s/clime-%s", defaultOwner, name)
}

func normalizeVersion(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

func isScriptSource(src string) bool {
	return strings.HasPrefix(src, "https://") || strings.HasPrefix(src, "http://")
}

const npmSourcePrefix = "npm:"

func isNpmSource(src string) bool {
	return strings.HasPrefix(src, npmSourcePrefix)
}

func npmPackageName(src string) string {
	return strings.TrimPrefix(src, npmSourcePrefix)
}
