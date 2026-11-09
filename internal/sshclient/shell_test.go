package sshclient

import "testing"

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"/etc/openclash/x.txt": `'/etc/openclash/x.txt'`,
		"a'b":                  `'a'\''b'`,
		"$(touch pwn)":         `'$(touch pwn)'`,
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}
