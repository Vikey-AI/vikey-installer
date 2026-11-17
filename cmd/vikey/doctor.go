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
	"github.com/vikey-ai/vikey-cli/internal/panel"
	"github.com/vikey-ai/vikey-cli/internal/state"
	"github.com/vikey-ai/vikey-cli/internal/ui"
)

func cmdDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	ui.Blank()
	ui.Title("Vikey Doctor")
	ui.Line(ui.Dim("Vikey CLI %s, data di %s"), version, state.Dir())
	ui.Blank()

	creds, err := state.LoadCredentials()
	if err != nil {
		return err
	}
	problems := 0
	if creds == nil {
		ui.Warn("Belum ada API key. Jalankan perintah pemasangan dari dashboard Vikey.")
		problems++
	} else {
		use := "Pay as you go"
		if creds.Profile.ID == "plan" {
			use = "Token Plan"
		}
		ui.Line("API key  %s (%s)", creds.KeyName, creds.KeyPrev)
		ui.Line("Dipakai  %s lewat %s", use, creds.Profile.OpenAIBaseURL)
		if err := panel.CheckKey(ctx, creds.Profile.OpenAIBaseURL, creds.APIKey, version); err != nil {
			ui.Fail("Koneksi ke Vikey gagal: %v", err)
			problems++
		} else {
			ui.OK("Koneksi ke Vikey berhasil")
		}
	}

	st, err := state.Load()
	if err != nil {
		return err
	}
	ui.Blank()
	for _, a := range agents.All() {
		d := a.Detect(ctx)
		t := st.Tools[a.ID()]
		switch {
		case !agents.Usable(a, d):
			ui.Line("  %-13s %s", a.Name(), ui.Dim("belum terpasang"))
			continue
		case t != nil && t.Mode == "separate":
			ui.OK("%-13s %s, dipakai lewat vikey claude", a.Name(), d.Version)
			continue
		}
		if d.Version == "" {
			d.Version = strings.Join(d.Desktop, " dan ")
		}
		s := a.Inspect(ctx, d)
		desktop := ""
		if len(d.Desktop) > 0 {
			desktop = ", juga " + strings.Join(d.Desktop, " dan ")
		}
		matches := creds != nil && s.BaseURL != "" && sameBase(s.BaseURL, creds.Profile)
		switch {
		case matches || (s.Configured && creds == nil):
			ui.OK("%-13s %s, memakai Vikey, model %s%s", a.Name(), d.Version, s.Model, desktop)
		case s.Configured:
			ui.Warn("%-13s %s, memakai Vikey di %s, bukan %s. Jalankan vikey setup lagi%s", a.Name(), d.Version, s.BaseURL, creds.Profile.OpenAIBaseURL, desktop)
			problems++
		case t != nil:
			ui.Warn("%-13s %s, pengaturan Vikey sudah tidak ada. Jalankan vikey setup lagi%s", a.Name(), d.Version, desktop)
			problems++
		default:
			ui.Line("  %-13s %s", a.Name(), ui.Dim(fmt.Sprintf("%s, belum memakai Vikey%s", d.Version, desktop)))
		}
		if s.Note != "" {
			ui.Warn("%s: %s", a.Name(), s.Note)
		}
	}
	ui.Blank()
	if problems > 0 {
		return exitError{code: 1}
	}
	return nil
}

func sameBase(base string, p panel.Profile) bool {
	b := strings.TrimRight(base, "/")
	return b == strings.TrimRight(p.OpenAIBaseURL, "/") || b == strings.TrimRight(p.AnthropicBaseURL, "/")
}
