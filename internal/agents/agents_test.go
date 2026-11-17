package agents

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/Vikey-AI/vikey-installer/internal/edit"
	"github.com/Vikey-AI/vikey-installer/internal/panel"
	"github.com/Vikey-AI/vikey-installer/internal/state"
)

func sandbox(t *testing.T) string {
	t.Helper()
	h := t.TempDir()
	t.Setenv("HOME", h)
	t.Setenv("USERPROFILE", h)
	t.Setenv("VIKEY_HOME", filepath.Join(h, ".vikey"))
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CODEX_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	return h
}

func settings(t *testing.T) Settings {
	return Settings{
		APIKey:    "vk-test-1234567890abcdef",
		Mode:      "default",
		BackupDir: filepath.Join(t.TempDir(), "backup"),
		Profile: panel.Profile{
			ID: "payg", OpenAIBaseURL: "https://api.vikey.ai/v1", AnthropicBaseURL: "https://api.vikey.ai",
			Tools: panel.ToolSettings{
				Claude: panel.ClaudeSettings{
					Model:          "vikey/agent-coding",
					Env:            map[string]string{"CLAUDE_CODE_ATTRIBUTION_HEADER": "0"},
					ModelOverrides: map[string]string{"claude-opus-5-5": "anthropic/claude-opus-5.5"},
				},
				Codex:    panel.CodexSettings{Model: "vikey/agent-gpt"},
				OpenCode: panel.OpenCodeSettings{Model: "vikey/agent-gpt", SmallModel: "openai/gpt-6-luna", Models: []panel.OpenCodeModel{{ID: "vikey/agent-gpt", Name: "Vikey Auto GPT"}}},
			},
		},
	}
}

func readJSONMap(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := edit.ParseJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := doc.Get("")
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("%v: %s", err, raw)
	}
	return m
}

func TestClaudeRoundTripKeepsUserSettings(t *testing.T) {
	h := sandbox(t)
	path := filepath.Join(h, ".claude", "settings.json")
	orig := `{
  "model": "opus",
  "env": { "ANTHROPIC_API_KEY": "sk-old", "MY_VAR": "1" },
  "modelOverrides": { "claude-haiku-5-5": "mine" },
  "permissions": { "allow": ["Bash(ls)"] }
}
`
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, []byte(orig), 0o644)
	before := readJSONMap(t, path)

	c := &Claude{}
	tool, _, err := c.Configure(context.Background(), Detection{}, settings(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	got := readJSONMap(t, path)
	env := got["env"].(map[string]any)
	if got["model"] != "vikey/agent-coding" || env["ANTHROPIC_AUTH_TOKEN"] != "vk-test-1234567890abcdef" || env["MY_VAR"] != "1" {
		t.Fatalf("configured = %v", got)
	}
	if _, ok := env["ANTHROPIC_API_KEY"]; ok {
		t.Fatal("ANTHROPIC_API_KEY should be removed while Vikey is active")
	}
	if got["modelOverrides"].(map[string]any)["claude-haiku-5-5"] != "mine" {
		t.Fatal("user override lost")
	}
	// Windows has no Unix permission bits, files there always report 0666.
	if info, _ := os.Stat(path); runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", info.Mode().Perm())
	}
	if st := c.Inspect(context.Background(), Detection{}); !st.Configured || st.Model != "vikey/agent-coding" {
		t.Fatalf("inspect = %+v", st)
	}

	// A second setup must not stack on the first.
	tool, _, err = c.Configure(context.Background(), Detection{}, settings(t), tool)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Unconfigure(context.Background(), Detection{}, tool); err != nil {
		t.Fatal(err)
	}
	if after := readJSONMap(t, path); !reflect.DeepEqual(after, before) {
		t.Fatalf("not restored:\nbefore %v\nafter  %v", before, after)
	}
}

func TestClaudeCreatedFileIsRemoved(t *testing.T) {
	h := sandbox(t)
	c := &Claude{}
	tool, _, err := c.Configure(context.Background(), Detection{}, settings(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Unconfigure(context.Background(), Detection{}, tool); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(h, ".claude", "settings.json")); !os.IsNotExist(err) {
		t.Fatal("file Vikey created should be gone")
	}
}

func TestClaudeConflictDetectsLogin(t *testing.T) {
	h := sandbox(t)
	c := &Claude{}
	if c.Conflict(nil) != "" {
		t.Fatal("no conflict expected")
	}
	_ = os.WriteFile(filepath.Join(h, ".claude.json"), []byte(`{"oauthAccount":{"emailAddress":"x"}}`), 0o600)
	if c.Conflict(nil) == "" {
		t.Fatal("login should be reported")
	}
}

const userCodex = `# my settings
model = "gpt-5.5"
model_provider = "openai"
approval_policy = "on-request"

[mcp_servers.docs]
command = "npx"

[model_providers.vikey]
name = "Vikey lama"
base_url = "https://api.vikey.ai/v1"
env_key = "VIKEY_API_KEY"
wire_api = "responses"
`

func TestCodexRoundTrip(t *testing.T) {
	h := sandbox(t)
	path := filepath.Join(h, ".codex", "config.toml")
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, []byte(userCodex), 0o644)
	before, _ := edit.ParseTOML(userCodex)

	c := &Codex{}
	tool, _, err := c.Configure(context.Background(), Detection{}, settings(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	m, err := edit.ParseTOML(string(data))
	if err != nil {
		t.Fatalf("%v\n%s", err, data)
	}
	p := m["model_providers"].(map[string]any)["vikey"].(map[string]any)
	if m["model"] != "vikey/agent-gpt" || m["model_provider"] != "vikey" || p["experimental_bearer_token"] != "vk-test-1234567890abcdef" || p["env_key"] != nil {
		t.Fatalf("configured:\n%s", data)
	}
	if m["approval_policy"] != "on-request" || m["mcp_servers"] == nil || !strings.Contains(string(data), "# my settings") {
		t.Fatalf("user settings lost:\n%s", data)
	}
	if st := c.Inspect(context.Background(), Detection{}); !st.Configured {
		t.Fatalf("inspect = %+v", st)
	}

	tool, _, err = c.Configure(context.Background(), Detection{}, settings(t), tool)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Unconfigure(context.Background(), Detection{}, tool); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if string(data) != userCodex {
		t.Fatalf("not restored byte for byte:\n%s", data)
	}
	_ = before
}

func TestCodexKeepsOrderAndOddEndings(t *testing.T) {
	for name, orig := range map[string]string{
		"no final newline": "# top comment\nmodel = \"gpt-5.5\"\n[tui]\nnotifications = true",
		"crlf":             "# top\r\nmodel = \"gpt-5.5\"\r\n\r\n[tui]\r\nnotifications = true\r\n",
		"only comment":     "# nothing yet\n",
	} {
		t.Run(name, func(t *testing.T) {
			h := sandbox(t)
			path := filepath.Join(h, ".codex", "config.toml")
			_ = os.MkdirAll(filepath.Dir(path), 0o755)
			_ = os.WriteFile(path, []byte(orig), 0o644)
			c := &Codex{}
			tool, _, err := c.Configure(context.Background(), Detection{}, settings(t), nil)
			if err != nil {
				t.Fatal(err)
			}
			if st := c.Inspect(context.Background(), Detection{}); !st.Configured {
				data, _ := os.ReadFile(path)
				t.Fatalf("not configured:\n%s", data)
			}
			if err := c.Unconfigure(context.Background(), Detection{}, tool); err != nil {
				t.Fatal(err)
			}
			if data, _ := os.ReadFile(path); string(data) != orig {
				t.Fatalf("got %q\nwant %q", data, orig)
			}
		})
	}
}

// Codex can rewrite config.toml itself (for example when trusting a folder)
// and drop comments. Uninstall must still take Vikey out.
func TestCodexUninstallWithoutMarkers(t *testing.T) {
	h := sandbox(t)
	path := filepath.Join(h, ".codex", "config.toml")
	c := &Codex{}
	tool, _, err := c.Configure(context.Background(), Detection{}, settings(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	rewritten := "model = \"vikey/agent-gpt\"\nmodel_provider = \"vikey\"\n\n[projects.\"/work\"]\ntrust_level = \"trusted\"\n\n[model_providers.vikey]\nname = \"Vikey\"\nbase_url = \"https://api.vikey.ai/v1\"\n"
	_ = os.WriteFile(path, []byte(rewritten), 0o600)
	if err := c.Unconfigure(context.Background(), Detection{}, tool); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	m, _ := edit.ParseTOML(string(data))
	if m["model_provider"] != nil || m["model_providers"] != nil || m["projects"] == nil {
		t.Fatalf("got:\n%s", data)
	}
}

func TestCodexNewFile(t *testing.T) {
	h := sandbox(t)
	c := &Codex{}
	tool, _, err := c.Configure(context.Background(), Detection{}, settings(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Unconfigure(context.Background(), Detection{}, tool); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(h, ".codex", "config.toml")); !os.IsNotExist(err) {
		t.Fatal("created config.toml should be gone")
	}
}

func TestCodexRefusesBrokenFile(t *testing.T) {
	h := sandbox(t)
	path := filepath.Join(h, ".codex", "config.toml")
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, []byte("model = \n[[["), 0o644)
	if _, _, err := (&Codex{}).Configure(context.Background(), Detection{}, settings(t), nil); err == nil {
		t.Fatal("expected an error")
	}
	if data, _ := os.ReadFile(path); string(data) != "model = \n[[[" {
		t.Fatal("broken file was changed")
	}
}

func TestOpenCodeRoundTripJSONC(t *testing.T) {
	h := sandbox(t)
	path := filepath.Join(h, ".config", "opencode", "opencode.jsonc")
	orig := `{
  "$schema": "https://opencode.ai/config.json",
  // my default
  "model": "anthropic/claude-sonnet-5.5",
  "provider": {
    "anthropic": {}
  },
}
`
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, []byte(orig), 0o644)
	before := readJSONMap(t, path)
	o := &OpenCode{}
	tool, _, err := o.Configure(context.Background(), Detection{}, settings(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	got := readJSONMap(t, path)
	if got["model"] != "vikey/vikey/agent-gpt" || got["small_model"] != "vikey/openai/gpt-6-luna" {
		t.Fatalf("configured = %v", got)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "// my default") {
		t.Fatalf("comment lost:\n%s", data)
	}
	if err := o.Unconfigure(context.Background(), Detection{}, tool); err != nil {
		t.Fatal(err)
	}
	if after := readJSONMap(t, path); !reflect.DeepEqual(after, before) {
		t.Fatalf("not restored: %v", after)
	}
}

func TestOpenCodeNewFileHasSchema(t *testing.T) {
	h := sandbox(t)
	o := &OpenCode{}
	tool, _, err := o.Configure(context.Background(), Detection{}, settings(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(h, ".config", "opencode", "opencode.json")
	m := readJSONMap(t, path)
	if m["$schema"] == nil {
		t.Fatal("new file should have $schema")
	}
	_ = o.Unconfigure(context.Background(), Detection{}, tool)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("created file should be gone")
	}
}

var _ = state.Tool{}
