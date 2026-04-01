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

## How It Works

Install a plugin, then use it as a subcommand — clime forwards all arguments to the underlying binary:

```sh
# Install a plugin via a custom install script
clime plugin install account --script https://example.com/install.sh --binary-path ~/.local/bin/clime-account

# Install a plugin from npm
clime plugin install opencli --npm @jackwener/opencli

# Install a plugin from Homebrew
clime plugin install lint --brew myorg/tap/clime-lint

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
clime init                                  # built-in defaults
clime init https://example.com/tools.yaml   # your team's plugin list
```

Run `clime help` or `clime <command> --help` for full usage details.
