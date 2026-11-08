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
		Router:   &FileDelta{Path: "/r", Changed: true, Written: true, Added: []string{"+.a.com"}, Removed: []string{}},
		Local:    &FileDelta{Path: "./l", Changed: true, Written: true, Added: []string{"+.a.com"}, Removed: []string{}},
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
	router, ok := got["router"].(map[string]any)
	if !ok || router["written"] != true {
		t.Errorf("router written = %v in %s", got["router"], buf.String())
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

func TestJSONUpdateAndError(t *testing.T) {
	r := &Report{
		OK:     true,
		Update: &UpdateInfo{Current: "v1.0.0", Latest: "v1.1.0", Available: true},
		Error:  &ErrorInfo{Code: 2, Message: "bad domain"},
	}
	var buf bytes.Buffer
	if err := JSON(&buf, r); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	update, ok := got["update"].(map[string]any)
	if !ok {
		t.Fatalf("update = %T in %s", got["update"], buf.String())
	}
	for _, key := range []string{"current", "latest", "available"} {
		if _, ok := update[key]; !ok {
			t.Errorf("missing update key %q in %s", key, buf.String())
		}
	}
	for _, key := range []string{"Current", "Latest", "Available"} {
		if _, ok := update[key]; ok {
			t.Errorf("unexpected update key %q in %s", key, buf.String())
		}
	}
	failure, ok := got["error"].(map[string]any)
	if !ok {
		t.Fatalf("error = %T in %s", got["error"], buf.String())
	}
	for _, key := range []string{"code", "message"} {
		if _, ok := failure[key]; !ok {
			t.Errorf("missing error key %q in %s", key, buf.String())
		}
	}
	for _, key := range []string{"Code", "Message"} {
		if _, ok := failure[key]; ok {
			t.Errorf("unexpected error key %q in %s", key, buf.String())
		}
	}
}

func TestHumanNoChanges(t *testing.T) {
	var buf bytes.Buffer
	Human(&buf, &Report{OK: true, Total: 3, Sections: 1, Router: &FileDelta{Path: "/r"}, Local: &FileDelta{Path: "./l"}})
	if !strings.Contains(buf.String(), "изменений нет") {
		t.Fatalf("output = %q", buf.String())
	}
	want := "domsync: изменений нет (3 домена, 1 секция)\n  роутер: без изменений\n  локально: без изменений\n"
	if buf.String() != want {
		t.Fatalf("output = %q, want %q", buf.String(), want)
	}
}

func TestHumanChanged(t *testing.T) {
	var buf bytes.Buffer
	Human(&buf, &Report{
		OK: true, Changed: true, Total: 2, Sections: 1,
		Router: &FileDelta{Path: "/r", Changed: true, Written: true, Added: []string{"+.a.com"}, Removed: []string{"+.b.com"}, Backup: "/r.domsync.bak"},
		Local:  &FileDelta{Path: "./l", Changed: true, Written: true, Added: []string{"+.a.com"}, Removed: []string{"+.b.com"}, Moved: []Move{{Pattern: "+.c.com", From: "media", To: "misc"}}},
	})
	want := "domsync: 2 домена, 1 секция\n" +
		"  роутер: +1 −1, записан, бэкап /r.domsync.bak\n" +
		"  локально: +1 −1, записан, перемещён 1\n"
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
	want := "domsync: 2 домена, 1 секция\n" +
		"  роутер: +1 −1, будет записан (dry-run)\n" +
		"  локально: +1 −1, перемещён 1, будет записан (dry-run)\n"
	if buf.String() != want {
		t.Fatalf("output = %q, want %q", buf.String(), want)
	}
}

func TestHumanDryRunRootFlag(t *testing.T) {
	var buf bytes.Buffer
	Human(&buf, &Report{
		OK: true, Changed: true, DryRun: true, Total: 2, Sections: 1,
		Router: &FileDelta{Path: "/r", Changed: true, Added: []string{"+.a.com"}, Removed: []string{"+.b.com"}},
		Local:  &FileDelta{Path: "./l", Changed: true, Added: []string{"+.a.com"}, Removed: []string{"+.b.com"}},
	})
	want := "domsync: 2 домена, 1 секция\n" +
		"  роутер: +1 −1, будет записан (dry-run)\n" +
		"  локально: +1 −1, будет записан (dry-run)\n"
	if buf.String() != want {
		t.Fatalf("output = %q, want %q", buf.String(), want)
	}
}

func TestHumanRouterIgnoresMoved(t *testing.T) {
	var buf bytes.Buffer
	Human(&buf, &Report{
		OK: true, Changed: true, Total: 1, Sections: 1,
		Router: &FileDelta{Path: "/r", Changed: true, Written: true, Added: []string{}, Removed: []string{}, Moved: []Move{{Pattern: "+.a.com", From: "media", To: "misc"}}},
	})
	want := "domsync: 1 домен, 1 секция\n  роутер: +0 −0, записан\n"
	if buf.String() != want {
		t.Fatalf("output = %q, want %q", buf.String(), want)
	}
}

func TestHumanCommandAndWarning(t *testing.T) {
	var buf bytes.Buffer
	Human(&buf, &Report{
		OK: true, Changed: true, Total: 2, Sections: 1,
		Router:   &FileDelta{Path: "/r"},
		Local:    &FileDelta{Path: "./l"},
		Commands: []Command{{Command: "restart", ExitCode: 0, DurationMS: 2100}},
		Warnings: []string{"test warning"},
	})
	want := "domsync: 2 домена, 1 секция\n" +
		"  роутер: без изменений\n" +
		"  локально: без изменений\n" +
		"  команда: restart — ok, 2.1s\n" +
		"  предупреждение: test warning\n"
	if buf.String() != want {
		t.Fatalf("output = %q, want %q", buf.String(), want)
	}
}

func TestHumanCommandDurations(t *testing.T) {
	cases := []struct {
		ms   int64
		want string
	}{
		{2100, "  команда: restart — ok, 2.1s\n"},
		{20, "  команда: restart — ok, 20ms\n"},
		{999, "  команда: restart — ok, 999ms\n"},
		{0, "  команда: restart — ok, 0s\n"},
	}
	for _, tc := range cases {
		var buf bytes.Buffer
		Human(&buf, &Report{
			OK: true, Changed: true, Total: 1, Sections: 1,
			Commands: []Command{{Command: "restart", ExitCode: 0, DurationMS: tc.ms}},
		})
		if !strings.Contains(buf.String(), tc.want) {
			t.Errorf("duration %dms output = %q, want substring %q", tc.ms, buf.String(), tc.want)
		}
	}
}

func TestHumanNotOKNoHeader(t *testing.T) {
	var buf bytes.Buffer
	Human(&buf, &Report{OK: false, Total: 1, Sections: 1})
	if buf.Len() != 0 {
		t.Fatalf("output = %q, want empty", buf.String())
	}
}

func TestHumanPlural(t *testing.T) {
	cases := []struct {
		total    int
		sections int
		want     string
	}{
		{1, 1, "domsync: изменений нет (1 домен, 1 секция)\n"},
		{2, 3, "domsync: изменений нет (2 домена, 3 секции)\n"},
		{5, 5, "domsync: изменений нет (5 доменов, 5 секций)\n"},
		{11, 11, "domsync: изменений нет (11 доменов, 11 секций)\n"},
		{14, 3, "domsync: изменений нет (14 доменов, 3 секции)\n"},
		{21, 22, "domsync: изменений нет (21 домен, 22 секции)\n"},
	}
	for _, tc := range cases {
		var buf bytes.Buffer
		Human(&buf, &Report{OK: true, Total: tc.total, Sections: tc.sections})
		if buf.String() != tc.want {
			t.Errorf("total=%d sections=%d output = %q, want %q", tc.total, tc.sections, buf.String(), tc.want)
		}
	}
}

func TestHumanPartialError(t *testing.T) {
	var buf bytes.Buffer
	Human(&buf, &Report{
		OK: false, Changed: true, Total: 1, Sections: 1,
		Router:   &FileDelta{Path: "/r", Changed: true, Written: true, Added: []string{"+.a.com"}, Removed: []string{}},
		Commands: []Command{{Command: "/etc/init.d/openclash restart", ExitCode: 1, DurationMS: 20}},
		Warnings: []string{},
		Error:    &ErrorInfo{Code: 5, Message: "command failed"},
	})
	want := "domsync: 1 домен, 1 секция\n" +
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

func TestFailureNilError(t *testing.T) {
	r := Failure(1, nil)
	if r.Error == nil || r.Error.Message != "" {
		t.Fatalf("report = %+v", r)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
