// Package app 装配 TUI 程序：持有当前屏幕并分发消息。
package app

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"
	"github.com/unieditdept/ued-uninstaller/internal/config"
	"github.com/unieditdept/ued-uninstaller/internal/ui"
	"github.com/unieditdept/ued-uninstaller/internal/ui/screens/home"
)

// Model 是整个程序的根模型。
type Model struct {
	cfg    config.Config
	screen ui.Screen
	w, h   int
}

// TerminalSize 返回终端可见尺寸；无法获取时返回 0, 0。
func TerminalSize() (int, int) {
	w, h, err := term.GetSize(os.Stdout.Fd())
	if err != nil || w <= 0 || h <= 0 {
		return 0, 0
	}
	return w, h
}

// New 创建根模型。width / height 为初始终端尺寸，未知时传 0。
func New(cfg config.Config, width, height int) *Model {
	m := &Model{cfg: cfg, w: width, h: height}
	m.screen = home.New(cfg, width, height)
	return m
}

// Init 初始化当前屏幕。
func (m *Model) Init() tea.Cmd {
	return m.screen.Init()
}

// swapScreen 切换屏幕。
// 新屏幕必须立刻拿到当前终端尺寸，否则它会用默认的 80x24 渲染，
// 表现为「重新扫描后界面突然缩小，改变窗口大小才恢复」。
func (m *Model) swapScreen(next ui.Screen) tea.Cmd {
	m.screen = next
	var resize tea.Cmd
	if m.w > 0 && m.h > 0 {
		updated, cmd := m.screen.Update(tea.WindowSizeMsg{Width: m.w, Height: m.h})
		m.screen = updated
		resize = cmd
	}
	return tea.Batch(m.screen.Init(), resize)
}

// Update 分发消息：优先处理全局消息，其余交给当前屏幕。
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	case ui.NavigateMsg:
		return m, m.swapScreen(msg.To)
	case ui.RescanMsg:
		// 结束旧屏幕后台的扫描 / 卸载任务，避免 goroutine 泄漏。
		if closer, ok := m.screen.(interface{ Close() }); ok {
			closer.Close()
		}
		return m, m.swapScreen(home.New(m.cfg, m.w, m.h))
	case ui.QuitMsg:
		return m, tea.Quit
	}

	next, cmd := m.screen.Update(msg)
	m.screen = next
	return m, cmd
}

// View 渲染当前屏幕。
func (m *Model) View() string {
	return m.screen.View()
}

// Run 启动 TUI 程序。width / height 为启动时探测到的终端尺寸，未知时传 0。
func Run(cfg config.Config, width, height int) error {
	// 故意不开 alt screen：在部分 Windows 主机上 alt screen 序列无法切换 buffer，
	// 会导致多次 View 输出叠加在主屏幕并夹杂 stderr 日志。直接走行级 diff
	// 渲染更稳定；配合 View 内部用 lipgloss.Place 固定输出尺寸，不会出现错位。
	program := tea.NewProgram(
		New(cfg, width, height),
		tea.WithMouseCellMotion(),
	)
	_, err := program.Run()
	return err
}
