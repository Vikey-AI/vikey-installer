//go:build !windows

package pathenv

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/vikey-ai/vikey-cli/internal/edit"
)

const (
	begin = "# >>> vikey >>>"
	end   = "# <<< vikey <<<"
)

func home() string {
	h, _ := os.UserHomeDir()
	return h
}

// rcFiles are the startup files of the person's shell that new terminals read.
func rcFiles() []string {
	h := home()
	shell := filepath.Base(os.Getenv("SHELL"))
	var files []string
	switch shell {
	case "zsh":
		files = append(files, filepath.Join(h, ".zshrc"))
	case "bash":
		files = append(files, filepath.Join(h, ".bashrc"))
		// macOS Terminal starts login shells, which read .bash_profile, not .bashrc.
		if runtime.GOOS == "darwin" || exists(filepath.Join(h, ".bash_profile")) {
			files = append(files, filepath.Join(h, ".bash_profile"))
		}
	case "fish":
		files = append(files, filepath.Join(h, ".config", "fish", "conf.d", "vikey.fish"))
	default:
		files = append(files, filepath.Join(h, ".profile"))
	}
	// zsh is the macOS default even when SHELL says otherwise in a script.
	if runtime.GOOS == "darwin" && shell != "zsh" && exists(filepath.Join(h, ".zshrc")) {
		files = append(files, filepath.Join(h, ".zshrc"))
	}
	return files
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// homeRelative writes $HOME/... so the block keeps working if the home folder moves.
func homeRelative(dir string) string {
	h := home()
	if rel, err := filepath.Rel(h, dir); err == nil && !strings.HasPrefix(rel, "..") {
		return "$HOME/" + filepath.ToSlash(rel)
	}
	return dir
}

func block(file string, dirs []string) string {
	var lines []string
	lines = append(lines, begin, "# Ditambahkan oleh Vikey CLI. Hapus dengan: vikey uninstall")
	fish := strings.HasSuffix(file, ".fish")
	for i := len(dirs) - 1; i >= 0; i-- {
		d := homeRelative(dirs[i])
		if fish {
			lines = append(lines, `if not contains "`+d+`" $PATH; set -gx PATH "`+d+`" $PATH; end`)
		} else {
			lines = append(lines, `case ":$PATH:" in *":`+d+`:"*) ;; *) export PATH="`+d+`:$PATH" ;; esac`)
		}
	}
	lines = append(lines, end)
	return strings.Join(lines, "\n")
}

func stripBlock(text string) string {
	lines := strings.Split(text, "\n")
	out := lines[:0:0]
	inside := false
	for _, l := range lines {
		switch strings.TrimSpace(l) {
		case begin:
			inside = true
			continue
		case end:
			inside = false
			continue
		}
		if !inside {
			out = append(out, l)
		}
	}
	return strings.TrimRight(strings.Join(out, "\n"), "\n")
}

func ensure(dirs []string) ([]string, error) {
	if len(dirs) == 0 {
		return nil, nil
	}
	var changed []string
	for _, f := range rcFiles() {
		data, _, err := edit.ReadFile(f)
		if err != nil {
			return changed, err
		}
		old := string(data)
		text := stripBlock(old)
		if text != "" {
			text += "\n\n"
		}
		text += block(f, dirs) + "\n"
		if text == old {
			continue
		}
		if err := edit.WriteFile(f, []byte(text), false); err != nil {
			return changed, err
		}
		changed = append(changed, f)
	}
	return changed, nil
}

func remove(dirs []string) ([]string, error) {
	var changed []string
	for _, f := range rcFiles() {
		data, existed, err := edit.ReadFile(f)
		if err != nil || !existed {
			continue
		}
		old := string(data)
		if !strings.Contains(old, begin) {
			continue
		}
		text := stripBlock(old)
		if text != "" {
			text += "\n"
		}
		if strings.HasSuffix(f, "vikey.fish") && strings.TrimSpace(text) == "" {
			_ = os.Remove(f)
		} else if err := edit.WriteFile(f, []byte(text), false); err != nil {
			return changed, err
		}
		changed = append(changed, f)
	}
	return changed, nil
}
