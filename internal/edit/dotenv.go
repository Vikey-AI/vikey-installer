package edit

import (
	"regexp"
	"strings"
)

// SetEnvLine sets NAME=value in .env text and returns the new text and the
// line it replaced ("" when the name was not set).
func SetEnvLine(text, name, value string) (string, string) {
	re := envLineRe(name)
	lines := strings.Split(text, "\n")
	prev := ""
	done := false
	for i, l := range lines {
		if re.MatchString(l) {
			if !done {
				prev = l
				lines[i] = name + "=" + value
				done = true
			} else {
				lines[i] = "" // a duplicate would win in some loaders
			}
		}
	}
	if !done {
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		lines = append(lines, name+"="+value, "")
	}
	return strings.Join(lines, "\n"), prev
}

// RestoreEnvLine puts back prev (a line SetEnvLine returned) or removes the
// name when prev is empty.
func RestoreEnvLine(text, name, prev string) string {
	re := envLineRe(name)
	lines := strings.Split(text, "\n")
	out := lines[:0:0]
	restored := false
	for _, l := range lines {
		if re.MatchString(l) {
			if prev != "" && !restored {
				out = append(out, prev)
				restored = true
			}
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

func envLineRe(name string) *regexp.Regexp {
	return regexp.MustCompile(`^\s*(export\s+)?` + regexp.QuoteMeta(name) + `\s*=`)
}
