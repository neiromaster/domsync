package domainlist

import (
	"strings"
	"testing"
)

func TestParseRemote(t *testing.T) {
	data := "\xef\xbb\xbf# [media]\r\n// old comment\r\n+.bitru.org\r\n\r\n*.example.org\r\n+.bitru.org\r\n"
	got, err := ParseRemote([]byte(data), "remote.txt")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "+.bitru.org,*.example.org" {
		t.Fatalf("patterns = %v", got)
	}
}

func TestParseRemoteRejectsInlineComment(t *testing.T) {
	_, err := ParseRemote([]byte("example.com # note\n"), "remote.txt")
	if err == nil || !strings.Contains(err.Error(), "whitespace") {
		t.Fatalf("error = %v, want whitespace error", err)
	}
}

func TestParseRemoteRejectsJunk(t *testing.T) {
	_, err := ParseRemote([]byte("+.a.com\ntypo!!\n"), "remote.txt")
	if err == nil || !strings.Contains(err.Error(), `"typo!!"`) {
		t.Fatalf("error = %v", err)
	}
}
