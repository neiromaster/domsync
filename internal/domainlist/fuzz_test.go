package domainlist

import "testing"

func FuzzParseLocal(f *testing.F) {
	f.Add([]byte("[media]\n+.example.com\n-*.old.example\n# note\n"))
	f.Add([]byte("\xef\xbb\xbfflat.example\r\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		lf, err := ParseLocal(data, "fuzz")
		if err != nil {
			return
		}
		plan, err := BuildPlan(Input{Local: lf, OtherSection: "other"})
		if err != nil {
			return
		}
		if _, err := ParseLocal(RenderLocal(plan), "fuzz"); err != nil {
			t.Fatalf("round trip failed: %v", err)
		}
	})
}

func FuzzParseRemote(f *testing.F) {
	f.Add([]byte("# [a]\n+.example.com\n"))
	f.Fuzz(func(_ *testing.T, data []byte) {
		_, _ = ParseRemote(data, "fuzz")
	})
}
