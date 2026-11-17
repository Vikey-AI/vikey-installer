package edit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tailscale/hujson"
)

// JSON is a JSON or JSONC document that keeps its comments, key order and
// indentation style through edits. Paths are JSON Pointers (RFC 6901), build
// them with Ptr.
type JSON struct {
	root   hujson.Value
	indent string
}

// Ptr builds a JSON Pointer from raw keys, escaping "~" and "/".
func Ptr(keys ...string) string {
	var b strings.Builder
	for _, k := range keys {
		b.WriteByte('/')
		b.WriteString(strings.NewReplacer("~", "~0", "/", "~1").Replace(k))
	}
	return b.String()
}

// ParseJSON reads a document. Empty input is an empty object.
func ParseJSON(data []byte) (*JSON, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		data = []byte("{}")
	}
	root, err := hujson.Parse(data)
	if err != nil {
		return nil, err
	}
	if _, ok := root.Value.(*hujson.Object); !ok {
		return nil, fmt.Errorf("isi file bukan objek JSON")
	}
	return &JSON{root: root, indent: detectIndent(data)}, nil
}

// Get returns the value at ptr as compact standard JSON.
func (j *JSON) Get(ptr string) (json.RawMessage, bool) {
	v := j.root.Find(ptr)
	if v == nil {
		return nil, false
	}
	return standard(v.Pack()), true
}

// GetString returns the string at ptr, or "" when missing or not a string.
func (j *JSON) GetString(ptr string) string {
	raw, ok := j.Get(ptr)
	if !ok {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return ""
	}
	return s
}

// Set writes value (any JSON-marshalable Go value) at ptr, creating parent
// objects as needed. A parent that exists but is not an object is an error,
// so a file is never silently restructured.
func (j *JSON) Set(ptr string, value any) error {
	// Objects go in spread over lines, so the formatter keeps them that way
	// instead of squeezing them onto one line.
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return j.SetRaw(ptr, raw)
}

// SetRaw is Set with an already encoded value.
func (j *JSON) SetRaw(ptr string, raw json.RawMessage) error {
	tokens := strings.Split(ptr, "/")[1:]
	for i := 1; i < len(tokens); i++ {
		parent := "/" + strings.Join(tokens[:i], "/")
		v := j.root.Find(parent)
		if v == nil {
			if err := j.patch("add", parent, json.RawMessage("{}")); err != nil {
				return err
			}
			continue
		}
		if _, ok := v.Value.(*hujson.Object); !ok {
			return fmt.Errorf("%s bukan objek, tidak bisa menambahkan %s", parent, ptr)
		}
	}
	return j.patch("add", ptr, raw)
}

// Delete removes the value at ptr. A missing value is not an error.
func (j *JSON) Delete(ptr string) error {
	if j.root.Find(ptr) == nil {
		return nil
	}
	return j.patch("remove", ptr, nil)
}

// Empty reports whether the document is an object with no members.
func (j *JSON) Empty() bool {
	obj, ok := j.root.Value.(*hujson.Object)
	return ok && len(obj.Members) == 0
}

// Bytes formats the document, keeping the indentation style it was read with.
// A document without comments or trailing commas comes out as plain
// indented JSON, the way most tools write it. One with comments goes through
// the HuJSON formatter, which keeps them.
func (j *JSON) Bytes() []byte {
	v := j.root.Clone()
	// Standardize blanks out comments and drops trailing commas, so the text
	// only stays the same when there were none.
	std := v.Clone()
	std.Standardize()
	if bytes.Equal(std.Pack(), v.Pack()) {
		var out bytes.Buffer
		if json.Indent(&out, std.Pack(), "", j.indent) == nil {
			out.WriteByte('\n')
			return out.Bytes()
		}
	}
	v.Format()
	out := v.Pack()
	if j.indent != "\t" {
		lines := bytes.Split(out, []byte("\n"))
		for i, l := range lines {
			n := 0
			for n < len(l) && l[n] == '\t' {
				n++
			}
			if n > 0 {
				lines[i] = append([]byte(strings.Repeat(j.indent, n)), l[n:]...)
			}
		}
		out = bytes.Join(lines, []byte("\n"))
	}
	if !bytes.HasSuffix(out, []byte("\n")) {
		out = append(out, '\n')
	}
	return out
}

func (j *JSON) patch(op, path string, value json.RawMessage) error {
	m := map[string]any{"op": op, "path": path}
	if value != nil {
		m["value"] = value
	}
	p, err := json.Marshal([]any{m})
	if err != nil {
		return err
	}
	return j.root.Patch(p)
}

func standard(b []byte) json.RawMessage {
	v, err := hujson.Parse(b)
	if err != nil {
		return json.RawMessage(bytes.TrimSpace(b))
	}
	v.Standardize()
	var out bytes.Buffer
	if json.Compact(&out, v.Pack()) != nil {
		return json.RawMessage(bytes.TrimSpace(v.Pack()))
	}
	return out.Bytes()
}

// detectIndent returns the indentation of the first indented line, or two
// spaces for a file with none.
func detectIndent(data []byte) string {
	for _, l := range bytes.Split(data, []byte("\n")) {
		if len(l) == 0 {
			continue
		}
		if l[0] == '\t' {
			return "\t"
		}
		n := 0
		for n < len(l) && l[n] == ' ' {
			n++
		}
		if n > 0 && n < len(l) {
			return strings.Repeat(" ", n)
		}
	}
	return "  "
}
