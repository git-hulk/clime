package skill

import (
	"testing"
)

func TestAddSkill(t *testing.T) {
	t.Parallel()
	m := &Manifest{}

	m.AddSkill(InstalledSkill{
		Name:   "my-skill",
		Source: "owner/repo",
		Path:   "my-skill",
	})
	if len(m.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(m.Skills))
	}

	// Update existing skill.
	m.AddSkill(InstalledSkill{
		Name:        "my-skill",
		Description: "updated",
		Source:      "owner/repo",
		Path:        "my-skill",
	})
	if len(m.Skills) != 1 {
		t.Fatalf("expected 1 skill after update, got %d", len(m.Skills))
	}
	if m.Skills[0].Description != "updated" {
		t.Fatalf("expected description 'updated', got %q", m.Skills[0].Description)
	}
}

func TestRemoveSkill(t *testing.T) {
	t.Parallel()
	m := &Manifest{
		Skills: []InstalledSkill{
			{Name: "skill-a"},
			{Name: "skill-b"},
		},
	}

	if !m.RemoveSkill("skill-a") {
		t.Fatal("expected RemoveSkill to return true")
	}
	if len(m.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(m.Skills))
	}

	if m.RemoveSkill("missing") {
		t.Fatal("expected RemoveSkill to return false for missing skill")
	}
}

func TestGetSkill(t *testing.T) {
	t.Parallel()
	m := &Manifest{
		Skills: []InstalledSkill{{Name: "my-skill", Source: "owner/repo"}},
	}

	s, ok := m.GetSkill("my-skill")
	if !ok {
		t.Fatal("expected to find skill")
	}
	if s.Source != "owner/repo" {
		t.Fatalf("expected source owner/repo, got %s", s.Source)
	}

	_, ok = m.GetSkill("missing")
	if ok {
		t.Fatal("expected not to find missing skill")
	}
}
