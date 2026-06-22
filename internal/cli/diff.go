package cli

import (
	"fmt"
	"strings"
)

// unifiedDiff renders a line-based unified diff between a and b, with the
// standard ---/+++ file headers and @@ hunks. It is a compact stdlib
// implementation (LCS over lines, three lines of context) so jl does not shell
// out to the diff binary or take a dependency for the resume diff command. An
// empty result means the texts are identical.
func unifiedDiff(labelA, labelB, a, b string) string {
	aLines := splitLines(a)
	bLines := splitLines(b)
	ops := diffLines(aLines, bLines)
	hunks := groupHunks(ops, 3)
	if len(hunks) == 0 {
		return ""
	}
	var out strings.Builder
	fmt.Fprintf(&out, "--- %s\n", labelA)
	fmt.Fprintf(&out, "+++ %s\n", labelB)
	for _, h := range hunks {
		fmt.Fprintf(&out, "@@ -%s +%s @@\n", hunkRange(h.aStart, h.aLen), hunkRange(h.bStart, h.bLen))
		for _, op := range h.ops {
			switch op.kind {
			case opEqual:
				out.WriteString(" " + op.text + "\n")
			case opDelete:
				out.WriteString("-" + op.text + "\n")
			case opInsert:
				out.WriteString("+" + op.text + "\n")
			}
		}
	}
	return out.String()
}

// splitLines splits text into lines, dropping a single trailing newline so a
// file ending in "\n" does not yield a spurious empty final line.
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	text = strings.TrimSuffix(text, "\n")
	return strings.Split(text, "\n")
}

type opKind int

const (
	opEqual opKind = iota
	opDelete
	opInsert
)

type diffOp struct {
	kind opKind
	text string
}

// diffLines computes a line-level diff via an LCS table. Equal lines are
// emitted as context, lines only in a as deletions, lines only in b as
// insertions, in original order.
func diffLines(a, b []string) []diffOp {
	n, m := len(a), len(b)
	// lcs[i][j] = length of the longest common subsequence of a[i:] and b[j:].
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	var ops []diffOp
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, diffOp{opEqual, a[i]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			ops = append(ops, diffOp{opDelete, a[i]})
			i++
		default:
			ops = append(ops, diffOp{opInsert, b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, diffOp{opDelete, a[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, diffOp{opInsert, b[j]})
	}
	return ops
}

type hunk struct {
	aStart, aLen int
	bStart, bLen int
	ops          []diffOp
}

// groupHunks collapses the op stream into hunks, each carrying up to context
// lines of equal context around the runs of change and dropping long equal
// stretches between them.
func groupHunks(ops []diffOp, context int) []hunk {
	// Index of every changed op.
	var changed []int
	for i, op := range ops {
		if op.kind != opEqual {
			changed = append(changed, i)
		}
	}
	if len(changed) == 0 {
		return nil
	}

	// Build [start,end) windows around changes, merged when their context overlaps.
	type window struct{ start, end int }
	var windows []window
	for _, idx := range changed {
		s := idx - context
		if s < 0 {
			s = 0
		}
		e := idx + context + 1
		if e > len(ops) {
			e = len(ops)
		}
		if len(windows) > 0 && s <= windows[len(windows)-1].end {
			if e > windows[len(windows)-1].end {
				windows[len(windows)-1].end = e
			}
		} else {
			windows = append(windows, window{s, e})
		}
	}

	// Running line numbers (1-based) into a and b as we walk the full op stream.
	aLine, bLine := 1, 1
	pos := 0
	var hunks []hunk
	for _, w := range windows {
		// Advance line counters to the window start.
		for ; pos < w.start; pos++ {
			switch ops[pos].kind {
			case opEqual:
				aLine++
				bLine++
			case opDelete:
				aLine++
			case opInsert:
				bLine++
			}
		}
		h := hunk{aStart: aLine, bStart: bLine}
		for ; pos < w.end; pos++ {
			op := ops[pos]
			h.ops = append(h.ops, op)
			switch op.kind {
			case opEqual:
				h.aLen++
				h.bLen++
				aLine++
				bLine++
			case opDelete:
				h.aLen++
				aLine++
			case opInsert:
				h.bLen++
				bLine++
			}
		}
		hunks = append(hunks, h)
	}
	return hunks
}

// hunkRange formats a unified-diff range. A zero-length side is rendered as
// "start,0" per the diff convention with start being the line it follows.
func hunkRange(start, length int) string {
	if length == 1 {
		return fmt.Sprintf("%d", start)
	}
	if length == 0 {
		return fmt.Sprintf("%d,0", start-1)
	}
	return fmt.Sprintf("%d,%d", start, length)
}
