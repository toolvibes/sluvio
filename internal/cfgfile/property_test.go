package cfgfile

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// normalized returns "<comment depth> <words>" for every line.
func normalized(d *Document) []string {
	out := make([]string, len(d.Lines))
	for i, l := range d.Lines {
		depth := commentDepth(l)
		s := l
		for k := 0; k < depth; k++ {
			s = stripComment(s)
		}
		out[i] = fmt.Sprintf("%d %s", depth, strings.Join(strings.Fields(s), " "))
	}
	return out
}

func at(l []string, i int) string {
	if i < len(l) {
		return l[i]
	}
	return "<eof>"
}

// TestToggleSequencesRestoreOriginal disables and enables random servers and
// services of the real-world fixture in random order. After switching
// everything back to its original state the file must be identical.
func TestToggleSequencesRestoreOriginal(t *testing.T) {
	orig := loadFixture(t, "realworld.cfg")
	type sw struct {
		op, backend, name string
		initial           bool
	}
	var switches []sw
	for _, b := range orig.Model().Backends {
		// Services that are the default backend cannot be switched off.
		def := false
		for _, u := range b.UsedBy {
			if u.Default {
				def = true
			}
		}
		if !def {
			switches = append(switches, sw{op: "service.setEnabled", backend: b.Name, initial: b.Enabled})
		}
		if b.Enabled {
			for _, s := range b.Servers {
				switches = append(switches, sw{op: "server.setEnabled", backend: b.Name, name: s.Name, initial: !s.Maintenance})
			}
		}
	}
	rng := rand.New(rand.NewSource(42))
	for round := 0; round < 40; round++ {
		d := orig
		state := map[int]bool{}
		for k, s := range switches {
			state[k] = s.initial
		}
		steps := 1 + rng.Intn(8)
		var order []int
		for i := 0; i < steps; i++ {
			k := rng.Intn(len(switches))
			// Servers of disabled services are not toggled.
			if switches[k].op == "server.setEnabled" {
				parent := -1
				for j, s := range switches {
					if s.op == "service.setEnabled" && s.backend == switches[k].backend {
						parent = j
					}
				}
				if parent >= 0 && !state[parent] {
					continue
				}
			}
			state[k] = !state[k]
			order = append(order, k)
			v := state[k]
			nd, err := d.Apply([]Op{{Op: switches[k].op, Backend: switches[k].backend, Name: switches[k].name, Enabled: &v}})
			if err != nil {
				t.Fatalf("round %d step %d (%+v -> %v): %v", round, i, switches[k], v, err)
			}
			d = nd
		}
		// Undo in reverse order.
		for i := len(order) - 1; i >= 0; i-- {
			k := order[i]
			state[k] = !state[k]
			v := state[k]
			nd, err := d.Apply([]Op{{Op: switches[k].op, Backend: switches[k].backend, Name: switches[k].name, Enabled: &v}})
			if err != nil {
				t.Fatalf("round %d undo: %v", round, err)
			}
			d = nd
		}
		if d.String() != orig.String() {
			// Re-disabling a block may place the '#' after the indentation
			// instead of in column 0 (the original style cannot be known
			// once a block was enabled). Content and comment depth must be
			// identical; enabling a service also enables its ACLs and
			// disabling keeps them (unused ACLs are harmless).
			a, b := normalized(orig), normalized(d)
			for i, j := 0, 0; i < len(a) || j < len(b); {
				if i < len(a) && j < len(b) && a[i] == b[j] {
					i++
					j++
					continue
				}
				if j < len(b) && strings.HasPrefix(b[j], "0 acl ") {
					if i < len(a) && "1"+a[i][1:] == "1"+b[j][1:] && strings.HasPrefix(a[i], "1 acl ") {
						i++
					}
					j++
					continue
				}
				t.Fatalf("round %d: line differs after undo:\norig: %q\nnow:  %q", round, at(a, i), at(b, j))
			}
		}
		haproxyCheck(t, d.String())
	}
}
