package cmd

import (
	"strings"
	"testing"

	"github.com/git-hulk/clime/internal/skill"
)

func TestSelectInstallCandidatesSkipsInstalledByDefault(t *testing.T) {
	manifest := &skill.Manifest{
		Skills: []skill.InstalledSkill{
			{Name: "alpha", Source: "owner/repo"},
		},
	}
	repoSkills := []skill.Entry{
		{Name: "alpha", Description: "first"},
		{Name: "beta", Description: "second"},
	}

	got := selectInstallCandidates(repoSkills, manifest, false)
	if len(got) != 1 {
		t.Fatalf("candidates = %d, want 1 (installed alpha filtered out)", len(got))
	}
	if got[0].entry.Name != "beta" {
		t.Fatalf("candidate = %q, want beta", got[0].entry.Name)
	}
	if strings.Contains(got[0].label, "(reinstall)") {
		t.Fatalf("beta is not installed, label must not be marked reinstall: %q", got[0].label)
	}
}

func TestSelectInstallCandidatesForceIncludesInstalled(t *testing.T) {
	manifest := &skill.Manifest{
		Skills: []skill.InstalledSkill{
			{Name: "alpha", Source: "owner/repo"},
		},
	}
	repoSkills := []skill.Entry{
		{Name: "alpha", Description: "first"},
		{Name: "beta", Description: "second"},
	}

	got := selectInstallCandidates(repoSkills, manifest, true)
	if len(got) != 2 {
		t.Fatalf("candidates = %d, want 2 (force keeps installed alpha)", len(got))
	}

	byName := map[string]installCandidate{}
	for _, c := range got {
		byName[c.entry.Name] = c
	}
	if !strings.Contains(byName["alpha"].label, "(reinstall)") {
		t.Fatalf("installed alpha should be marked reinstall under force, got %q", byName["alpha"].label)
	}
	if strings.Contains(byName["beta"].label, "(reinstall)") {
		t.Fatalf("not-installed beta must not be marked reinstall, got %q", byName["beta"].label)
	}
}

func TestSelectInstallCandidatesForceEmptyRepo(t *testing.T) {
	manifest := &skill.Manifest{}
	if got := selectInstallCandidates(nil, manifest, true); len(got) != 0 {
		t.Fatalf("candidates = %d, want 0 for empty repo", len(got))
	}
}
