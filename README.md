# clime

A unified CLI that dispatches to plugin binaries (`clime-<name>`) on your `PATH`, with built-in plugin management and self-update.

## Installation

```sh
curl -sSfL https://raw.githubusercontent.com/git-hulk/clime/master/scripts/install.sh | sh
```

Or build from source (requires Go 1.25+):

```sh
git clone https://github.com/git-hulk/clime.git && cd clime && make install
```

The install script detects your OS (macOS / Linux) and architecture (amd64 / arm64) automatically. Override with `CLIME_OS`, `CLIME_ARCH`, or `CLIME_INSTALL_DIR` environment variables. Set `GITHUB_TOKEN` to avoid GitHub API rate limits.

## Quick Start

```sh
# Install plugins
clime plugin install account                                # from git-hulk/clime-account (default)
clime plugin install account --repo your-org/clime-account  # from a custom GitHub repo
clime plugin install opencli --npm @jackwener/opencli        # from npm
clime plugin install tool --script https://example.com/install.sh --binary-path ~/.local/bin/clime-tool

# Use a plugin — any installed clime-<name> binary becomes a subcommand
clime account

# Manage plugins
clime plugin list                                # list all discovered plugins
clime plugin update account                      # update one plugin
clime plugin update all                          # update all plugins
clime plugin remove account                      # remove a plugin

# Manage clime itself
clime version                                    # show version, commit, and build date
clime update                                     # self-update clime binary
```

## Commands

### `clime version`

Prints the version, git commit, and build date.

### `clime update`

Self-updates the `clime` binary from GitHub Releases.

| Flag | Description |
|------|-------------|
| `--repo <owner/repo>` | Use a custom GitHub repository (default: `git-hulk/clime`) |
| `--force` | Re-download even if the installed version matches the latest |

### `clime init`

Installs a set of plugins in one command. Accepts an optional URL or local file path to a YAML plugin list. Without arguments, installs the built-in default set.

```sh
clime init                                        # built-in defaults
clime init https://example.com/org-plugins.yaml   # remote list
clime init ./plugins.yaml                         # local file
```

| Flag | Description |
|------|-------------|
| `--tags <t1>,<t2>,...` | Only install plugins matching these tags (untagged plugins are always included) |

**Plugin list YAML format:**

```yaml
plugins:
  - name: deploy
    repo: your-org/clime-deploy                   # GitHub Releases
  - name: account
    script: https://example.com/install.sh        # install via script
    binary_path: ~/.local/bin/clime-account       # required with script
  - name: opencli
    npm: @jackwener/opencli                       # npm package
  - name: devops-tool
    repo: my-org/special-tool
    tags: [devops, infra]                         # only installed when --tags matches
```

| Field | Required | Description |
|-------|----------|-------------|
| `name` | yes | Plugin subcommand name (`account` &rarr; `clime account`) |
| `description` | no | Short description shown in help output |
| `repo` | no | GitHub `owner/repo`; defaults to `git-hulk/clime-<name>` |
| `script` | no | Install script URL (executed via `curl \| sh`) |
| `binary_path` | no | Path to the installed binary; required with `script`, supports `~` |
| `npm` | no | npm package to install globally |
| `tags` | no | List of tags for selective installation with `--tags` |

### `clime plugin install <name>`

Installs a plugin. Only one source flag (`--repo`, `--npm`, or `--script`) can be used at a time.

| Flag | Description |
|------|-------------|
| `--repo <owner/repo>` | Install from a GitHub repo (default: `git-hulk/clime-<name>`) |
| `--npm <package>` | Install from npm globally |
| `--script <url>` | Install via a remote shell script |
| `--binary-path <path>` | Path to the binary after script install (required with `--script`) |
| `--description <text>` | Short description for help output |

### `clime plugin update <name|*|all>`

Updates one or all plugins. Source repo is resolved in order: `--repo` flag &rarr; manifest entry &rarr; default `git-hulk/clime-<name>`.

| Flag | Description |
|------|-------------|
| `--repo <owner/repo>` | Override the source repo (single plugin only) |
| `--force` | Re-download even if versions match |

### `clime plugin list`

Lists all discovered plugins from `~/.clime/plugins/` and `PATH`.

### `clime plugin remove <name>`

Removes an installed plugin and its manifest entry.

## Plugin Discovery

Any binary named `clime-<name>` is automatically available as `clime <name>`.

**Lookup order:**

1. `~/.clime/plugins/` — managed plugins (installed via `clime plugin install`)
2. `$PATH` — any matching binary on your system path

Managed plugin metadata is stored in `~/.clime/plugins.yaml`. Plugins installed via `--npm` or `--script` are symlinked into `~/.clime/plugins/`.

## Development

```sh
make build     # build ./clime with version metadata
make install   # go install + run clime init
make test      # go test ./... -v
make lint      # go vet ./...
make clean     # remove ./clime binary
```
