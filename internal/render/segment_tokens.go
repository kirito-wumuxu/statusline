package render

import (
	"fmt"
	"math"
	"strings"

	"statusline/internal/model"
)

// FormatTokenCount formats a token number into a compact string with carryover:
// Ensures the integer part never exceeds 3 digits (e.g. 1000k -> 1M, 1000M -> 1B, 1000B -> 1T).
// e.g. 0 -> "0", 1500 -> "1.5k", 94589 -> "95k", 230601 -> "231k", 1500000 -> "1.5M", 2590000000 -> "2.59B", 1000000000000 -> "1T"
func FormatTokenCount(n int64) string {
	if n <= 0 {
		return "0"
	}
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}

	type unitDef struct {
		val    float64
		suffix string
	}
	units := []unitDef{
		{val: 1e12, suffix: "T"},
		{val: 1e9, suffix: "B"},
		{val: 1e6, suffix: "M"},
		{val: 1e3, suffix: "k"},
	}

	f := float64(n)
	for i, u := range units {
		if f >= u.val*0.9995 {
			v := f / u.val
			var s string
			if v < 10 {
				s = fmt.Sprintf("%.2f", v)
			} else if v < 100 {
				s = fmt.Sprintf("%.1f", v)
			} else {
				s = fmt.Sprintf("%.0f", math.Round(v))
			}
			// Trim trailing zeros after decimal point
			s = strings.TrimRight(strings.TrimRight(s, "0"), ".")

			// If rounding caused an overflow to 1000
			if s == "1000" && i > 0 {
				nextU := units[i-1]
				nextV := f / nextU.val
				var nextS string
				if nextV < 10 {
					nextS = fmt.Sprintf("%.2f", nextV)
				} else if nextV < 100 {
					nextS = fmt.Sprintf("%.1f", nextV)
				} else {
					nextS = fmt.Sprintf("%.0f", math.Round(nextV))
				}
				nextS = strings.TrimRight(strings.TrimRight(nextS, "0"), ".")
				return nextS + nextU.suffix
			}
			return s + u.suffix
		}
	}
	return fmt.Sprintf("%d", n)
}

// FormatTokens formats total input (↑) and output (↓) token counts for the conversation.
// Example: `↑0 ↓0` or `↑231k ↓95k`
func FormatTokens(data *model.StatusLineData) string {
	var input, output int64
	if data != nil {
		input = data.ContextWindow.TotalInputTokens
		output = data.ContextWindow.TotalOutputTokens
	}

	inStr := FormatTokenCount(input)
	outStr := FormatTokenCount(output)

	return fmt.Sprintf("%s↑%s%s%s%s %s↓%s%s%s%s",
		AnsiCyan, AnsiReset, AnsiWhite, inStr, AnsiReset,
		AnsiMagenta, AnsiReset, AnsiWhite, outStr, AnsiReset)
}
