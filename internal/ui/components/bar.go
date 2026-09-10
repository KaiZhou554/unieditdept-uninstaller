package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/unieditdept/ued-uninstaller/internal/ui/ascii"
	"github.com/unieditdept/ued-uninstaller/internal/ui/theme"
)

// Bar 渲染一条渐变进度条。ratio 取值 0~1，shift 用于产生流动光泽。
func Bar(width int, ratio float64, shift int) string {
	if width <= 0 {
		return ""
	}
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	filled := int(ratio*float64(width) + 0.5)
	if filled > width {
		filled = width
	}
	empty := width - filled
	track := lipgloss.NewStyle().Foreground(theme.Track).Render(strings.Repeat("░", empty))
	return ascii.Gradient(strings.Repeat("█", filled), shift) + track
}
