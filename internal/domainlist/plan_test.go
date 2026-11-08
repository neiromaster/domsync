package domainlist

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func local(t *testing.T, src string) *LocalFile {
	t.Helper()
	lf, err := ParseLocal([]byte(src), "domains.txt")
	if err != nil {
		t.Fatal(err)
	}
	return lf
}

func patterns(plan *Plan) []string {
	var out []string
	for _, s := range plan.Sections {
		out = append(out, s.Patterns...)
	}
	return append(out, plan.Flat...)
}

func TestBuildPlanUnionPutsRemoteOnlyInOther(t *testing.T) {
	plan, err := BuildPlan(Input{
		Local:        local(t, "[media]\n+.a.com\n"),
		Remote:       []string{"+.a.com", "+.z.com"},
		OtherSection: "other",
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Sections[1].Name != "other" || !reflect.DeepEqual(plan.Sections[1].Patterns, []string{"+.z.com"}) {
		t.Fatalf("sections = %+v", plan.Sections)
	}
	if len(plan.RouterAdded) != 0 {
		t.Fatalf("router added = %v, want none", plan.RouterAdded)
	}
	if !reflect.DeepEqual(plan.LocalAdded, []string{"+.z.com"}) {
		t.Fatalf("local added = %v", plan.LocalAdded)
	}
}

func TestBuildPlanMove(t *testing.T) {
	plan, err := BuildPlan(Input{
		Local:        local(t, "[media]\n-+.s.com\n[misc]\n+.s.com\n"),
		Remote:       []string{"+.s.com"},
		OtherSection: "other",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.LocalMoved) != 1 || plan.LocalMoved[0].From != "media" || plan.LocalMoved[0].To != "misc" {
		t.Fatalf("moved = %+v", plan.LocalMoved)
	}
	if len(plan.Sections[0].Patterns) != 0 {
		t.Fatalf("media should be empty: %+v", plan.Sections[0])
	}
}

func TestBuildPlanDeleteRemoteOnly(t *testing.T) {
	plan, err := BuildPlan(Input{
		Local:        local(t, "[media]\n-+.z.com\n+.a.com\n"),
		Remote:       []string{"+.z.com", "+.a.com"},
		OtherSection: "other",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan.RouterRemoved, []string{"+.z.com"}) {
		t.Fatalf("router removed = %v", plan.RouterRemoved)
	}
}

func TestBuildPlanSameSectionNegativeWarns(t *testing.T) {
	plan, err := BuildPlan(Input{
		Local:        local(t, "[media]\n+.a.com\n-+.a.com\n"),
		Remote:       nil,
		OtherSection: "other",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Warnings) != 1 || !strings.Contains(plan.Warnings[0], "redundant") {
		t.Fatalf("warnings = %v", plan.Warnings)
	}
}

func TestBuildPlanMissingDeleteTargetWarns(t *testing.T) {
	plan, err := BuildPlan(Input{
		Local:        local(t, "[media]\n-+.gone.com\n+.a.com\n"),
		Remote:       nil,
		OtherSection: "other",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Warnings) != 1 || !strings.Contains(plan.Warnings[0], "not found") {
		t.Fatalf("warnings = %v", plan.Warnings)
	}
}

func TestBuildPlanErrors(t *testing.T) {
	tests := []struct {
		name  string
		local string
		other string
		want  string
	}{
		{"duplicate positive", "[a]\n+.x.com\n[b]\n+.x.com\n", "other", "duplicate"},
		{"duplicate negative", "[a]\n-+.x.com\n-+.x.com\n", "other", "duplicate"},
		{"empty final", "[a]\n-+.x.com\n", "other", "empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := BuildPlan(Input{Local: local(t, tt.local), OtherSection: tt.other})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestBuildPlanFlatAppendsRemote(t *testing.T) {
	plan, err := BuildPlan(Input{Local: local(t, "+.a.com\n"), Remote: []string{"+.z.com"}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan.Flat, []string{"+.a.com", "+.z.com"}) {
		t.Fatalf("flat = %v", plan.Flat)
	}
}

func TestGoldenBasic(t *testing.T) {
	read := func(name string) []byte {
		t.Helper()
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "golden", "basic", name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	lf, err := ParseLocal(read("local.txt"), "local.txt")
	if err != nil {
		t.Fatal(err)
	}
	remote, err := ParseRemote(read("remote.txt"), "remote.txt")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := BuildPlan(Input{Local: lf, Remote: remote, OtherSection: "other"})
	if err != nil {
		t.Fatal(err)
	}
	if got := RenderLocal(plan); !bytes.Equal(got, read("want-local.txt")) {
		t.Fatalf("local render mismatch:\n%s", got)
	}
	if got := RenderRemote(plan); !bytes.Equal(got, read("want-remote.txt")) {
		t.Fatalf("remote render mismatch:\n%s", got)
	}
}

func TestPlanRoundTrip(t *testing.T) {
	plan, err := BuildPlan(Input{
		Local:        local(t, "# top\n[media]\n+.a.com\n[misc]\n+.b.com\n-+.z.com\n"),
		Remote:       []string{"+.z.com"},
		OtherSection: "other",
	})
	if err != nil {
		t.Fatal(err)
	}
	reparsed, err := ParseLocal(RenderLocal(plan), "domains.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(reparsed.Negatives) != 0 {
		t.Fatalf("directives must not survive: %+v", reparsed.Negatives)
	}
	if got := reparsed.Sections[0].Entries; !reflect.DeepEqual(got, []string{"+.a.com"}) {
		t.Fatalf("entries = %v", got)
	}
}
