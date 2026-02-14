package intl

import (
	"unicode/utf8"

	jsvalue "github.com/nnstd/gun/runtime/jsvalue"
)

// Segmenter segments strings into grapheme-like units.
// This is a simplified Go equivalent of Intl.Segmenter that segments by rune.
type Segmenter struct{}

// SegmentData holds a single segment from the segmenter.
type SegmentData struct {
	Segment string
}

// NewSegmenter creates a new Segmenter.
func NewSegmenter() *Segmenter {
	return &Segmenter{}
}

// Segment splits the input into individual rune segments.
// Accepts string or *jsvalue.JSValue.
func (s *Segmenter) Segment(v any) []SegmentData {
	var str string
	switch val := v.(type) {
	case string:
		str = val
	case *jsvalue.JSValue:
		str = val.String()
	default:
		return nil
	}

	var result []SegmentData
	for i := 0; i < len(str); {
		r, size := utf8.DecodeRuneInString(str[i:])
		result = append(result, SegmentData{Segment: string(r)})
		i += size
	}
	return result
}
