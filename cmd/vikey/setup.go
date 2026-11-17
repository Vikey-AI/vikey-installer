package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/vikey-ai/vikey-cli/internal/agents"
	"github.com/vikey-ai/vikey-cli/internal/panel"
	"github.com/vikey-ai/vikey-cli/internal/pathenv"
	"github.com/vikey-ai/vikey-cli/internal/state"
	"github.com/vikey-ai/vikey-cli/internal/ui"
)

type setupFlags struct {
	code       string
	panelURL   string
	yes        bool
	tools      string
	use        string
	claudeMode string
	noInstall  bool
}

func cmdSetup(args []string) error {
	var f setupFlags
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	fs.StringVar(&f.code, "code", "", "kode pemasangan dari dashboard Vikey")
	fs.StringVar(&f.panelURL, "panel-url", panel.DefaultURL, "alamat panel Vikey")
	fs.BoolVar(&f.yes, "yes", false, "jalan tanpa bertanya, pakai pilihan bawaan")
	fs.StringVar(&f.tools, "tools", "", "agent yang diatur, contoh claude,codex,opencode,hermes atau all")
	fs.StringVar(&f.use, "use", "", "plan untuk Token Plan, payg untuk Pay as you go")
	fs.StringVar(&f.claudeMode, "claude-mode", "", "default, atau separate untuk memakai lewat vikey claude saja")
	fs.BoolVar(&f.noInstall, "no-install", false, "jangan memasang agent yang belum ada")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	interactive := ui.Interactive() && !f.yes

	ui.Blank()
	ui.Title("Vikey Setup")
	ui.Line(ui.Dim("Atur coding agent di perangkat ini supaya memakai Vikey."))
	ui.Blank()

	st, err := state.Load()
	if err != nil {
		return fmt.Errorf("state Vikey di %s tidak bisa dibaca: %v", state.Dir(), err)
	}

	// Choose the agents before using the code, so stopping here keeps the code valid.
	ui.Step("Memeriksa coding agent di perangkat ini")
	all := agents.All()
	found := detectAll(ctx, all)
	chosen, err := chooseAgents(ctx, all, found, f, interactive)
	if err != nil {
		return err
	}
	if len(chosen) == 0 {
		ui.Line("Tidak ada agent yang dipilih. Jalankan perintah ini lagi kalau berubah pikiran.")
		return nil
	}

	creds, err := credentials(ctx, f, interactive)
	if err != nil {
		return err
	}
	profile, err := chooseProfile(creds, f, interactive)
	if err != nil {
		return err
	}
	creds.Profile = profile
	if err := state.SaveCredentials(creds); err != nil {
		return fmt.Errorf("API key tidak bisa disimpan di %s: %v", state.Dir(), err)
	}

	backupDir := state.BackupDir(time.Now())
	var done []doneAgent
	var pathDirs []string
	installedNow := false
	for _, a := range chosen {
		ui.Blank()
		d := found[a.ID()]
		if !agents.Usable(a, d) {
			if f.noInstall {
				ui.Warn("%s belum terpasang, dilewati.", a.Name())
				continue
			}
			ok, err := install(ctx, a)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			installedNow = true
			d = a.Detect(ctx)
			if !d.Installed {
				ui.Fail("%s sudah dipasang tapi perintahnya belum ditemukan. Buka terminal baru lalu jalankan vikey setup lagi.", a.Name())
				continue
			}
			ui.OK("%s %s terpasang", a.Name(), d.Version)
		}

		mode, err := chooseMode(a, st.Tools[a.ID()], f, interactive)
		if err != nil {
			return err
		}
		ui.Step("Mengatur %s", a.Name())
		t, notes, err := a.Configure(ctx, d, agents.Settings{
			APIKey: creds.APIKey, Profile: profile, Mode: mode, BackupDir: filepath.Join(backupDir, a.ID()),
		}, st.Tools[a.ID()])
		if err != nil {
			ui.Fail("%s tidak bisa diatur: %v", a.Name(), err)
			continue
		}
		t.ConfiguredAt = time.Now().UTC()
		st.Tools[a.ID()] = t
		if err := st.Save(); err != nil {
			return fmt.Errorf("state Vikey tidak bisa disimpan: %v", err)
		}
		ui.OK("%s memakai Vikey", a.Name())
		for _, n := range notes {
			ui.Warn("%s", n)
		}
		done = append(done, doneAgent{agent: a, mode: mode, desktopOnly: !d.Installed})
		if dir := filepath.Dir(d.Path); d.Path != "" && !pathenv.OnPath(dir) {
			pathDirs = append(pathDirs, dir)
		}
	}

	// The vikey command itself, for doctor, uninstall and `vikey claude`.
	if exe, err := os.Executable(); err == nil {
		if dir := filepath.Dir(exe); samePath(dir, state.BinDir()) && !pathenv.OnPath(dir) {
			pathDirs = append([]string{dir}, pathDirs...)
		}
	}
	pathChanged := false
	if len(pathDirs) > 0 {
		st.PathDirs = uniq(append(st.PathDirs, pathDirs...))
		changed, err := pathenv.Ensure(st.PathDirs)
		if err != nil {
			ui.Warn("PATH tidak bisa diperbarui: %v", err)
		}
		pathChanged = len(changed) > 0
		_ = st.Save()
	}

	ui.Blank()
	if err := panel.CheckKey(ctx, profile.OpenAIBaseURL, creds.APIKey, version); err != nil {
		ui.Warn("Cek koneksi ke Vikey gagal: %v", err)
	} else {
		ui.OK("Koneksi ke Vikey berhasil")
	}

	summary(done, profile, backupDir, pathChanged || installedNow)
	if len(done) < len(chosen) {
		return exitError{code: 1}
	}
	return nil
}

type doneAgent struct {
	agent       agents.Agent
	mode        string
	desktopOnly bool
}

func detectAll(ctx context.Context, all []agents.Agent) map[string]agents.Detection {
	found := make(map[string]agents.Detection, len(all))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, a := range all {
		wg.Add(1)
		go func(a agents.Agent) {
			defer wg.Done()
			d := a.Detect(ctx)
			mu.Lock()
			found[a.ID()] = d
			mu.Unlock()
		}(a)
	}
	wg.Wait()
	return found
}

func statusLabel(ctx context.Context, a agents.Agent, d agents.Detection) string {
	var parts []string
	if d.Installed || agents.Usable(a, d) {
		s := strings.Join(d.Desktop, " dan ") + " terpasang"
		if d.Installed {
			s = "terpasang"
			if d.Version != "" {
				s += ", versi " + d.Version
			}
		}
		parts = append(parts, s)
		if a.Inspect(ctx, d).Configured {
			parts = append(parts, "sudah memakai Vikey")
		}
		if !d.Installed {
			return strings.Join(parts, ", ")
		}
	} else if in := a.Installer(); in.Manual != "" {
		parts = append(parts, "belum terpasang, perlu dipasang manual")
	} else {
		parts = append(parts, "belum terpasang, akan dipasang")
	}
	if len(d.Desktop) > 0 {
		parts = append(parts, "juga "+strings.Join(d.Desktop, " dan "))
	}
	return strings.Join(parts, ", ")
}

func chooseAgents(ctx context.Context, all []agents.Agent, found map[string]agents.Detection, f setupFlags, interactive bool) ([]agents.Agent, error) {
	if f.tools != "" {
		if f.tools == "all" {
			return all, nil
		}
		var out []agents.Agent
		for _, id := range strings.Split(f.tools, ",") {
			a := agents.ByID(strings.TrimSpace(id))
			if a == nil {
				return nil, fmt.Errorf("agent %q tidak dikenal. Pilihan: claude, codex, opencode, hermes", id)
			}
			out = append(out, a)
		}
		return out, nil
	}

	anyInstalled := false
	for _, a := range all {
		anyInstalled = anyInstalled || agents.Usable(a, found[a.ID()])
	}
	if !interactive {
		// Without a terminal only agents that are already here are set up.
		var out []agents.Agent
		for _, a := range all {
			if agents.Usable(a, found[a.ID()]) {
				out = append(out, a)
			}
		}
		for _, a := range out {
			ui.Line("  %-13s %s", a.Name(), statusLabel(ctx, a, found[a.ID()]))
		}
		return out, nil
	}

	choices := make([]ui.Choice, 0, len(all))
	for i, a := range all {
		d := found[a.ID()]
		choices = append(choices, ui.Choice{
			Label:    fmt.Sprintf("%-13s %s", a.Name(), ui.Dim(statusLabel(ctx, a, d))),
			Value:    a.ID(),
			Selected: agents.Usable(a, d) || (!anyInstalled && i == 0),
		})
	}
	ids, err := ui.MultiSelect(
		"Pilih coding agent yang mau memakai Vikey",
		"Yang belum terpasang dipasang dulu lewat installer resmi pembuatnya.",
		choices,
	)
	if err != nil {
		return nil, err
	}
	var out []agents.Agent
	for _, id := range ids {
		out = append(out, agents.ByID(id))
	}
	return out, nil
}

var codeRe = regexp.MustCompile(`(?i)(?:/i/)?\b([0-9A-HJKMNP-TV-Z]{8})\b`)

// credentials trades the one-time code for the key, or reuses the key saved
// by an earlier setup when no code was given.
func credentials(ctx context.Context, f setupFlags, interactive bool) (*state.Credentials, error) {
	code := strings.TrimSpace(f.code)
	if code == "" {
		saved, err := state.LoadCredentials()
		if err != nil {
			return nil, err
		}
		if saved != nil && saved.APIKey != "" {
			use := "saved"
			if interactive {
				use, err = ui.Select("API key mana yang dipakai?", "", []ui.Choice{
					{Label: fmt.Sprintf("Yang tersimpan, %s (%s)", saved.KeyName, saved.KeyPrev), Value: "saved"},
					{Label: "Kode baru dari dashboard Vikey", Value: "new"},
				})
				if err != nil {
					return nil, err
				}
			}
			if use == "saved" {
				ui.OK("Memakai API key %s (%s)", saved.KeyName, saved.KeyPrev)
				return saved, nil
			}
		}
		if !interactive {
			return nil, errors.New("butuh kode pemasangan. Buka halaman API Key di dashboard Vikey, pilih Pasang ke coding agent, lalu jalankan perintah yang muncul")
		}
		input, err := ui.Input(
			"Tempel kode pemasangan",
			"Ada di dashboard Vikey, halaman API Key, tombol Pasang ke coding agent.",
			"contoh K7QX9P2M",
			func(s string) error {
				if codeRe.FindStringSubmatch(strings.TrimSpace(s)) == nil {
					return errors.New("kode terdiri dari 8 huruf dan angka")
				}
				return nil
			},
		)
		if err != nil {
			return nil, err
		}
		code = input
	}
	m := codeRe.FindStringSubmatch(code)
	if m == nil {
		return nil, errors.New("kode pemasangan tidak dikenali. Kode terdiri dari 8 huruf dan angka")
	}

	ui.Step("Menghubungkan ke Vikey")
	inst, err := panel.Exchange(ctx, f.panelURL, strings.ToUpper(m[1]), version)
	if err != nil {
		return nil, err
	}
	ui.OK("Terhubung dengan API key %s (%s)", inst.Key.Name, inst.Key.Preview)
	creds := &state.Credentials{
		APIKey:   inst.APIKey,
		KeyName:  inst.Key.Name,
		KeyPrev:  inst.Key.Preview,
		Profile:  inst.Profiles[0],
		Profiles: inst.Profiles,
	}
	if inst.Plan.Active {
		creds.PlanName = inst.Plan.Name
		creds.PlanUntil = inst.Plan.ExpiresAt
	}
	// Saved now: the code is spent, so a stop after this point must not lose the key.
	if err := state.SaveCredentials(creds); err != nil {
		return nil, fmt.Errorf("API key tidak bisa disimpan di %s: %v", state.Dir(), err)
	}
	return creds, nil
}

var months = []string{"Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}

func tanggal(t time.Time) string {
	t = t.Local()
	return fmt.Sprintf("%d %s %d", t.Day(), months[t.Month()-1], t.Year())
}

func chooseProfile(creds *state.Credentials, f setupFlags, interactive bool) (panel.Profile, error) {
	profiles := creds.Profiles
	if len(profiles) == 0 {
		profiles = []panel.Profile{creds.Profile}
	}
	if f.use != "" {
		for _, p := range profiles {
			if p.ID == f.use {
				return p, nil
			}
		}
		if f.use == "plan" {
			return panel.Profile{}, errors.New("akun ini tidak punya Token Plan yang aktif")
		}
		return panel.Profile{}, fmt.Errorf("pilihan --use %q tidak dikenal. Pakai plan atau payg", f.use)
	}
	if len(profiles) == 1 || !interactive {
		return profiles[0], nil
	}
	choices := make([]ui.Choice, 0, len(profiles))
	for _, p := range profiles {
		label := "Pay as you go, dipotong dari saldo sesuai pemakaian"
		if p.ID == "plan" {
			label = "Token Plan"
			switch {
			case creds.PlanName != "" && !creds.PlanUntil.IsZero():
				label += fmt.Sprintf(", %s aktif sampai %s", creds.PlanName, tanggal(creds.PlanUntil))
			case creds.PlanName != "":
				label += ", " + creds.PlanName
			}
		}
		choices = append(choices, ui.Choice{Label: label, Value: p.ID})
	}
	id, err := ui.Select("Kamu punya Token Plan aktif. Mau pakai yang mana?",
		"Token Plan memakai kuota paket dengan model vikey/plan. Pay as you go memotong saldo dan bisa memakai semua model.",
		choices)
	if err != nil {
		return panel.Profile{}, err
	}
	for _, p := range profiles {
		if p.ID == id {
			return p, nil
		}
	}
	return profiles[0], nil
}

func chooseMode(a agents.Agent, prev *state.Tool, f setupFlags, interactive bool) (string, error) {
	if a.ID() != "claude" {
		return "default", nil
	}
	if f.claudeMode == "default" || f.claudeMode == "separate" {
		return f.claudeMode, nil
	}
	if f.claudeMode != "" {
		return "", fmt.Errorf("pilihan --claude-mode %q tidak dikenal. Pakai default atau separate", f.claudeMode)
	}
	if prev != nil && prev.Mode == "separate" && !interactive {
		return "separate", nil
	}
	conflict := a.Conflict(prev)
	if conflict == "" || !interactive {
		if conflict != "" {
			ui.Warn("%s", conflict)
		}
		return "default", nil
	}
	return ui.Select(conflict, "", []ui.Choice{
		{Label: "Jadikan Vikey default untuk Claude Code", Value: "default"},
		{Label: "Pakai Vikey hanya lewat perintah vikey claude", Value: "separate"},
	})
}

func install(ctx context.Context, a agents.Agent) (bool, error) {
	in := a.Installer()
	if in.Manual != "" {
		ui.Warn("%s belum terpasang. %s", a.Name(), in.Manual)
		return false, nil
	}
	ui.Step("Memasang %s lewat %s", a.Name(), in.Vendor)
	ui.Indent(in.Display)
	cmd := exec.CommandContext(ctx, in.Argv[0], in.Argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(), in.Env...)
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return false, ui.ErrCancelled
		}
		ui.Fail("Pemasangan %s gagal (%v). Coba jalankan perintah di atas sendiri, lalu vikey setup lagi.", a.Name(), err)
		return false, nil
	}
	return true, nil
}

func summary(done []doneAgent, p panel.Profile, backupDir string, newTerminal bool) {
	if len(done) == 0 {
		ui.Blank()
		ui.Line("Belum ada agent yang berhasil diatur.")
		return
	}
	ui.Blank()
	ui.Title("Selesai")
	ui.Line("Agent yang sekarang memakai Vikey:")
	for _, d := range done {
		model := ""
		switch d.agent.ID() {
		case "claude":
			model = p.Tools.Claude.Model
		case "codex":
			model = p.Tools.Codex.Model
		case "opencode":
			model = p.Tools.OpenCode.Model
		case "hermes":
			model = p.Tools.Hermes.Model
		}
		// Pad before styling: escape codes would count toward the width.
		hint := "jalankan " + d.agent.RunHint(d.mode)
		if do, ok := d.agent.(agents.DesktopOnly); ok && d.desktopOnly {
			hint = do.DesktopHint()
		}
		ui.Line("  %-13s %s %s", d.agent.Name(), ui.Code(fmt.Sprintf("%-22s", hint)), ui.Dim("model "+model))
	}
	ui.Blank()
	if p.ID == "plan" {
		ui.Line("Semua agent memakai Token Plan dengan model vikey/plan.")
	} else {
		ui.Line("Model bisa diganti dari dalam agent, misalnya dengan /model.")
	}
	if entries, err := os.ReadDir(backupDir); err == nil && len(entries) > 0 {
		ui.Line("Salinan pengaturan lama: %s", backupDir)
	}
	ui.Line("Kembalikan seperti semula: %s", ui.Code("vikey uninstall"))
	if newTerminal {
		ui.Blank()
		if runtime.GOOS == "windows" {
			ui.Warn("Buka jendela PowerShell baru dulu supaya perintah di atas dikenali.")
		} else {
			ui.Warn("Buka terminal baru dulu supaya perintah di atas dikenali.")
		}
	}
	ui.Blank()
}

func uniq(list []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range list {
		k := s
		if runtime.GOOS == "windows" {
			k = strings.ToLower(s)
		}
		if !seen[k] {
			seen[k] = true
			out = append(out, s)
		}
	}
	return out
}

func samePath(a, b string) bool {
	ca, _ := filepath.Abs(a)
	cb, _ := filepath.Abs(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(ca, cb)
	}
	return ca == cb
}
