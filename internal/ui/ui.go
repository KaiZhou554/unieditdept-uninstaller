// Package ui 定义屏幕（Screen）抽象与跨屏幕共享的消息类型。
package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// FrameRate 是动画帧率。
const FrameRate = 24

// FrameInterval 是相邻两帧的时间间隔。
const FrameInterval = time.Second / FrameRate

// Screen 是界面的一个状态。应用持有当前屏幕，并把消息转交给它。
type Screen interface {
	Init() tea.Cmd
	Update(tea.Msg) (Screen, tea.Cmd)
	View() string
}

// NavigateMsg 请求切换到另一个屏幕。
type NavigateMsg struct{ To Screen }

// Navigate 返回切换屏幕的命令。
func Navigate(to Screen) tea.Cmd {
	return func() tea.Msg { return NavigateMsg{To: to} }
}

// QuitMsg 请求退出程序。
type QuitMsg struct{}

// Quit 返回退出程序的命令。
func Quit() tea.Cmd { return func() tea.Msg { return QuitMsg{} } }

// RescanMsg 请求重新扫描（用于打破屏幕之间的包依赖环）。
type RescanMsg struct{}

// Rescan 返回请求重新扫描的命令。
func Rescan() tea.Cmd { return func() tea.Msg { return RescanMsg{} } }

// FrameMsg 表示动画的新的一帧。
type FrameMsg struct{ At time.Time }

// NextFrame 返回等待下一帧的命令。
func NextFrame() tea.Cmd {
	return tea.Tick(FrameInterval, func(t time.Time) tea.Msg { return FrameMsg{At: t} })
}
