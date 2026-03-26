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

## Quick Start

```sh
clime plugin install hr                          # install from git-hulk/clime-<name>
clime plugin install hr --repo your-org/clime-hr # install from explicit repo
clime plugin install opencli --npm @jackwener/opencli  # install from npm

clime plugin list                                # list plugins
clime plugin update hr                           # update one
clime plugin update all                          # update all
clime plugin remove hr                           # remove

clime version                                    # show version info
clime update                                     # self-update clime
```

## Commands

| Command | Description |
|---------|-------------|
| clime version | Print version, commit, and build date |
| clime update [--repo owner/name] [--force] | Self-update the `clime` binary |
| clime plugin list | List discovered plugins |
| clime plugin install <name> [--repo] [--npm] | Install a plugin |
| clime plugin update <name\|*\|all> [--repo] [--force] | Update plugin(s) |
| clime plugin remove <name> | Remove a plugin |
| clime init [url] | Install default plugin set |

Plugin update resolves the source repo via: `--repo` flag > manifest entry > default `git-hulk/clime-<name>`.

## Plugin Discovery

- Binaries follow the `clime-<name>` naming convention
- Lookup order: `~/.clime/plugins` (managed), then `PATH`
- Metadata stored in `~/.clime/plugins.yaml`
- npm plugins are auto-symlinked into `~/.clime/plugins/`

## Clime Init

Installs a default set of plugins in one command. Pass a URL to use a remote plugin list maintained by your organization:

```sh
clime init https://example.com/org-plugins.yaml
```

Remote YAML format:

```yaml
plugins:
  - name: deploy
    repo: your-org/clime-deploy                   # GitHub Releases (custom repo)
  - name: account
    script: https://example.com/install.sh        # install script
    binary_path: ~/.local/bin/clime-account       # required for script-based plugins
  - name: opencli
    npm: @jackwener/opencli                       # npm package
```

| Field | Required | Description |
|-------|----------|-------------|
| name | yes | Plugin subcommand name (`hr` -> `clime hr`) |
| repo | no | GitHub `owner/repo`; defaults to `git-hulk/clime-<name>` |
| script | no | Install script URL (`curl \| sh`) |
| binary_path | no | Binary location (required with `script`) |
| npm | no | npm package to install globally |
