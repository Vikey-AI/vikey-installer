//go:build windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
)

// runPassthrough runs bin with this console and exits with its code.
// Windows has no exec, so Ctrl+C is left to the child.
func runPassthrough(bin string, args []string) error {
	signal.Ignore(os.Interrupt)
	cmd := exec.Command(bin, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		os.Exit(exit.ExitCode())
	}
	if err != nil {
		return err
	}
	os.Exit(0)
	return nil
}
