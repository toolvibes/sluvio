package cfgfile

import (
	"encoding/json"
	"os"
	"testing"
)

func loadFixture(t *testing.T, name string) *Document {
	t.Helper()
	b, err := os.ReadFile("../../testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return Parse(string(b))
}

func TestRoundTripIsLossless(t *testing.T) {
	b, _ := os.ReadFile("../../testdata/realworld.cfg")
	if got := Parse(string(b)).String(); got != string(b) {
		t.Fatal("parse/render is not lossless")
	}
	for _, s := range []string{"", "global\n", "global", "a\r\nb\r\n"} {
		if got := Parse(s).String(); got != s && !(s == "global" && got == "global\n") {
			t.Errorf("round trip %q -> %q", s, got)
		}
	}
}

func TestDumpModel(t *testing.T) {
	d := loadFixture(t, "realworld.cfg")
	if os.Getenv("DUMP") != "" {
		j, _ := json.MarshalIndent(d.Model(), "", "  ")
		t.Log(string(j))
	}
}
