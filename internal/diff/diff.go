// Package diff computes line based diffs for configuration files.
package diff

import (
	"fmt"
	"strings"
)

// Op is the type of a diff line.
type Op string

const (
	Equal  Op = "="
	Insert Op = "+"
	Delete Op = "-"
)

// Line is one line of a diff.
type Line struct {
	Op   Op     `json:"op"`
	Text string `json:"text"`
	Old  int    `json:"old,omitempty"` // 1-based line number in the old text
	New  int    `json:"new,omitempty"` // 1-based line number in the new text
}

// Hunk is a group of changes with surrounding context.
type Hunk struct {
	OldStart int    `json:"oldStart"`
	NewStart int    `json:"newStart"`
	Lines    []Line `json:"lines"`
}

// Result summarises a diff.
type Result struct {
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
	Hunks   []Hunk `json:"hunks"`
}

func split(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// Lines returns the full line diff between a and b.
func Lines(a, b string) []Line {
	x, y := split(a), split(b)
	// Strip common prefix and suffix to keep the DP table small.
	pre := 0
	for pre < len(x) && pre < len(y) && x[pre] == y[pre] {
		pre++
	}
	suf := 0
	for suf < len(x)-pre && suf < len(y)-pre && x[len(x)-1-suf] == y[len(y)-1-suf] {
		suf++
	}
	xm, ym := x[pre:len(x)-suf], y[pre:len(y)-suf]
	n, m := len(xm), len(ym)
	lcs := make([][]int32, n+1)
	for i := range lcs {
		lcs[i] = make([]int32, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if xm[i] == ym[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	var out []Line
	for k := 0; k < pre; k++ {
		out = append(out, Line{Op: Equal, Text: x[k], Old: k + 1, New: k + 1})
	}
	i, j := 0, 0
	for i < n || j < m {
		switch {
		case i < n && j < m && xm[i] == ym[j]:
			out = append(out, Line{Op: Equal, Text: xm[i], Old: pre + i + 1, New: pre + j + 1})
			i++
			j++
		case j < m && (i == n || lcs[i][j+1] > lcs[i+1][j]):
			out = append(out, Line{Op: Insert, Text: ym[j], New: pre + j + 1})
			j++
		default:
			out = append(out, Line{Op: Delete, Text: xm[i], Old: pre + i + 1})
			i++
		}
	}
	for k := 0; k < suf; k++ {
		out = append(out, Line{Op: Equal, Text: x[len(x)-suf+k], Old: len(x) - suf + k + 1, New: len(y) - suf + k + 1})
	}
	return out
}

// Compute returns hunks with the given number of context lines.
func Compute(a, b string, context int) Result {
	lines := Lines(a, b)
	var r Result
	for _, l := range lines {
		switch l.Op {
		case Insert:
			r.Added++
		case Delete:
			r.Removed++
		}
	}
	r.Hunks = []Hunk{}
	for k := 0; k < len(lines); {
		if lines[k].Op == Equal {
			k++
			continue
		}
		start := k - context
		if start < 0 {
			start = 0
		}
		end := k
		// extend while changes are within 2*context of each other
		for end < len(lines) {
			if lines[end].Op != Equal {
				end++
				continue
			}
			run := end
			for run < len(lines) && lines[run].Op == Equal {
				run++
			}
			if run < len(lines) && run-end <= 2*context {
				end = run
				continue
			}
			end += context
			if end > len(lines) {
				end = len(lines)
			}
			break
		}
		h := Hunk{Lines: append([]Line(nil), lines[start:end]...)}
		for _, l := range h.Lines {
			if l.Old > 0 && h.OldStart == 0 {
				h.OldStart = l.Old
			}
			if l.New > 0 && h.NewStart == 0 {
				h.NewStart = l.New
			}
		}
		r.Hunks = append(r.Hunks, h)
		k = end
	}
	return r
}

// Unified renders a result in unified diff format.
func Unified(r Result, oldName, newName string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "--- %s\n+++ %s\n", oldName, newName)
	for _, h := range r.Hunks {
		fmt.Fprintf(&b, "@@ -%d +%d @@\n", h.OldStart, h.NewStart)
		for _, l := range h.Lines {
			switch l.Op {
			case Equal:
				b.WriteString(" " + l.Text + "\n")
			case Insert:
				b.WriteString("+" + l.Text + "\n")
			case Delete:
				b.WriteString("-" + l.Text + "\n")
			}
		}
	}
	return b.String()
}
