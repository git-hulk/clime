package cmd

import (
	"strings"
	"testing"
)

func TestUniquePluginNames(t *testing.T) {
	t.Parallel()

	got := uniquePluginNames([]string{
		"account",
		"account",
		"opencli",
		"opencli",
		"cmdb",
	})

	want := []string{"account", "opencli", "cmdb"}
	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestPluginUninstallArgsAllowsMultiple(t *testing.T) {
	t.Parallel()

	if err := pluginUninstallCmd.Args(pluginUninstallCmd, []string{"foo"}); err != nil {
		t.Fatalf("single arg should be allowed: %v", err)
	}
	if err := pluginUninstallCmd.Args(pluginUninstallCmd, []string{"foo", "bar"}); err != nil {
		t.Fatalf("multiple args should be allowed: %v", err)
	}
	if err := pluginUninstallCmd.Args(pluginUninstallCmd, nil); err == nil {
		t.Fatal("zero args should fail")
	}
}

func TestPluginUninstallWarnsWhenPluginDoesNotExist(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	output := captureStdout(t, func() {
		if err := pluginUninstallCmd.RunE(pluginUninstallCmd, []string{"missing-plugin"}); err != nil {
			t.Fatalf("pluginUninstallCmd.RunE() error = %v", err)
		}
	})

	if !strings.Contains(output, `Plugin "missing-plugin" is not installed; skipping.`) {
		t.Fatalf("stdout = %q, want missing-plugin warning", output)
	}
	if strings.Contains(output, `Removed plugin "missing-plugin"`) {
		t.Fatalf("stdout = %q, should not report plugin removal", output)
	}
}
