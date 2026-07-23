package api

import "testing"

func TestParseByteRange(t *testing.T) {
	tests := []struct {
		name       string
		header     string
		size       int64
		start, end int64
		ok         bool
	}{
		{name: "bounded", header: "bytes=10-19", size: 100, start: 10, end: 19, ok: true},
		{name: "open ended", header: "bytes=90-", size: 100, start: 90, end: 99, ok: true},
		{name: "suffix", header: "bytes=-12", size: 100, start: 88, end: 99, ok: true},
		{name: "clamp", header: "bytes=95-120", size: 100, start: 95, end: 99, ok: true},
		{name: "past end", header: "bytes=100-", size: 100, ok: false},
		{name: "multiple", header: "bytes=0-1,4-5", size: 100, ok: false},
		{name: "invalid unit", header: "items=0-1", size: 100, ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, ok := parseByteRange(tt.header, tt.size)
			if ok != tt.ok || start != tt.start || end != tt.end {
				t.Fatalf("parseByteRange(%q, %d) = (%d, %d, %v), want (%d, %d, %v)", tt.header, tt.size, start, end, ok, tt.start, tt.end, tt.ok)
			}
		})
	}
}
