package ui

import (
	"fmt"
	"strings"

	"statusline/internal/session"

	"github.com/charmbracelet/lipgloss"
)

// RenderSessionList renders the scrollable list of session cards.
func RenderSessionList(items []session.SessionItem, cursor int, width, height int, active bool) string {
	if len(items) == 0 {
		emptyMsg := lipgloss.NewStyle().
			Foreground(ColorMuted).
			Padding(2, 2).
			Render("（未找到符合条件的活跃会话）")
		borderStyle := StylePaneBorder
		if active {
			borderStyle = StylePaneBorderFocused
		}
		return borderStyle.Width(width).Height(height).Render(emptyMsg)
	}

	contentHeight := height - 2
	if contentHeight < 1 {
		contentHeight = 1
	}

	// Calculate scrolling window
	startIdx := 0
	endIdx := len(items)
	if len(items) > contentHeight {
		startIdx = cursor - contentHeight/2
		if startIdx < 0 {
			startIdx = 0
		}
		endIdx = startIdx + contentHeight
		if endIdx > len(items) {
			endIdx = len(items)
			startIdx = endIdx - contentHeight
			if startIdx < 0 {
				startIdx = 0
			}
		}
	}

	var lines []string
	for i := startIdx; i < endIdx; i++ {
		item := items[i]
		isSelected := i == cursor

		cursorMark := "  "
		if isSelected {
			cursorMark = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("> ")
		}

		// Tool badge
		var badge string
		prod := strings.ToLower(item.Record.ProductType)
		if strings.Contains(prod, "antigravity") || strings.Contains(prod, "agy") {
			badge = StyleBadgeAgy.Render("[AGY]")
		} else if strings.Contains(prod, "claude") {
			badge = StyleBadgeClaude.Render("[Claude]")
		} else {
			badge = StyleBadgeOther.Render("[" + item.Record.ProductType + "]")
		}

		// Title
		name := item.DisplayName
		if isSelected {
			name = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffffff")).Render(name)
		} else {
			name = lipgloss.NewStyle().Foreground(lipgloss.Color("#e2e8f0")).Render(name)
		}

		// Branch
		branchStr := ""
		if item.Record.GitBranch != "" {
			dirtyMark := ""
			if strings.Contains(item.Record.GitBranch, "*") {
				dirtyMark = "*"
			}
			branchStr = " " + lipgloss.NewStyle().Foreground(ColorLavender).Render("("+item.Record.GitBranch+dirtyMark+")")
		}

		// Time & node tag
		timeTag := lipgloss.NewStyle().Foreground(ColorMuted).Render(item.RelativeTime)
		nodeTag := ""
		if !item.IsLocalNode && item.Record.NodeID != "" {
			shortNode := item.Record.NodeID
			if len(shortNode) > 10 {
				shortNode = shortNode[:10]
			}
			nodeTag = " " + lipgloss.NewStyle().Foreground(ColorAmber).Render("["+shortNode+"]")
		}

		line := fmt.Sprintf("%s%s %s%s %s%s", cursorMark, badge, name, branchStr, timeTag, nodeTag)
		if isSelected {
			line = lipgloss.NewStyle().Background(lipgloss.Color("#1e293b")).Width(width - 4).Render(line)
		}
		lines = append(lines, line)
	}

	content := strings.Join(lines, "\n")
	borderStyle := StylePaneBorder
	if active {
		borderStyle = StylePaneBorderFocused
	}

	header := fmt.Sprintf(" 会话列表 (%d/%d) ", cursor+1, len(items))
	return borderStyle.Width(width).Height(height).Render(header + "\n" + content)
}
