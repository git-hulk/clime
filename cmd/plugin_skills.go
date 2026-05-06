package cmd

import (
	"os/exec"
	"strings"

	"github.com/git-hulk/clime/internal/skill"
)

func tryInstallPluginSkills(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}

	bin, err := exec.LookPath("clime-" + name)
	if err != nil {
		return
	}

	out, err := exec.Command(bin, "skills").Output()
	if err != nil {
		return
	}

	source := strings.TrimSpace(string(out))
	if source == "" {
		return
	}

	manifest, err := skill.LoadManifest()
	if err != nil {
		return
	}

	repoManifest, err := skill.FetchRepoManifest(source)
	if err != nil {
		return
	}

	dir, cleanup, err := skill.PrepareRepoDir(source)
	if err != nil {
		return
	}
	defer cleanup()

	for _, entry := range repoManifest.Skills {
		if _, found := manifest.GetSkill(entry.Name); found {
			continue
		}
		_ = installSkillEntry(manifest, &entry, source, dir)
	}
}
