//go:build !windows

package main

import (
	"os"
	"syscall"
)

// runPassthrough replaces this process with bin, so signals and the exit
// code belong to the agent.
func runPassthrough(bin string, args []string) error {
	return syscall.Exec(bin, append([]string{bin}, args...), os.Environ())
}
