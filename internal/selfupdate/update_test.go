package selfupdate

import (
	"errors"
	"testing"

	"github.com/git-hulk/clime/internal/githubrelease"
)

func TestUpdateSkipsWhenAlreadyLatest(t *testing.T) {
	t.Parallel()

	u := &Updater{
		fetchLatest: func(repo string) (*githubrelease.Release, error) {
			if repo != "git-hulk/clime" {
				t.Fatalf("unexpected repo: %s", repo)
			}
			return &githubrelease.Release{TagName: "v1.2.3"}, nil
		},
		downloadBinary: func(url, binaryName string) ([]byte, error) {
			t.Fatal("downloadBinary should not be called when already up-to-date")
			return nil, nil
		},
		resolveExecutablePath: func() (string, error) {
			t.Fatal("resolveExecutablePath should not be called when already up-to-date")
			return "", nil
		},
		replaceExecutable: func(destPath, binaryName string, binaryContent []byte) error {
			t.Fatal("replaceExecutable should not be called when already up-to-date")
			return nil
		},
	}

	result, err := u.Update(Options{
		Repo:           "git-hulk/clime",
		CurrentVersion: "1.2.3",
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if result.Updated {
		t.Fatal("Update() should not mark updated when versions match")
	}
	if result.LatestVersion != "1.2.3" {
		t.Fatalf("LatestVersion = %q, want %q", result.LatestVersion, "1.2.3")
	}
}

func TestUpdateAppliesBinary(t *testing.T) {
	t.Parallel()

	var replacedPath, replacedBinary string
	var replacedContent []byte

	u := &Updater{
		fetchLatest: func(repo string) (*githubrelease.Release, error) {
			return &githubrelease.Release{
				TagName: "v1.2.4",
				Assets: []githubrelease.Asset{
					{
						Name:               "clime_1.2.4_testos_testarch.tar.gz",
						BrowserDownloadURL: "https://example.com/clime.tar.gz",
					},
				},
			}, nil
		},
		downloadBinary: func(url, binaryName string) ([]byte, error) {
			if url != "https://example.com/clime.tar.gz" {
				t.Fatalf("unexpected download url: %s", url)
			}
			if binaryName != "clime" {
				t.Fatalf("unexpected binaryName: %s", binaryName)
			}
			return []byte("binary-content"), nil
		},
		resolveExecutablePath: func() (string, error) {
			return "/tmp/clime", nil
		},
		replaceExecutable: func(destPath, binaryName string, binaryContent []byte) error {
			replacedPath = destPath
			replacedBinary = binaryName
			replacedContent = binaryContent
			return nil
		},
	}

	result, err := u.Update(Options{
		Repo:           "git-hulk/clime",
		CurrentVersion: "1.2.3",
		TargetOS:       "testos",
		TargetArch:     "testarch",
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if !result.Updated {
		t.Fatal("Update() should mark updated")
	}
	if replacedPath != "/tmp/clime" {
		t.Fatalf("replace path = %q", replacedPath)
	}
	if replacedBinary != "clime" {
		t.Fatalf("replace binary = %q", replacedBinary)
	}
	if string(replacedContent) != "binary-content" {
		t.Fatalf("replace content = %q", string(replacedContent))
	}
}

func TestUpdateValidation(t *testing.T) {
	t.Parallel()

	u := New()
	if _, err := u.Update(Options{}); err == nil {
		t.Fatal("Update() should fail when repo is empty")
	}
}

func TestUpdatePropagatesReplaceError(t *testing.T) {
	t.Parallel()

	u := &Updater{
		fetchLatest: func(repo string) (*githubrelease.Release, error) {
			return &githubrelease.Release{
				TagName: "v1.2.4",
				Assets: []githubrelease.Asset{
					{
						Name:               "clime_1.2.4_testos_testarch.tar.gz",
						BrowserDownloadURL: "https://example.com/clime.tar.gz",
					},
				},
			}, nil
		},
		downloadBinary: func(url, binaryName string) ([]byte, error) {
			return []byte("binary-content"), nil
		},
		resolveExecutablePath: func() (string, error) {
			return "/tmp/clime", nil
		},
		replaceExecutable: func(destPath, binaryName string, binaryContent []byte) error {
			return errors.New("permission denied")
		},
	}

	if _, err := u.Update(Options{
		Repo:           "git-hulk/clime",
		CurrentVersion: "1.2.3",
		TargetOS:       "testos",
		TargetArch:     "testarch",
	}); err == nil {
		t.Fatal("Update() should propagate replace errors")
	}
}
