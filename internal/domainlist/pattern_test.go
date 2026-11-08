package domainlist

import (
	"errors"
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
		"example-.com",
	}
	for _, in := range invalid {
		if _, err := NormalizePattern(in); err == nil {
			t.Errorf("NormalizePattern(%q) succeeded, want error", in)
		} else if !errors.Is(err, ErrInvalidPattern) {
			t.Errorf("NormalizePattern(%q) error = %v, want ErrInvalidPattern", in, err)
		}
	}
}
