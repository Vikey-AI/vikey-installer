// Command e2echeck runs after the one-line installer in CI. It starts every
// agent once with a one-shot prompt, checks against the mock's request log
// that each one reached Vikey with the installed key, then runs doctor and
// uninstall and checks the agents' files are back.
//
//	e2echeck -mock http://127.0.0.1:18080 -vikey ~/.vikey/bin/vikey
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/vikey-ai/vikey-cli/internal/agents"
)

type oneShot struct {
	id    string
	args  []string
	agent string // substring of the User-Agent the mock should see
}

var shots = []oneShot{
	{"claude", []string{"-p", "balas ok"}, "claude-cli"},
	{"codex", []string{"exec", "--skip-git-repo-check", "balas ok"}, "codex"},
	{"opencode", []string{"run", "balas ok"}, "opencode"},
	{"hermes", []string{"-z", "balas ok"}, "OpenAI/Python"},
}

func main() {
	mock := flag.String("mock", "http://127.0.0.1:18080", "mock API")
	vikey := flag.String("vikey", "", "path of the installed vikey binary")
	only := flag.String("tools", "claude,codex,opencode,hermes", "agents to check")
	flag.Parse()

	failed := 0
	fail := func(format string, a ...any) {
		failed++
		fmt.Printf("FAIL "+format+"\n", a...)
	}
	ctx := context.Background()
	work, _ := os.MkdirTemp("", "vikey-e2e-")

	for _, s := range shots {
		if !strings.Contains(","+*only+",", ","+s.id+",") {
			continue
		}
		a := agents.ByID(s.id)
		d := a.Detect(ctx)
		if !d.Installed {
			fail("%s is not installed", s.id)
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		cmd := exec.CommandContext(cctx, d.Path, s.args...)
		cmd.Dir = work
		out, err := cmd.CombinedOutput()
		cancel()
		text := strings.TrimSpace(string(out))
		if err != nil || !strings.Contains(strings.ToLower(text), "ok") {
			fail("%s %s: %v\n%s", s.id, d.Version, err, tail(text, 30))
			continue
		}
		fmt.Printf("ok   %s %s answered\n", s.id, d.Version)
	}

	res, err := http.Get(*mock + "/__log")
	if err != nil {
		fail("mock log: %v", err)
	} else {
		var seen []map[string]any
		sc := bufio.NewScanner(res.Body)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			var r map[string]any
			if json.Unmarshal(sc.Bytes(), &r) == nil {
				seen = append(seen, r)
			}
		}
		res.Body.Close()
		for _, s := range shots {
			if !strings.Contains(","+*only+",", ","+s.id+",") {
				continue
			}
			hit := false
			for _, r := range seen {
				path, _ := r["path"].(string)
				agent, _ := r["userAgent"].(string)
				auth, _ := r["auth"].(string)
				if strings.Contains(agent, s.agent) && strings.HasPrefix(auth, "Bearer vk-e2e-") &&
					(strings.HasSuffix(path, "/messages") || strings.HasSuffix(path, "/responses") || strings.HasSuffix(path, "/chat/completions")) {
					hit = true
				}
			}
			if !hit {
				fail("%s: no chat request with the installed key reached the mock", s.id)
			} else {
				fmt.Printf("ok   %s used the Vikey key\n", s.id)
			}
		}
	}

	if *vikey != "" {
		if out, err := exec.Command(*vikey, "doctor").CombinedOutput(); err != nil {
			fail("vikey doctor: %v\n%s", err, out)
		} else {
			fmt.Printf("ok   vikey doctor\n%s\n", indent(out))
		}
		if out, err := exec.Command(*vikey, "uninstall", "--yes").CombinedOutput(); err != nil {
			fail("vikey uninstall: %v\n%s", err, out)
		} else {
			fmt.Printf("ok   vikey uninstall\n%s\n", indent(out))
		}
		// Every agent's settings either belong to someone else again or are gone.
		for _, s := range shots {
			if !strings.Contains(","+*only+",", ","+s.id+",") {
				continue
			}
			a := agents.ByID(s.id)
			if st := a.Inspect(ctx, a.Detect(ctx)); st.Configured {
				fail("%s still points at Vikey after uninstall (%s)", s.id, st.BaseURL)
			}
		}
		home, _ := os.UserHomeDir()
		if _, err := os.Stat(filepath.Join(home, ".vikey", "credentials.json")); err == nil {
			fail("credentials.json is still there after uninstall")
		}
	}

	if failed > 0 {
		fmt.Printf("\n%d check(s) failed\n", failed)
		os.Exit(1)
	}
	fmt.Println("\nall checks passed")
}

func tail(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func indent(b []byte) string {
	var out bytes.Buffer
	for _, l := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		out.WriteString("     " + l + "\n")
	}
	return out.String()
}
