package cmd

import (
	"testing"

	"github.com/git-hulk/clime/internal/skill"
	"github.com/stretchr/testify/require"
)

func TestSelectInstallCandidates(t *testing.T) {
	t.Parallel()
	manifest := &skill.Manifest{Skills: []skill.InstalledSkill{{Name: "beta"}}}
	entries := []skill.Entry{
		{Name: "zebra", Path: "first"},
		{Name: "beta", Path: "second"},
		{Name: "alpha", Path: "third"},
	}
	for _, tt := range []struct {
		name    string
		entries []skill.Entry
		force   bool
		want    []installCandidate
	}{
		{"skip installed and sort", entries, false, []installCandidate{
			{entry: entries[2], label: "alpha"}, {entry: entries[0], label: "zebra"},
		}},
		{"force includes and labels reinstall", entries, true, []installCandidate{
			{entry: entries[2], label: "alpha"}, {entry: entries[1], label: "beta (reinstall)"}, {entry: entries[0], label: "zebra"},
		}},
		{"empty source", nil, true, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, selectInstallCandidates(tt.entries, manifest, tt.force))
		})
	}
}
