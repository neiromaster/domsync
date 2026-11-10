package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestJSONNestsServersAndPairs(t *testing.T) {
	rep := &Report{
		OK: true, Changed: true, Total: 3, Sections: 1,
		Servers: []ServerResult{{
			Name: "router", OK: true, Changed: true,
			Files: []FileResult{{
				Source: "/l/domains.txt", Target: "/r/media.txt", OK: true, Changed: true,
				Total: 3, Sections: 1,
				Router: &FileDelta{Path: "/r/media.txt", Changed: true, Written: true, Added: []string{"+.a.com"}},
				Local:  &FileDelta{Path: "/l/domains.txt", Changed: false},
			}},
			Commands: []Command{},
		}},
		Warnings: []string{},
	}
	var buf bytes.Buffer
	if err := JSON(&buf, rep); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["router"]; ok {
		t.Fatal("top-level router field must be gone")
	}
	servers, ok := got["servers"].([]any)
	if !ok || len(servers) != 1 {
		t.Fatalf("servers = %v", got["servers"])
	}
	server := servers[0].(map[string]any)
	files := server["files"].([]any)
	pair := files[0].(map[string]any)
	if pair["source"] != "/l/domains.txt" || pair["target"] != "/r/media.txt" {
		t.Fatalf("pair = %v", pair)
	}
	if pair["router"].(map[string]any)["written"] != true {
		t.Fatalf("router delta = %v", pair["router"])
	}
	if pair["local"].(map[string]any)["changed"] != false {
		t.Fatalf("local delta = %v", pair["local"])
	}
	if _, ok := server["commands"].([]any); !ok {
		t.Fatal("commands must be an empty array, not null")
	}
}

func TestJSONNormalizesNestedNilSlices(t *testing.T) {
	rep := &Report{
		Servers: []ServerResult{{
			Name:  "router",
			Files: []FileResult{{Source: "/l", Target: "/r", Router: &FileDelta{}}},
		}},
		Warnings: []string{},
	}
	var buf bytes.Buffer
	if err := JSON(&buf, rep); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "null") {
		t.Fatalf("null in output: %s", buf.String())
	}
}

func TestHumanPrintsServerBlocks(t *testing.T) {
	rep := &Report{
		OK: true, Changed: true, Total: 2, Sections: 1,
		Servers: []ServerResult{
			{Name: "router", Host: "192.168.1.1", OK: true, Changed: true, Files: []FileResult{{
				Source: "/l/domains.txt", Target: "/r/media.txt", OK: true, Changed: true, Total: 2, Sections: 1,
				Router: &FileDelta{Path: "/r/media.txt", Changed: true, Written: true, Added: []string{"+.a.com"}},
			}}},
			{Name: "vps-1", OK: true, Skipped: true},
		},
	}
	var buf bytes.Buffer
	Human(&buf, rep)
	want := "router (192.168.1.1)\n  /l/domains.txt → /r/media.txt\n    роутер: +1 −0, записан\nvps-1: пропущен (disabled)\ndomsync: 2 домена, 1 секция\n"
	if buf.String() != want {
		t.Fatalf("human = %q, want %q", buf.String(), want)
	}
}

func TestJSONMoveTags(t *testing.T) {
	r := &Report{
		Servers: []ServerResult{{
			Name: "router",
			Files: []FileResult{{
				Source: "/l", Target: "/r",
				Local: &FileDelta{Moved: []Move{{Pattern: "+.a.com", From: "media", To: "misc"}}},
			}},
		}},
		Warnings: []string{},
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
	for _, want := range []string{`"warnings":[]`, `"servers":[]`} {
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
	Human(&buf, &Report{
		OK: true, Total: 3, Sections: 1,
		Servers: []ServerResult{{
			Name: "router", Host: "192.168.1.1",
			Files: []FileResult{{Source: "/l", Target: "/r", Router: &FileDelta{Path: "/r"}, Local: &FileDelta{Path: "/l"}}},
		}},
	})
	want := "router (192.168.1.1)\n  /l → /r: без изменений\ndomsync: изменений нет (3 домена, 1 секция)\n"
	if buf.String() != want {
		t.Fatalf("output = %q, want %q", buf.String(), want)
	}
}

func TestHumanChanged(t *testing.T) {
	var buf bytes.Buffer
	Human(&buf, &Report{
		OK: true, Changed: true, Total: 2, Sections: 1,
		Servers: []ServerResult{{
			Name: "router", Host: "192.168.1.1",
			Files: []FileResult{{
				Source: "/l", Target: "/r",
				Router: &FileDelta{Path: "/r", Changed: true, Written: true, Added: []string{"+.a.com"}, Removed: []string{"+.b.com"}, Backup: "/r.domsync.bak"},
				Local:  &FileDelta{Path: "/l", Changed: true, Written: true, Added: []string{"+.a.com"}, Removed: []string{"+.b.com"}, Moved: []Move{{Pattern: "+.c.com", From: "media", To: "misc"}}},
			}},
		}},
	})
	want := "router (192.168.1.1)\n" +
		"  /l → /r\n" +
		"    роутер: +1 −1, записан, бэкап /r.domsync.bak\n" +
		"    локально: +1 −1, перемещён 1\n" +
		"domsync: 2 домена, 1 секция\n"
	if buf.String() != want {
		t.Fatalf("output = %q, want %q", buf.String(), want)
	}
}

func TestHumanDryRun(t *testing.T) {
	var buf bytes.Buffer
	Human(&buf, &Report{
		OK: true, Changed: true, DryRun: true, Total: 2, Sections: 1,
		Servers: []ServerResult{{
			Name: "router", Host: "192.168.1.1",
			Files: []FileResult{{
				Source: "/l", Target: "/r",
				Router: &FileDelta{Path: "/r", Changed: true, Skipped: true, Added: []string{"+.a.com"}, Removed: []string{"+.b.com"}},
				Local:  &FileDelta{Path: "/l", Changed: true, Skipped: true, Added: []string{"+.a.com"}, Removed: []string{"+.b.com"}, Moved: []Move{{Pattern: "+.c.com", From: "media", To: "misc"}}},
			}},
		}},
	})
	want := "router (192.168.1.1)\n" +
		"  /l → /r\n" +
		"    роутер: +1 −1, будет записан (dry-run)\n" +
		"    локально: +1 −1, перемещён 1, будет записан (dry-run)\n" +
		"domsync: 2 домена, 1 секция\n"
	if buf.String() != want {
		t.Fatalf("output = %q, want %q", buf.String(), want)
	}
}

func TestHumanDryRunRootFlag(t *testing.T) {
	var buf bytes.Buffer
	Human(&buf, &Report{
		OK: true, Changed: true, DryRun: true, Total: 2, Sections: 1,
		Servers: []ServerResult{{
			Name: "router", Host: "192.168.1.1",
			Files: []FileResult{{
				Source: "/l", Target: "/r",
				Router: &FileDelta{Path: "/r", Changed: true, Added: []string{"+.a.com"}, Removed: []string{"+.b.com"}},
				Local:  &FileDelta{Path: "/l", Changed: true, Added: []string{"+.a.com"}, Removed: []string{"+.b.com"}},
			}},
		}},
	})
	want := "router (192.168.1.1)\n" +
		"  /l → /r\n" +
		"    роутер: +1 −1, будет записан (dry-run)\n" +
		"    локально: +1 −1, будет записан (dry-run)\n" +
		"domsync: 2 домена, 1 секция\n"
	if buf.String() != want {
		t.Fatalf("output = %q, want %q", buf.String(), want)
	}
}

func TestHumanRouterIgnoresMoved(t *testing.T) {
	var buf bytes.Buffer
	Human(&buf, &Report{
		OK: true, Changed: true, Total: 1, Sections: 1,
		Servers: []ServerResult{{
			Name: "router", Host: "192.168.1.1",
			Files: []FileResult{{
				Source: "/l", Target: "/r",
				Router: &FileDelta{Path: "/r", Changed: true, Written: true, Added: []string{}, Removed: []string{}, Moved: []Move{{Pattern: "+.a.com", From: "media", To: "misc"}}},
			}},
		}},
	})
	want := "router (192.168.1.1)\n  /l → /r\n    роутер: +0 −0, записан\ndomsync: 1 домен, 1 секция\n"
	if buf.String() != want {
		t.Fatalf("output = %q, want %q", buf.String(), want)
	}
}

func TestHumanCommandAndWarning(t *testing.T) {
	var buf bytes.Buffer
	Human(&buf, &Report{
		OK: true, Changed: true, Total: 2, Sections: 1,
		Servers: []ServerResult{{
			Name: "router", Host: "192.168.1.1",
			Files:    []FileResult{{Source: "/l", Target: "/r", Router: &FileDelta{Path: "/r"}, Local: &FileDelta{Path: "/l"}}},
			Commands: []Command{{Command: "restart", ExitCode: 0, DurationMS: 2100}},
		}},
		Warnings: []string{"test warning"},
	})
	want := "router (192.168.1.1)\n" +
		"  /l → /r: без изменений\n" +
		"  команда: restart — ok, 2.1s\n" +
		"  предупреждение: test warning\n" +
		"domsync: 2 домена, 1 секция\n"
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
			Servers: []ServerResult{{
				Name:     "router",
				Commands: []Command{{Command: "restart", ExitCode: 0, DurationMS: tc.ms}},
			}},
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
		Servers: []ServerResult{{
			Name: "router", Host: "192.168.1.1", OK: false, Changed: true,
			Files: []FileResult{{
				Source: "/l", Target: "/r", OK: false,
				Router:   &FileDelta{Path: "/r", Changed: true, Written: true, Added: []string{"+.a.com"}, Removed: []string{}},
				Warnings: []string{"pair warning"},
				Error:    &ErrorInfo{Code: 2, Message: "bad pair"},
			}},
			Commands: []Command{{Command: "/etc/init.d/openclash restart", ExitCode: 1, DurationMS: 20}},
		}},
		Warnings: []string{},
		Error:    &ErrorInfo{Code: 5, Message: "command failed"},
	})
	want := "router (192.168.1.1)\n" +
		"  /l → /r\n" +
		"    роутер: +1 −0, записан\n" +
		"    ошибка: bad pair\n" +
		"    предупреждение: pair warning\n" +
		"  команда: /etc/init.d/openclash restart — ошибка 1\n" +
		"domsync: 1 домен, 1 секция\n"
	if buf.String() != want {
		t.Fatalf("output = %q, want %q", buf.String(), want)
	}
}

func TestHumanFailedPairPrintsNoChangesLine(t *testing.T) {
	var buf bytes.Buffer
	Human(&buf, &Report{
		OK: false,
		Servers: []ServerResult{{
			Name: "router", Host: "192.168.1.1", OK: false,
			Files: []FileResult{{
				Source: "/l", Target: "/r", OK: false,
				Error: &ErrorInfo{Code: 2, Message: "bad domain"},
			}},
		}},
	})
	want := "router (192.168.1.1)\n  /l → /r\n    ошибка: bad domain\n"
	if buf.String() != want {
		t.Fatalf("output = %q, want %q", buf.String(), want)
	}
}

func TestHumanServerError(t *testing.T) {
	var buf bytes.Buffer
	Human(&buf, &Report{
		OK: false,
		Servers: []ServerResult{{
			Name: "vps-1", Host: "10.0.0.1", OK: false, SkippedPairs: 2,
			Files: []FileResult{}, Commands: []Command{},
			Error: &ErrorInfo{Code: 3, Message: "dial tcp: connection refused"},
		}},
	})
	want := "vps-1: ошибка: dial tcp: connection refused (пропущено пар: 2)\n"
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
	if r.Warnings == nil || r.Servers == nil {
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
