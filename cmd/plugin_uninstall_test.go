package cmd

import "testing"

func TestUniquePluginNames(t *testing.T) {
	t.Parallel()

	got := uniquePluginNames([]string{
		"account",
		"",
		"account",
		"opencli",
		"  opencli  ",
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
