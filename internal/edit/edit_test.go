package edit

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestJSONKeepsCommentsAndIndent(t *testing.T) {
	in := `{
    // my provider
    "theme": "dark",
    "provider": {
        "other": { "name": "Other" }, // keep me
    },
}
`
	doc, err := ParseJSON([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Set(Ptr("provider", "vikey"), map[string]string{"name": "Vikey"}); err != nil {
		t.Fatal(err)
	}
	if err := doc.Set(Ptr("env", "A"), "1"); err != nil {
		t.Fatal(err)
	}
	out := string(doc.Bytes())
	for _, want := range []string{"// my provider", "// keep me", `"vikey"`, `"env"`, "    \"theme\""} {
		if !strings.Contains(out, want) {
			t.Errorf("output lost %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\t") {
		t.Errorf("tabs leaked into a 4-space file:\n%s", out)
	}
}

func TestJSONStrictStaysStrict(t *testing.T) {
	in := "{\n  \"model\": \"opus\",\n  \"permissions\": {\"allow\": [\"Bash(ls)\"]}\n}\n"
	doc, err := ParseJSON([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	prev, ok := doc.Get(Ptr("model"))
	if !ok || string(prev) != `"opus"` {
		t.Fatalf("prev = %s %v", prev, ok)
	}
	_ = doc.Set(Ptr("model"), "vikey/agent-coding")
	_ = doc.Set(Ptr("env", "ANTHROPIC_BASE_URL"), "https://api.vikey.ai")
	_ = doc.Set(Ptr("modelOverrides", "claude-opus-5-5"), "anthropic/claude-opus-5.5")
	out := doc.Bytes()
	if !json.Valid(out) {
		t.Fatalf("not strict JSON:\n%s", out)
	}
	var m map[string]any
	_ = json.Unmarshal(out, &m)
	if m["model"] != "vikey/agent-coding" || m["env"].(map[string]any)["ANTHROPIC_BASE_URL"] != "https://api.vikey.ai" {
		t.Fatalf("values not set: %s", out)
	}
	if err := doc.Delete(Ptr("missing", "key")); err != nil {
		t.Fatalf("delete of missing key: %v", err)
	}
}

func TestJSONRefusesNonObjectParent(t *testing.T) {
	doc, _ := ParseJSON([]byte(`{"env": "oops"}`))
	if err := doc.Set(Ptr("env", "A"), "1"); err == nil {
		t.Fatal("expected an error for a string parent")
	}
}

func TestPtrEscapes(t *testing.T) {
	if got := Ptr("models", "vikey/agent-gpt", "a~b"); got != "/models/vikey~1agent-gpt/a~0b" {
		t.Fatal(got)
	}
}

const codexConfig = `# my settings
model = "gpt-5.5"
model_reasoning_effort = "high"
developer_instructions = """
Use [brackets] freely.
model = "not a key"
"""

[mcp_servers.docs]
command = "npx"

[model_providers.vikey]
name = "Old"
base_url = "https://old.example/v1"

[model_providers.vikey.http_headers]
X-Test = "1"

[profiles.fast]
model = "o4-mini"
`

func TestTOMLTakeKeysAndTables(t *testing.T) {
	text, taken := TakeTopLevelKeys(codexConfig, "model", "model_provider")
	if taken["model"] != `model = "gpt-5.5"` || len(taken) != 1 {
		t.Fatalf("taken = %v", taken)
	}
	if !strings.Contains(text, `model = "not a key"`) || !strings.Contains(text, `model = "o4-mini"`) {
		t.Fatalf("removed a line inside a string or a table:\n%s", text)
	}
	text, removed := TakeTables(text, "model_providers.vikey")
	if !strings.Contains(removed, "X-Test") || !strings.Contains(removed, "Old") {
		t.Fatalf("removed = %q", removed)
	}
	if strings.Contains(text, "model_providers") || !strings.Contains(text, "[profiles.fast]") {
		t.Fatalf("text = %s", text)
	}
	if _, err := ParseTOML(text); err != nil {
		t.Fatal(err)
	}
}

func TestMarkedBlockRoundTrip(t *testing.T) {
	text := MarkedBlock("vikey", "note", `model = "x"`) + "\n\nother = 1\n"
	if got := strings.TrimSpace(RemoveMarkedBlock(text, "vikey")); got != "other = 1" {
		t.Fatalf("got %q", got)
	}
}

func TestEnvLines(t *testing.T) {
	text := "# keys\nOPENAI_API_KEY=a\nexport VIKEY_API_KEY=old\n"
	out, prev := SetEnvLine(text, "VIKEY_API_KEY", "new")
	if prev != "export VIKEY_API_KEY=old" || !strings.Contains(out, "VIKEY_API_KEY=new") {
		t.Fatalf("out=%q prev=%q", out, prev)
	}
	if back := RestoreEnvLine(out, "VIKEY_API_KEY", prev); back != text {
		t.Fatalf("restore: %q", back)
	}
	out, prev = SetEnvLine("A=1\n", "VIKEY_API_KEY", "k")
	if prev != "" || out != "A=1\nVIKEY_API_KEY=k\n" {
		t.Fatalf("append: %q", out)
	}
	if back := RestoreEnvLine(out, "VIKEY_API_KEY", ""); back != "A=1\n" {
		t.Fatalf("remove: %q", back)
	}
}

func TestTOMLStringEscapes(t *testing.T) {
	s := TOMLString("a\"b\\c\n")
	m, err := ParseTOML("x = " + s)
	if err != nil || m["x"] != "a\"b\\c\n" {
		t.Fatalf("%s %v %v", s, m, err)
	}
}

func TestJSONNewFileIsPlainIndented(t *testing.T) {
	doc, _ := ParseJSON(nil)
	_ = doc.Set(Ptr("model"), "x")
	_ = doc.Set(Ptr("env", "A"), "1")
	want := "{\n  \"model\": \"x\",\n  \"env\": {\n    \"A\": \"1\"\n  }\n}\n"
	if got := string(doc.Bytes()); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}
