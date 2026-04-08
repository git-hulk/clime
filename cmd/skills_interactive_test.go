package cmd

import (
	"errors"
	"testing"

	"github.com/git-hulk/clime/internal/prompt"
	"github.com/git-hulk/clime/internal/skill"
)

func TestRunInteractiveSkillsInstallEscFromActionReturnsToSourceMenu(t *testing.T) {
	manifest := &skill.Manifest{
		Skills: []skill.InstalledSkill{
			{Name: "alpha", Source: "owner/repo"},
		},
	}

	restore := stubSkillPrompts(t)
	defer restore()

	selectCalls := 0
	selectPrompt = func(config prompt.SelectConfig) (int, error) {
		selectCalls++
		switch selectCalls {
		case 1:
			return 0, nil
		case 2:
			return 0, prompt.ErrBack
		case 3:
			return 1, nil
		default:
			t.Fatalf("unexpected select call %d", selectCalls)
			return 0, nil
		}
	}

	inputPrompt = func(label string) (string, error) {
		return "another/repo", nil
	}

	var (
		gotRepo   string
		gotAction sourceAction
	)
	skillsActionRunner = func(manifest *skill.Manifest, repo string, action sourceAction) error {
		gotRepo = repo
		gotAction = action
		return nil
	}

	if err := runInteractiveSkillsInstall(manifest); err != nil {
		t.Fatalf("runInteractiveSkillsInstall() error = %v", err)
	}
	if gotRepo != "another/repo" {
		t.Fatalf("repo = %q, want %q", gotRepo, "another/repo")
	}
	if gotAction != actionBrowseInstall {
		t.Fatalf("action = %v, want %v", gotAction, actionBrowseInstall)
	}
}

func TestRunInteractiveSkillsInstallEscAtTopLevelKeepsUIOpen(t *testing.T) {
	manifest := &skill.Manifest{
		Skills: []skill.InstalledSkill{
			{Name: "alpha", Source: "owner/repo"},
		},
	}

	restore := stubSkillPrompts(t)
	defer restore()

	selectCalls := 0
	selectPrompt = func(config prompt.SelectConfig) (int, error) {
		selectCalls++
		switch selectCalls {
		case 1:
			return 0, prompt.ErrBack
		case 2:
			return 1, nil
		default:
			t.Fatalf("unexpected select call %d", selectCalls)
			return 0, nil
		}
	}

	inputPrompt = func(label string) (string, error) {
		return "new/repo", nil
	}

	called := false
	skillsActionRunner = func(manifest *skill.Manifest, repo string, action sourceAction) error {
		called = true
		if repo != "new/repo" {
			t.Fatalf("repo = %q, want %q", repo, "new/repo")
		}
		return nil
	}

	if err := runInteractiveSkillsInstall(manifest); err != nil {
		t.Fatalf("runInteractiveSkillsInstall() error = %v", err)
	}
	if !called {
		t.Fatal("expected skillsActionRunner to be called")
	}
}

func TestRunInteractiveSkillsInstallEscFromInstallReturnsToActionMenu(t *testing.T) {
	manifest := &skill.Manifest{
		Skills: []skill.InstalledSkill{
			{Name: "alpha", Source: "owner/repo"},
		},
	}

	restore := stubSkillPrompts(t)
	defer restore()

	selectCalls := 0
	selectPrompt = func(config prompt.SelectConfig) (int, error) {
		selectCalls++
		switch selectCalls {
		case 1:
			return 0, nil
		case 2:
			return 0, nil
		case 3:
			return 1, nil
		default:
			t.Fatalf("unexpected select call %d", selectCalls)
			return 0, nil
		}
	}

	var actions []sourceAction
	skillsActionRunner = func(manifest *skill.Manifest, repo string, action sourceAction) error {
		actions = append(actions, action)
		if len(actions) == 1 {
			return prompt.ErrBack
		}
		return nil
	}

	if err := runInteractiveSkillsInstall(manifest); err != nil {
		t.Fatalf("runInteractiveSkillsInstall() error = %v", err)
	}
	if len(actions) != 2 {
		t.Fatalf("actions length = %d, want 2", len(actions))
	}
	if actions[0] != actionBrowseInstall || actions[1] != actionUpdate {
		t.Fatalf("actions = %v, want [%v %v]", actions, actionBrowseInstall, actionUpdate)
	}
}

func TestInteractiveUninstallEscKeepsMenuOpen(t *testing.T) {
	manifest := &skill.Manifest{
		Skills: []skill.InstalledSkill{
			{Name: "alpha"},
		},
	}

	restore := stubSkillPrompts(t)
	defer restore()

	calls := 0
	multiSelectPrompt = func(config prompt.SelectConfig) ([]int, error) {
		calls++
		if calls == 1 {
			return nil, prompt.ErrBack
		}
		return nil, nil
	}

	if err := interactiveUninstall(manifest); err != nil {
		t.Fatalf("interactiveUninstall() error = %v", err)
	}
	if calls != 2 {
		t.Fatalf("multiSelectPrompt calls = %d, want 2", calls)
	}
}

func TestInteractiveUninstallInterruptPropagates(t *testing.T) {
	manifest := &skill.Manifest{
		Skills: []skill.InstalledSkill{
			{Name: "alpha"},
		},
	}

	restore := stubSkillPrompts(t)
	defer restore()

	multiSelectPrompt = func(config prompt.SelectConfig) ([]int, error) {
		return nil, prompt.ErrInterrupted
	}

	err := interactiveUninstall(manifest)
	if !errors.Is(err, prompt.ErrInterrupted) {
		t.Fatalf("interactiveUninstall() error = %v, want ErrInterrupted", err)
	}
}

func stubSkillPrompts(t *testing.T) func() {
	t.Helper()

	origSelect := selectPrompt
	origMultiSelect := multiSelectPrompt
	origInput := inputPrompt
	origRunner := skillsActionRunner

	return func() {
		selectPrompt = origSelect
		multiSelectPrompt = origMultiSelect
		inputPrompt = origInput
		skillsActionRunner = origRunner
	}
}

func TestValidateSkillRepoSourceAllowsLocalCurrentDir(t *testing.T) {
	if err := validateSkillRepoSource("."); err != nil {
		t.Fatalf("validateSkillRepoSource(.) error = %v", err)
	}
}
