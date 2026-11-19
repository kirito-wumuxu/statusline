package ui

import (
	"fmt"
	"strings"

	"statusline/internal/render"
	"statusline/internal/session"

	"github.com/charmbracelet/lipgloss"
)

// RenderDetailPane renders the metadata card for the selected session.
func RenderDetailPane(item *session.SessionItem, width, height int) string {
	borderStyle := StylePaneBorder.Width(width).Height(height).MaxHeight(height)
	if item == nil {
		return borderStyle.Render(" 会话元数据详情 \n\n  未选择会话")
	}

	labelStyle := lipgloss.NewStyle().Foreground(ColorMuted).Width(10)
	valStyle := lipgloss.NewStyle().Foreground(ColorText)

	// Tool name
	prod := item.Record.ProductType
	if strings.Contains(strings.ToLower(prod), "antigravity") {
		prod = "Google Antigravity (agy)"
	} else if strings.Contains(strings.ToLower(prod), "claude") {
		prod = "Anthropic Claude Code"
	}

	// CWD status with truncation to prevent wrapping
	maxCwdLen := width - 18
	if maxCwdLen < 15 {
		maxCwdLen = 15
	}
	cwdStr := item.Record.CWD
	if len(cwdStr) > maxCwdLen {
		cwdStr = "..." + cwdStr[len(cwdStr)-maxCwdLen+3:]
	}
	if !item.HasValidCWD {
		cwdStr += " " + lipgloss.NewStyle().Foreground(ColorRed).Bold(true).Render("⚠ [缺失]")
	} else {
		cwdStr += " " + lipgloss.NewStyle().Foreground(ColorGreen).Render("✓")
	}

	// Branch info
	branchInfo := "-"
	if item.Record.GitRepoName != "" || item.Record.GitBranch != "" {
		branchInfo = item.Record.GitRepoName
		if item.Record.GitBranch != "" {
			branchInfo += fmt.Sprintf(" (%s)", item.Record.GitBranch)
		}
	}

	// Model name
	modelStr := item.Record.LatestModelID
	if modelStr == "" {
		modelStr = "-"
	}

	// Token formatting
	inTokens := render.FormatTokenCount(item.Record.TotalInputTokens)
	outTokens := render.FormatTokenCount(item.Record.TotalOutputTokens)
	cacheTokens := render.FormatTokenCount(item.Record.TotalCacheReadTokens)
	totalTokens := render.FormatTokenCount(item.Record.TotalTokens)

	var sb strings.Builder
	sb.WriteString(" 会话元数据详情 \n")
	sb.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("工具类型:"), valStyle.Render(prod)))
	sb.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("会话 ID:"), valStyle.Render(item.Record.SessionID)))
	sb.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("工作目录:"), cwdStr))
	sb.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("Git 仓库:"), valStyle.Render(branchInfo)))
	sb.WriteString(fmt.Sprintf("%s %s\n", labelStyle.Render("活跃模型:"), valStyle.Render(modelStr)))
	sb.WriteString(fmt.Sprintf("%s ↑%s ↓%s ⚡%s (总计: %s)\n", labelStyle.Render("累计吞吐:"), inTokens, outTokens, cacheTokens, totalTokens))
	sb.WriteString(fmt.Sprintf("%s %s", labelStyle.Render("活跃时间:"), item.RelativeTime+" ("+item.Record.LastActiveAt+")"))

	return borderStyle.Render(sb.String())
}
