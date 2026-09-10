// Package ascii 提供终端动画效果：渐变着色、波形、指示器与擦除动画。
package ascii

import (
	"math"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/unieditdept/ued-uninstaller/internal/ui/theme"
)

// spinnerFrames 是点阵动画的帧序列。
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Spinner 返回一帧旋转指示器。
func Spinner(tick int) string {
	if tick < 0 {
		tick = 0
	}
	frame := spinnerFrames[tick%len(spinnerFrames)]
	return lipgloss.NewStyle().Foreground(theme.Primary).Bold(true).Render(frame)
}

// waveRunes 是波形条的字符集。
var waveRunes = []rune("▁▂▃▄▅▆▇█▇▆▅▄▃▂")

// Wave 返回一行流动的波形。
func Wave(width, tick int) string {
	if width <= 0 {
		return ""
	}
	var sb strings.Builder
	for x := 0; x < width; x++ {
		v := math.Sin(float64(x)*0.35 - float64(tick)*0.25)
		idx := int(math.Round((v*0.5 + 0.5) * float64(len(waveRunes)-1)))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(waveRunes) {
			idx = len(waveRunes) - 1
		}
		sb.WriteRune(waveRunes[idx])
	}
	return Gradient(sb.String(), -tick*2)
}

// Rule 返回一条带流光的分隔线。
func Rule(width, tick int) string {
	if width <= 0 {
		return ""
	}
	return Gradient(strings.Repeat("─", width), -tick)
}

// Sparkles 生成一行缓慢闪烁的星点，用于营造氛围。
func Sparkles(width, tick int) string {
	if width <= 0 {
		return ""
	}
	dim := lipgloss.NewStyle().Foreground(theme.PrimaryDark)
	bright := lipgloss.NewStyle().Foreground(theme.Primary)
	phase := tick / 8
	var sb strings.Builder
	for x := 0; x < width; x++ {
		switch h := (x*31 + phase*17) % 23; {
		case h == 0:
			sb.WriteString(bright.Render("✦"))
		case h == 1:
			sb.WriteString(lipgloss.NewStyle().Foreground(theme.PrimarySoft).Render("✧"))
		case h < 5:
			sb.WriteString(lipgloss.NewStyle().Foreground(theme.PrimaryDeep).Render("•"))
		case h < 9:
			sb.WriteString(dim.Render("∙"))
		default:
			sb.WriteRune(' ')
		}
	}
	return sb.String()
}

// Dissolve 表现「正在擦除」的效果：左侧被替换为暗色块，光标处高亮。
// 输入必须是纯文本（不含 ANSI 序列），否则转义序列会被破坏。
func Dissolve(s string, progress float64) string {
	runes := []rune(s)
	if len(runes) == 0 {
		return s
	}
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	cut := int(math.Floor(float64(len(runes)) * progress))
	if cut >= len(runes) {
		return lipgloss.NewStyle().Foreground(theme.PrimaryDark).Render(strings.Repeat("░", len(runes)))
	}
	var head strings.Builder
	for _, r := range runes[:cut] {
		if r == ' ' {
			head.WriteRune(' ')
		} else {
			head.WriteRune('░')
		}
	}
	erased := lipgloss.NewStyle().Foreground(theme.PrimaryDark).Render(head.String())
	cursor := lipgloss.NewStyle().Foreground(theme.PrimaryPale).Bold(true).Render("█")
	return erased + cursor + string(runes[cut+1:])
}

/* ------------------------------- 渐变着色 ------------------------------- */

var palette = sync.OnceValue(func() []lipgloss.Style {
	colors := theme.Ramp(24, theme.PrimaryDark, theme.PrimaryDeep, theme.Primary, theme.PrimarySoft, theme.PrimaryPale)
	return theme.RampStyles(colors)
})

// Gradient 为字符串施加横向渐变着色，shift 用于产生流动效果。
// 输入必须是纯文本（不含 ANSI 序列）。
func Gradient(s string, shift int) string {
	pal := palette()
	if len(pal) == 0 {
		return s
	}
	runes := []rune(s)
	var out strings.Builder
	var buf strings.Builder
	current := -1
	flush := func() {
		if buf.Len() > 0 && current >= 0 {
			out.WriteString(pal[current].Render(buf.String()))
			buf.Reset()
		}
	}
	for i, r := range runes {
		idx := gradientIndex(i, shift, len(pal))
		if idx != current {
			flush()
			current = idx
		}
		buf.WriteRune(r)
	}
	flush()
	return out.String()
}

func gradientIndex(i, shift, n int) int {
	v := math.Sin(float64(i+shift) * 0.22)
	t := v*0.5 + 0.5
	k := int(math.Round(t * float64(n-1)))
	if k < 0 {
		k = 0
	}
	if k >= n {
		k = n - 1
	}
	return k
}
