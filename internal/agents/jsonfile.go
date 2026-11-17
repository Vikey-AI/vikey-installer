package agents

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/vikey-ai/vikey-cli/internal/edit"
	"github.com/vikey-ai/vikey-cli/internal/state"
)

// jsonOp is one change to a JSON settings file.
type jsonOp struct {
	ptr   string
	value any
	del   bool
}

// applyJSON makes ops on the file at path and records, for each pointer, what
// was there before. Parent objects it has to create are recorded too.
func applyJSON(path string, ops []jsonOp, backupDir string, secret bool) (state.File, error) {
	data, existed, err := edit.ReadFile(path)
	if err != nil {
		return state.File{}, err
	}
	doc, err := edit.ParseJSON(data)
	if err != nil {
		return state.File{}, fmt.Errorf("%s tidak bisa dibaca sebagai JSON (%v). Perbaiki atau pindahkan file itu dulu", path, err)
	}
	f := state.File{Path: path, Created: !existed}
	if existed {
		if f.Backup, err = edit.Backup(path, backupDir); err != nil {
			return state.File{}, err
		}
	}
	for _, op := range ops {
		tokens := strings.Split(op.ptr, "/")[1:]
		if !op.del {
			for i := 1; i < len(tokens); i++ {
				parent := "/" + strings.Join(tokens[:i], "/")
				if _, ok := doc.Get(parent); !ok {
					f.JSON = append(f.JSON, state.JSONKey{Ptr: parent, Container: true})
				}
			}
		}
		prev, _ := doc.Get(op.ptr)
		if op.del {
			if prev == nil {
				continue
			}
			if err := doc.Delete(op.ptr); err != nil {
				return state.File{}, err
			}
		} else if err := doc.Set(op.ptr, op.value); err != nil {
			return state.File{}, fmt.Errorf("%s: %w", path, err)
		}
		f.JSON = append(f.JSON, state.JSONKey{Ptr: op.ptr, Prev: prev})
	}
	if err := edit.WriteFile(path, doc.Bytes(), secret); err != nil {
		return state.File{}, err
	}
	return f, nil
}

// undoJSON puts back the values applyJSON replaced, newest first.
func undoJSON(f state.File) error {
	data, existed, err := edit.ReadFile(f.Path)
	if err != nil || !existed {
		return err
	}
	doc, err := edit.ParseJSON(data)
	if err != nil {
		return fmt.Errorf("%s tidak bisa dibaca sebagai JSON, pengaturan Vikey di dalamnya tidak bisa dihapus otomatis: %v", f.Path, err)
	}
	for i := len(f.JSON) - 1; i >= 0; i-- {
		k := f.JSON[i]
		switch {
		case k.Container:
			if raw, ok := doc.Get(k.Ptr); ok && isEmptyObject(raw) {
				if err := doc.Delete(k.Ptr); err != nil {
					return err
				}
			}
		case len(k.Prev) == 0:
			if err := doc.Delete(k.Ptr); err != nil {
				return err
			}
		default:
			if err := doc.SetRaw(k.Ptr, k.Prev); err != nil {
				return err
			}
		}
	}
	if f.Created && doc.Empty() {
		return os.Remove(f.Path)
	}
	return edit.WriteFile(f.Path, doc.Bytes(), false)
}

func isEmptyObject(raw json.RawMessage) bool {
	var m map[string]json.RawMessage
	return json.Unmarshal(raw, &m) == nil && len(m) == 0
}

// undoFiles undoes every recorded JSON file change of a tool.
func undoFiles(t *state.Tool) error {
	if t == nil {
		return nil
	}
	for i := len(t.Files) - 1; i >= 0; i-- {
		if len(t.Files[i].JSON) == 0 {
			continue
		}
		if err := undoJSON(t.Files[i]); err != nil {
			return err
		}
	}
	return nil
}
