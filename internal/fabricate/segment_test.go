package fabricate

import (
	"bytes"
	"testing"
)

func concat(segs []Segment) []byte {
	var b []byte
	for _, s := range segs {
		b = append(b, s.Bytes...)
	}
	return b
}

func TestSplitSegments_ExactPartition(t *testing.T) {
	cases := [][]byte{
		[]byte("package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hi\")\n}\n"),
		[]byte("one line, no newline"),
		[]byte(""),
		[]byte("a\n"),
		[]byte("a\nb\nc"),
	}
	for i, c := range cases {
		segs := SplitSegments(c)
		if got := concat(segs); !bytes.Equal(got, c) {
			t.Fatalf("case %d: concat(segments) = %q, want %q", i, got, c)
		}
	}
}

func TestSplitSegments_IndicesContiguous(t *testing.T) {
	segs := SplitSegments([]byte("a\n\nb\n\nc\n\nd\n\ne\n"))
	for i, s := range segs {
		if s.Index != i {
			t.Fatalf("segment %d has Index %d", i, s.Index)
		}
	}
}

func TestSplitSegments_CoalescesShortBlocks(t *testing.T) {
	const numBlocks = 30
	var input []byte
	for i := 0; i < numBlocks; i++ {
		input = append(input, []byte("l\n\n")...)
	}

	segs := SplitSegments(input)

	if len(segs) >= numBlocks {
		t.Fatalf("expected coalescing: got %d segments for %d blocks", len(segs), numBlocks)
	}
	for i, s := range segs {
		if i == len(segs)-1 {
			continue // final segment absorbs the remainder
		}
		if s.EndLine-s.StartLine < minSegmentLines {
			t.Fatalf("segment %d spans %d lines, want >= %d",
				i, s.EndLine-s.StartLine, minSegmentLines)
		}
	}
	if got := concat(segs); !bytes.Equal(got, input) {
		t.Fatalf("concat(segments) = %q, want %q", got, input)
	}
}

func TestSplitSegments_EmptyFileOneSegment(t *testing.T) {
	segs := SplitSegments([]byte(""))
	if len(segs) != 1 {
		t.Fatalf("empty file should yield 1 segment, got %d", len(segs))
	}
}
