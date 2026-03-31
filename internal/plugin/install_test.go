package plugin

import "testing"

func TestParseVersionOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output string
		want   string
	}{
		{name: "empty output", output: "", want: VersionLatest},
		{name: "whitespace only", output: "  \n  ", want: VersionLatest},
		{name: "semver", output: "1.2.3", want: "1.2.3"},
		{name: "semver with v prefix", output: "v1.2.3", want: "1.2.3"},
		{name: "semver in sentence", output: "clime-foo version 2.0.1", want: "2.0.1"},
		{name: "semver with v in sentence", output: "my-plugin v0.9.0 (built 2024-01-01)", want: "0.9.0"},
		{name: "single word version", output: "dev", want: "dev"},
		{name: "single word with whitespace", output: "  beta  ", want: "beta"},
		{name: "multi-word no semver", output: "unknown version info", want: VersionLatest},
		{name: "multiple semver picks first", output: "v1.0.0 built with go1.21.0", want: "1.0.0"},
		{name: "semver with trailing newline", output: "3.4.5\n", want: "3.4.5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseVersionOutput(tt.output)
			if got != tt.want {
				t.Errorf("parseVersionOutput(%q) = %q, want %q", tt.output, got, tt.want)
			}
		})
	}
}
