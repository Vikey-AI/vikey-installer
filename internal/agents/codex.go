package agents

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/vikey-ai/vikey-cli/internal/edit"
	"github.com/vikey-ai/vikey-cli/internal/state"
)

// Codex is the Codex CLI. Codex Desktop reads the same ~/.codex/config.toml.
//
// The key goes in experimental_bearer_token rather than env_key: apps opened
// from the Dock or Start menu never see variables exported in a shell, and
// with the token in the file the CLI and the desktop app both just work.
// Checked with Codex CLI 0.169.0 on 2026-11-17.
//
// The person's own lines are never moved. A conflicting model or
// model_provider line, or an older [model_providers.vikey] table, is
// commented out where it stands, and Vikey's settings sit in two marked
// blocks at the top and the bottom. Uninstall removes the blocks and
// uncomments the lines, which gives back the file byte for byte.
type Codex struct{}

func (c *Codex) ID() string   { return "codex" }
func (c *Codex) Name() string { return "Codex" }

func codexConfigPath() string {
	dir := os.Getenv("CODEX_HOME")
	if dir == "" {
		dir = readHome(".codex")
	}
	return filepath.Join(dir, "config.toml")
}

func (c *Codex) Detect(ctx context.Context) Detection {
	dirs := []string{readHome(".local", "bin")}
	if isWindows() {
		dirs = append(dirs, filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "OpenAI", "Codex", "bin"))
	}
	d := detect(ctx, "codex", dirs...)
	d.Desktop = codexDesktop(d.Installed)
	return d
}

// codexDesktop finds the Codex desktop app. On macOS it ships as its own
// Codex.app or inside the ChatGPT desktop app. Elsewhere the app has no
// fixed path to look for, but it keeps its settings in ~/.codex like the
// CLI, so without the CLI a ~/.codex that has been used means the app.
func codexDesktop(haveCLI bool) []string {
	if d := macApp("Codex Desktop", "Codex.app", "ChatGPT.app"); d != nil {
		return d
	}
	if isWindows() {
		local := os.Getenv("LOCALAPPDATA")
		for _, p := range []string{
			filepath.Join(local, "Microsoft", "WindowsApps", "ChatGPT.exe"),
			filepath.Join(local, "Programs", "ChatGPT", "ChatGPT.exe"),
			filepath.Join(local, "Programs", "Codex", "Codex.exe"),
		} {
			if fileExists(p) {
				return []string{"Codex Desktop"}
			}
		}
	}
	if haveCLI || (runtime.GOOS != "darwin" && !isWindows()) {
		return nil
	}
	home := filepath.Dir(codexConfigPath())
	for _, name := range []string{"auth.json", "config.toml", "sessions"} {
		if fileExists(filepath.Join(home, name)) {
			return []string{"Codex Desktop"}
		}
	}
	return nil
}

// DesktopIsEnough: Codex Desktop reads the same config.toml, so setup can
// configure it without installing the CLI.
func (c *Codex) DesktopIsEnough() bool { return true }

func (c *Codex) Installer() Installer {
	var in Installer
	if isWindows() {
		in = psInstaller("installer resmi OpenAI", "irm https://chatgpt.com/codex/install.ps1 | iex")
	} else {
		in = shellInstaller("installer resmi OpenAI", "https://chatgpt.com/codex/install.sh", "sh")
	}
	// Without this the installer offers to start Codex in the middle of our setup.
	in.Env = append(in.Env, "CODEX_NON_INTERACTIVE=1")
	return in
}

func (c *Codex) Conflict(prev *state.Tool) string { return "" }

const (
	codexTop      = "vikey"
	codexProvider = "vikey-provider"
	codexNote     = "Diatur oleh Vikey CLI. Kembalikan seperti semula dengan: vikey uninstall"
)

func (c *Codex) Configure(ctx context.Context, d Detection, s Settings, prev *state.Tool) (*state.Tool, []string, error) {
	if err := c.Unconfigure(ctx, d, prev); err != nil {
		return nil, nil, err
	}
	path := codexConfigPath()
	data, existed, err := edit.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	text, crlf := lf(string(data))
	if _, err := edit.ParseTOML(text); err != nil {
		return nil, nil, fmt.Errorf("%s tidak valid (%v). Perbaiki file itu dulu, lalu jalankan setup lagi", path, err)
	}
	f := state.File{Path: path, Created: !existed}
	if existed {
		if f.Backup, err = edit.Backup(path, s.BackupDir); err != nil {
			return nil, nil, err
		}
	}
	// Leftovers of a setup whose state file is gone.
	text = edit.EnableDisabled(unwrapCodex(text))

	body := edit.DisableTopLevelKeys(text, "model", "model_provider")
	body = edit.DisableTables(body, "model_providers.vikey")
	rest, err := edit.ParseTOML(body)
	if err != nil {
		return nil, nil, fmt.Errorf("%s tidak bisa diubah otomatis: %v", path, err)
	}
	if providers, ok := rest["model_providers"].(map[string]any); ok {
		if _, ok := providers["vikey"]; ok {
			return nil, nil, fmt.Errorf("%s mendefinisikan model_providers.vikey dengan format yang tidak dikenali. Hapus bagian itu, lalu jalankan setup lagi", path)
		}
	}

	model := s.Profile.Tools.Codex.Model
	top := edit.MarkedBlock(codexTop, codexNote,
		"model = "+edit.TOMLString(model),
		`model_provider = "vikey"`,
	)
	provider := edit.MarkedBlock(codexProvider, "",
		"[model_providers.vikey]",
		`name = "Vikey"`,
		"base_url = "+edit.TOMLString(trimURL(s.Profile.OpenAIBaseURL)),
		`wire_api = "responses"`,
		// Codex Desktop tries a WebSocket connection to custom providers first.
		"supports_websockets = false",
		"experimental_bearer_token = "+edit.TOMLString(s.APIKey),
	)
	out, added := edit.Wrap(top, body, provider)
	f.TOMLAddedNewline = added

	// Never write a file Codex could not read.
	parsed, err := edit.ParseTOML(out)
	if err != nil {
		return nil, nil, fmt.Errorf("hasil pengaturan Codex tidak valid, file tidak diubah: %v", err)
	}
	if parsed["model"] != model || parsed["model_provider"] != "vikey" {
		return nil, nil, fmt.Errorf("pengaturan Codex tidak terbaca seperti seharusnya, file tidak diubah")
	}
	if err := edit.WriteFile(path, []byte(restoreEOL(out, crlf)), true); err != nil {
		return nil, nil, err
	}
	var notes []string
	if len(d.Desktop) > 0 {
		notes = append(notes, "Kalau Codex Desktop sedang terbuka, tutup penuh aplikasinya (Quit, bukan cuma jendelanya) lalu buka lagi supaya pengaturan Vikey terbaca.")
	}
	if _, ok := rest["profile"]; ok {
		notes = append(notes, "config.toml Codex memakai setelan profile. Kalau profile itu memilih model_provider lain, Codex tidak memakai Vikey.")
	}
	return &state.Tool{Mode: "default", Profile: s.Profile.ID, Files: []state.File{f}}, notes, nil
}

// unwrapCodex removes Vikey's blocks. When Codex rewrote the file and the
// markers are gone, it falls back to removing the values Vikey set.
func unwrapCodex(text string) string {
	out, found := edit.Unwrap(text, codexTop, codexProvider)
	if found {
		return out
	}
	m, err := edit.ParseTOML(text)
	if err != nil || m["model_provider"] != "vikey" {
		return text
	}
	text, _ = edit.TakeTopLevelKeys(text, "model", "model_provider")
	text, _ = edit.TakeTables(text, "model_providers.vikey")
	return text
}

func (c *Codex) Unconfigure(ctx context.Context, d Detection, t *state.Tool) error {
	if t == nil {
		return nil
	}
	for _, f := range t.Files {
		data, existed, err := edit.ReadFile(f.Path)
		if err != nil {
			return err
		}
		if !existed {
			continue
		}
		text, crlf := lf(string(data))
		out := edit.EnableDisabled(unwrapCodex(text))
		if f.TOMLAddedNewline {
			out = strings.TrimSuffix(out, "\n")
		}
		if strings.TrimSpace(out) == "" && f.Created {
			if err := os.Remove(f.Path); err != nil && !os.IsNotExist(err) {
				return err
			}
			continue
		}
		if _, err := edit.ParseTOML(out); err != nil {
			return fmt.Errorf("%s tidak bisa dikembalikan otomatis: %v. Salinan aslinya ada di %s", f.Path, err, f.Backup)
		}
		if err := edit.WriteFile(f.Path, []byte(restoreEOL(out, crlf)), false); err != nil {
			return err
		}
	}
	return nil
}

// lf works on "\n" line ends. A file written on Windows keeps its "\r\n".
func lf(text string) (string, bool) {
	if strings.Contains(text, "\r\n") {
		return strings.ReplaceAll(text, "\r\n", "\n"), true
	}
	return text, false
}

func restoreEOL(text string, crlf bool) string {
	if crlf {
		return strings.ReplaceAll(text, "\n", "\r\n")
	}
	return text
}

func (c *Codex) Inspect(ctx context.Context, d Detection) Status {
	data, existed, _ := edit.ReadFile(codexConfigPath())
	if !existed {
		return Status{}
	}
	m, err := edit.ParseTOML(string(data))
	if err != nil {
		return Status{Note: "config.toml tidak valid"}
	}
	st := Status{}
	st.Model, _ = m["model"].(string)
	provider, _ := m["model_provider"].(string)
	if providers, ok := m["model_providers"].(map[string]any); ok {
		if p, ok := providers[provider].(map[string]any); ok {
			st.BaseURL, _ = p["base_url"].(string)
		}
	}
	st.Configured = provider == "vikey" && st.BaseURL != ""
	return st
}

func (c *Codex) RunHint(string) string { return "codex" }

// DesktopHint is shown instead of RunHint when only the desktop app is here.
func (c *Codex) DesktopHint() string { return "buka Codex Desktop" }
