package domainlist

import "testing"

func TestRenderLocalSectioned(t *testing.T) {
	plan := &Plan{
		HasSections: true,
		Preamble:    []string{"# top"},
		Sections: []PlanSection{
			{Name: "media", Comments: []string{"# video"}, Patterns: []string{"+.a.com", "+.b.com"}},
			{Name: "other", Patterns: []string{"+.z.com"}},
		},
	}
	want := "# top\n\n[media]\n# video\n+.a.com\n+.b.com\n\n[other]\n+.z.com\n"
	if got := string(RenderLocal(plan)); got != want {
		t.Fatalf("local render = %q, want %q", got, want)
	}
}

func TestRenderLocalFlat(t *testing.T) {
	plan := &Plan{Flat: []string{"+.a.com", "example.com"}}
	if got, want := string(RenderLocal(plan)), "+.a.com\nexample.com\n"; got != want {
		t.Fatalf("local render = %q, want %q", got, want)
	}
}

func TestRenderRemoteSectioned(t *testing.T) {
	plan := &Plan{
		HasSections: true,
		Sections: []PlanSection{
			{Name: "media", Patterns: []string{"+.a.com"}},
			{Name: "other", Patterns: []string{"+.z.com"}},
		},
	}
	want := "# [media]\n+.a.com\n# [other]\n+.z.com\n"
	if got := string(RenderRemote(plan)); got != want {
		t.Fatalf("remote render = %q, want %q", got, want)
	}
}

func TestRenderRemoteFlat(t *testing.T) {
	plan := &Plan{Flat: []string{"+.a.com", "example.com"}}
	if got, want := string(RenderRemote(plan)), "+.a.com\nexample.com\n"; got != want {
		t.Fatalf("remote render = %q, want %q", got, want)
	}
}

func TestRenderSkipsEmptySections(t *testing.T) {
	plan := &Plan{HasSections: true, Sections: []PlanSection{{Name: "empty"}, {Name: "full", Patterns: []string{"+.a.com"}}}}
	if got, want := string(RenderLocal(plan)), "[full]\n+.a.com\n"; got != want {
		t.Fatalf("render = %q, want %q", got, want)
	}
}
