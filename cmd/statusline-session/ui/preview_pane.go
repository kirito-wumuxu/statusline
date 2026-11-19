package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// RenderPreviewPane renders the dialogue transcript excerpt for the selected session.
func RenderPreviewPane(userPrompt, assistantReply string, width, height int) string {
	borderStyle := StylePaneBorder.Width(width).Height(height).MaxHeight(height)
	if strings.TrimSpace(userPrompt) == "" && strings.TrimSpace(assistantReply) == "" {
		emptyMsg := lipgloss.NewStyle().
			Foreground(ColorMuted).
			Padding(1, 1).
			Render("（暂无转录对话历史记录）")
		return borderStyle.Render(" 对话转录预览 \n" + emptyMsg)
	}

	var sb strings.Builder
	sb.WriteString(" 对话转录预览 \n")

	// Calculate line quotas
	availLines := height - 3 // 1 title line, 2 border lines
	if availLines < 2 {
		availLines = 2
	}

	maxPromptLines := availLines / 2
	if maxPromptLines < 1 {
		maxPromptLines = 1
	}
	maxReplyLines := availLines - maxPromptLines - 1
	if maxReplyLines < 1 {
		maxReplyLines = 1
	}

	userPrompt = strings.TrimSpace(userPrompt)
	assistantReply = strings.TrimSpace(assistantReply)

	if userPrompt != "" {
		sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorCyan).Render("👤 User:") + " ")
		pLines := strings.Split(userPrompt, "\n")
		for i, l := range pLines {
			if i >= maxPromptLines {
				sb.WriteString(lipgloss.NewStyle().Foreground(ColorMuted).Render("... (截断)") + "\n")
				break
			}
			t := strings.TrimSpace(l)
			if len(t) > width-8 {
				t = t[:width-11] + "..."
			}
			sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#e2e8f0")).Render(t) + "\n")
		}
	}

	if assistantReply != "" {
		sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorGreen).Render("🤖 Assistant:") + " ")
		rLines := strings.Split(assistantReply, "\n")
		for i, l := range rLines {
			if i >= maxReplyLines {
				sb.WriteString(lipgloss.NewStyle().Foreground(ColorMuted).Render("... (截断)") + "\n")
				break
			}
			t := strings.TrimSpace(l)
			if len(t) > width-8 {
				t = t[:width-11] + "..."
			}
			sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#cbd5e1")).Render(t) + "\n")
		}
	}

	return borderStyle.Render(sb.String())
}

// RenderHelpBar renders the bottom shortcuts and status bar.
func RenderHelpBar(width int, searchActive, onlyLocalNode bool) string {
	nodeScope := "本机设备"
	if !onlyLocalNode {
		nodeScope = "全部设备"
	}

	tmuxDesc := "Tmux恢复"
	if os.Getenv("TMUX") != "" {
		tmuxDesc = "Tmux新窗口"
	}

	keys := []string{
		StyleHelpKey.Render("[Enter]") + " " + StyleHelpDesc.Render("恢复接管"),
		StyleHelpKey.Render("[t]") + " " + StyleHelpDesc.Render(tmuxDesc),
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
	return lipgloss.NewStyle().Width(width).MaxHeight(1).Padding(0, 1).Render(bar)
}
