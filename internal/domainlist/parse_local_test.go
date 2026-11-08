package domainlist

import (
	"errors"
	"strings"
	"testing"
)

func TestParseLocalSectioned(t *testing.T) {
	data := "\xef\xbb\xbf# top\r\n\r\n[media]\r\n# video\r\n+.bitru.org\r\n-+.old.example\r\n\r\n[misc]\r\n+.example.org\r\n"
	got, err := ParseLocal([]byte(data), "domains.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !got.HasSections || len(got.Sections) != 2 {
		t.Fatalf("sections = %+v", got.Sections)
	}
	if got.Preamble[0] != "# top" {
		t.Fatalf("preamble = %q", got.Preamble)
	}
	if got.Sections[0].Name != "media" || got.Sections[0].Comments[0] != "# video" {
		t.Fatalf("section 0 = %+v", got.Sections[0])
	}
	if got.Sections[0].Entries[0] != "+.bitru.org" {
		t.Fatalf("entry = %q", got.Sections[0].Entries[0])
	}
	if len(got.Negatives) != 1 || got.Negatives[0].Pattern != "+.old.example" || got.Negatives[0].Section != "media" {
		t.Fatalf("negatives = %+v", got.Negatives)
	}
}

func TestParseLocalFlat(t *testing.T) {
	got, err := ParseLocal([]byte("# note\nexample.com\n*.example.org\n"), "domains.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got.HasSections {
		t.Fatal("should be flat")
	}
	if strings.Join(got.Flat, ",") != "example.com,*.example.org" {
		t.Fatalf("flat = %v", got.Flat)
	}
}

func TestParseLocalErrors(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{"entry before section", "example.com\n[media]\n+.a.com\n", "outside a section"},
		{"duplicate section", "[a]\n+.x.com\n[a]\n+.y.com\n", "duplicate section"},
		{"bad pattern", "[a]\nnot a domain\n", `"not a domain"`},
		{"bad header", "[a\n+.x.com\n", "invalid section header"},
		{"padded pattern", "[a]\n +.x.com\n", "whitespace"},
		{"section name too long", "[" + strings.Repeat("a", 65) + "]\n+.x.com\n", "64"},
		{"control char in section name", "[\x01]\n+.x.com\n", "control"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseLocal([]byte(tt.data), "domains.txt")
			if err == nil {
				t.Fatal("expected error")
			}
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("error = %T %v, want *ValidationError", err, err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want contains %q", err, tt.want)
			}
		})
	}
}

func TestParseLocalNegativeBeforeSection(t *testing.T) {
	_, err := ParseLocal([]byte("-old.example\n[media]\n+.a.com\n"), "domains.txt")
	if err == nil {
		t.Fatal("expected error")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error = %T %v, want *ValidationError", err, err)
	}
	if ve.File != "domains.txt" || ve.Line != 1 || ve.Text != "-old.example" {
		t.Fatalf("located at %s:%d %q", ve.File, ve.Line, ve.Text)
	}
	if !strings.Contains(err.Error(), "outside a section") {
		t.Fatalf("error = %v, want outside a section", err)
	}
}

func TestParseLocalFlatNegativesOnly(t *testing.T) {
	got, err := ParseLocal([]byte("-old.example\n-*.old.example.org\n"), "domains.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got.HasSections {
		t.Fatal("should be flat")
	}
	if len(got.Negatives) != 2 {
		t.Fatalf("negatives = %+v", got.Negatives)
	}
	if got.Negatives[0].Pattern != "old.example" || got.Negatives[0].Section != "" || got.Negatives[0].Line != 1 {
		t.Fatalf("negative 0 = %+v", got.Negatives[0])
	}
	if got.Negatives[1].Pattern != "*.old.example.org" || got.Negatives[1].Section != "" || got.Negatives[1].Line != 2 {
		t.Fatalf("negative 1 = %+v", got.Negatives[1])
	}
}

func TestParseLocalFlatCommentIsPreamble(t *testing.T) {
	got, err := ParseLocal([]byte("# note\nexample.com\n"), "domains.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Preamble) != 1 || got.Preamble[0] != "# note" {
		t.Fatalf("preamble = %q", got.Preamble)
	}
}

func TestParseLocalReportsFirstRecordBeforeSection(t *testing.T) {
	_, err := ParseLocal([]byte("-one.example\n-two.example\n[media]\n+.a.com\n"), "domains.txt")
	if err == nil {
		t.Fatal("expected error")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error = %T %v, want *ValidationError", err, err)
	}
	if ve.File != "domains.txt" || ve.Line != 1 || ve.Text != "-one.example" {
		t.Fatalf("located at %s:%d %q", ve.File, ve.Line, ve.Text)
	}
	if !strings.Contains(err.Error(), "outside a section") {
		t.Fatalf("error = %v, want outside a section", err)
	}
}
