package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateWalksEveryEnabledPair(t *testing.T) {
	cfgPath, _ := setupServers(t, []serverSpec{
		{name: "router", files: []pairSpec{
			{source: "media.txt", local: "[media]\n+.a.com\n+.b.com\n", target: "/etc/openclash/media.txt"},
			{source: "ai.txt", local: "+.c.com\n+.d.com\n+.e.com\n", target: "/etc/openclash/ai.txt"},
		}},
		{name: "vps-1", files: []pairSpec{
			{source: "vps.txt", local: "[misc]\n+.f.com\n", target: "/etc/mihomo/vps.txt"},
		}},
	})
	res, err := validate(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK || len(res.Servers) != 2 {
		t.Fatalf("result = %+v", res)
	}
	dir := filepath.Dir(cfgPath)
	assertFile := func(file validateFile, source string, total, sections int) {
		t.Helper()
		if !file.OK || file.Error != nil {
			t.Fatalf("file %s not ok: %+v", file.Source, file)
		}
		if want := filepath.Join(dir, source); file.Source != want {
			t.Fatalf("source = %q, want %q", file.Source, want)
		}
		if file.Total != total || file.Sections != sections {
			t.Fatalf("%s counts = %d/%d, want %d/%d", file.Source, file.Total, file.Sections, total, sections)
		}
	}
	router := res.Servers[0]
	if router.Name != "router" || !router.OK || router.Skipped || len(router.Files) != 2 {
		t.Fatalf("router = %+v", router)
	}
	assertFile(router.Files[0], "media.txt", 2, 1)
	assertFile(router.Files[1], "ai.txt", 3, 0)
	vps := res.Servers[1]
	if vps.Name != "vps-1" || !vps.OK || vps.Skipped || len(vps.Files) != 1 {
		t.Fatalf("vps = %+v", vps)
	}
	assertFile(vps.Files[0], "vps.txt", 1, 1)
}

func TestValidateContinuesAfterOneBadPair(t *testing.T) {
	cfgPath, _ := setupServer(t, "router", []pairSpec{
		{source: "broken.txt", local: "not a valid line\n", target: "/etc/broken.txt"},
		{source: "ok.txt", local: "[media]\n+.a.com\n+.b.com\n", target: "/etc/ok.txt"},
	})
	res, err := validate(cfgPath)
	if err != nil {
		t.Fatalf("pair failure must be recorded, not returned: %v", err)
	}
	if res.OK || len(res.Servers) != 1 {
		t.Fatalf("result = %+v", res)
	}
	router := res.Servers[0]
	if router.OK || len(router.Files) != 2 {
		t.Fatalf("router = %+v", router)
	}
	broken := router.Files[0]
	if broken.OK || broken.Error == nil || broken.Error.Code != 2 {
		t.Fatalf("broken = %+v", broken)
	}
	if !strings.Contains(broken.Error.Message, "broken.txt") {
		t.Fatalf("message = %q", broken.Error.Message)
	}
	good := router.Files[1]
	if !good.OK || good.Error != nil || good.Total != 2 || good.Sections != 1 {
		t.Fatalf("good = %+v", good)
	}

	var stdout, stderr bytes.Buffer
	if code := runValidate(CLI{Config: cfgPath}, &stdout, &stderr); code != 2 {
		t.Fatalf("exit = %d, want 2; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), filepath.Join(filepath.Dir(cfgPath), "ok.txt")) {
		t.Fatalf("human output hides the second pair: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "ошибка: ") {
		t.Fatalf("human output hides the pair error: %q", stdout.String())
	}
}

func TestValidateSkipsDisabledServer(t *testing.T) {
	cfgPath, _ := setupServers(t, []serverSpec{
		{name: "router", files: []pairSpec{
			{source: "media.txt", local: "[media]\n+.a.com\n", target: "/etc/media.txt"},
		}},
		{name: "parked", disabled: true, files: []pairSpec{
			{source: "parked.txt", local: "not a valid line\n", target: "/etc/parked.txt"},
		}},
	})
	res, err := validate(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK || len(res.Servers) != 2 {
		t.Fatalf("result = %+v", res)
	}
	parked := res.Servers[1]
	if !parked.Skipped || !parked.OK || parked.Files == nil || len(parked.Files) != 0 {
		t.Fatalf("parked = %+v", parked)
	}

	var stdout, stderr bytes.Buffer
	if code := runValidate(CLI{Config: cfgPath}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "parked: пропущен (disabled)") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestValidateTreatsAbsentSourceAsEmpty(t *testing.T) {
	cfgPath, _ := setupServer(t, "router", []pairSpec{
		{source: "missing.txt", local: "", target: "/etc/missing.txt"},
	})
	if err := os.Remove(filepath.Join(filepath.Dir(cfgPath), "missing.txt")); err != nil {
		t.Fatal(err)
	}
	res, err := validate(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	file := res.Servers[0].Files[0]
	if !file.OK || file.Error != nil || file.Total != 0 || file.Sections != 0 {
		t.Fatalf("file = %+v", file)
	}
}

func TestRunValidateJSONShape(t *testing.T) {
	cfgPath, _ := setupServers(t, []serverSpec{
		{name: "router", files: []pairSpec{
			{source: "media.txt", local: "[media]\n+.a.com\n+.b.com\n", target: "/etc/media.txt"},
			{source: "broken.txt", local: "not a valid line\n", target: "/etc/broken.txt"},
		}},
		{name: "parked", disabled: true, files: []pairSpec{
			{source: "parked.txt", local: "+.c.com\n", target: "/etc/parked.txt"},
		}},
	})
	var stdout, stderr bytes.Buffer
	if code := runValidate(CLI{Config: cfgPath, JSON: true}, &stdout, &stderr); code != 2 {
		t.Fatalf("exit = %d, want 2; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
	var got map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("%v: %q", err, stdout.String())
	}
	for _, key := range []string{"config", "domains", "total", "sections"} {
		if _, ok := got[key]; ok {
			t.Fatalf("top-level %q must be gone: %v", key, got)
		}
	}
	if _, ok := got["error"]; ok {
		t.Fatalf("pair failures belong on the pair, not the run: %v", got)
	}
	if got["ok"] != false {
		t.Fatalf("ok = %v", got["ok"])
	}
	servers, ok := got["servers"].([]any)
	if !ok || len(servers) != 2 {
		t.Fatalf("servers = %v", got["servers"])
	}
	router := servers[0].(map[string]any)
	if router["name"] != "router" || router["ok"] != false || router["skipped"] != false {
		t.Fatalf("router = %v", router)
	}
	files, ok := router["files"].([]any)
	if !ok || len(files) != 2 {
		t.Fatalf("files = %v", router["files"])
	}
	pair := files[0].(map[string]any)
	if pair["source"] != filepath.Join(filepath.Dir(cfgPath), "media.txt") ||
		pair["ok"] != true || pair["total"] != float64(2) || pair["sections"] != float64(1) {
		t.Fatalf("pair = %v", pair)
	}
	broken := files[1].(map[string]any)
	if broken["ok"] != false {
		t.Fatalf("broken = %v", broken)
	}
	errInfo, ok := broken["error"].(map[string]any)
	if !ok || errInfo["code"] != float64(2) {
		t.Fatalf("broken error = %v", broken["error"])
	}
	parked := servers[1].(map[string]any)
	if parked["name"] != "parked" || parked["skipped"] != true || parked["ok"] != true {
		t.Fatalf("parked = %v", parked)
	}
	if files, ok := parked["files"].([]any); !ok || len(files) != 0 {
		t.Fatalf("parked files = %v", parked["files"])
	}
}

func TestRunValidateHumanOutput(t *testing.T) {
	cfgPath, _ := setupServers(t, []serverSpec{
		{name: "router", files: []pairSpec{
			{source: "media.txt", local: "[media]\n+.a.com\n+.b.com\n", target: "/etc/media.txt"},
			{source: "ai.txt", local: "+.c.com\n", target: "/etc/ai.txt"},
		}},
		{name: "parked", disabled: true, files: []pairSpec{
			{source: "parked.txt", local: "+.d.com\n", target: "/etc/parked.txt"},
		}},
	})
	dir := filepath.Dir(cfgPath)
	var stdout, stderr bytes.Buffer
	if code := runValidate(CLI{Config: cfgPath}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
	want := "конфигурация: " + cfgPath + "\n" +
		"router\n" +
		"  " + filepath.Join(dir, "media.txt") + ": 2 домена, 1 секция\n" +
		"  " + filepath.Join(dir, "ai.txt") + ": 1 домен, 0 секций\n" +
		"parked: пропущен (disabled)\n" +
		"domsync: 3 домена, 1 секция\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestRunValidateConfigError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runValidate(CLI{Config: filepath.Join(t.TempDir(), "nope.yaml"), JSON: true}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	var res validateOutput
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		t.Fatalf("%v: %q", err, stdout.String())
	}
	if res.OK || res.Error == nil || res.Error.Code != 1 || len(res.Servers) != 0 {
		t.Fatalf("result = %+v", res)
	}
	if stderr.Len() == 0 {
		t.Fatal("config failure must be reported on stderr")
	}
}

func TestRunValidateReturnsFirstFailureCode(t *testing.T) {
	cfgPath, _ := setupServer(t, "router", []pairSpec{
		{source: "huge.txt", local: strings.Repeat("+.a.com\n", maxLocalSize/8+1), target: "/etc/huge.txt"},
		{source: "broken.txt", local: "not a valid line\n", target: "/etc/broken.txt"},
	})
	var stdout, stderr bytes.Buffer
	if code := runValidate(CLI{Config: cfgPath}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d, want 1 (first failure in config order); stdout=%q", code, stdout.String())
	}
}
