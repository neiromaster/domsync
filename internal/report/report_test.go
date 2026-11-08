package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestJSONShape(t *testing.T) {
	r := &Report{
		OK: true, Changed: true, Total: 2, Sections: 1,
		Router:   &FileDelta{Path: "/r", Changed: true, Added: []string{"+.a.com"}, Removed: []string{}},
		Local:    &FileDelta{Path: "./l", Changed: true, Added: []string{"+.a.com"}, Removed: []string{}},
		Warnings: []string{}, Commands: []Command{},
	}
	var buf bytes.Buffer
	if err := JSON(&buf, r); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"ok", "changed", "total", "sections", "router", "local", "warnings", "commands"} {
		if _, ok := got[key]; !ok {
			t.Errorf("missing key %q in %s", key, buf.String())
		}
	}
}

func TestJSONMoveTags(t *testing.T) {
	r := &Report{
		Local:    &FileDelta{Moved: []Move{{Pattern: "+.a.com", From: "media", To: "misc"}}},
		Warnings: []string{},
		Commands: []Command{},
	}
	var buf bytes.Buffer
	if err := JSON(&buf, r); err != nil {
		t.Fatal(err)
	}
	want := `"moved":[{"pattern":"+.a.com","from":"media","to":"misc"}]`
	if !strings.Contains(buf.String(), want) {
		t.Fatalf("output = %s, want substring %s", buf.String(), want)
	}
}

func TestJSONNilSlicesAreArrays(t *testing.T) {
	var buf bytes.Buffer
	if err := JSON(&buf, &Report{OK: true}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"warnings":[]`, `"commands":[]`} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("output = %s, want substring %s", buf.String(), want)
		}
	}
}

func TestHumanNoChanges(t *testing.T) {
	var buf bytes.Buffer
	Human(&buf, &Report{OK: true, Total: 3, Sections: 1, Router: &FileDelta{Path: "/r"}, Local: &FileDelta{Path: "./l"}})
	if !strings.Contains(buf.String(), "изменений нет") {
		t.Fatalf("output = %q", buf.String())
	}
	want := "domsync: изменений нет (3 доменов, 1 секций)\n  роутер: без изменений\n  локально: без изменений\n"
	if buf.String() != want {
		t.Fatalf("output = %q, want %q", buf.String(), want)
	}
}

func TestHumanChanged(t *testing.T) {
	var buf bytes.Buffer
	Human(&buf, &Report{
		OK: true, Changed: true, Total: 2, Sections: 1,
		Router: &FileDelta{Path: "/r", Changed: true, Added: []string{"+.a.com"}, Removed: []string{"+.b.com"}, Backup: "/r.domsync.bak"},
		Local:  &FileDelta{Path: "./l", Changed: true, Added: []string{"+.a.com"}, Removed: []string{"+.b.com"}, Moved: []Move{{Pattern: "+.c.com", From: "media", To: "misc"}}},
	})
	want := "domsync: 2 доменов, 1 секций\n" +
		"  роутер: +1 −1, записан, бэкап /r.domsync.bak\n" +
		"  локально: +1 −1, перемещён 1\n"
	if buf.String() != want {
		t.Fatalf("output = %q, want %q", buf.String(), want)
	}
}

func TestHumanDryRun(t *testing.T) {
	var buf bytes.Buffer
	Human(&buf, &Report{
		OK: true, Changed: true, DryRun: true, Total: 2, Sections: 1,
		Router: &FileDelta{Path: "/r", Changed: true, Skipped: true, Added: []string{"+.a.com"}, Removed: []string{"+.b.com"}},
		Local:  &FileDelta{Path: "./l", Changed: true, Skipped: true, Added: []string{"+.a.com"}, Removed: []string{"+.b.com"}, Moved: []Move{{Pattern: "+.c.com", From: "media", To: "misc"}}},
	})
	want := "domsync: 2 доменов, 1 секций\n" +
		"  роутер: +1 −1, будет записан (dry-run)\n" +
		"  локально: +1 −1, перемещён 1, будет записан (dry-run)\n"
	if buf.String() != want {
		t.Fatalf("output = %q, want %q", buf.String(), want)
	}
}

func TestHumanPartialError(t *testing.T) {
	var buf bytes.Buffer
	Human(&buf, &Report{
		OK: false, Changed: true, Total: 1, Sections: 1,
		Router:   &FileDelta{Path: "/r", Changed: true, Added: []string{"+.a.com"}, Removed: []string{}},
		Commands: []Command{{Command: "/etc/init.d/openclash restart", ExitCode: 1, DurationMS: 20}},
		Warnings: []string{},
		Error:    &ErrorInfo{Code: 5, Message: "command failed"},
	})
	want := "domsync: 1 доменов, 1 секций\n" +
		"  роутер: +1 −0, записан\n" +
		"  команда: /etc/init.d/openclash restart — ошибка 1\n"
	if buf.String() != want {
		t.Fatalf("output = %q, want %q", buf.String(), want)
	}
}

func TestHumanFailureWritesNothing(t *testing.T) {
	var buf bytes.Buffer
	Human(&buf, Failure(2, errString("bad domain")))
	if buf.Len() != 0 {
		t.Fatalf("output = %q, want empty", buf.String())
	}
}

func TestFailure(t *testing.T) {
	r := Failure(2, errString("bad domain"))
	if r.OK || r.Error == nil || r.Error.Code != 2 {
		t.Fatalf("report = %+v", r)
	}
	if r.Warnings == nil || r.Commands == nil {
		t.Fatalf("empty slices missing: %+v", r)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
