package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeManifestFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "skills.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

const commentedManifest = `# Reviewed by the platform team.
acme/agent-skills:
  # Locked after the v1.4.2 review.
  version: v1.4.2
  skills:
    - rest-api-design # keep this one
    - write-technical-design

# Internal skills follow.
github.company.com/platform/private-skills:
  version: 8f9f4e0b67b9f6c627e93ab4e56ee48d623aa0958f9f4e0b67b9f6c627e93ab4
  skills:
    - internal-review
`

func TestLoadManifestFromParsesRepositories(t *testing.T) {
	path := writeManifestFile(t, commentedManifest)
	m, err := LoadManifestFrom(path)
	require.NoError(t, err)
	require.Len(t, m.Repos, 2)

	r := m.Repos[0]
	assert.Equal(t, "acme/agent-skills", r.Key)
	assert.Equal(t, "github.com/acme/agent-skills", r.ID.Canonical())
	assert.Equal(t, "v1.4.2", r.Version)
	assert.Equal(t, []string{"rest-api-design", "write-technical-design"}, r.Skills)
	assert.Equal(t, "github.company.com/platform/private-skills", m.Repos[1].ID.Canonical())
}

func TestManifestMutationsPreserveCommentsOnUntouchedNodes(t *testing.T) {
	path := writeManifestFile(t, commentedManifest)

	// Round trip 1: update the private repo's version.
	m, err := LoadManifestFrom(path)
	require.NoError(t, err)
	m.SetVersion(m.Repos[1], "1111111111111111111111111111111111111111")
	require.NoError(t, m.Save())

	// Round trip 2: install a new skill into the first repo.
	m, err = LoadManifestFrom(path)
	require.NoError(t, err)
	m.AddSkills(m.Repos[0], []string{"security-review"})
	require.NoError(t, m.Save())

	// Round trip 3: uninstall a skill.
	m, err = LoadManifestFrom(path)
	require.NoError(t, err)
	_, ok := m.RemoveSkill("write-technical-design")
	require.True(t, ok, "RemoveSkill failed")
	require.NoError(t, m.Save())

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	text := string(data)
	for _, comment := range []string{
		"# Reviewed by the platform team.",
		"# Locked after the v1.4.2 review.",
		"# keep this one",
		"# Internal skills follow.",
	} {
		assert.Contains(t, text, comment, "comment lost after round trips")
	}
	// Relative order of comments and entries is unchanged.
	assert.Less(t, strings.Index(text, "# Reviewed by the platform team."), strings.Index(text, "acme/agent-skills:"),
		"document comment moved below the first repository")
	assert.Less(t, strings.Index(text, "# Internal skills follow."), strings.Index(text, "github.company.com/platform/private-skills:"),
		"repository comment moved below its entry")
	assert.NotContains(t, text, "write-technical-design", "removed skill still present")
	assert.Contains(t, text, "security-review", "added skill missing")
	assert.Contains(t, text, "1111111111111111111111111111111111111111", "updated version missing")
}

func TestManifestKeySpellingPreservedOnMutation(t *testing.T) {
	path := writeManifestFile(t, "github.com/acme/agent-skills:\n  version: v1.0.0\n  skills:\n    - a-skill\n")
	m, err := LoadManifestFrom(path)
	require.NoError(t, err)
	m.SetVersion(m.Repos[0], "v2.0.0")
	require.NoError(t, m.Save())

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "github.com/acme/agent-skills:", "key spelling was rewritten")
}

func TestAddSkillsInsertsSorted(t *testing.T) {
	path := writeManifestFile(t, "acme/skills:\n  version: v1.0.0\n  skills:\n    - alpha\n    - gamma\n")
	m, err := LoadManifestFrom(path)
	require.NoError(t, err)
	m.AddSkills(m.Repos[0], []string{"delta", "beta", "alpha"})
	assert.Equal(t, []string{"alpha", "beta", "delta", "gamma"}, m.Repos[0].Skills)
}

func TestRemoveLastSkillRemovesRepository(t *testing.T) {
	path := writeManifestFile(t, "acme/skills:\n  version: v1.0.0\n  skills:\n    - only-one\n")
	m, err := LoadManifestFrom(path)
	require.NoError(t, err)
	_, ok := m.RemoveSkill("only-one")
	require.True(t, ok, "RemoveSkill failed")
	assert.Empty(t, m.Repos)
	require.NoError(t, m.Save())

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "acme/skills", "repository entry still present")
}

func TestValidateReportsEveryConflict(t *testing.T) {
	path := writeManifestFile(t, `acme/agent-skills:
  version: v1.0.0
  skills:
    - shared-skill
    - dup-skill
    - dup-skill
github.com/acme/agent-skills:
  version: v2.0.0
  skills:
    - shared-skill
other/repo:
  version: ""
  skills: []
`)
	m, err := LoadManifestFrom(path)
	require.NoError(t, err)

	err = m.Validate()
	require.Error(t, err, "want conflicts")
	for _, want := range []string{
		"identify the same repository",
		`lists skill "dup-skill" twice`,
		`skill "shared-skill" is selected by both`,
		"has no version",
		"selects no skills",
	} {
		assert.ErrorContains(t, err, want)
	}
}

func TestLoadManifestFromDetectsLegacy(t *testing.T) {
	path := writeManifestFile(t, "skills:\n  - name: a\n    source: acme/skills\n    path: skills/a\n")
	_, err := LoadManifestFrom(path)
	assert.ErrorIs(t, err, errLegacyManifest)
}

func TestLoadManifestFromMissingOrEmptyFile(t *testing.T) {
	m, err := LoadManifestFrom(filepath.Join(t.TempDir(), "missing.yaml"))
	require.NoError(t, err)
	assert.Empty(t, m.Repos)

	path := writeManifestFile(t, "")
	m, err = LoadManifestFrom(path)
	require.NoError(t, err)
	assert.Empty(t, m.Repos)
}

func TestSaveNewManifestAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skills.yaml")
	m := newEmptyManifest(path)
	_, err := m.AddRepo("acme/agent-skills", "v1.4.2", []string{"write-technical-design", "rest-api-design"})
	require.NoError(t, err)
	require.NoError(t, m.Save())

	reloaded, err := LoadManifestFrom(path)
	require.NoError(t, err)
	require.Len(t, reloaded.Repos, 1)
	r := reloaded.Repos[0]
	assert.Equal(t, "v1.4.2", r.Version)
	assert.Equal(t, []string{"rest-api-design", "write-technical-design"}, r.Skills)
}

func TestFindRepoMatchesNormalizedAliases(t *testing.T) {
	path := writeManifestFile(t, "acme/agent-skills:\n  version: v1.0.0\n  skills:\n    - a-skill\n")
	m, err := LoadManifestFrom(path)
	require.NoError(t, err)
	id, err := ParseRepo("https://github.com/acme/agent-skills.git")
	require.NoError(t, err)
	assert.NotNil(t, m.FindRepo(id), "FindRepo did not match the normalized alias")
}
