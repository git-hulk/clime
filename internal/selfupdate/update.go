package selfupdate

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/git-hulk/clime/internal/githubrelease"
)

const DefaultBinaryName = "clime"

// Options configures a self-update run.
type Options struct {
	Repo           string
	CurrentVersion string
	Force          bool

	// Optional fields for advanced use/testing.
	BinaryName     string
	TargetOS       string
	TargetArch     string
	ExecutablePath string
}

// Result reports the outcome of a self-update operation.
type Result struct {
	CurrentVersion string
	LatestVersion  string
	Updated        bool
	Path           string
}

type Updater struct {
	fetchLatest           func(repo string) (*githubrelease.Release, error)
	downloadBinary        func(url, binaryName string) ([]byte, error)
	resolveExecutablePath func() (string, error)
	replaceExecutable     func(destPath, binaryName string, binaryContent []byte) error
}

func New() *Updater {
	return &Updater{
		fetchLatest:           githubrelease.FetchLatest,
		downloadBinary:        githubrelease.DownloadTarGzBinary,
		resolveExecutablePath: executablePath,
		replaceExecutable:     replaceExecutable,
	}
}

// Update runs self-update using the default updater.
func Update(opts Options) (*Result, error) {
	return New().Update(opts)
}

// Update updates the target executable to the latest release.
func (u *Updater) Update(opts Options) (*Result, error) {
	opts = opts.withDefaults()
	if err := opts.validate(); err != nil {
		return nil, err
	}

	release, err := u.fetchLatest(opts.Repo)
	if err != nil {
		return nil, fmt.Errorf("fetch latest release: %w", err)
	}

	latest := release.Version()
	result := &Result{
		CurrentVersion: opts.CurrentVersion,
		LatestVersion:  latest,
	}

	if !opts.Force && opts.CurrentVersion != "dev" && normalizeVersion(opts.CurrentVersion) == normalizeVersion(latest) {
		return result, nil
	}

	asset, err := release.FindTarGzAsset(opts.BinaryName+"_", opts.TargetOS, opts.TargetArch)
	if err != nil {
		return nil, err
	}

	execPath := opts.ExecutablePath
	if execPath == "" {
		var resolveErr error
		execPath, resolveErr = u.resolveExecutablePath()
		if resolveErr != nil {
			return nil, fmt.Errorf("resolve executable path: %w", resolveErr)
		}
	}

	binaryContent, err := u.downloadBinary(asset.BrowserDownloadURL, opts.BinaryName)
	if err != nil {
		return nil, err
	}

	if err := u.replaceExecutable(execPath, opts.BinaryName, binaryContent); err != nil {
		return nil, err
	}

	result.Path = execPath
	result.Updated = true
	return result, nil
}

func normalizeVersion(v string) string {
	return strings.TrimPrefix(v, "v")
}

func (o Options) withDefaults() Options {
	if o.BinaryName == "" {
		o.BinaryName = DefaultBinaryName
	}
	if o.TargetOS == "" {
		o.TargetOS = runtime.GOOS
	}
	if o.TargetArch == "" {
		o.TargetArch = runtime.GOARCH
	}
	return o
}

func (o Options) validate() error {
	if o.Repo == "" {
		return fmt.Errorf("repo is required")
	}
	if o.BinaryName == "" {
		return fmt.Errorf("binary name is required")
	}
	if o.TargetOS == "" || o.TargetArch == "" {
		return fmt.Errorf("target OS/arch are required")
	}
	return nil
}

func executablePath() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", err
	}
	return execPath, nil
}

func replaceExecutable(destPath, binaryName string, binaryContent []byte) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("prepare destination directory: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(destPath), binaryName+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		tmp.Close()
		_ = os.Remove(tmpPath)
	}

	if _, err := tmp.Write(binaryContent); err != nil {
		cleanup()
		return fmt.Errorf("write binary: %w", err)
	}

	if err := tmp.Chmod(0755); err != nil {
		cleanup()
		return fmt.Errorf("set binary permissions: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace executable at %s: %w", destPath, err)
	}
	return nil
}
