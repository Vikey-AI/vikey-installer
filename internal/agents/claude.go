package agents

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Vikey-AI/vikey-installer/internal/edit"
	"github.com/Vikey-AI/vikey-installer/internal/state"
)

// Claude is Claude Code. Its user settings file (~/.claude/settings.json) is
// also read by the VS Code and JetBrains extensions and by the Claude desktop
// app, so one change covers all of them.
//
// Checked with Claude Code 2.1.326 on 2026-11-17: with ANTHROPIC_AUTH_TOKEN in
// the env block the first start skips the login screen, and the token wins
// over a claude.ai login.
type Claude struct{}

func (c *Claude) ID() string   { return "claude" }
func (c *Claude) Name() string { return "Claude Code" }

func claudeDir() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	return readHome(".claude")
}

func claudeSettingsPath() string { return filepath.Join(claudeDir(), "settings.json") }

// SeparateSettingsPath is the settings file `vikey claude` passes to Claude
// Code with --settings, so the person's own files stay untouched.
func SeparateSettingsPath() string { return filepath.Join(state.Dir(), "claude-settings.json") }

func (c *Claude) Detect(ctx context.Context) Detection {
	d := detect(ctx, "claude", readHome(".local", "bin"), readHome(".claude", "local"))
	d.Desktop = macApp("Claude Desktop", "Claude.app")
	return d
}

func (c *Claude) Installer() Installer {
	if isWindows() {
		return psInstaller("installer resmi Anthropic", "irm https://claude.ai/install.ps1 | iex")
	}
	return shellInstaller("installer resmi Anthropic", "https://claude.ai/install.sh", "bash")
}

func (c *Claude) Conflict(prev *state.Tool) string {
	if prev != nil && prev.Mode == "default" {
		return ""
	}
	// ~/.claude.json holds the claude.ai account after a login.
	if data, err := os.ReadFile(claudeAccountFile()); err == nil {
		var acct struct {
			OAuthAccount json.RawMessage `json:"oauthAccount"`
		}
		if json.Unmarshal(data, &acct) == nil && len(acct.OAuthAccount) > 0 && string(acct.OAuthAccount) != "null" {
			return "Claude Code di laptop ini sudah login dengan akun Claude. Kalau Vikey dijadikan default, langganan Claude kamu tidak dipakai sampai kamu menjalankan vikey uninstall."
		}
	}
	if data, _, _ := edit.ReadFile(claudeSettingsPath()); data != nil {
		if doc, err := edit.ParseJSON(data); err == nil {
			if base := doc.GetString(edit.Ptr("env", "ANTHROPIC_BASE_URL")); base != "" && !strings.Contains(base, "vikey") {
				return "Claude Code sudah diarahkan ke " + base + ". Kalau Vikey dijadikan default, pengaturan itu diganti (salinannya disimpan dan bisa dikembalikan dengan vikey uninstall)."
			}
		}
	}
	return ""
}

func claudeAccountFile() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return filepath.Join(d, ".claude.json")
	}
	return readHome(".claude.json")
}

// claudeValues is the settings Vikey needs, as a flat list of JSON changes.
func claudeValues(s Settings) []jsonOp {
	cs := s.Profile.Tools.Claude
	ops := []jsonOp{
		{ptr: edit.Ptr("model"), value: cs.Model},
		{ptr: edit.Ptr("env", "ANTHROPIC_BASE_URL"), value: trimURL(s.Profile.AnthropicBaseURL)},
		{ptr: edit.Ptr("env", "ANTHROPIC_AUTH_TOKEN"), value: s.APIKey},
		// Two credentials at once make Claude Code warn and pick one.
		{ptr: edit.Ptr("env", "ANTHROPIC_API_KEY"), del: true},
	}
	for _, k := range sortedKeys(cs.Env) {
		ops = append(ops, jsonOp{ptr: edit.Ptr("env", k), value: cs.Env[k]})
	}
	for _, k := range sortedKeys(cs.ModelOverrides) {
		ops = append(ops, jsonOp{ptr: edit.Ptr("modelOverrides", k), value: cs.ModelOverrides[k]})
	}
	return ops
}

func (c *Claude) Configure(ctx context.Context, d Detection, s Settings, prev *state.Tool) (*state.Tool, []string, error) {
	if err := c.Unconfigure(ctx, d, prev); err != nil {
		return nil, nil, err
	}
	t := &state.Tool{Mode: s.Mode, Profile: s.Profile.ID}
	var notes []string
	if s.Mode == "separate" {
		f, err := applyJSON(SeparateSettingsPath(), claudeValues(s), s.BackupDir, true)
		if err != nil {
			return nil, nil, err
		}
		t.Files = append(t.Files, f)
		return t, notes, nil
	}
	f, err := applyJSON(claudeSettingsPath(), claudeValues(s), s.BackupDir, true)
	if err != nil {
		return nil, nil, err
	}
	t.Files = append(t.Files, f)
	for _, name := range []string{"ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_API_KEY", "ANTHROPIC_MODEL"} {
		if os.Getenv(name) != "" {
			notes = append(notes, "Terminal ini punya variabel "+name+" sendiri. Hapus export-nya dari file shell kamu supaya tidak bentrok dengan pengaturan Vikey.")
		}
	}
	return t, notes, nil
}

func (c *Claude) Unconfigure(ctx context.Context, d Detection, t *state.Tool) error {
	return undoFiles(t)
}

func (c *Claude) Inspect(ctx context.Context, d Detection) Status {
	data, _, _ := edit.ReadFile(claudeSettingsPath())
	if data == nil {
		return Status{}
	}
	doc, err := edit.ParseJSON(data)
	if err != nil {
		return Status{Note: "settings.json tidak valid"}
	}
	base := doc.GetString(edit.Ptr("env", "ANTHROPIC_BASE_URL"))
	return Status{
		// doctor compares BaseURL with the saved profile, this is the fallback guess.
		Configured: strings.Contains(base, "vikey") && doc.GetString(edit.Ptr("env", "ANTHROPIC_AUTH_TOKEN")) != "",
		BaseURL:    base,
		Model:      doc.GetString(edit.Ptr("model")),
	}
}

func (c *Claude) RunHint(mode string) string {
	if mode == "separate" {
		return "vikey claude"
	}
	return "claude"
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
