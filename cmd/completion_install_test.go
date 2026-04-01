package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		if got := normalizeShell(tt.in); got != tt.want {
			t.Fatalf("normalizeShell(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestDetectShellFromEnv(t *testing.T) {
	t.Parallel()

	got, err := detectShellFromEnv("/bin/zsh", false)
	if err != nil {
		t.Fatalf("detectShellFromEnv() error = %v", err)
	}
	if got != "zsh" {
		t.Fatalf("shell = %q, want %q", got, "zsh")
	}

	got, err = detectShellFromEnv("", true)
	if err != nil {
		t.Fatalf("windows detect should succeed: %v", err)
	}
	if got != "powershell" {
		t.Fatalf("shell = %q, want %q", got, "powershell")
	}

	if _, err := detectShellFromEnv("unknown", false); err == nil {
		t.Fatal("expected error for unknown shell")
	}
}

func TestEnsureLineInFileIdempotent(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, ".bashrc")
	marker := "# clime completion"
	line := "[ -f '/tmp/clime' ] && source '/tmp/clime'"

	changed, err := ensureLineInFile(path, marker, line)
	if err != nil {
		t.Fatalf("first ensureLineInFile() error = %v", err)
	}
	if !changed {
		t.Fatal("first ensureLineInFile() should report changed")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read profile: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, marker) || !strings.Contains(content, line) {
		t.Fatalf("profile content missing marker/line: %q", content)
	}

	changed, err = ensureLineInFile(path, marker, line)
	if err != nil {
		t.Fatalf("second ensureLineInFile() error = %v", err)
	}
	if changed {
		t.Fatal("second ensureLineInFile() should be idempotent")
	}
}
