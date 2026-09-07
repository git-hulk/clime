// Package githubcli shares GitHub CLI authentication checks across downloads.
package githubcli

import "os/exec"

// Authenticated reports whether gh has a working login for github.com.
func Authenticated() bool {
	return exec.Command("gh", "auth", "status", "--hostname", "github.com").Run() == nil
}
