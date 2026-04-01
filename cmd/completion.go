package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

func init() {
	completionCmd.AddCommand(completionInstallCmd)
	completionCmd.AddCommand(completionBashCmd)
	completionCmd.AddCommand(completionZshCmd)
	completionCmd.AddCommand(completionFishCmd)
	completionCmd.AddCommand(completionPowerShellCmd)
	rootCmd.AddCommand(completionCmd)
}

var completionCmd = &cobra.Command{
	Use:   "completion [install|bash|zsh|fish|powershell]",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for clime.

Completions include built-in commands, flags, and discovered plugin names.

Bash:
  # Current session:
  source <(clime completion bash)

  # Permanent (Linux):
  clime completion bash > /etc/bash_completion.d/clime

  # Permanent (macOS with Homebrew):
  clime completion bash > $(brew --prefix)/etc/bash_completion.d/clime

Zsh:
  # Current session:
  source <(clime completion zsh)

  # Permanent (add to your ~/.zshrc):
  eval "$(clime completion zsh)"

  # Or write to fpath:
  clime completion zsh > "${fpath[1]}/_clime"

Fish:
  # Current session:
  clime completion fish | source

  # Permanent:
  clime completion fish > ~/.config/fish/completions/clime.fish

PowerShell:
  # Current session:
  clime completion powershell | Out-String | Invoke-Expression

  # Permanent (add to your profile):
  clime completion powershell >> $PROFILE`,
}

var completionBashCmd = &cobra.Command{
	Use:                   "bash",
	Short:                 "Generate bash completion script",
	SilenceUsage:          true,
	DisableFlagsInUseLine: true,
	Args:                  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return rootCmd.GenBashCompletionV2(os.Stdout, true)
	},
}

var completionZshCmd = &cobra.Command{
	Use:                   "zsh",
	Short:                 "Generate zsh completion script",
	SilenceUsage:          true,
	DisableFlagsInUseLine: true,
	Args:                  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return rootCmd.GenZshCompletion(os.Stdout)
	},
}

var completionFishCmd = &cobra.Command{
	Use:                   "fish",
	Short:                 "Generate fish completion script",
	SilenceUsage:          true,
	DisableFlagsInUseLine: true,
	Args:                  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return rootCmd.GenFishCompletion(os.Stdout, true)
	},
}

var completionPowerShellCmd = &cobra.Command{
	Use:                   "powershell",
	Short:                 "Generate PowerShell completion script",
	SilenceUsage:          true,
	DisableFlagsInUseLine: true,
	Args:                  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
	},
}
