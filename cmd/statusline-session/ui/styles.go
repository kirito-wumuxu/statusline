package ui

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	ColorCyan      = lipgloss.Color("#38bdf8")
	ColorAmber     = lipgloss.Color("#fbbf24")
	ColorLavender  = lipgloss.Color("#b388ff")
	ColorGreen     = lipgloss.Color("#4ade80")
	ColorRed       = lipgloss.Color("#f87171")
	ColorMuted     = lipgloss.Color("#64748b")
	ColorDim       = lipgloss.Color("#334155")
	ColorText      = lipgloss.Color("#f1f5f9")
	ColorBorder    = lipgloss.Color("#1e293b")
	ColorBorderSel = lipgloss.Color("#0284c7")

	StyleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#ffffff")).
			Background(lipgloss.Color("#0369a1")).
			Padding(0, 1)

	StyleBadgeAgy = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorCyan).
			Background(lipgloss.Color("#0c4a6e")).
			Padding(0, 1)

	StyleBadgeClaude = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorAmber).
				Background(lipgloss.Color("#78350f")).
				Padding(0, 1)

	StyleBadgeOther = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorMuted).
			Background(lipgloss.Color("#1e293b")).
			Padding(0, 1)

	StylePaneBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Padding(0, 1)

	StylePaneBorderFocused = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorBorderSel).
				Padding(0, 1)

	StyleItemActive = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#ffffff")).
			Background(lipgloss.Color("#0f172a"))

	StyleItemInactive = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#cbd5e1"))

	StyleHelpKey = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorCyan)

	StyleHelpDesc = lipgloss.NewStyle().
			Foreground(ColorMuted)
)
