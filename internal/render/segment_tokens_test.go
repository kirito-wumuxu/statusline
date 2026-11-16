package render

import (
	"testing"
)

func TestFormatTokenCount(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0"},
		{-10, "0"},
		{500, "500"},
		{999, "999"},
		{1000, "1k"},
		{1500, "1.5k"},
		{94589, "94.6k"},
		{230601, "231k"},
		{999400, "999k"},
		{999950, "1M"},
		{1000000, "1M"},
		{1500000, "1.5M"},
		{16000000, "16M"},
		{202000000, "202M"},
		{999000000, "999M"},
		{999950000, "1B"},
		{1000000000, "1B"},
		{2599301144, "2.6B"},
		{999000000000, "999B"},
		{999950000000, "1T"},
		{1000000000000, "1T"},
	}

	for _, tc := range tests {
		actual := FormatTokenCount(tc.input)
		if actual != tc.expected {
			t.Errorf("FormatTokenCount(%d) = %q, expected %q", tc.input, actual, tc.expected)
		}
	}
}
