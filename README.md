# clime

**Install, run, and update CLI tools and AI agent skills from one command.**

- **CLI plugins:** install from GitHub Releases, npm, Homebrew, or scripts; run them as `clime <name>`.
- **Agent skills:** manage skills for Claude Code and Codex from repositories or local directories.
- **Team setup:** share a manifest to install the same tools and skills across your team.

## Install

```sh
curl -sSfL https://raw.githubusercontent.com/git-hulk/clime/master/scripts/install.sh | sh
```

Supports macOS and Linux on amd64 and arm64. Installs to `~/.local/bin` and adds it to your shell profile's `PATH` if needed.

## CLI plugins

```sh
clime plugin install opencli --npm @jackwener/opencli
clime opencli --help

clime plugin list
clime plugin update all
clime plugin uninstall opencli
```

Any `clime-<name>` binary on `PATH` or in `~/.clime/plugins/` becomes a `clime <name>` subcommand. Arguments pass through to the binary.

Run `clime plugin install` for an interactive setup, or choose a source:

```sh
clime plugin install mytools --repo owner/repo
clime plugin install golangci-lint --brew golangci-lint
clime plugin install account --script https://example.com/install.sh --binary-path ~/.local/bin/clime-account
```

## AI agent skills

```sh
clime skills install owner/repo  # choose skills from a repository
clime skills install /local/path # or use a local directory
clime skills list
clime skills update              # check all sources for updates
clime skills sync                # restore skills from the saved manifest
clime skills uninstall my-skill
```

Skills live in `~/.agents/skills/<name>/`, with links in `~/.claude/skills/` when `~/.claude` exists. Source repositories can store skills under `skills/`, `.agents/skills/`, or `.claude/skills/`, with a `SKILL.md` in each skill directory. Explicit catalog manifests take precedence; otherwise, clime uses the first directory containing skills in that order.

To teach agents how to use clime, install its bundled skill with `clime install skill`.

### Versions and manifest

Selected skills are saved by source in `~/.clime/skills.yaml`:

```yaml
AfterShip/Skills:
  skills:
    - rest-api-design
  version: latest
```

Choose a version with `clime skills install owner/repo@<version>`:

| Version | Behavior |
| --- | --- |
| `latest` | Highest stable semver tag; falls back to a prerelease, then default-branch HEAD if no semver tags exist. |
| Branch, e.g. `main` | Follows the branch's current commit. |
| Tag or commit, e.g. `v1.2.3` | Pins installation and sync to that revision. |
| Range, e.g. `v1` or `v1.2` | Selects the highest matching semver tag. |

Installing without a version uses the saved version, or `latest` for a new source. `latest`, tag names, and branch names stay unchanged in the manifest.

- **Install and sync** check branches and `latest` remotely; cached tags and commits work offline.
- **Update** follows saved branches and `latest`, and moves pinned versions to latest. Unchanged installed revisions are skipped.
- **Custom manifests:** use `clime skills sync --manifest ./skills.yaml`; `--manifest` works with every `clime skills` command. Local sources omit `version`.

## Team setup

Bootstrap CLI plugins from a shared manifest:

```sh
clime init                                 # saved source or built-in defaults
clime init https://example.com/tools.yaml   # use and remember your team's manifest
```

Share a `skills.yaml` separately and apply it with `clime skills sync --manifest ./skills.yaml`.

## More commands

```sh
clime update                 # update clime itself
clime completion install     # install shell completions
clime help                   # browse commands
clime <command> --help       # see options and examples
```

To build from source, install Go 1.25+ and run:

```sh
git clone https://github.com/git-hulk/clime.git
cd clime
make install
```
