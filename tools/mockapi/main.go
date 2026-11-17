// Command mockapi is a stand-in for the Vikey inference and panel APIs, used by
// the end-to-end tests. It answers every chat request with the text "ok" in the
// format the caller asked for (OpenAI chat completions, OpenAI responses or
// Anthropic messages, streamed or not) and records each request so a test can
// check which model and key a coding agent actually sent.
//
//	GET  /dl/<file>        files from -dl, standing in for GitHub releases
//	GET  /i/<code>[.ps1]   bootstrap script from -bootstrap, like the panel serves it
//	POST /api/install/exchange   a canned install for -code, pointing at this server
//	GET  /__log            recorded requests as JSON lines
//	POST /__reset          clear the log
//	GET  /v1/models        model list
//	POST /v1/chat/completions, /v1/responses, /v1/messages, /v1/messages/count_tokens
//
// Every path also works under /plan, so one server can play both
// api.vikey.ai and api-plan.vikey.ai.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type record struct {
	Time   string `json:"time"`
	Method string `json:"method"`
	Path   string `json:"path"`
	Auth   string `json:"auth"`
	APIKey string `json:"xApiKey,omitempty"`
	Model  string `json:"model,omitempty"`
	Stream bool   `json:"stream"`
	Agent  string `json:"userAgent"`
}

var (
	mu      sync.Mutex
	records []record
	models  = []string{"vikey/agent-coding", "vikey/agent-gpt", "vikey/agent-mocin", "vikey/plan", "openai/gpt-6-luna", "anthropic/claude-sonnet-5.5"}
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	dl := flag.String("dl", "", "folder served at /dl/ (CLI binaries for the bootstrap script)")
	bootstrap := flag.String("bootstrap", "", "folder with install.sh and install.ps1 templates")
	code := flag.String("code", "E2ETEST1", "the one install code this server accepts")
	plan := flag.Bool("plan", false, "offer a Token Plan profile too")
	flag.Parse()
	self := "http://" + *addr

	mux := http.NewServeMux()
	mux.HandleFunc("/__log", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/x-ndjson")
		enc := json.NewEncoder(w)
		for _, rec := range records {
			_ = enc.Encode(rec)
		}
	})
	mux.HandleFunc("/__reset", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		records = nil
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	if *dl != "" {
		files := http.StripPrefix("/dl/", http.FileServer(http.Dir(*dl)))
		mux.HandleFunc("/dl/", func(w http.ResponseWriter, r *http.Request) {
			log.Printf("download %s (%s)", r.URL.Path, r.Header.Get("User-Agent"))
			files.ServeHTTP(w, r)
		})
	}
	if *bootstrap != "" {
		mux.HandleFunc("/i/", func(w http.ResponseWriter, r *http.Request) {
			serveBootstrap(w, r, *bootstrap, *dl, *code, self)
		})
	}
	mux.HandleFunc("/api/install/exchange", func(w http.ResponseWriter, r *http.Request) {
		exchange(w, r, *code, self, *plan)
	})
	mux.HandleFunc("/", route)

	log.Printf("mockapi listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

func route(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/plan")
	body, _ := io.ReadAll(io.LimitReader(r.Body, 32<<20))
	var req struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	_ = json.Unmarshal(body, &req)

	mu.Lock()
	records = append(records, record{
		Time:   time.Now().UTC().Format(time.RFC3339Nano),
		Method: r.Method,
		Path:   r.URL.Path,
		Auth:   r.Header.Get("Authorization"),
		APIKey: r.Header.Get("X-Api-Key"),
		Model:  req.Model,
		Stream: req.Stream,
		Agent:  r.Header.Get("User-Agent"),
	})
	mu.Unlock()
	log.Printf("%s %s model=%q stream=%v", r.Method, r.URL.Path, req.Model, req.Stream)

	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer vk-") && !strings.HasPrefix(r.Header.Get("X-Api-Key"), "vk-") {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": map[string]any{"message": "Invalid API key", "type": "authentication_error"}})
		return
	}

	switch {
	case r.Method == http.MethodGet && (path == "/v1/models" || path == "/models"):
		data := make([]map[string]any, 0, len(models))
		for _, id := range models {
			data = append(data, map[string]any{"id": id, "object": "model", "created": 1767225600, "owned_by": "vikey", "type": "model", "display_name": id})
		}
		writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data, "has_more": false})
	case path == "/v1/chat/completions" || path == "/chat/completions":
		chat(w, req.Model, req.Stream)
	case path == "/v1/responses" || path == "/responses":
		responses(w, req.Model, req.Stream)
	case path == "/v1/messages/count_tokens":
		writeJSON(w, http.StatusOK, map[string]any{"input_tokens": 12})
	case path == "/v1/messages":
		messages(w, req.Model, req.Stream)
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"error": map[string]any{"message": "not found: " + r.URL.Path}})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type sse struct {
	w http.ResponseWriter
	f http.Flusher
}

func newSSE(w http.ResponseWriter) *sse {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	f, _ := w.(http.Flusher)
	return &sse{w, f}
}

func (s *sse) send(event string, v any) {
	b, _ := json.Marshal(v)
	if event != "" {
		fmt.Fprintf(s.w, "event: %s\n", event)
	}
	fmt.Fprintf(s.w, "data: %s\n\n", b)
	if s.f != nil {
		s.f.Flush()
	}
}

func chat(w http.ResponseWriter, model string, stream bool) {
	now := time.Now().Unix()
	usage := map[string]any{"prompt_tokens": 10, "completion_tokens": 1, "total_tokens": 11}
	if !stream {
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "chatcmpl-mock", "object": "chat.completion", "created": now, "model": model,
			"choices": []any{map[string]any{"index": 0, "finish_reason": "stop",
				"message": map[string]any{"role": "assistant", "content": "ok"}}},
			"usage": usage,
		})
		return
	}
	s := newSSE(w)
	chunk := func(delta map[string]any, finish any) map[string]any {
		return map[string]any{"id": "chatcmpl-mock", "object": "chat.completion.chunk", "created": now, "model": model,
			"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}}
	}
	s.send("", chunk(map[string]any{"role": "assistant", "content": ""}, nil))
	s.send("", chunk(map[string]any{"content": "ok"}, nil))
	s.send("", chunk(map[string]any{}, "stop"))
	s.send("", map[string]any{"id": "chatcmpl-mock", "object": "chat.completion.chunk", "created": now, "model": model, "choices": []any{}, "usage": usage})
	fmt.Fprint(w, "data: [DONE]\n\n")
	if s.f != nil {
		s.f.Flush()
	}
}

func responses(w http.ResponseWriter, model string, stream bool) {
	now := time.Now().Unix()
	text := map[string]any{"type": "output_text", "text": "ok", "annotations": []any{}}
	item := map[string]any{"id": "msg_mock", "type": "message", "role": "assistant", "status": "completed", "content": []any{text}}
	usage := map[string]any{"input_tokens": 10, "output_tokens": 1, "total_tokens": 11,
		"input_tokens_details": map[string]any{"cached_tokens": 0}, "output_tokens_details": map[string]any{"reasoning_tokens": 0}}
	resp := func(status string, output []any) map[string]any {
		r := map[string]any{"id": "resp_mock", "object": "response", "created_at": now, "status": status, "model": model, "output": output}
		if status == "completed" {
			r["usage"] = usage
		}
		return r
	}
	if !stream {
		writeJSON(w, http.StatusOK, resp("completed", []any{item}))
		return
	}
	s := newSSE(w)
	seq := 0
	send := func(typ string, fields map[string]any) {
		fields["type"] = typ
		fields["sequence_number"] = seq
		seq++
		s.send(typ, fields)
	}
	send("response.created", map[string]any{"response": resp("in_progress", []any{})})
	send("response.in_progress", map[string]any{"response": resp("in_progress", []any{})})
	send("response.output_item.added", map[string]any{"output_index": 0,
		"item": map[string]any{"id": "msg_mock", "type": "message", "role": "assistant", "status": "in_progress", "content": []any{}}})
	send("response.content_part.added", map[string]any{"item_id": "msg_mock", "output_index": 0, "content_index": 0,
		"part": map[string]any{"type": "output_text", "text": "", "annotations": []any{}}})
	send("response.output_text.delta", map[string]any{"item_id": "msg_mock", "output_index": 0, "content_index": 0, "delta": "ok"})
	send("response.output_text.done", map[string]any{"item_id": "msg_mock", "output_index": 0, "content_index": 0, "text": "ok"})
	send("response.content_part.done", map[string]any{"item_id": "msg_mock", "output_index": 0, "content_index": 0, "part": text})
	send("response.output_item.done", map[string]any{"output_index": 0, "item": item})
	send("response.completed", map[string]any{"response": resp("completed", []any{item})})
}

func messages(w http.ResponseWriter, model string, stream bool) {
	usage := map[string]any{"input_tokens": 10, "output_tokens": 1, "cache_read_input_tokens": 0, "cache_creation_input_tokens": 0}
	if !stream {
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "msg_mock", "type": "message", "role": "assistant", "model": model,
			"content":     []any{map[string]any{"type": "text", "text": "ok"}},
			"stop_reason": "end_turn", "stop_sequence": nil, "usage": usage,
		})
		return
	}
	s := newSSE(w)
	s.send("message_start", map[string]any{"type": "message_start", "message": map[string]any{
		"id": "msg_mock", "type": "message", "role": "assistant", "model": model, "content": []any{},
		"stop_reason": nil, "stop_sequence": nil, "usage": usage}})
	s.send("content_block_start", map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "text", "text": ""}})
	s.send("content_block_delta", map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": "ok"}})
	s.send("content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
	s.send("message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "end_turn", "stop_sequence": nil}, "usage": map[string]any{"output_tokens": 1}})
	s.send("message_stop", map[string]any{"type": "message_stop"})
}

var targets = []string{"darwin-arm64", "darwin-amd64", "linux-amd64", "linux-arm64", "windows-amd64", "windows-arm64"}

// serveBootstrap renders the templates the same way vikey-api's
// utils/installCode.js does, with this server as panel and download host.
func serveBootstrap(w http.ResponseWriter, r *http.Request, dir, dl, code, self string) {
	log.Printf("bootstrap %s (%s)", r.URL.Path, r.Header.Get("User-Agent"))
	name := strings.TrimPrefix(r.URL.Path, "/i/")
	kind := "sh"
	if strings.HasSuffix(name, ".ps1") {
		kind, name = "ps1", strings.TrimSuffix(name, ".ps1")
	}
	tpl, err := os.ReadFile(filepath.Join(dir, "install."+kind))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !strings.EqualFold(name, code) {
		fmt.Fprint(w, "echo wrong code; exit 1\n")
		return
	}
	var lines []string
	for _, t := range targets {
		file := "vikey-" + t
		if strings.HasPrefix(t, "windows-") {
			file += ".exe"
		}
		data, err := os.ReadFile(filepath.Join(dl, file))
		if err != nil {
			continue
		}
		sum := sha256.Sum256(data)
		sha := hex.EncodeToString(sum[:])
		switch {
		case kind == "ps1" && strings.HasPrefix(t, "windows-"):
			lines = append(lines, fmt.Sprintf("    '%s' = '%s'", t, sha))
		case kind == "sh" && !strings.HasPrefix(t, "windows-"):
			lines = append(lines, fmt.Sprintf("    %s) echo \"%s\" ;;", t, sha))
		}
	}
	placeholder := "__ASSET_CASES__"
	if kind == "ps1" {
		placeholder = "__ASSET_HASH__"
	}
	out := strings.NewReplacer(
		"__CODE__", strings.ToUpper(code),
		"__VERSION__", "0.0.0-e2e",
		"__PANEL_URL__", self,
		"__DOWNLOAD_BASE__", self+"/dl",
		placeholder, strings.Join(lines, "\n"),
	).Replace(string(tpl))
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, out)
}

// exchange mirrors routes/install.js in vikey-api, with fixed answers.
func exchange(w http.ResponseWriter, r *http.Request, code, self string, plan bool) {
	var body struct {
		Code string `json:"code"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	log.Printf("exchange code=%q (%s)", body.Code, r.Header.Get("User-Agent"))
	if !strings.EqualFold(body.Code, code) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": map[string]any{"message": "Kode pemasangan tidak berlaku.", "code": "invalid_code"}})
		return
	}
	payg := profile("payg", self, "vikey/agent-coding", "vikey/agent-gpt", "openai/gpt-6-luna")
	profiles := []any{payg}
	planInfo := map[string]any{"active": false}
	if plan {
		profiles = []any{profile("plan", self+"/plan", "vikey/plan", "vikey/plan", "vikey/plan"), payg}
		planInfo = map[string]any{"active": true, "name": "PRO PLAN", "expiresAt": time.Now().Add(30 * 24 * time.Hour)}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"apiKey":   "vk-e2e-0000-1111-2222-333344445555",
		"key":      map[string]any{"name": "Kunci tes", "preview": "vk-e2e-0000-...5555"},
		"plan":     planInfo,
		"profiles": profiles,
	})
}

func profile(id, base, claudeModel, model, small string) map[string]any {
	overrides := map[string]string{"claude-opus-5-5": claudeModel, "claude-sonnet-5-5": claudeModel, "claude-haiku-5-5": small}
	return map[string]any{
		"id": id, "openaiBaseUrl": base + "/v1", "anthropicBaseUrl": base,
		"tools": map[string]any{
			"claude":   map[string]any{"model": claudeModel, "env": map[string]string{"CLAUDE_CODE_ATTRIBUTION_HEADER": "0"}, "modelOverrides": overrides},
			"codex":    map[string]any{"model": model},
			"opencode": map[string]any{"model": model, "smallModel": small, "models": []any{map[string]string{"id": model, "name": model}, map[string]string{"id": small, "name": small}}},
			"hermes":   map[string]any{"model": model, "models": []string{model}},
		},
	}
}
