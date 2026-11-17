package ui

import (
	"fmt"
	"strings"

	"statusline/internal/render"
	"statusline/internal/session"

	"github.com/charmbracelet/lipgloss"
)

// RenderDetailPane renders the metadata card for the selected session.
func RenderDetailPane(item *session.SessionItem, width int) string {
	if item == nil {
		return StylePaneBorder.Width(width).Render("未选择会话")
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

	// CWD status
	cwdStr := item.Record.CWD
	if !item.HasValidCWD {
		cwdStr += " " + lipgloss.NewStyle().Foreground(ColorRed).Bold(true).Render("⚠ (本地目录不存在)")
	}

	// Token formatting
	inTokens := render.FormatTokenCount(item.Record.TotalInputTokens)
	outTokens := render.FormatTokenCount(item.Record.TotalOutputTokens)
	cacheTokens := render.FormatTokenCount(item.Record.TotalCacheReadTokens)
	totalTokens := render.FormatTokenCount(item.Record.TotalTokens)

	var sb strings.Builder
	sb.WriteString("┌─ 会话元数据 ───────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("│ %s %s\n", labelStyle.Render("工具类型:"), valStyle.Render(prod)))
	sb.WriteString(fmt.Sprintf("│ %s %s\n", labelStyle.Render("会话 ID:"), valStyle.Render(item.Record.SessionID)))
	sb.WriteString(fmt.Sprintf("│ %s %s\n", labelStyle.Render("工作目录:"), cwdStr))

	if item.Record.GitRepoName != "" || item.Record.GitBranch != "" {
		branchInfo := item.Record.GitRepoName
		if item.Record.GitBranch != "" {
			branchInfo += fmt.Sprintf(" (%s)", item.Record.GitBranch)
		}
		sb.WriteString(fmt.Sprintf("│ %s %s\n", labelStyle.Render("Git 仓库:"), valStyle.Render(branchInfo)))
	}

	modelStr := item.Record.LatestModelID
	if modelStr == "" {
		modelStr = "-"
	}
	sb.WriteString(fmt.Sprintf("│ %s %s\n", labelStyle.Render("活跃模型:"), valStyle.Render(modelStr)))
	sb.WriteString(fmt.Sprintf("│ %s ↑%s ↓%s ⚡%s (总计: %s)\n", labelStyle.Render("累计吞吐:"), inTokens, outTokens, cacheTokens, totalTokens))

	if item.Record.ContextUsedPct > 0 {
		sb.WriteString(fmt.Sprintf("│ %s %.1f%%\n", labelStyle.Render("上下文:"), item.Record.ContextUsedPct))
	}
	sb.WriteString(fmt.Sprintf("│ %s %s\n", labelStyle.Render("活跃时间:"), item.RelativeTime+" ("+item.Record.LastActiveAt+")"))
	sb.WriteString("└─────────────────────────────────────────────")

	return StylePaneBorder.Width(width).Render(sb.String())
}
