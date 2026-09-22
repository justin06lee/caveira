package tools

import (
	"fmt"
	"strings"
)

// unifiedDiff renders a line diff of before → after with n lines of context.
// It is a plain LCS over lines: files edited by the agent are small enough
// for that, and the output only has to read well, not be applied by patch.
func unifiedDiff(before, after string, n int) string {
	a := splitLines(before)
	b := splitLines(after)

	// Trim the common prefix and suffix first so the LCS only sees the
	// changed region; that keeps the quadratic part tiny for local edits.
	pre := 0
	for pre < len(a) && pre < len(b) && a[pre] == b[pre] {
		pre++
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre && a[len(a)-1-suf] == b[len(b)-1-suf] {
		suf++
	}
	midA := a[pre : len(a)-suf]
	midB := b[pre : len(b)-suf]

	type op struct {
		kind byte // ' ', '-', '+'
		text string
	}
	var ops []op
	if len(midA)*len(midB) > 4_000_000 {
		// Too big to diff nicely; show it as a straight replacement.
		for _, l := range midA {
			ops = append(ops, op{'-', l})
		}
		for _, l := range midB {
			ops = append(ops, op{'+', l})
		}
	} else {
		ops = lcsOps(midA, midB, func(k byte, s string) op { return op{k, s} })
	}

	// Context from the trimmed prefix/suffix.
	ctxBefore := a[max(0, pre-n):pre]
	ctxAfter := a[len(a)-suf : min(len(a), len(a)-suf+n)]

	var sb strings.Builder
	oldStart := max(0, pre-n) + 1
	newStart := oldStart
	oldLen := len(ctxBefore) + len(midA) + len(ctxAfter)
	newLen := len(ctxBefore) + len(midB) + len(ctxAfter)
	fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@\n", oldStart, oldLen, newStart, newLen)
	for _, l := range ctxBefore {
		sb.WriteString(" " + l + "\n")
	}
	for _, o := range ops {
		sb.WriteByte(o.kind)
		sb.WriteString(o.text)
		sb.WriteByte('\n')
	}
	for _, l := range ctxAfter {
		sb.WriteString(" " + l + "\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// lcsOps walks a longest-common-subsequence table to emit ' ', '-', '+' ops.
func lcsOps[T any](a, b []string, mk func(byte, string) T) []T {
	m, n := len(a), len(b)
	table := make([][]int, m+1)
	for i := range table {
		table[i] = make([]int, n+1)
	}
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			if a[i] == b[j] {
				table[i][j] = table[i+1][j+1] + 1
			} else {
				table[i][j] = max(table[i+1][j], table[i][j+1])
			}
		}
	}
	var out []T
	i, j := 0, 0
	for i < m && j < n {
		switch {
		case a[i] == b[j]:
			out = append(out, mk(' ', a[i]))
			i++
			j++
		case table[i+1][j] >= table[i][j+1]:
			out = append(out, mk('-', a[i]))
			i++
		default:
			out = append(out, mk('+', b[j]))
			j++
		}
	}
	for ; i < m; i++ {
		out = append(out, mk('-', a[i]))
	}
	for ; j < n; j++ {
		out = append(out, mk('+', b[j]))
	}
	return out
}
