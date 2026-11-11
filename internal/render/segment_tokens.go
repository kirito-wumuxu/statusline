package render

import (
	"fmt"
	"math"

	"statusline/internal/model"
)

// FormatTokenCount formats a token number into a compact string:
// e.g. 0 -> "0", 1500 -> "1.5k", 94589 -> "95k", 230601 -> "231k", 1500000 -> "1.5M"
func FormatTokenCount(n int64) string {
	if n < 0 {
		return "0"
	}
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 10000 {
		m := float64(n) / 1000.0
		if n%1000 == 0 {
			return fmt.Sprintf("%dk", n/1000)
		}
		return fmt.Sprintf("%.1fk", m)
	}
	if n < 1000000 {
		k := int64(math.Round(float64(n) / 1000.0))
		return fmt.Sprintf("%dk", k)
	}
	m := float64(n) / 1000000.0
	if n%1000000 == 0 {
		return fmt.Sprintf("%dM", n/1000000)
	}
	return fmt.Sprintf("%.1fM", m)
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
