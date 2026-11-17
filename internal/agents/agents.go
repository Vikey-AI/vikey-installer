// Package agents knows each coding agent Vikey sets up: how to find it, how
// to install it with its vendor's own installer, where it keeps its settings
// and how to point it at Vikey and back.
package agents

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/vikey-ai/vikey-cli/internal/panel"
	"github.com/vikey-ai/vikey-cli/internal/state"
)

// Detection is what was found on this machine.
type Detection struct {
	Installed bool
	Path      string
	Version   string
	// Desktop apps that read the same settings, for example "Codex Desktop".
	Desktop []string
}

// Installer describes how to install an agent that is missing.
type Installer struct {
	// Argv runs the vendor's installer. Empty when it cannot be installed
	// automatically here; Manual then says what to do.
	Argv    []string
	Env     []string
	Display string // the command as shown to the person
	Vendor  string // "installer resmi Anthropic"
	Manual  string
}

// Settings is what Configure writes.
type Settings struct {
	APIKey    string
	Profile   panel.Profile
	Mode      string // "default", or "separate" (Claude Code only)
	BackupDir string
}

// Status is what doctor reports about an agent's settings.
type Status struct {
	Configured bool   // the agent points at Vikey
	BaseURL    string // the base URL it uses, when it points somewhere
	Model      string
	Note       string
}

// Agent is one coding agent.
type Agent interface {
	ID() string
	Name() string
	Detect(ctx context.Context) Detection
	Installer() Installer
	// Conflict explains why making Vikey the default may surprise the person
	// (another login or provider is active). Empty when there is none.
	Conflict(prev *state.Tool) string
	// Configure points the agent at Vikey. prev is what an earlier setup did,
	// it is undone first so values never pile up.
	Configure(ctx context.Context, d Detection, s Settings, prev *state.Tool) (*state.Tool, []string, error)
	// Unconfigure puts back what Configure replaced.
	Unconfigure(ctx context.Context, d Detection, t *state.Tool) error
	Inspect(ctx context.Context, d Detection) Status
	// RunHint is the command that starts the agent.
	RunHint(mode string) string
}

// DesktopOnly is implemented by agents whose desktop app reads the same
// settings, so a machine with only the app needs no install.
type DesktopOnly interface {
	DesktopIsEnough() bool
	DesktopHint() string
}

// Usable reports whether setup can configure the agent as it is now.
func Usable(a Agent, d Detection) bool {
	if d.Installed {
		return true
	}
	if do, ok := a.(DesktopOnly); ok && do.DesktopIsEnough() && len(d.Desktop) > 0 {
		return true
	}
	return false
}

// All returns the agents in the order the setup menu shows them.
func All() []Agent {
	return []Agent{&Claude{}, &Codex{}, &OpenCode{}, &Hermes{}}
}

// ByID returns the agent with this id, or nil.
func ByID(id string) Agent {
	for _, a := range All() {
		if a.ID() == id {
			return a
		}
	}
	return nil
}

func home() string {
	h, _ := os.UserHomeDir()
	return h
}

func isWindows() bool { return runtime.GOOS == "windows" }

// findBinary looks on PATH first, then in the folders vendor installers use,
// because a fresh install is often not on this process's PATH yet.
func findBinary(name string, dirs ...string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	exts := []string{""}
	if isWindows() {
		exts = []string{".exe", ".cmd", ".bat"}
	}
	for _, d := range dirs {
		if d == "" {
			continue
		}
		for _, ext := range exts {
			p := filepath.Join(d, name+ext)
			if info, err := os.Stat(p); err == nil && !info.IsDir() {
				return p
			}
		}
	}
	return ""
}

var versionRe = regexp.MustCompile(`v?(\d+\.\d+\.\d+[0-9A-Za-z.\-]*)`)

// version runs "<bin> --version" and returns the first version number in its output.
func version(ctx context.Context, bin string) string {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "--version")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	_ = cmd.Run()
	if m := versionRe.FindStringSubmatch(out.String()); m != nil {
		return m[1]
	}
	return ""
}

func detect(ctx context.Context, name string, dirs ...string) Detection {
	p := findBinary(name, dirs...)
	if p == "" {
		return Detection{}
	}
	return Detection{Installed: true, Path: p, Version: version(ctx, p)}
}

// macApp returns label when one of the app bundles exists.
func macApp(label string, names ...string) []string {
	if runtime.GOOS != "darwin" {
		return nil
	}
	for _, n := range names {
		for _, dir := range []string{"/Applications", filepath.Join(home(), "Applications")} {
			if _, err := os.Stat(filepath.Join(dir, n)); err == nil {
				return []string{label}
			}
		}
	}
	return nil
}

func have(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// shellInstaller runs "curl -fsSL url | <shell> -s -- args" on macOS and Linux.
func shellInstaller(vendor, url, shell string, args ...string) Installer {
	pipe := shell
	if len(args) > 0 {
		pipe += " -s -- " + strings.Join(args, " ")
	}
	display := "curl -fsSL " + url + " | " + pipe
	if !have("curl") {
		return Installer{Vendor: vendor, Display: display, Manual: "Butuh curl untuk memasang. Pasang curl dulu, lalu jalankan setup lagi."}
	}
	if !have(shell) {
		return Installer{Vendor: vendor, Display: display, Manual: "Butuh " + shell + " untuk memasang. Pasang " + shell + " dulu, lalu jalankan setup lagi."}
	}
	// pipefail makes a failed download fail the install instead of piping an
	// empty script into the shell. dash (sh on Debian and Ubuntu) has no
	// pipefail and exits on the unknown option, so it is only set in bash.
	if have("bash") {
		return Installer{Vendor: vendor, Display: display, Argv: []string{"bash", "-c", "set -o pipefail; " + display}}
	}
	return Installer{Vendor: vendor, Display: display, Argv: []string{"sh", "-c", display}}
}

// psInstaller runs a PowerShell installer the way its vendor documents it.
func psInstaller(vendor, script string) Installer {
	return Installer{
		Vendor:  vendor,
		Display: script,
		Argv:    []string{"powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script},
	}
}

func readHome(parts ...string) string {
	return filepath.Join(append([]string{home()}, parts...)...)
}

func samePath(a, b string) bool {
	ca, _ := filepath.Abs(a)
	cb, _ := filepath.Abs(b)
	if isWindows() {
		return strings.EqualFold(ca, cb)
	}
	return ca == cb
}

func trimURL(u string) string { return strings.TrimRight(u, "/") }
