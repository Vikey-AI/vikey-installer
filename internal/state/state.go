// Package state keeps what the CLI knows between runs in ~/.vikey:
//
//	credentials.json  the API key and the chosen profile (mode 0600)
//	state.json        what setup changed in each agent's files, for uninstall
//	backups/          copies of agent files taken before the first change
//	bin/vikey         the CLI itself, put there by the bootstrap script
package state

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/vikey-ai/vikey-cli/internal/edit"
	"github.com/vikey-ai/vikey-cli/internal/panel"
)

// Dir is ~/.vikey, or VIKEY_HOME when set (used by the tests).
func Dir() string {
	if d := os.Getenv("VIKEY_HOME"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".vikey")
}

func BinDir() string { return filepath.Join(Dir(), "bin") }

// BackupDir is a fresh folder for this run's backups.
func BackupDir(now time.Time) string {
	return filepath.Join(Dir(), "backups", now.Format("20060102-150405"))
}

// Credentials is the key and the settings it was installed with.
type Credentials struct {
	APIKey  string        `json:"apiKey"`
	KeyName string        `json:"keyName"`
	KeyPrev string        `json:"keyPreview"`
	Profile panel.Profile `json:"profile"`
	// Every profile the panel offered, so a later setup can switch without a new code.
	Profiles  []panel.Profile `json:"profiles,omitempty"`
	PlanName  string          `json:"planName,omitempty"`
	PlanUntil time.Time       `json:"planUntil,omitempty"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

// JSONKey is one value setup wrote into a JSON file, with what was there before.
type JSONKey struct {
	Ptr  string          `json:"ptr"`
	Prev json.RawMessage `json:"prev,omitempty"` // empty when the key did not exist
	// Container marks a parent object setup had to create. Uninstall removes
	// it only when it is empty again, so values added later are kept.
	Container bool `json:"container,omitempty"`
}

// File records the changes to one file.
type File struct {
	Path    string    `json:"path"`
	Created bool      `json:"created"`
	Backup  string    `json:"backup,omitempty"`
	JSON    []JSONKey `json:"json,omitempty"`
	// TOML: the file had no final newline before setup added one.
	TOMLAddedNewline bool `json:"tomlAddedNewline,omitempty"`
	// .env: the line each name had before ("" when unset).
	EnvPrev map[string]string `json:"envPrev,omitempty"`
}

// HermesKey is one `hermes config set` setup ran, with the earlier value.
type HermesKey struct {
	Key     string `json:"key"`
	Prev    string `json:"prev,omitempty"`
	Existed bool   `json:"existed"`
}

// Tool is everything setup did for one agent.
type Tool struct {
	Mode         string      `json:"mode"` // "default", or "separate" for `vikey claude`
	Profile      string      `json:"profile"`
	ConfiguredAt time.Time   `json:"configuredAt"`
	Files        []File      `json:"files,omitempty"`
	Hermes       []HermesKey `json:"hermes,omitempty"`
}

type State struct {
	Version   int              `json:"version"`
	Tools     map[string]*Tool `json:"tools"`
	PathDirs  []string         `json:"pathDirs,omitempty"` // dirs setup added to PATH
	UpdatedAt time.Time        `json:"updatedAt"`
}

func readJSON(name string, v any) error {
	data, err := os.ReadFile(filepath.Join(Dir(), name))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func writeJSON(name string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return edit.WriteFile(filepath.Join(Dir(), name), append(data, '\n'), true)
}

func Load() (*State, error) {
	s := &State{Version: 1, Tools: map[string]*Tool{}}
	if err := readJSON("state.json", s); err != nil {
		return nil, err
	}
	if s.Tools == nil {
		s.Tools = map[string]*Tool{}
	}
	return s, nil
}

func (s *State) Save() error {
	s.UpdatedAt = time.Now().UTC()
	return writeJSON("state.json", s)
}

// LoadCredentials returns nil when setup never ran.
func LoadCredentials() (*Credentials, error) {
	var c Credentials
	data, err := os.ReadFile(filepath.Join(Dir(), "credentials.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func SaveCredentials(c *Credentials) error {
	c.UpdatedAt = time.Now().UTC()
	return writeJSON("credentials.json", c)
}

func RemoveCredentials() error {
	err := os.Remove(filepath.Join(Dir(), "credentials.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
