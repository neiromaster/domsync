package domainlist

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

type ValidationError struct {
	File string
	Line int
	Text string
	Err  error
}

func (e *ValidationError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("%s:%d: %q: %v", e.File, e.Line, e.Text, e.Err)
	}
	return fmt.Sprintf("%s: %q: %v", e.File, e.Text, e.Err)
}

func (e *ValidationError) Unwrap() error {
	return e.Err
}

type Negative struct {
	Pattern string
	Section string
	Line    int
}

type Section struct {
	Name     string
	Comments []string
	Entries  []string
}

type LocalFile struct {
	Preamble    []string
	Sections    []Section
	Flat        []string
	HasSections bool
	Negatives   []Negative
}

var sectionHeader = regexp.MustCompile(`^\[([^\[\]\r\n]+)\]$`)

const maxSectionNameRunes = 64

func ParseLocal(data []byte, filename string) (*LocalFile, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	lines := strings.Split(string(data), "\n")
	file := &LocalFile{}
	seen := make(map[string]struct{})
	firstFlatLine := 0
	firstFlatText := ""

	invalid := func(line int, text string, err error) error {
		return &ValidationError{File: filename, Line: line, Text: text, Err: err}
	}

	for index, raw := range lines {
		line := strings.TrimSuffix(raw, "\r")
		trimmed := strings.TrimSpace(line)
		lineNo := index + 1

		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, "#") {
			if file.HasSections {
				last := len(file.Sections) - 1
				file.Sections[last].Comments = append(file.Sections[last].Comments, trimmed)
			} else {
				file.Preamble = append(file.Preamble, trimmed)
			}
			continue
		}

		if match := sectionHeader.FindStringSubmatch(trimmed); match != nil {
			name := strings.TrimSpace(match[1])
			if name == "" {
				return nil, invalid(lineNo, line, errors.New("invalid section header: empty section name"))
			}
			if _, ok := seen[name]; ok {
				return nil, invalid(lineNo, line, fmt.Errorf("duplicate section %q", name))
			}
			if runes := utf8.RuneCountInString(name); runes > maxSectionNameRunes {
				return nil, invalid(lineNo, line, fmt.Errorf("section name %q has %d runes; the maximum is %d", name, runes, maxSectionNameRunes))
			}
			for _, r := range name {
				if unicode.IsControl(r) {
					return nil, invalid(lineNo, line, fmt.Errorf("section name %q contains control character %q", name, r))
				}
			}
			if firstFlatLine != 0 {
				return nil, invalid(firstFlatLine, firstFlatText, errors.New("entry outside a section: add a [section] header before this entry"))
			}
			seen[name] = struct{}{}
			file.HasSections = true
			file.Sections = append(file.Sections, Section{Name: name})
			continue
		}

		if strings.HasPrefix(trimmed, "[") {
			return nil, invalid(lineNo, line, errors.New("invalid section header"))
		}

		if line != trimmed {
			return nil, invalid(lineNo, line, errors.New("entry has leading or trailing whitespace: start the pattern at the beginning of its line"))
		}

		if strings.HasPrefix(line, "-") {
			pattern, err := NormalizePattern(line[1:])
			if err != nil {
				return nil, invalid(lineNo, line, err)
			}
			section := ""
			if file.HasSections {
				section = file.Sections[len(file.Sections)-1].Name
			}
			file.Negatives = append(file.Negatives, Negative{Pattern: pattern, Section: section, Line: lineNo})
			continue
		}

		pattern, err := NormalizePattern(line)
		if err != nil {
			return nil, invalid(lineNo, line, err)
		}
		if file.HasSections {
			last := len(file.Sections) - 1
			file.Sections[last].Entries = append(file.Sections[last].Entries, pattern)
			continue
		}
		if firstFlatLine == 0 {
			firstFlatLine = lineNo
			firstFlatText = line
		}
		file.Flat = append(file.Flat, pattern)
	}

	return file, nil
}
