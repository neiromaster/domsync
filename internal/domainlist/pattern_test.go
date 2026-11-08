package domainlist

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizePatternValid(t *testing.T) {
	valid := map[string]string{
		"example.com":             "example.com",
		"Example.COM":             "example.com",
		".example.com":            ".example.com",
		"*.example.com":           "*.example.com",
		"+.example.com":           "+.example.com",
		"*.*.example.com":         "*.*.example.com",
		"subdomain.*.example.com": "subdomain.*.example.com",
		"+.com":                   "+.com",
		"_dmarc.example.com":      "_dmarc.example.com",
		"xn--e1afmkfd.xn--p1ai":   "xn--e1afmkfd.xn--p1ai",
		"*":                       "*",
	}
	for in, want := range valid {
		got, err := NormalizePattern(in)
		if err != nil {
			t.Errorf("NormalizePattern(%q) error = %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("NormalizePattern(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizePatternInvalid(t *testing.T) {
	invalid := []string{
		"", "example.com.", " example.com", "example.com ",
		"a..b", "a b.com", "+example.com", "example.+com", "a+.com",
		"a*b.com", "example.com/path", "пример.рф", "+", "-example.com",
		"example-.com", "a\x00b.com", "a\x7fb.com", "a\x01b.com",
	}
	for _, in := range invalid {
		if _, err := NormalizePattern(in); err == nil {
			t.Errorf("NormalizePattern(%q) succeeded, want error", in)
		} else if !errors.Is(err, ErrInvalidPattern) {
			t.Errorf("NormalizePattern(%q) error = %v, want ErrInvalidPattern", in, err)
		}
	}
}

func TestNormalizePatternNonASCIIMentionsPunycode(t *testing.T) {
	_, err := NormalizePattern("пример.рф")
	if err == nil {
		t.Fatal("NormalizePattern error = nil, want error")
	}
	if !errors.Is(err, ErrInvalidPattern) {
		t.Errorf("NormalizePattern(\"пример.рф\") error = %v, want ErrInvalidPattern", err)
	}
	if !strings.Contains(err.Error(), "punycode") {
		t.Errorf("NormalizePattern(\"пример.рф\") error = %v, want message mentioning punycode", err)
	}
}
