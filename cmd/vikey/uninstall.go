package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/vikey-ai/vikey-cli/internal/agents"
	"github.com/vikey-ai/vikey-cli/internal/pathenv"
	"github.com/vikey-ai/vikey-cli/internal/state"
	"github.com/vikey-ai/vikey-cli/internal/ui"
)

func cmdUninstall(args []string) error {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	yes := fs.Bool("yes", false, "jalan tanpa bertanya")
	tools := fs.String("tools", "", "hanya agent ini, contoh claude,codex")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	st, err := state.Load()
	if err != nil {
		return err
	}
	var targets []agents.Agent
	for _, a := range agents.All() {
		if st.Tools[a.ID()] == nil {
			continue
		}
		if *tools != "" && !strings.Contains(","+*tools+",", ","+a.ID()+",") {
			continue
		}
		targets = append(targets, a)
	}

	ui.Blank()
	ui.Title("Vikey Uninstall")
	if len(targets) == 0 {
		ui.Line("Tidak ada pengaturan Vikey yang perlu dikembalikan.")
	} else {
		names := make([]string, len(targets))
		for i, a := range targets {
			names[i] = a.Name()
		}
		ui.Line("Pengaturan %s dikembalikan seperti sebelum memakai Vikey.", strings.Join(names, ", "))
		if ui.Interactive() && !*yes {
			ok, err := ui.Select("Lanjutkan?", "", []ui.Choice{
				{Label: "Ya, kembalikan", Value: "yes"},
				{Label: "Batal", Value: "no"},
			})
			if err != nil {
				return err
			}
			if ok != "yes" {
				return ui.ErrCancelled
			}
		}
	}

	failed := 0
	for _, a := range targets {
		d := a.Detect(ctx)
		if err := a.Unconfigure(ctx, d, st.Tools[a.ID()]); err != nil {
			ui.Fail("%s: %v", a.Name(), err)
			failed++
			continue
		}
		delete(st.Tools, a.ID())
		if err := st.Save(); err != nil {
			return err
		}
		ui.OK("%s dikembalikan", a.Name())
	}
	if _, err := os.Stat(agents.SeparateSettingsPath()); err == nil && st.Tools["claude"] == nil {
		_ = os.Remove(agents.SeparateSettingsPath())
	}

	if len(st.Tools) == 0 && failed == 0 {
		if _, err := pathenv.Remove(st.PathDirs); err != nil {
			ui.Warn("PATH tidak bisa dikembalikan: %v", err)
		}
		st.PathDirs = nil
		_ = st.Save()
		if err := state.RemoveCredentials(); err != nil {
			ui.Warn("API key tersimpan tidak bisa dihapus: %v", err)
		} else {
			ui.OK("API key yang tersimpan di perangkat ini dihapus")
		}
		ui.Blank()
		ui.Line("Vikey CLI masih ada di %s. Hapus folder itu kalau sudah tidak dipakai.", state.Dir())
	}
	ui.Blank()
	if failed > 0 {
		return exitError{code: 1}
	}
	return nil
}

// cmdClaude starts Claude Code with Vikey through --settings, for people who
// keep their own Claude subscription as the default.
func cmdClaude(args []string) error {
	creds, err := state.LoadCredentials()
	if err != nil {
		return err
	}
	if creds == nil {
		return errors.New("belum ada API key. Jalankan perintah pemasangan dari dashboard Vikey dulu")
	}
	c := &agents.Claude{}
	d := c.Detect(context.Background())
	if !d.Installed {
		return errors.New("Claude Code belum terpasang. Jalankan vikey setup untuk memasangnya")
	}
	settings := agents.SeparateSettingsPath()
	if _, err := os.Stat(settings); err != nil {
		return fmt.Errorf("pengaturan Vikey untuk Claude Code belum ada. Jalankan vikey setup lalu pilih pakai lewat perintah vikey claude")
	}
	return runPassthrough(d.Path, append([]string{"--settings", settings}, args...))
}
