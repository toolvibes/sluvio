package diff

import "testing"

func TestCompute(t *testing.T) {
	a := "a\nb\nc\nd\ne\nf\ng\nh\n"
	b := "a\nb\nC\nd\ne\nf\ng\nh\ni\n"
	r := Compute(a, b, 1)
	if r.Added != 2 || r.Removed != 1 {
		t.Fatalf("got +%d -%d", r.Added, r.Removed)
	}
	if len(r.Hunks) != 2 {
		t.Fatalf("expected 2 hunks, got %d", len(r.Hunks))
	}
	if r.Hunks[0].OldStart != 2 {
		t.Fatalf("hunk starts at %d", r.Hunks[0].OldStart)
	}
	if got := Compute(a, a, 3); got.Added+got.Removed != 0 || len(got.Hunks) != 0 {
		t.Fatal("identical input must produce no hunks")
	}
	if got := Compute("", "x\n", 3); got.Added != 1 {
		t.Fatal("insert into empty file")
	}
}
