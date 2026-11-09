package app

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestValidateCountsSectionsAndEntries(t *testing.T) {
	cfgPath, _ := setup(t, "[media]\n+.a.com\n+.b.com\n\n[misc]\n+.c.com\n\n[empty]\n", "")
	res, err := validate(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK || res.Total != 3 || res.Sections != 2 {
		t.Fatalf("result = %+v", res)
	}
	if res.Config != cfgPath || res.Domains != filepath.Join(filepath.Dir(cfgPath), "domains.txt") {
		t.Fatalf("paths = %q %q", res.Config, res.Domains)
	}
}

func TestValidateCountsFlatList(t *testing.T) {
	cfgPath, _ := setup(t, "+.a.com\n+.b.com\n", "")
	res, err := validate(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 2 || res.Sections != 0 {
		t.Fatalf("result = %+v", res)
	}
}

func TestValidateRejectsInvalidLocalList(t *testing.T) {
	cfgPath, _ := setup(t, "+.a.com # inline\n", "")
	_, err := validate(cfgPath)
	if err == nil || ExitCode(err) != 2 {
		t.Fatalf("code=%d err=%v", ExitCode(err), err)
	}
}

func TestRunValidateJSON(t *testing.T) {
	cfgPath, _ := setup(t, "+.a.com\n", "")
	var stdout, stderr bytes.Buffer
	code := runValidate(CLI{Config: cfgPath, JSON: true}, &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	var res validateOutput
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		t.Fatalf("%v: %q", err, stdout.String())
	}
	if !res.OK || res.Total != 1 || res.Sections != 0 || res.Config != cfgPath {
		t.Fatalf("result = %+v", res)
	}
}

func TestRunValidateFailureIsJSON(t *testing.T) {
	cfgPath, _ := setup(t, "not a valid line\n", "")
	var stdout, stderr bytes.Buffer
	code := runValidate(CLI{Config: cfgPath, JSON: true}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("code = %d", code)
	}
	var res validateOutput
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		t.Fatalf("%v: %q", err, stdout.String())
	}
	if res.OK || res.Error == nil || res.Error.Code != 2 {
		t.Fatalf("result = %+v", res)
	}
}

func TestRunValidateHuman(t *testing.T) {
	cfgPath, _ := setup(t, "[media]\n+.a.com\n+.b.com\n", "")
	var stdout, stderr bytes.Buffer
	code := runValidate(CLI{Config: cfgPath}, &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	want := "domsync: конфигурация и локальный список в порядке (доменов: 2, секций: 1)\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q", stdout.String())
	}
}
