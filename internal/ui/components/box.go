package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/unieditdept/ued-uninstaller/internal/ui/theme"
)

// Box 是一个带标题的圆角边框容器，尺寸精确可控。
type Box struct {
	Title  string
	Width  int
	Height int
	Active bool // 高亮边框，表示焦点所在
}

// Render 渲染盒子；内容超出时截断，不足时补空行。
func (b Box) Render(content string) string {
	w, h := b.Width, b.Height
	if w < 10 {
		w = 10
	}
	if h < 3 {
		h = 3
	}
	innerW := w - 4
	innerH := h - 2

	border := theme.Border
	titleColor := theme.PrimaryDeep
	if b.Active {
		border = theme.Primary
		titleColor = theme.Primary
	}
	borderStyle := lipgloss.NewStyle().Foreground(border)
	titleStyle := lipgloss.NewStyle().Foreground(titleColor).Bold(true)

	// 顶边：╭─ 标题 ─────╮
	label := ""
	if b.Title != "" {
		label = " " + b.Title + " "
	}
	if Width(label) > w-4 {
		label = " " + Truncate(b.Title, w-6, "…") + " "
	}
	fill := w - 3 - Width(label)
	if fill < 0 {
		fill = 0
	}
	top := borderStyle.Render("╭─") + titleStyle.Render(label) + borderStyle.Render(strings.Repeat("─", fill)+"╮")
	bottom := borderStyle.Render("╰" + strings.Repeat("─", w-2) + "╯")

	lines := strings.Split(content, "\n")
	for len(lines) < innerH {
		lines = append(lines, "")
	}
	if len(lines) > innerH {
		lines = lines[:innerH]
	}

	bar := borderStyle.Render("│")
	var sb strings.Builder
	sb.WriteString(top)
	for _, line := range lines {
		sb.WriteString("\n")
		sb.WriteString(bar)
		sb.WriteString(" ")
		sb.WriteString(Fit(line, innerW))
		sb.WriteString(" ")
		sb.WriteString(bar)
	}
	sb.WriteString("\n")
	sb.WriteString(bottom)
	return sb.String()
}
