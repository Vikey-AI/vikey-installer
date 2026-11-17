package edit

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// TOML editing works on text, line by line, so the person's comments and
// layout stay exactly as they were. Every result is parsed again before it
// is written (ParseTOML), so a mistake here can never produce a broken file.

var tableHeader = regexp.MustCompile(`^\s*\[\[?\s*([^\]]*?)\s*\]\]?\s*(#.*)?$`)

type tomlLine struct {
	text     string
	header   string // table name when the line is a [table] or [[array]] header
	inString bool   // inside a multi-line string started on an earlier line
}

func scanTOML(text string) []tomlLine {
	raw := strings.Split(text, "\n")
	out := make([]tomlLine, 0, len(raw))
	delim := ""
	for _, l := range raw {
		tl := tomlLine{text: l}
		if delim != "" {
			tl.inString = true
			if strings.Count(l, delim)%2 == 1 {
				delim = ""
			}
			out = append(out, tl)
			continue
		}
		if m := tableHeader.FindStringSubmatch(l); m != nil {
			tl.header = strings.ReplaceAll(strings.ReplaceAll(m[1], `"`, ""), " ", "")
		} else {
			for _, d := range []string{`"""`, `'''`} {
				if strings.Count(l, d)%2 == 1 {
					delim = d
					break
				}
			}
		}
		out = append(out, tl)
	}
	return out
}

func joinTOML(lines []tomlLine) string {
	parts := make([]string, len(lines))
	for i, l := range lines {
		parts[i] = l.text
	}
	return strings.Join(parts, "\n")
}

func markerStart(name string) string { return "# >>> " + name + " >>>" }
func markerEnd(name string) string   { return "# <<< " + name + " <<<" }

// RemoveMarkedBlock removes a block written earlier by MarkedBlock, markers
// included, and returns the text without it.
func RemoveMarkedBlock(text, name string) string {
	lines := strings.Split(text, "\n")
	out := lines[:0:0]
	inside := false
	for _, l := range lines {
		t := strings.TrimSpace(l)
		switch {
		case t == markerStart(name):
			inside = true
		case t == markerEnd(name):
			inside = false
		case !inside:
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// MarkedBlock wraps lines in start and end markers so they can be found and
// removed again.
func MarkedBlock(name, note string, lines ...string) string {
	all := []string{markerStart(name)}
	if note != "" {
		all = append(all, "# "+note)
	}
	all = append(all, lines...)
	all = append(all, markerEnd(name))
	return strings.Join(all, "\n")
}

// TakeTopLevelKeys removes top-level assignments of keys (before the first
// table, outside multi-line strings) and returns the removed lines by key.
func TakeTopLevelKeys(text string, keys ...string) (string, map[string]string) {
	taken := map[string]string{}
	lines := scanTOML(text)
	out := lines[:0:0]
	topLevel := true
	for _, l := range lines {
		if l.header != "" {
			topLevel = false
		}
		if topLevel && !l.inString {
			if k := assignedKey(l.text); k != "" && contains(keys, k) {
				taken[k] = strings.TrimSpace(l.text)
				continue
			}
		}
		out = append(out, l)
	}
	return joinTOML(out), taken
}

// TakeTables removes the table named name and its sub-tables
// (name.something) and returns the removed text.
func TakeTables(text, name string) (string, string) {
	lines := scanTOML(text)
	var kept, removed []tomlLine
	inTable := false
	for _, l := range lines {
		if l.header != "" {
			inTable = l.header == name || strings.HasPrefix(l.header, name+".")
		}
		if inTable {
			removed = append(removed, l)
		} else {
			kept = append(kept, l)
		}
	}
	return joinTOML(kept), strings.TrimRight(joinTOML(removed), "\n")
}

var keyRe = regexp.MustCompile(`^\s*("?)([A-Za-z0-9_\-]+)("?)\s*=`)

func assignedKey(line string) string {
	m := keyRe.FindStringSubmatch(line)
	if m == nil || m[1] != m[3] {
		return ""
	}
	return m[2]
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// TOMLString encodes s as a TOML basic string.
func TOMLString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u%04X`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// ParseTOML parses text into a generic map.
func ParseTOML(text string) (map[string]any, error) {
	m := map[string]any{}
	if err := toml.Unmarshal([]byte(text), &m); err != nil {
		return nil, err
	}
	return m, nil
}

// TidyBlankLines collapses runs of blank lines and trims the ends.
func TidyBlankLines(text string) string {
	lines := strings.Split(text, "\n")
	out := lines[:0:0]
	blank := 0
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			blank++
			if blank > 1 {
				continue
			}
		} else {
			blank = 0
		}
		out = append(out, l)
	}
	return strings.Trim(strings.Join(out, "\n"), "\n")
}

// DisabledPrefix marks a line setup commented out. Uninstall removes the
// prefix again, so the line comes back exactly where it was.
const DisabledPrefix = "# vikey-disabled: "

// DisableTopLevelKeys comments out top-level assignments of keys in place.
func DisableTopLevelKeys(text string, keys ...string) string {
	lines := scanTOML(text)
	topLevel := true
	for i, l := range lines {
		if l.header != "" {
			topLevel = false
		}
		if topLevel && !l.inString {
			if k := assignedKey(l.text); k != "" && contains(keys, k) {
				lines[i].text = DisabledPrefix + l.text
			}
		}
	}
	return joinTOML(lines)
}

// DisableTables comments out the table name and its sub-tables in place.
// Blank lines and comments inside it stay as they are.
func DisableTables(text, name string) string {
	lines := scanTOML(text)
	inTable := false
	for i, l := range lines {
		if l.header != "" {
			inTable = l.header == name || strings.HasPrefix(l.header, name+".")
		}
		t := strings.TrimSpace(l.text)
		if inTable && t != "" && !strings.HasPrefix(t, "#") {
			lines[i].text = DisabledPrefix + l.text
		}
	}
	return joinTOML(lines)
}

// EnableDisabled undoes DisableTopLevelKeys and DisableTables.
func EnableDisabled(text string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimPrefix(l, DisabledPrefix)
	}
	return strings.Join(lines, "\n")
}

// Wrap puts top before body and bottom after it, each separated by one blank
// line. addedNewline reports that body had no final newline and got one.
func Wrap(top, body, bottom string) (out string, addedNewline bool) {
	if body == "" {
		return top + "\n\n" + bottom + "\n", false
	}
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
		addedNewline = true
	}
	return top + "\n\n" + body + "\n" + bottom + "\n", addedNewline
}

// Unwrap removes the marked blocks Wrap added and the blank lines next to
// them, and reports whether any block was found.
func Unwrap(text, topName, bottomName string) (string, bool) {
	found := false
	if i := strings.Index(text, markerStart(topName)); i >= 0 {
		if j := strings.Index(text[i:], markerEnd(topName)); j >= 0 {
			end := i + j + len(markerEnd(topName))
			if strings.HasPrefix(text[end:], "\n") {
				end++
			}
			if strings.HasPrefix(text[end:], "\n") {
				end++
			}
			text = text[:i] + text[end:]
			found = true
		}
	}
	if i := strings.Index(text, markerStart(bottomName)); i >= 0 {
		if j := strings.Index(text[i:], markerEnd(bottomName)); j >= 0 {
			end := i + j + len(markerEnd(bottomName))
			if strings.HasPrefix(text[end:], "\n") {
				end++
			}
			start := i
			if start >= 2 && text[start-1] == '\n' && text[start-2] == '\n' {
				start--
			}
			text = text[:start] + text[end:]
			found = true
		}
	}
	return text, found
}
