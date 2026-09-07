package cmd

import (
	"fmt"
	"strings"
	"testing"

	"github.com/git-hulk/clime/internal/prompt"
	"github.com/git-hulk/clime/internal/skill"
)

func TestSkillListPagesNavigateAtBoundaries(t *testing.T) {
	for _, count := range []int{1, 10, 11, 20, 21} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			defer stubSkillPrompts(t)()
			var rows [][]string
			for i := 1; i <= count; i++ {
				rows = append(rows, []string{fmt.Sprintf("skill-%02d", i)})
			}
			pageCount := (count-1)/10 + 1
			page, backward := 0, false
			var starts []int
			selectPrompt = func(config prompt.SelectConfig) (int, error) {
				if config.LeftOption != "Previous page" || config.RightOption != "Next page" {
					t.Fatal("left/right arrows must activate previous/next page")
				}
				starts = append(starts, page*10)
				if config.Label != fmt.Sprintf("Page %d/%d", page+1, pageCount) {
					t.Fatalf("unexpected page label %q", config.Label)
				}
				choice := "Next page"
				if page == pageCount-1 {
					backward = true
				}
				if backward {
					choice = "Previous page"
				}
				if backward && page == 0 {
					choice = "Done"
				}
				for i, option := range config.Options {
					if page == 0 && option == "Previous page" || page == pageCount-1 && option == "Next page" {
						t.Fatalf("invalid navigation %q on page %d", option, page+1)
					}
					if option == choice {
						if choice == "Next page" {
							page++
						} else if choice == "Previous page" {
							page--
						}
						return i, nil
					}
				}
				t.Fatalf("navigation %q missing", choice)
				return 0, nil
			}
			output := captureStdout(t, func() {
				if err := printSkillPages([]string{"NAME"}, rows); err != nil {
					t.Fatal(err)
				}
			})
			if pageCount == 1 {
				if len(starts) != 0 {
					t.Fatal("a single page must not prompt")
				}
				starts = []int{0}
			}
			pages := strings.Split(output, "NAME")[1:]
			if len(pages) != len(starts) {
				t.Fatalf("rendered %d pages, want %d", len(pages), len(starts))
			}
			for i, page := range pages {
				if got, want := strings.Count(page, "skill-"), min(10, count-starts[i]); got != want {
					t.Fatalf("page %d has %d rows, want %d", i, got, want)
				}
				if !strings.Contains(page, fmt.Sprintf("skill-%02d", starts[i]+1)) {
					t.Fatalf("page %d starts with the wrong skill", i)
				}
			}
		})
	}
}

func TestSkillListPipedOutputIncludesEverySkillInNameOrder(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	manifest := &skill.Manifest{}
	for i := 21; i >= 1; i-- {
		manifest.AddSkill(skill.InstalledSkill{Name: fmt.Sprintf("skill-%02d", i), Source: "owner/repo"})
	}
	if err := manifest.Save(); err != nil {
		t.Fatal(err)
	}
	output := captureStdout(t, func() {
		if err := skillsListCmd.RunE(skillsListCmd, nil); err != nil {
			t.Fatal(err)
		}
	})
	if got := strings.Count(output, "skill-"); got != 21 {
		t.Fatalf("piped output contains %d skills, want 21", got)
	}
	previous := -1
	for i := 1; i <= 21; i++ {
		name := fmt.Sprintf("skill-%02d", i)
		index := strings.Index(output, name)
		if index <= previous {
			t.Fatalf("%s is missing or out of ascending order", name)
		}
		previous = index
	}
}
