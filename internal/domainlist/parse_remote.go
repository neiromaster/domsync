package domainlist

import (
	"bytes"
	"errors"
	"strings"
)

func ParseRemote(data []byte, filename string) ([]string, error) {
	data = bytes.TrimPrefix(data, utf8BOM)
	lines := strings.Split(string(data), "\n")
	patterns := make([]string, 0)
	seen := make(map[string]struct{})

	for index, raw := range lines {
		line := strings.TrimSuffix(raw, "\r")
		trimmed := strings.TrimSpace(line)
		lineNo := index + 1

		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
			continue
		}

		if line != trimmed {
			return nil, &ValidationError{File: filename, Line: lineNo, Text: line, Err: errors.New("line has leading or trailing whitespace: start the pattern at the beginning of its line")}
		}

		pattern, err := NormalizePattern(line)
		if err != nil {
			return nil, &ValidationError{File: filename, Line: lineNo, Text: line, Err: err}
		}
		if _, ok := seen[pattern]; ok {
			continue
		}
		seen[pattern] = struct{}{}
		patterns = append(patterns, pattern)
	}

	return patterns, nil
}
