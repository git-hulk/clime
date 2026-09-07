package selfupdate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/git-hulk/clime/internal/githubrelease"
	"github.com/stretchr/testify/require"
)

func TestUpdateSkipsWhenAlreadyLatest(t *testing.T) {
	t.Parallel()

	u := &Updater{
		fetchLatest: func(repo string) (*githubrelease.Release, error) {
			require.Equal(t, "git-hulk/clime", repo)
			return &githubrelease.Release{TagName: "v1.2.3"}, nil
		},
		downloadBinary: func(url, binaryName string) ([]byte, error) {
			require.FailNow(t, "downloadBinary should not be called when already up-to-date")
			return nil, nil
		},
		resolveExecutablePath: func() (string, error) {
			require.FailNow(t, "resolveExecutablePath should not be called when already up-to-date")
			return "", nil
		},
		replaceExecutable: func(destPath, binaryName string, binaryContent []byte) error {
			require.FailNow(t, "replaceExecutable should not be called when already up-to-date")
			return nil
		},
	}

	result, err := u.Update(Options{
		Repo:           "git-hulk/clime",
		CurrentVersion: "1.2.3",
	})
	require.NoError(t, err)
	require.False(t, result.Updated, "Update() should not mark updated when versions match")
	require.Equal(t, "1.2.3", result.LatestVersion)
}

func TestUpdateReplacesExecutable(t *testing.T) {
	t.Parallel()
	for _, fail := range []bool{false, true} {
		name := "success"
		if fail {
			name = "replacement fails"
		}
		t.Run(name, func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), "clime")
			if fail {
				// A directory cannot be replaced by the downloaded executable.
				require.NoError(t, os.Mkdir(dest, 0o755))
			} else {
				require.NoError(t, os.WriteFile(dest, []byte("old-binary"), 0o755))
			}
			u := New()
			u.fetchLatest = func(string) (*githubrelease.Release, error) {
				return &githubrelease.Release{TagName: "v1.2.4", Assets: []githubrelease.Asset{{
					Name:               "clime_1.2.4_testos_testarch.tar.gz",
					BrowserDownloadURL: "https://example.com/clime.tar.gz",
				}}}, nil
			}
			u.downloadBinary = func(url, binaryName string) ([]byte, error) {
				require.Equal(t, "https://example.com/clime.tar.gz", url)
				require.Equal(t, "clime", binaryName)
				return []byte("binary-content"), nil
			}
			u.resolveExecutablePath = func() (string, error) { return dest, nil }
			result, err := u.Update(Options{
				Repo: "git-hulk/clime", CurrentVersion: "1.2.3",
				TargetOS: "testos", TargetArch: "testarch",
			})
			if fail {
				require.ErrorContains(t, err, "replace executable")
				require.DirExists(t, dest)
			} else {
				require.NoError(t, err)
				require.True(t, result.Updated)
				require.Equal(t, dest, result.Path)
				content, err := os.ReadFile(dest)
				require.NoError(t, err)
				require.Equal(t, "binary-content", string(content))
				info, err := os.Stat(dest)
				require.NoError(t, err)
				require.Equal(t, os.FileMode(0o755), info.Mode().Perm())
			}
			files, err := os.ReadDir(filepath.Dir(dest))
			require.NoError(t, err)
			require.Len(t, files, 1, "temporary downloads must be cleaned up")
		})
	}
}

func TestUpdateValidation(t *testing.T) {
	t.Parallel()

	u := New()

	_, err := u.Update(Options{})
	require.Error(t, err, "Update() should fail when repo is empty")
}
