package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeShell(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
	}{
		{in: "/bin/bash", want: "bash"},
		{in: "/bin/zsh", want: "zsh"},
		{in: "/usr/bin/fish", want: "fish"},
		{in: "pwsh", want: "powershell"},
		{in: "powershell", want: "powershell"},
		{in: "unknown", want: ""},
	}

	for _, tt := range tests {
		require.Equal(t, tt.want, normalizeShell(tt.in))
	}
}

func TestDetectShellFromEnv(t *testing.T) {
	t.Parallel()

	got, err := detectShellFromEnv("/bin/zsh", false)
	require.NoError(t, err)
	require.Equal(t, "zsh", got)

	got, err = detectShellFromEnv("", true)
	require.NoError(t, err)
	require.Equal(t, "powershell", got)

	_, err = detectShellFromEnv("unknown", false)
	require.Error(t, err, "expected error for unknown shell")
}

func TestEnsureLineInFileIdempotent(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, ".bashrc")
	marker := "# clime completion"
	line := "[ -f '/tmp/clime' ] && source '/tmp/clime'"

	changed, err := ensureLineInFile(path, marker, line)
	require.NoError(t, err)
	require.True(t, changed, "first ensureLineInFile() should report changed")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(data)
	require.Contains(t, content, marker)
	require.Contains(t, content, line)

	changed, err = ensureLineInFile(path, marker, line)
	require.NoError(t, err)
	require.False(t, changed, "second ensureLineInFile() should be idempotent")
}
