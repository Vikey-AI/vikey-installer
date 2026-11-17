// Command vikey sets up coding agents (Claude Code, Codex, opencode, Hermes)
// to use Vikey, and undoes it again.
//
//	vikey setup      pick agents, install the missing ones, point them at Vikey
//	vikey doctor     show what is installed and whether it points at Vikey
//	vikey uninstall  put the agents' settings back as they were
//	vikey claude     start Claude Code with Vikey without touching its settings
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/Vikey-AI/vikey-installer/internal/ui"
)

// version is set at build time with -ldflags "-X main.version=1.2.3".
var version = "dev"

const usage = `Vikey CLI %s

Pemakaian:
  vikey setup       Atur coding agent supaya memakai Vikey
  vikey doctor      Cek agent yang terpasang dan koneksinya ke Vikey
  vikey uninstall   Kembalikan pengaturan agent seperti sebelum Vikey
  vikey claude      Jalankan Claude Code dengan Vikey tanpa mengubah pengaturannya
  vikey version     Tampilkan versi

Bantuan tiap perintah: vikey <perintah> --help
`

func main() {
	args := os.Args[1:]
	cmd := ""
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	var err error
	switch cmd {
	case "setup":
		err = cmdSetup(args)
	case "doctor", "status":
		err = cmdDoctor(args)
	case "uninstall":
		err = cmdUninstall(args)
	case "claude":
		err = cmdClaude(args)
	case "version", "--version", "-v":
		fmt.Println(version)
	case "", "help", "--help", "-h":
		fmt.Printf(usage, version)
	default:
		fmt.Fprintf(os.Stderr, "Perintah %q tidak dikenal.\n\n", cmd)
		fmt.Fprintf(os.Stderr, usage, version)
		os.Exit(2)
	}
	if err != nil {
		var exit exitError
		if errors.As(err, &exit) {
			os.Exit(exit.code)
		}
		if errors.Is(err, ui.ErrCancelled) {
			ui.Blank()
			ui.Line("Dibatalkan.")
			os.Exit(130)
		}
		ui.Blank()
		ui.Fail("%v", err)
		os.Exit(1)
	}
}

// exitError ends the program with a code after the message was already shown.
type exitError struct{ code int }

func (e exitError) Error() string { return fmt.Sprintf("exit %d", e.code) }
