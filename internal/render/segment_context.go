package render

import (
	"fmt"
	"math"
	"strings"

	"statusline/internal/model"
)

// FormatContext formats the 8-segment context bar, usage percentage, and window capacity.
// Example: `\\\\\\\\ 0% [1M]`
func FormatContext(data *model.StatusLineData) string {
	window := int64(1048576) // default 1M
	pct := 0

	if data != nil {
		if data.ContextWindow.ContextWindowSize > 0 {
			window = data.ContextWindow.ContextWindowSize
		}

		if data.ContextWindow.UsedPercentage > 0 {
			pct = int(math.Round(data.ContextWindow.UsedPercentage))
		} else if data.ContextWindow.TotalInputTokens+data.ContextWindow.TotalOutputTokens > 0 && window > 0 {
			pct = int(math.Round(float64(data.ContextWindow.TotalInputTokens+data.ContextWindow.TotalOutputTokens) / float64(window) * 100.0))
		}
	}

	if pct < 0 {
		pct = 0
	} else if pct > 100 {
		pct = 100
	}

	capStr := formatCapacity(window)

	// Calculate filled segments (0-8)
	filled := int(math.Round(float64(pct) / 100.0 * 8.0))
	if filled > 8 {
		filled = 8
	}
	if filled < 0 {
		filled = 0
	}

	barColor := AnsiGreen
	if pct >= 90 {
		barColor = AnsiRed
	} else if pct >= 70 {
		barColor = AnsiYellow
	}

	var bar string
	if filled == 0 {
		bar = fmt.Sprintf("%s\\\\\\\\\\\\\\\\%s", AnsiDarkGray, AnsiReset)
	} else if filled == 8 {
		bar = fmt.Sprintf("%s\\\\\\\\\\\\\\\\%s", barColor, AnsiReset)
	} else {
		bar = fmt.Sprintf("%s%s%s%s%s",
			barColor, strings.Repeat(`\`, filled),
			AnsiDarkGray, strings.Repeat(`\`, 8-filled),
			AnsiReset)
	}

	pctStr := fmt.Sprintf("%s%d%%%s", AnsiBrightWhite, pct, AnsiReset)
	capStyled := fmt.Sprintf("%s[%s%s%s]%s", AnsiDarkGray, AnsiWhite, capStr, AnsiDarkGray, AnsiReset)

	return fmt.Sprintf("%s %s %s", bar, pctStr, capStyled)
}

func formatCapacity(tokens int64) string {
	if tokens >= 1000000 {
		m := float64(tokens) / 1000000.0
		if m >= 1.0 && tokens%1000000 == 0 {
			return fmt.Sprintf("%dM", tokens/1000000)
		}
		if tokens == 1048576 {
			return "1M"
		}
		if tokens == 2097152 {
			return "2M"
		}
		return fmt.Sprintf("%.1fM", m)
	}
	if tokens >= 1000 {
		return fmt.Sprintf("%dk", tokens/1000)
	}
	return fmt.Sprintf("%d", tokens)
}
