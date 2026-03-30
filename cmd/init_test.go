package cmd

import (
	"testing"
)

func TestFilterPluginsByTags(t *testing.T) {
	plugins := []defaultPlugin{
		{Name: "core"},                                     // untagged
		{Name: "devops-tool", Tags: []string{"devops"}},    // single tag
		{Name: "email", Tags: []string{"devops", "email"}}, // multi tag
		{Name: "infra", Tags: []string{"infra"}},           // single tag
	}

	tests := []struct {
		name     string
		tags     []string
		wantName []string
	}{
		{
			name:     "nil tags installs only untagged",
			tags:     nil,
			wantName: []string{"core"},
		},
		{
			name:     "empty tags installs only untagged",
			tags:     []string{},
			wantName: []string{"core"},
		},
		{
			name:     "single matching tag",
			tags:     []string{"devops"},
			wantName: []string{"core", "devops-tool", "email"},
		},
		{
			name:     "multiple tags OR matching",
			tags:     []string{"devops", "infra"},
			wantName: []string{"core", "devops-tool", "email", "infra"},
		},
		{
			name:     "no matching tag returns only untagged",
			tags:     []string{"nonexistent"},
			wantName: []string{"core"},
		},
		{
			name:     "whitespace in tags is trimmed",
			tags:     []string{" devops "},
			wantName: []string{"core", "devops-tool", "email"},
		},
		{
			name:     "empty string tag is ignored",
			tags:     []string{""},
			wantName: []string{"core"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterPluginsByTags(plugins, tt.tags)
			if len(got) != len(tt.wantName) {
				t.Fatalf("got %d plugins, want %d", len(got), len(tt.wantName))
			}
			for i, p := range got {
				if p.Name != tt.wantName[i] {
					t.Errorf("plugin[%d] = %q, want %q", i, p.Name, tt.wantName[i])
				}
			}
		})
	}
}
