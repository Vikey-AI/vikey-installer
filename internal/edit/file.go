// Package edit changes agent config files in place: JSON and JSONC with
// comments kept, TOML by surgical text edits, and .env lines. Every editor
// reports the value it replaced so the change can be undone later.
package edit

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

// ReadFile returns the file's bytes, or nil and existed=false when it is missing.
func ReadFile(path string) (data []byte, existed bool, err error) {
	data, err = os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

// WriteFile replaces path atomically: a crash leaves either the old or the
// new file, never half of one. Files holding an API key get mode 0600.
func WriteFile(path string, data []byte, secret bool) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	mode := fs.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	if secret {
		mode = 0o600
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".vikey-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(tmpName, mode); err != nil {
			return err
		}
	}
	return os.Rename(tmpName, path)
}

// Backup copies an existing file into dir and returns the copy's path.
func Backup(path, dir string) (string, error) {
	data, existed, err := ReadFile(path)
	if err != nil || !existed {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, filepath.Base(path))
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		return "", err
	}
	return dst, nil
}
