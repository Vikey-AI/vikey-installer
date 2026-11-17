package agents

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/vikey-ai/vikey-cli/internal/edit"
	"github.com/vikey-ai/vikey-cli/internal/state"
)

// OpenCode is opencode. It uses the "provider" config format, which both
// opencode 1.19.6 and 2.3.0 read (2.x marks it deprecated in favour of
// "providers", but 1.x does not read the new one). Checked 2026-11-17.
type OpenCode struct{}

func (o *OpenCode) ID() string   { return "opencode" }
func (o *OpenCode) Name() string { return "opencode" }

func opencodeDir() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" && !isWindows() {
		return filepath.Join(x, "opencode")
	}
	return readHome(".config", "opencode")
}

// opencodeConfigPath picks the file opencode already uses, or opencode.json.
func opencodeConfigPath() string {
	dir := opencodeDir()
	for _, name := range []string{"opencode.jsonc", "opencode.json"} {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return filepath.Join(dir, "opencode.json")
}

func (o *OpenCode) Detect(ctx context.Context) Detection {
	return detect(ctx, "opencode", readHome(".opencode", "bin"))
}

func (o *OpenCode) Installer() Installer {
	if !isWindows() {
		return shellInstaller("installer resmi opencode", "https://opencode.ai/install", "bash")
	}
	switch {
	case have("scoop"):
		return Installer{Vendor: "Scoop", Display: "scoop install opencode", Argv: []string{"scoop", "install", "opencode"}}
	case have("npm"):
		return Installer{Vendor: "npm", Display: "npm install -g opencode-ai", Argv: []string{"npm", "install", "-g", "opencode-ai"}}
	}
	return Installer{
		Vendor:  "opencode",
		Display: "npm install -g opencode-ai",
		Manual:  "opencode tidak bisa dipasang otomatis di Windows ini. Pasang Node.js lalu jalankan npm install -g opencode-ai, atau pakai WSL seperti saran panduan opencode. Setelah itu jalankan setup lagi.",
	}
}

func (o *OpenCode) Conflict(prev *state.Tool) string { return "" }

func (o *OpenCode) Configure(ctx context.Context, d Detection, s Settings, prev *state.Tool) (*state.Tool, []string, error) {
	if err := o.Unconfigure(ctx, d, prev); err != nil {
		return nil, nil, err
	}
	oc := s.Profile.Tools.OpenCode
	models := map[string]any{}
	for _, m := range oc.Models {
		models[m.ID] = map[string]string{"name": m.Name}
	}
	provider := map[string]any{
		"npm":  "@ai-sdk/openai-compatible",
		"name": "Vikey",
		"options": map[string]string{
			"baseURL": trimURL(s.Profile.OpenAIBaseURL),
			"apiKey":  s.APIKey,
		},
		"models": models,
	}
	path := opencodeConfigPath()
	var ops []jsonOp
	if _, err := os.Stat(path); err != nil {
		ops = append(ops, jsonOp{ptr: edit.Ptr("$schema"), value: "https://opencode.ai/config.json"})
	}
	ops = append(ops,
		jsonOp{ptr: edit.Ptr("provider", "vikey"), value: provider},
		jsonOp{ptr: edit.Ptr("model"), value: "vikey/" + oc.Model},
	)
	if oc.SmallModel != "" {
		ops = append(ops, jsonOp{ptr: edit.Ptr("small_model"), value: "vikey/" + oc.SmallModel})
	}
	f, err := applyJSON(path, ops, s.BackupDir, true)
	if err != nil {
		return nil, nil, err
	}
	var notes []string
	if os.Getenv("OPENCODE_CONFIG") != "" || os.Getenv("OPENCODE_CONFIG_CONTENT") != "" {
		notes = append(notes, "Terminal ini memakai OPENCODE_CONFIG sendiri. Config itu bisa menimpa pengaturan Vikey.")
	}
	return &state.Tool{Mode: "default", Profile: s.Profile.ID, Files: []state.File{f}}, notes, nil
}

func (o *OpenCode) Unconfigure(ctx context.Context, d Detection, t *state.Tool) error {
	return undoFiles(t)
}

func (o *OpenCode) Inspect(ctx context.Context, d Detection) Status {
	data, _, _ := edit.ReadFile(opencodeConfigPath())
	if data == nil {
		return Status{}
	}
	doc, err := edit.ParseJSON(data)
	if err != nil {
		return Status{Note: "config opencode tidak valid"}
	}
	base := doc.GetString(edit.Ptr("provider", "vikey", "options", "baseURL"))
	model := doc.GetString(edit.Ptr("model"))
	return Status{Configured: base != "" && strings.HasPrefix(model, "vikey/"), BaseURL: base, Model: strings.TrimPrefix(model, "vikey/")}
}

func (o *OpenCode) RunHint(string) string { return "opencode" }
