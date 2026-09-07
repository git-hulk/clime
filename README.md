# clime

As more agents move from MCP servers to CLIs, it gets hard to track what's installed and keep things up to date. For organizations with many internal tools, there's often no single place for employees to discover and download them.

**clime** solves this — a unified CLI manager that lets you install, discover, and update CLI plugins from one place.

## Features

- **Unified entry point** — any `clime-<name>` binary becomes a `clime <name>` subcommand, no config needed
- **Discover & manage** — list, install, update, and remove plugins with simple commands
- **Multiple sources** — install plugins from GitHub Releases, npm, Homebrew, or custom scripts
- **Team toolchains** — share a YAML manifest so everyone gets the same set of tools via `clime init`
- **Self-updating** — keep both clime and its plugins up to date with one command

## Installation

```sh
curl -sSfL https://raw.githubusercontent.com/git-hulk/clime/master/scripts/install.sh | sh
```

Or build from source (requires Go 1.25+):

```sh
git clone https://github.com/git-hulk/clime.git && cd clime && make install
```

The install script detects your OS (macOS / Linux) and architecture (amd64 / arm64) automatically.
By default it installs `clime` to `~/.local/bin` and updates your shell profile if that directory is not already on `PATH`.

## How It Works

Install a plugin, then use it as a subcommand — clime forwards all arguments to the underlying binary:

```sh
# Install a plugin from GitHub Releases (default — looks for git-hulk/clime-<name>)
clime plugin install mytools
clime plugin install mytools --repo owner/repo   # custom GitHub repo

# Install a plugin via a custom install script
clime plugin install account --script https://example.com/install.sh --binary-path ~/.local/bin/clime-account

# Install a plugin from npm
clime plugin install opencli --npm @jackwener/opencli

# Install a plugin from Homebrew
clime plugin install golangci-lint --brew golangci-lint

# Interactive mode — wizard prompts for name, source, and details
clime plugin install

# Now use it — clime dispatches to the clime-<name> binary
clime account login --user hulk
clime account list
clime opencli --help
```

Any binary named `clime-<name>` on your `PATH` or in `~/.clime/plugins/` is automatically discovered — no extra config needed.

Manage all your plugins with a handful of commands:

```sh
clime plugin list               # discover installed plugins
clime plugin update all         # keep everything up to date
clime plugin remove account     # uninstall cleanly
```

Bootstrap an entire toolchain at once with `clime init`, using the built-in defaults or a custom YAML manifest:

```sh
clime init                                  # recorded URL, or built-in defaults
clime init https://example.com/tools.yaml   # use and record your team's plugin list
```

## AI Agent Skills

clime can manage AI agent skills for Claude Code and Codex.

### Built-in skill

Install the bundled clime-cli skill so agents can discover and manage plugins on your behalf:

```sh
clime install skill
```

This installs the skill into `~/.agents/skills/clime-cli/`. When `~/.claude`
exists, `~/.claude/skills/clime-cli` is a symlink to that directory.

### Skills from repositories

Skills use the same layout: files in `~/.agents/skills/<name>/`, a symlink in
`~/.claude/skills/<name>` when `~/.claude` exists. Reinstalling or syncing an
existing skill replaces its old Claude directory with the symlink. Uninstall
removes the shared files and Claude link.

Repositories can provide `skills.yaml`, `skills.yml`,
`.claude-plugin/marketplace.json`, or `.claude-plugin/plugin.json`. As a final
fallback, clime discovers `skills/<name>/SKILL.md` directly. It reads names and
descriptions from frontmatter, using the directory name when no name is provided.

Install, update, sync, list, and uninstall skills from GitHub repositories or local paths:

```sh
clime skills install owner/repo         # use the locked version, or latest if none
clime skills install owner/repo@latest  # highest stable semver tag, like go get
clime skills install owner/repo@v1      # highest v1.x.y tag
clime skills install owner/repo@v1.2.3  # pin to a tag, branch, or commit SHA
clime skills install /local/path        # install from a local directory
clime skills install                    # interactive mode — pick a source and skills
clime skills update                     # update every source to its latest version
clime skills update owner/repo          # update one source to latest
clime skills update owner/repo@v1.2.3   # move one source to a specific version
clime skills sync                       # reinstall skills at the versions locked in the manifest
clime skills list                       # list installed skills
clime skills uninstall <name>           # remove a skill
```

The installed-skills list and install picker show 10 skills per page. Use the
left/right arrows to change pages. The list also offers Next page, Previous page,
and Done. In the picker, up/down moves between skills, and selections are kept
across pages. The numbered picker
supports `n` and `p` for navigation. Piping `clime skills list` prints every skill.

The manifest at `~/.clime/skills.yaml` records each source's locked version.
Installing without a version uses that lock and reuses the local cache without
network access when available. If the locked version is not cached, it is fetched;
if there is no lock, install resolves latest. An explicit `@latest` checks upstream.
`sync` applies those versions as-is (from the local cache when available, so it works
offline), while `update` resolves a newer version and rewrites the lock. An update
is refused when the new version no longer provides an installed skill, so skills
are never removed implicitly.

After a successful `install`, `update`, or `sync`, clime keeps the installed
version's snapshot and deletes the other cached versions of that repository.
Failed operations retain existing snapshots. Downgrading to a removed version
fetches it again.

For GitHub operations, clime checks `gh auth status --hostname github.com`.
When logged in, it uses `gh` for version lookups, skill archive downloads,
and release downloads. Skill installs, updates, and syncs share this behavior.
If `gh` is unavailable or logged out, skills use Git and releases use HTTP.
Other Git hosts continue to use Git. Downloads show progress during installation.

## Shell Completions

Generate shell completion scripts for bash, zsh, fish, or PowerShell:

```sh
clime completion install           # auto-detect shell and install completions
clime completion bash              # output bash completion script
clime completion zsh               # output zsh completion script
```

Run **clime help** or **clime <command> --help** for full usage details.
