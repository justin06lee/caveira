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

func TestSplitSegments_EmptyFileOneSegment(t *testing.T) {
	segs := SplitSegments([]byte(""))
	if len(segs) != 1 {
		t.Fatalf("empty file should yield 1 segment, got %d", len(segs))
	}
}
