package cmd

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/git-hulk/clime/internal/prompt"
	"github.com/git-hulk/clime/internal/skill"
	"github.com/stretchr/testify/require"
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
			// Select existing source "owner/repo".
			return 0, nil
		case 2:
			// Esc from the action menu to go back.
			return 0, prompt.ErrBack
		case 3:
			// Select "Enter a new repository..." (after Plugin Skills option).
			return 2, nil
		default:
			require.FailNow(t, "unexpected select call", "call %d", selectCalls)
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

	require.NoError(t, runInteractiveSkillsInstall(manifest))
	require.Equal(t, "another/repo", gotRepo)
	require.Equal(t, actionBrowseInstall, gotAction)
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
			// Esc at source menu.
			return 0, prompt.ErrBack
		case 2:
			// Select "Enter a new repository..." (after Plugin Skills option).
			return 2, nil
		default:
			require.FailNow(t, "unexpected select call", "call %d", selectCalls)
			return 0, nil
		}
	}

	inputPrompt = func(label string) (string, error) {
		return "new/repo", nil
	}

	called := false
	skillsActionRunner = func(manifest *skill.Manifest, repo string, action sourceAction) error {
		called = true
		require.Equal(t, "new/repo", repo)
		return nil
	}

	output := captureStdout(t, func() {
		require.NoError(t, runInteractiveSkillsInstall(manifest))
	})
	require.Equal(t, "\n", output)
	require.True(t, called, "expected skillsActionRunner to be called")
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

	output := captureStdout(t, func() {
		require.NoError(t, interactiveUninstall(manifest))
	})
	require.True(t, strings.HasPrefix(output, "\n"))
	require.False(t, strings.HasPrefix(output, "\n\n"))
	require.Equal(t, 2, calls)
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
	require.ErrorIs(t, err, prompt.ErrInterrupted)
}

func TestInteractiveUninstallUsesSortedSelection(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	manifest := &skill.Manifest{Skills: []skill.InstalledSkill{
		{Name: "zebra"}, {Name: "beta"}, {Name: "alpha"},
	}}
	defer stubSkillPrompts(t)()
	multiSelectPrompt = func(config prompt.SelectConfig) ([]int, error) {
		require.Equal(t, []string{"alpha", "beta", "zebra"}, config.Options)
		return []int{0, 1}, nil
	}
	require.NoError(t, interactiveUninstall(manifest))
	require.Len(t, manifest.Skills, 1)
	require.Equal(t, "zebra", manifest.Skills[0].Name)
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(r)
		done <- string(data)
	}()

	defer func() {
		os.Stdout = origStdout
	}()

	fn()

	require.NoError(t, w.Close())
	output := <-done
	require.NoError(t, r.Close())
	return output
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
			require.FailNow(t, "unexpected select call", "call %d", selectCalls)
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

	output := captureStdout(t, func() {
		require.NoError(t, runInteractiveSkillsInstall(manifest))
	})
	require.Equal(t, []sourceAction{actionBrowseInstall, actionUpdate}, actions)
	require.Equal(t, "\n\n", output, "returning to the action menu must not add a spacer")
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
