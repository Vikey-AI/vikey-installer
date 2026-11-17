// Package panel talks to the Vikey panel (app.vikey.ai) and checks a key
// against the inference API.
package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"
)

// DefaultURL is the panel the bootstrap scripts point at.
const DefaultURL = "https://app.vikey.ai"

// Profile is one way to use the key: pay as you go or the Token Plan. The
// panel decides the values (routes/install.js and config/cliInstall.js in
// vikey-api), the CLI decides where each agent keeps them.
type Profile struct {
	ID               string       `json:"id"` // "payg" or "plan"
	OpenAIBaseURL    string       `json:"openaiBaseUrl"`
	AnthropicBaseURL string       `json:"anthropicBaseUrl"`
	Tools            ToolSettings `json:"tools"`
}

type ToolSettings struct {
	Claude   ClaudeSettings   `json:"claude"`
	Codex    CodexSettings    `json:"codex"`
	OpenCode OpenCodeSettings `json:"opencode"`
	Hermes   HermesSettings   `json:"hermes"`
}

type ClaudeSettings struct {
	Model          string            `json:"model"`
	Env            map[string]string `json:"env"`
	ModelOverrides map[string]string `json:"modelOverrides"`
}

type CodexSettings struct {
	Model string `json:"model"`
}

type OpenCodeModel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type OpenCodeSettings struct {
	Model      string          `json:"model"`
	SmallModel string          `json:"smallModel"`
	Models     []OpenCodeModel `json:"models"`
}

type HermesSettings struct {
	Model  string   `json:"model"`
	Models []string `json:"models"`
}

type KeyInfo struct {
	Name    string `json:"name"`
	Preview string `json:"preview"`
}

type PlanInfo struct {
	Active    bool      `json:"active"`
	Name      string    `json:"name"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// Install is what the panel returns for a valid one-time code.
type Install struct {
	APIKey   string    `json:"apiKey"`
	Key      KeyInfo   `json:"key"`
	Plan     PlanInfo  `json:"plan"`
	Profiles []Profile `json:"profiles"`
}

// Error is a refusal from the panel with a message meant for the person.
type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }

var client = &http.Client{Timeout: 30 * time.Second}

// UserAgent identifies the CLI to Vikey's servers.
func UserAgent(version string) string {
	return fmt.Sprintf("vikey-cli/%s (%s; %s)", version, runtime.GOOS, runtime.GOARCH)
}

// Exchange trades a one-time code for the API key and the agent settings.
// The code stops working once this succeeds.
func Exchange(ctx context.Context, panelURL, code, version string) (*Install, error) {
	body, _ := json.Marshal(map[string]string{"code": code})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(panelURL, "/")+"/api/install/exchange", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgent(version))
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tidak bisa terhubung ke Vikey (%s). Cek koneksi internet lalu coba lagi", hostOf(panelURL))
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != http.StatusOK {
		var e struct {
			Error struct {
				Message string `json:"message"`
				Code    string `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(data, &e)
		msg := e.Error.Message
		if msg == "" {
			msg = fmt.Sprintf("Vikey menolak kode ini (HTTP %d)", res.StatusCode)
		}
		return nil, &Error{Status: res.StatusCode, Code: e.Error.Code, Message: msg}
	}
	var install Install
	if err := json.Unmarshal(data, &install); err != nil {
		return nil, errors.New("jawaban server Vikey tidak bisa dibaca. Coba perbarui Vikey CLI")
	}
	if len(install.APIKey) < 20 || strings.ContainsAny(install.APIKey, " \t\r\n\"'\\") {
		return nil, errors.New("server Vikey tidak mengirim API key")
	}
	if len(install.Profiles) == 0 {
		return nil, errors.New("server Vikey tidak mengirim pengaturan agent")
	}
	return &install, nil
}

// CheckKey calls GET /v1/models with the key. It is free and proves the key
// and base URL work together.
func CheckKey(ctx context.Context, openAIBaseURL, apiKey, version string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(openAIBaseURL, "/")+"/models", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("User-Agent", UserAgent(version))
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("tidak bisa terhubung ke %s", hostOf(openAIBaseURL))
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4<<20))
	switch {
	case res.StatusCode == http.StatusOK:
		return nil
	case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
		return fmt.Errorf("API key ditolak (HTTP %d). Cek apakah key masih aktif di dashboard", res.StatusCode)
	default:
		return fmt.Errorf("server menjawab HTTP %d", res.StatusCode)
	}
}

func hostOf(u string) string {
	u = strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
	if i := strings.IndexByte(u, '/'); i >= 0 {
		u = u[:i]
	}
	return u
}
