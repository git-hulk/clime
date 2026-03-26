package plugin

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// Exec replaces the current process with the plugin binary.
// For compiled binaries, it uses syscall.Exec for full process replacement.
// Falls back to os/exec for interpreted scripts (e.g. shell scripts).
func Exec(binPath string, args []string) {
	argv := append([]string{binPath}, args...)
	err := syscall.Exec(binPath, argv, os.Environ())
	if err == nil {
		return
	}

	// Fallback: use os/exec for scripts or other interpreted executables
	if err == syscall.ENOEXEC {
		cmd := exec.Command(binPath, args...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				os.Exit(exitErr.ExitCode())
			}
			fmt.Fprintf(os.Stderr, "Error: failed to run plugin %s: %v\n", binPath, err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	fmt.Fprintf(os.Stderr, "Error: failed to exec plugin %s: %v\n", binPath, err)
	os.Exit(1)
}
