package fabricate

import "bytes"

// minSegmentLines is the smallest number of source lines a segment may hold
// (except the final segment, which absorbs the remainder). Blank-line-delimited
// blocks shorter than this are coalesced with the following block.
const minSegmentLines = 8

// Segment is one contiguous slice of a file's bytes. The Bytes of a file's
// segments, concatenated in Index order, reproduce the file exactly.
type Segment struct {
	Index     int
	StartLine int // 0-based, inclusive
	EndLine   int // 0-based, exclusive
	Bytes     []byte
}

// SplitSegments partitions content into an ordered list of segments. Segments
// follow blank-line-delimited block boundaries, coalesced so each (except the
// last) spans at least minSegmentLines lines. An empty file yields exactly one
// empty segment.
func SplitSegments(content []byte) []Segment {
	lines := splitLines(content)
	if len(lines) == 0 {
		return []Segment{{Index: 0, StartLine: 0, EndLine: 0, Bytes: []byte{}}}
	}

	// Block boundaries: a boundary follows every blank line.
	var blockEnds []int
	for i, ln := range lines {
		if isBlank(ln) {
			blockEnds = append(blockEnds, i+1)
		}
	}
	if len(blockEnds) == 0 || blockEnds[len(blockEnds)-1] != len(lines) {
		blockEnds = append(blockEnds, len(lines))
	}

	var segs []Segment
	start := 0
	for _, end := range blockEnds {
		if end <= start {
			continue
		}
		// Coalesce: if this candidate segment is too short and is not the last,
		// keep extending by deferring the cut.
		if end-start < minSegmentLines && end != len(lines) {
			continue
		}
		segs = append(segs, makeSegment(len(segs), lines, start, end))
		start = end
	}
	if start < len(lines) {
		segs = append(segs, makeSegment(len(segs), lines, start, len(lines)))
	}
	if len(segs) == 0 {
		segs = append(segs, makeSegment(0, lines, 0, len(lines)))
	}
	return segs
}

func makeSegment(index int, lines [][]byte, start, end int) Segment {
	var b []byte
	for _, ln := range lines[start:end] {
		b = append(b, ln...)
	}
	return Segment{Index: index, StartLine: start, EndLine: end, Bytes: b}
}

// splitLines splits content into lines, each retaining its trailing '\n'.
// A final line without '\n' is retained as-is. Empty content yields no lines.
func splitLines(content []byte) [][]byte {
	if len(content) == 0 {
		return nil
	}
	var lines [][]byte
	start := 0
	for i := 0; i < len(content); i++ {
		if content[i] == '\n' {
			lines = append(lines, content[start:i+1])
			start = i + 1
		}
	}
	if start < len(content) {
		lines = append(lines, content[start:])
	}
	return lines
}

func isBlank(line []byte) bool {
	return len(bytes.TrimSpace(line)) == 0
}
