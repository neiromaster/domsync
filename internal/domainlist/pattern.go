package domainlist

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

var ErrInvalidPattern = errors.New("invalid domain pattern")

func NormalizePattern(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("empty pattern: %w", ErrInvalidPattern)
	}
	if strings.HasSuffix(raw, ".") {
		return "", fmt.Errorf("pattern %q has a trailing dot: %w", raw, ErrInvalidPattern)
	}
	trimmed := strings.TrimFunc(raw, unicode.IsSpace)
	if trimmed != raw {
		return "", fmt.Errorf("pattern %q has leading or trailing whitespace: %w", raw, ErrInvalidPattern)
	}
	for _, r := range trimmed {
		if r > unicode.MaxASCII {
			return "", fmt.Errorf("pattern %q contains non-ASCII %q, punycode (xn-- form) only: %w", raw, r, ErrInvalidPattern)
		}
	}
	if strings.ContainsRune(trimmed, '/') {
		return "", fmt.Errorf("pattern %q contains a slash: %w", raw, ErrInvalidPattern)
	}
	if strings.IndexFunc(trimmed, unicode.IsSpace) >= 0 {
		return "", fmt.Errorf("pattern %q contains internal whitespace: %w", raw, ErrInvalidPattern)
	}
	for _, r := range trimmed {
		if r < 0x20 || r == 0x7F {
			return "", fmt.Errorf("pattern %q contains control character %q (U+%04X): %w", raw, r, r, ErrInvalidPattern)
		}
	}

	lowered := strings.ToLower(trimmed)
	parts := strings.Split(lowered, ".")
	for i, part := range parts {
		if part != "" {
			continue
		}
		if i == 0 && len(parts) > 1 {
			continue
		}
		return "", fmt.Errorf("pattern %q has an empty label: %w", raw, ErrInvalidPattern)
	}
	for _, part := range parts {
		if part == "" {
			continue
		}
		if strings.HasPrefix(part, "-") || strings.HasSuffix(part, "-") {
			return "", fmt.Errorf("pattern %q has label %q containing a leading or trailing hyphen: %w", raw, part, ErrInvalidPattern)
		}
	}
	for i, part := range parts {
		if strings.Contains(part, "+") && (i != 0 || part != "+" || len(parts) == 1) {
			return "", fmt.Errorf("pattern %q misplaces '+': it must be the whole first label of a multi-label pattern: %w", raw, ErrInvalidPattern)
		}
		if strings.Contains(part, "*") && part != "*" {
			return "", fmt.Errorf("pattern %q misplaces '*': it must occupy a whole label: %w", raw, ErrInvalidPattern)
		}
	}
	return lowered, nil
}
