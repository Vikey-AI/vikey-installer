package agents

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Vikey-AI/vikey-installer/internal/edit"
	"github.com/Vikey-AI/vikey-installer/internal/state"
)

// Hermes is Hermes Agent. The CLI and Hermes Desktop share one data folder,
// so one change covers both.
//
// Its config.yaml is a 130 KB commented file, so settings go through Hermes's
// own `hermes config set` (which validates keys) instead of a YAML rewrite.
// The key goes in Hermes's .env, which Hermes loads itself, so it also works
// in Desktop where shell variables are not seen. Checked with Hermes Agent
// 0.23.2 on 2026-11-17.
type Hermes struct{}

func (h *Hermes) ID() string   { return "hermes" }
func (h *Hermes) Name() string { return "Hermes" }

const hermesKeyEnv = "VIKEY_API_KEY"

func hermesHome() string {
	if d := os.Getenv("HERMES_HOME"); d != "" {
		return d
	}
	if isWindows() {
		local := filepath.Join(os.Getenv("LOCALAPPDATA"), "hermes")
		if _, err := os.Stat(filepath.Join(local, "config.yaml")); err == nil {
			return local
		}
		if old := readHome(".hermes"); fileExists(filepath.Join(old, "config.yaml")) {
			return old
		}
		return local
	}
	return readHome(".hermes")
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func (h *Hermes) Detect(ctx context.Context) Detection {
	dirs := []string{readHome(".local", "bin")}
	if isWindows() {
		dirs = append(dirs, filepath.Join(os.Getenv("LOCALAPPDATA"), "hermes", "bin"))
	}
	d := detect(ctx, "hermes", dirs...)
	d.Desktop = macApp("Hermes Desktop", "Hermes.app", "Hermes Desktop.app", "Hermes Agent.app")
	return d
}

func (h *Hermes) Installer() Installer {
	if isWindows() {
		// -NonInteractive skips Hermes's own provider wizard, Vikey sets the provider.
		return psInstaller("installer resmi Nous Research",
			"& ([scriptblock]::Create((irm https://hermes-agent.nousresearch.com/install.ps1))) -NonInteractive")
	}
	return shellInstaller("installer resmi Nous Research", "https://hermes-agent.nousresearch.com/install.sh", "bash", "--non-interactive")
}

func (h *Hermes) Conflict(prev *state.Tool) string { return "" }

// hermesValues are the settings in the order they are written.
func hermesValues(s Settings) [][2]string {
	base := trimURL(s.Profile.OpenAIBaseURL)
	model := s.Profile.Tools.Hermes.Model
	return [][2]string{
		{"model.default", model},
		{"model.provider", "vikey"},
		{"model.base_url", base},
		{"model.key_env", hermesKeyEnv},
		{"model.api_mode", "chat_completions"},
		{"providers.vikey.name", "Vikey"},
		{"providers.vikey.base_url", base},
		{"providers.vikey.key_env", hermesKeyEnv},
		{"providers.vikey.model", model},
	}
}

// hermesConfig reads config.yaml into a generic tree.
func hermesConfig() map[string]any {
	data, err := os.ReadFile(filepath.Join(hermesHome(), "config.yaml"))
	if err != nil {
		return nil
	}
	var m map[string]any
	if yaml.Unmarshal(data, &m) != nil {
		return nil
	}
	return m
}

func lookup(m map[string]any, dotted string) (any, bool) {
	var cur any = m
	for _, part := range strings.Split(dotted, ".") {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		if cur, ok = mm[part]; !ok {
			return nil, false
		}
	}
	return cur, true
}

func (h *Hermes) hermes(ctx context.Context, d Detection, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, d.Path, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("hermes %s gagal: %v\n%s", strings.Join(args, " "), err, strings.TrimSpace(out.String()))
	}
	return nil
}

func (h *Hermes) Configure(ctx context.Context, d Detection, s Settings, prev *state.Tool) (*state.Tool, []string, error) {
	if !d.Installed {
		return nil, nil, fmt.Errorf("perintah hermes tidak ditemukan")
	}
	if err := h.Unconfigure(ctx, d, prev); err != nil {
		return nil, nil, err
	}
	t := &state.Tool{Mode: "default", Profile: s.Profile.ID}
	// Missing before Hermes's first run; `hermes config set` creates it.
	cfg := hermesConfig()
	if backup, err := edit.Backup(filepath.Join(hermesHome(), "config.yaml"), s.BackupDir); err == nil && backup != "" {
		t.Files = append(t.Files, state.File{Path: filepath.Join(hermesHome(), "config.yaml"), Backup: backup})
	}

	// The key first, so Hermes never points at Vikey without one.
	envPath := filepath.Join(hermesHome(), ".env")
	data, _, err := edit.ReadFile(envPath)
	if err != nil {
		return nil, nil, err
	}
	text, prevLine := edit.SetEnvLine(string(data), hermesKeyEnv, s.APIKey)
	if err := edit.WriteFile(envPath, []byte(text), true); err != nil {
		return nil, nil, err
	}
	t.Files = append(t.Files, state.File{Path: envPath, EnvPrev: map[string]string{hermesKeyEnv: prevLine}})

	_, hadProvider := lookup(cfg, "providers.vikey")
	t.Hermes = append(t.Hermes, state.HermesKey{Key: "providers.vikey", Existed: hadProvider})
	for _, kv := range hermesValues(s) {
		prev, existed := lookup(cfg, kv[0])
		hk := state.HermesKey{Key: kv[0], Existed: existed && prev != nil}
		if hk.Existed {
			hk.Prev = fmt.Sprint(prev)
		}
		if err := h.hermes(ctx, d, "config", "set", kv[0], kv[1]); err != nil {
			t.Hermes = append(t.Hermes, hk)
			_ = h.Unconfigure(ctx, d, t)
			return nil, nil, err
		}
		t.Hermes = append(t.Hermes, hk)
	}
	return t, nil, nil
}

func (h *Hermes) Unconfigure(ctx context.Context, d Detection, t *state.Tool) error {
	if t == nil {
		return nil
	}
	if len(t.Hermes) > 0 && !d.Installed {
		return fmt.Errorf("perintah hermes tidak ditemukan, pengaturan Hermes tidak bisa dikembalikan otomatis")
	}
	for i := len(t.Hermes) - 1; i >= 0; i-- {
		k := t.Hermes[i]
		var err error
		switch {
		case k.Key == "providers.vikey":
			if !k.Existed {
				err = h.hermes(ctx, d, "config", "unset", k.Key)
			}
		case k.Existed:
			err = h.hermes(ctx, d, "config", "set", k.Key, k.Prev)
		case strings.HasPrefix(k.Key, "providers.vikey."):
			// Removed together with providers.vikey when Vikey added it.
		default:
			err = h.hermes(ctx, d, "config", "unset", k.Key)
		}
		if err != nil {
			return err
		}
	}
	for _, f := range t.Files {
		if f.EnvPrev == nil {
			continue
		}
		data, existed, err := edit.ReadFile(f.Path)
		if err != nil || !existed {
			continue
		}
		text := string(data)
		for name, prev := range f.EnvPrev {
			text = edit.RestoreEnvLine(text, name, prev)
		}
		if err := edit.WriteFile(f.Path, []byte(text), true); err != nil {
			return err
		}
	}
	return nil
}

func (h *Hermes) Inspect(ctx context.Context, d Detection) Status {
	cfg := hermesConfig()
	if cfg == nil {
		return Status{}
	}
	provider, _ := lookup(cfg, "model.provider")
	base, _ := lookup(cfg, "model.base_url")
	model, _ := lookup(cfg, "model.default")
	st := Status{BaseURL: fmt.Sprint(nilToEmpty(base)), Model: fmt.Sprint(nilToEmpty(model))}
	st.Configured = provider == "vikey" && st.BaseURL != ""
	return st
}

func nilToEmpty(v any) any {
	if v == nil {
		return ""
	}
	return v
}

func (h *Hermes) RunHint(string) string { return "hermes" }
