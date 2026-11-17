//go:build windows

package pathenv

import (
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const pathValue = `HKCU\Environment\Path`

func readUserPath() (string, uint32, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE)
	if err != nil {
		return "", registry.EXPAND_SZ, err
	}
	defer k.Close()
	v, typ, err := k.GetStringValue("Path")
	if err == registry.ErrNotExist {
		return "", registry.EXPAND_SZ, nil
	}
	return v, typ, err
}

func writeUserPath(v string, typ uint32) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if typ == registry.SZ {
		err = k.SetStringValue("Path", v)
	} else {
		err = k.SetExpandStringValue("Path", v)
	}
	if err != nil {
		return err
	}
	broadcast()
	return nil
}

// broadcast tells Explorer the environment changed, so terminals opened from
// now on get the new Path without signing out.
func broadcast() {
	env, _ := syscall.UTF16PtrFromString("Environment")
	user32 := windows.NewLazySystemDLL("user32.dll")
	proc := user32.NewProc("SendMessageTimeoutW")
	const hwndBroadcast = 0xffff
	const wmSettingChange = 0x001A
	const smtoAbortIfHung = 0x0002
	var result uintptr
	_, _, _ = proc.Call(hwndBroadcast, wmSettingChange, 0, uintptr(unsafe.Pointer(env)), smtoAbortIfHung, 5000, uintptr(unsafe.Pointer(&result)))
}

func split(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ";") {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return out
}

func has(list []string, dir string) bool {
	for _, p := range list {
		if strings.EqualFold(filepath.Clean(p), filepath.Clean(dir)) {
			return true
		}
	}
	return false
}

func ensure(dirs []string) ([]string, error) {
	cur, typ, err := readUserPath()
	if err != nil {
		return nil, err
	}
	parts := split(cur)
	added := false
	for i := len(dirs) - 1; i >= 0; i-- {
		if !has(parts, dirs[i]) {
			parts = append([]string{dirs[i]}, parts...)
			added = true
		}
	}
	if !added {
		return nil, nil
	}
	return []string{pathValue}, writeUserPath(strings.Join(parts, ";"), typ)
}

func remove(dirs []string) ([]string, error) {
	cur, typ, err := readUserPath()
	if err != nil {
		return nil, err
	}
	parts := split(cur)
	kept := parts[:0:0]
	for _, p := range parts {
		if !has(dirs, p) {
			kept = append(kept, p)
		}
	}
	if len(kept) == len(parts) {
		return nil, nil
	}
	return []string{pathValue}, writeUserPath(strings.Join(kept, ";"), typ)
}
