package main

import "testing"

func TestParseCode(t *testing.T) {
	cases := map[string]string{
		"K7QX9P2MAB4Z":     "K7QX9P2MAB4Z",
		"  k7qx9p2mab4z  ": "K7QX9P2MAB4Z",
		"K7QX9P2M":         "K7QX9P2M", // codes made before 12 characters
		"curl -fsSL https://app.vikey.ai/i/K7QX9P2MAB4Z | sh": "K7QX9P2MAB4Z",
		"irm https://app.vikey.ai/i/K7QX9P2MAB4Z.ps1 | iex":   "K7QX9P2MAB4Z",
		"0123456789ABCDEF": "0123456789ABCDEF",
	}
	for in, want := range cases {
		got, ok := parseCode(in)
		if !ok || got != want {
			t.Errorf("parseCode(%q) = %q, %v, want %q", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "K7QX9P2", "K7QX9P2MAB4ZK7QX9", "K7QX9P2MAB4I", "hello world"} {
		if got, ok := parseCode(bad); ok {
			t.Errorf("parseCode(%q) = %q, want no code", bad, got)
		}
	}
}
