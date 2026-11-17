package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// RenderPreviewPane renders the dialogue transcript excerpt for the selected session.
func RenderPreviewPane(userPrompt, assistantReply string, width, height int) string {
	if strings.TrimSpace(userPrompt) == "" && strings.TrimSpace(assistantReply) == "" {
		emptyMsg := lipgloss.NewStyle().
			Foreground(ColorMuted).
			Padding(1, 1).
			Render("（暂无转录对话历史记录）")
		return StylePaneBorder.Width(width).Height(height).Render(" 对话转录预览 \n" + emptyMsg)
	}

	var sb strings.Builder
	sb.WriteString(" 对话转录预览 \n")

	if userPrompt != "" {
		header := lipgloss.NewStyle().Bold(true).Foreground(ColorCyan).Render("👤 User (最近提问):")
		promptBody := lipgloss.NewStyle().Foreground(lipgloss.Color("#e2e8f0")).Render(userPrompt)
		sb.WriteString(header + "\n" + promptBody + "\n\n")
	}

	if assistantReply != "" {
		header := lipgloss.NewStyle().Bold(true).Foreground(ColorGreen).Render("🤖 Assistant (结论摘要):")
		replyBody := lipgloss.NewStyle().Foreground(lipgloss.Color("#cbd5e1")).Render(assistantReply)
		sb.WriteString(header + "\n" + replyBody + "\n")
	}

	return StylePaneBorder.Width(width).Height(height).Render(sb.String())
}

// RenderHelpBar renders the bottom shortcuts and status bar.
func RenderHelpBar(width int, searchActive, onlyLocalNode bool) string {
	nodeScope := "本机设备"
	if !onlyLocalNode {
		nodeScope = "全部设备"
	}

	keys := []string{
		StyleHelpKey.Render("[Enter]") + " " + StyleHelpDesc.Render("恢复接管"),
		StyleHelpKey.Render("[t]") + " " + StyleHelpDesc.Render("Tmux打开"),
		StyleHelpKey.Render("[c]") + " " + StyleHelpDesc.Render("复制命令"),
		StyleHelpKey.Render("[/]") + " " + StyleHelpDesc.Render("搜索过滤"),
		StyleHelpKey.Render("[a]") + " " + StyleHelpDesc.Render(fmt.Sprintf("范围(%s)", nodeScope)),
		StyleHelpKey.Render("[r]") + " " + StyleHelpDesc.Render("刷新"),
		StyleHelpKey.Render("[q]") + " " + StyleHelpDesc.Render("退出"),
	}

	if searchActive {
		keys = append([]string{StyleHelpKey.Render("[Esc]") + " " + StyleHelpDesc.Render("完成输入")}, keys...)
	}

	bar := strings.Join(keys, "  ")
	return lipgloss.NewStyle().Width(width).Padding(0, 1).Render(bar)
}
