package history

import "testing"

func TestStore(t *testing.T) {
	s, err := Open(t.TempDir(), 3)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, err := s.Save("v"+string(rune('0'+i)), Entry{Kind: "change"}); err != nil {
			t.Fatal(err)
		}
	}
	l, _ := s.List(0)
	if len(l) != 3 || l[0].ID != 5 || l[2].ID != 3 {
		t.Fatalf("unexpected list %+v", l)
	}
	_, c, err := s.Get(4)
	if err != nil || c != "v3" {
		t.Fatalf("get: %q %v", c, err)
	}
	p, pc, _ := s.Previous(5)
	if p == nil || p.ID != 4 || pc != "v3" {
		t.Fatalf("previous: %+v", p)
	}
	if _, _, err := s.Get(1); err == nil {
		t.Fatal("pruned version should be gone")
	}
}
