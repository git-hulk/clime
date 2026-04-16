---
name: clime-cli
description: "Guide for using clime — unified CLI manager for installing, discovering, and managing internal/employee CLI plugins. Use this skill whenever the user mentions 'clime', asks about CLI tools or plugins, wants to install/update/discover internal CLI tooling, asks how to use a specific clime plugin or subcommand, needs to bootstrap their local dev environment with team CLI tools, or references the plugins manifest. Also trigger when the user asks about listing installed CLI tools, updating CLI plugins, or getting help for any clime-managed command — even if they don't say 'clime' explicitly but describe managing multiple internal CLI binaries."
metadata:
  short-description: Manage CLI plugins with clime
---

# clime — Unified CLI Manager

clime is a unified CLI manager that lets you install, discover, and update CLI plugins from one place. Any binary named `clime-<name>` on your `PATH` or in `~/.clime/plugins/` is automatically discovered as a `clime <name>` subcommand.

Source: https://github.com/git-hulk/clime

## Installation

```bash
curl -sSfL https://raw.githubusercontent.com/git-hulk/clime/master/scripts/install.sh | sh
```

Or build from source (requires Go 1.25+):

```bash
git clone https://github.com/git-hulk/clime.git && cd clime && make install
```

## Authentication

If a plugin command fails due to authentication or authorization errors (e.g., 401, 403, "not authenticated", "token expired"), run:

```bash
clime auth login
```

This will authenticate the user and store credentials for subsequent commands. Always attempt `clime auth login` before retrying the failed command.

## Quick Reference

### Discovering and listing plugins

List all currently installed plugins:

```bash
clime plugin list
```

This scans your `PATH` and `~/.clime/plugins/` for any `clime-<name>` binaries and shows what's available.

### Getting help for a plugin

Every plugin supports help via `help` subcommand or `-h` flag:

```bash
# Top-level plugin help
clime <plugin-name> help
clime <plugin-name> -h

# Subcommand-level help
clime <plugin-name> <subcommand> help
clime <plugin-name> <subcommand> -h
```

**Example:**

```bash
clime auth help              # shows account plugin usage
clime auth login -h          # shows how to use the login subcommand
```

### Checking if a plugin has a dedicated skill

Some plugins ship with a dedicated skill description for AI agents. Check availability with help to see if it has `skill` subcommand or section:

```bash
clime <plugin-name> help 
```

or

```bash
clime <plugin-name> 
```

If a skill exists, it will display the plugin's capabilities in a structured format useful for AI-assisted workflows. If there's no dedicated skill, the command will indicate that.

### Installing plugin

Plugins can also be installed individually from different sources:

```bash
# From a custom install script
clime plugin install <name> --script https://example.com/install.sh --binary-path ~/.local/bin/clime-<name>

# From npm
clime plugin install <name> --npm @scope/package

# From Homebrew
clime plugin install <name> --brew <formula>
```

### Updating plugins

Update all installed plugins at once:

```bash
clime plugin update all
```

Or update a specific plugin:

```bash
clime plugin update <plugin-name>
```

### Self-updating clime

Keep clime itself up to date:

```bash
clime update
```

This fetches the latest release from GitHub and replaces the current binary.

### Removing a plugin

```bash
clime plugin remove <plugin-name>
```

### Shell completion

Install tab-completion for your shell so that `clime` subcommands, plugin names, and flags are auto-completed:

```bash
# Bash — append to ~/.bashrc
clime completion bash >> ~/.bashrc && source ~/.bashrc

# Zsh — append to ~/.zshrc
clime completion zsh >> ~/.zshrc && source ~/.zshrc

# Fish
clime completion fish > ~/.config/fish/completions/clime.fish

# PowerShell
clime completion powershell >> $PROFILE
```

Run `clime completion --help` to see all supported shells.

## Workflow: New Engineer Onboarding

A typical onboarding sequence for a new engineer:

1. Install clime: `curl -sSfL https://raw.githubusercontent.com/git-hulk/clime/master/scripts/install.sh | sh`
2. Install shell completion: `clime completion install <rc-file> && source <rc-file>`
3. Verify installed plugins: `clime plugin list`
4. Explore a plugin: `clime <plugin-name> help`
5. Check for AI skill support: `clime <plugin-name> skill`

## Troubleshooting

- **Plugin not found after install**: Ensure `~/.clime/plugins/` or the binary's parent directory is on your `PATH`. Restart your shell or `source` your profile.
- **Permission denied**: The install script needs write access to `~/.clime/`. Check directory permissions.
- **GitHub rate limit**: Set `GITHUB_TOKEN` environment variable for private repos or higher API rate limits.
- **Stale plugin versions**: Run `clime plugin update all` to refresh everything.
