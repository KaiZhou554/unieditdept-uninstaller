// Package components 提供可复用的界面构件。
package components

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// reset 用于在截断后兜底重置样式，避免未闭合的转义序列污染后续输出。
const reset = "\x1b[0m"

// 全项目统一使用 charmbracelet/x/ansi 的宽度定义（基于 uniseg）。
//
// 不要改用 mattn/go-runewidth：它在中文 Windows 上会把 EastAsianWidth 置为 true，
// 于是 ↑ ↓ · ▸ 这类 ambiguous 字符被算成 2 列，而 lipgloss/bubbletea 渲染时按 1 列
// 定位。两套宽度不一致会让每行多算若干列，最终在转义序列中间截断，表现为
// 「底部提示只剩 ↑/」这种半行内容凭空消失。

// Width 返回字符串的终端显示宽度（忽略 ANSI 序列，按字素计数）。
func Width(s string) int { return ansi.StringWidth(s) }

// StripANSI 移除字符串中的 ANSI 转义序列。
func StripANSI(s string) string { return ansi.Strip(s) }

// Truncate 按显示宽度裁剪字符串。
// 基于 x/ansi 实现，保证不会在 ANSI 转义序列中间切断。
func Truncate(s string, w int, tail string) string {
	if w <= 0 {
		return ""
	}
	if Width(s) <= w {
		return s
	}
	return ansi.Truncate(s, w, tail) + reset
}

// Pad 在字符串右侧补空格到指定宽度。
func Pad(s string, w int) string {
	n := Width(s)
	if n >= w {
		return s
	}
	return s + strings.Repeat(" ", w-n)
}

// PadLeft 在字符串左侧补空格到指定宽度。
func PadLeft(s string, w int) string {
	n := Width(s)
	if n >= w {
		return s
	}
	return strings.Repeat(" ", w-n) + s
}

// Fit 将字符串裁剪并补齐到固定宽度。
func Fit(s string, w int) string {
	return Pad(Truncate(s, w, "…"), w)
}

// Center 将字符串居中到指定宽度。
func Center(s string, w int) string {
	n := Width(s)
	if n >= w {
		return Truncate(s, w, "…")
	}
	left := (w - n) / 2
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", w-n-left)
}
