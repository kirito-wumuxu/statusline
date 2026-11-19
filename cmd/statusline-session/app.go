package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"statusline/cmd/statusline-session/ui"
	"statusline/internal/session"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type previewLoadedMsg struct {
	prompt string
	reply  string
}

// AppModel represents the Bubble Tea state machine for statusline-session.
type AppModel struct {
	service        session.SessionService
	allItems       []session.SessionItem
	filteredItems  []session.SessionItem
	cursor         int
	onlyLocalNode  bool
	searchActive   bool
	searchInput    textinput.Model
	previewPrompt  string
	previewReply   string
	statusMessage  string
	termWidth      int
	termHeight     int
	launchAction   *session.SessionItem
	launchMode     string // "exec", "tmux", "copy"
}

// NewAppModel initializes the TUI application model.
func NewAppModel(service session.SessionService, initialOnlyLocal bool) *AppModel {
	ti := textinput.New()
	ti.Placeholder = "输入关键词过滤 (Esc 退出)..."
	ti.Prompt = "🔍 过滤: "
	ti.CharLimit = 64
	// 注意：保持 ti.Width 为 0（自适应）。
	// bubbles/textinput 在 Width > 0 时对含中文字符的 Placeholder 计算视觉列宽后切片 rune 会发生 bounds out of range panic。

	m := &AppModel{
		service:       service,
		onlyLocalNode: initialOnlyLocal,
		searchInput:   ti,
		termWidth:     100,
		termHeight:    30,
	}
	m.loadSessions()
	return m
}

func (m *AppModel) Init() tea.Cmd {
	return nil
}

func (m *AppModel) loadSessions() {
	if m.service == nil {
		return
	}
	items, err := m.service.ListSessions(context.Background(), m.onlyLocalNode)
	if err == nil {
		m.allItems = items
		m.applyFilter()
	}
}

func (m *AppModel) applyFilter() {
	query := strings.ToLower(strings.TrimSpace(m.searchInput.Value()))
	if query == "" {
		m.filteredItems = m.allItems
	} else {
		var matched []session.SessionItem
		for _, item := range m.allItems {
			searchable := strings.ToLower(fmt.Sprintf("%s %s %s %s %s",
				item.DisplayName,
				item.Record.GitBranch,
				item.Record.ProductType,
				item.Record.SessionID,
				item.Record.CWD,
			))
			if strings.Contains(searchable, query) {
				matched = append(matched, item)
			}
		}
		m.filteredItems = matched
	}

	if m.cursor >= len(m.filteredItems) {
		m.cursor = len(m.filteredItems) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.updatePreview()
}

func (m *AppModel) updatePreview() {
	if len(m.filteredItems) == 0 || m.cursor < 0 || m.cursor >= len(m.filteredItems) {
		m.previewPrompt = ""
		m.previewReply = ""
		return
	}
	item := m.filteredItems[m.cursor]
	tPath := session.ResolveTranscriptPath(item.Record.ProductType, item.Record.SessionID, item.Record.TranscriptPath)
	if tPath != "" {
		prompt, reply, _ := session.LoadTranscriptPreview(tPath)
		m.previewPrompt = prompt
		m.previewReply = reply
	} else {
		m.previewPrompt = ""
		m.previewReply = ""
	}
}

func (m *AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.termWidth = msg.Width
		m.termHeight = msg.Height
		return m, nil

	case tea.KeyMsg:
		m.statusMessage = "" // Clear transient status message on keypress

		// When searching
		if m.searchActive {
			switch msg.Type {
			case tea.KeyEsc:
				m.searchActive = false
				m.searchInput.Blur()
				return m, nil
			case tea.KeyEnter:
				m.searchActive = false
				m.searchInput.Blur()
				return m, nil
			default:
				var cmd tea.Cmd
				m.searchInput, cmd = m.searchInput.Update(msg)
				m.applyFilter()
				return m, cmd
			}
		}

		// Normal navigation mode
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.updatePreview()
			}
			return m, nil

		case "down", "j":
			if m.cursor < len(m.filteredItems)-1 {
				m.cursor++
				m.updatePreview()
			}
			return m, nil

		case "/":
			m.searchActive = true
			m.searchInput.Focus()
			return m, textinput.Blink

		case "a":
			m.onlyLocalNode = !m.onlyLocalNode
			m.loadSessions()
			return m, nil

		case "r":
			m.loadSessions()
			m.statusMessage = "✓ 已重新加载会话列表"
			return m, nil

		case "c":
			if len(m.filteredItems) > 0 && m.cursor < len(m.filteredItems) {
				item := m.filteredItems[m.cursor]
				_, _, _, fullCmd := session.BuildResumeCommand(item)
				if err := session.CopyToClipboard(fullCmd); err == nil {
					m.statusMessage = "✓ 已将恢复命令复制到剪贴板！"
				} else {
					m.statusMessage = "✗ 复制失败: " + err.Error()
				}
			}
			return m, nil

		case "t":
			if len(m.filteredItems) > 0 && m.cursor < len(m.filteredItems) {
				item := m.filteredItems[m.cursor]
				// 1. Check if tmux binary is installed
				if _, err := exec.LookPath("tmux"); err != nil {
					m.statusMessage = "✗ 未安装 tmux: 请先安装 tmux (如 sudo apt install tmux)"
					return m, nil
				}

				// 2. If already inside tmux, open a new window in the active session
				if os.Getenv("TMUX") != "" {
					_, _, cwd, fullCmd := session.BuildResumeCommand(item)
					name := item.DisplayName
					if err := session.LaunchInTmux(fullCmd, name, cwd); err == nil {
						m.statusMessage = "✓ 已在当前 Tmux 新窗口打开该会话！"
					} else {
						m.statusMessage = "✗ Tmux 新窗口拉起失败: " + err.Error()
					}
					return m, nil
				}

				// 3. If outside tmux, exit TUI and launch/attach into Tmux
				m.launchAction = &item
				m.launchMode = "tmux"
				return m, tea.Quit
			}
			return m, nil

		case "enter":
			if len(m.filteredItems) > 0 && m.cursor < len(m.filteredItems) {
				item := m.filteredItems[m.cursor]
				m.launchAction = &item
				m.launchMode = "exec"
				return m, tea.Quit
			}
		}
	}

	return m, nil
}

func (m *AppModel) View() string {
	width := m.termWidth
	height := m.termHeight
	if width < 20 || height < 10 {
		return "终端窗口尺寸过小，请放大终端以查看 statusline-session"
	}

	// 1. Header
	titleStr := ui.StyleTitle.Render(" statusline-session (sls) 终端会话管理器 ")
	nodeScopeStr := lipgloss.NewStyle().Foreground(ui.ColorCyan).Render("[范围: 本机设备]")
	if !m.onlyLocalNode {
		nodeScopeStr = lipgloss.NewStyle().Foreground(ui.ColorAmber).Render("[范围: 全部设备]")
	}
	header := lipgloss.JoinHorizontal(lipgloss.Center, titleStr, " ", nodeScopeStr)

	// 2. Search bar
	var searchView string
	if m.searchActive || m.searchInput.Value() != "" {
		searchView = m.searchInput.View()
	} else {
		searchView = lipgloss.NewStyle().Foreground(ui.ColorMuted).Render("🔍 按 [/] 键开启过滤搜索 (项目名、分支、工具、ID)...")
	}
	searchBar := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(ui.ColorBorder).
		Width(width - 2).
		Padding(0, 1).
		Render(searchView)

	// 3. Main layout: Left Pane (List) + Right Pane (Details & Preview)
	mainHeight := height - 5
	if mainHeight < 8 {
		mainHeight = 8
	}

	leftWidth := (width * 38) / 100
	if leftWidth < 30 {
		leftWidth = 30
	}
	rightWidth := width - leftWidth - 3
	if rightWidth < 30 {
		rightWidth = 30
	}

	leftPane := ui.RenderSessionList(m.filteredItems, m.cursor, leftWidth, mainHeight, !m.searchActive)

	// Right pane: detail (top, 10 lines) + preview (bottom, remaining lines)
	detailHeight := 10
	if mainHeight < 16 {
		detailHeight = mainHeight / 2
	}
	previewHeight := mainHeight - detailHeight

	var currentItem *session.SessionItem
	if len(m.filteredItems) > 0 && m.cursor >= 0 && m.cursor < len(m.filteredItems) {
		currentItem = &m.filteredItems[m.cursor]
	}

	detailPane := ui.RenderDetailPane(currentItem, rightWidth, detailHeight)
	previewPane := ui.RenderPreviewPane(m.previewPrompt, m.previewReply, rightWidth, previewHeight)

	rightCol := lipgloss.JoinVertical(lipgloss.Left, detailPane, previewPane)
	mainContent := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightCol)

	// 4. Status and Help footer
	helpBar := ui.RenderHelpBar(width-2, m.searchActive, m.onlyLocalNode)
	if m.statusMessage != "" {
		statusStyled := lipgloss.NewStyle().Foreground(ui.ColorAmber).Bold(true).Render(m.statusMessage)
		helpBar = statusStyled + " | " + helpBar
	}
	helpBar = lipgloss.NewStyle().Width(width - 2).MaxHeight(1).Render(helpBar)

	return lipgloss.JoinVertical(lipgloss.Left, header, searchBar, mainContent, helpBar)
}
