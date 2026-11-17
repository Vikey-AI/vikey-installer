// Package pathenv adds folders to the user's PATH for new terminals and
// removes them again: a marked block in the shell startup files on macOS and
// Linux, the user Path in the registry on Windows.
package pathenv

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// OnPath reports whether dir is already on this process's PATH.
func OnPath(dir string) bool {
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		if p == "" {
			continue
		}
		if runtime.GOOS == "windows" {
			if strings.EqualFold(filepath.Clean(p), filepath.Clean(dir)) {
				return true
			}
		} else if filepath.Clean(p) == filepath.Clean(dir) {
			return true
		}
	}
	return false
}

// Ensure makes dirs reachable from new terminals. It returns the startup
// files or registry value it changed, empty when nothing needed changing.
func Ensure(dirs []string) ([]string, error) { return ensure(dirs) }

// Remove takes Vikey's PATH changes out again.
func Remove(dirs []string) ([]string, error) { return remove(dirs) }
