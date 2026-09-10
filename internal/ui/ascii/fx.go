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

// Rule 返回一条带流光的分隔线。
// 每 3 帧才推进一格，比默认的渐变慢，流光更从容。
func Rule(width, tick int) string {
	if width <= 0 {
		return ""
	}
	return Gradient(strings.Repeat("─", width), -tick/3)
}

// fps 与 ui.FrameRate 保持一致，用于把动画速度写成「秒」。
const fps = 24

// Spark 返回一枚缓慢呼吸的星，用来代替静态的装饰符号。
// 星形每 2 秒在实心与空心之间切换，同时颜色在暗粉与亮粉之间平滑往复。
func Spark(tick int) string {
	cycle := float64(4 * fps) // 4 秒一个完整周期
	phase := math.Sin(float64(tick)/cycle*2*math.Pi)*0.5 + 0.5
	glyph := "✦"
	if (tick/(2*fps))%2 == 1 {
		glyph = "✧"
	}
	return lipgloss.NewStyle().
		Foreground(theme.Mix(theme.PrimaryDeep, theme.PrimarySoft, phase)).
		Bold(true).
		Render(glyph)
}

// Dissolve 表现「正在擦除」的效果：左侧被替换为暗色块，光标处高亮。
// 输入必须是纯文本（不含 ANSI 序列），否则转义序列会被破坏。
func Dissolve(s string, progress float64) string {
	return dissolve(s, progress, theme.PrimaryDark, theme.PrimaryPale)
}

// CoolDissolve 与 Dissolve 相同，但擦除块与光标走蓝紫色，
// 用于非 UniEditDept 的条目。
func CoolDissolve(s string, progress float64) string {
	return dissolve(s, progress, theme.CoolScanDark, theme.CoolScanPale)
}

func dissolve(s string, progress float64, block, cursorColor lipgloss.Color) string {
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
		return lipgloss.NewStyle().Foreground(block).Render(strings.Repeat("░", len(runes)))
	}
	var head strings.Builder
	for _, r := range runes[:cut] {
		if r == ' ' {
			head.WriteRune(' ')
		} else {
			head.WriteRune('░')
		}
	}
	erased := lipgloss.NewStyle().Foreground(block).Render(head.String())
	cursor := lipgloss.NewStyle().Foreground(cursorColor).Bold(true).Render("█")
	return erased + cursor + string(runes[cut+1:])
}

/* ------------------------------- 扫描光带 ------------------------------- */

// scanPalette 是扫描光带用到的背景色档位（由暗到亮）。
var scanPalette = sync.OnceValue(func() []lipgloss.Style {
	return scanPaletteOf(theme.FlashOff, theme.PrimaryDeep, theme.FlashOn, theme.FlashInk)
})

// coolScanPalette 是外部条目用的蓝紫光带。
var coolScanPalette = sync.OnceValue(func() []lipgloss.Style {
	return scanPaletteOf(theme.CoolScanDark, theme.CoolScanMid, theme.CoolScanPale, theme.CoolScanInk)
})

func scanPaletteOf(dark, mid, pale, ink lipgloss.Color) []lipgloss.Style {
	colors := theme.Ramp(12, dark, mid, pale)
	styles := make([]lipgloss.Style, len(colors))
	for i, c := range colors {
		styles[i] = lipgloss.NewStyle().
			Background(c).
			Foreground(ink).
			Bold(true)
	}
	return styles
}

// ScanHighlight 给文本套上一条缓慢扫过的高亮光带。
//
// 它用来替代「整块反复闪烁」：亮点在行内平移，亮度变化平缓，
// 既能提示「这里需要再确认一次」，又不会一直刺激视觉。
// 返回值不改变文本的显示宽度，因此不影响调用方的排版计算。
func ScanHighlight(s string, tick int) string {
	return highlight(s, tick, scanPalette())
}

// CoolScanHighlight 与 ScanHighlight 相同，但光带走蓝紫色（深蓝 → 蓝 → 蓝紫）。
// 待确认时 UniEditDept 的条目扫过粉红光带，外部条目用这一条区分开；
// 亮端刻意压暗一档，免得冷色比主色还晃眼。
func CoolScanHighlight(s string, tick int) string {
	return highlight(s, tick, coolScanPalette())
}

func highlight(s string, tick int, pal []lipgloss.Style) string {
	if s == "" || len(pal) == 0 {
		return s
	}
	var out strings.Builder
	var buf strings.Builder
	current := -1
	flush := func() {
		if buf.Len() > 0 && current >= 0 {
			out.WriteString(pal[current].Render(buf.String()))
			buf.Reset()
		}
	}
	for i, r := range []rune(s) {
		idx := scanIndex(i, tick, len(pal))
		if idx != current {
			flush()
			current = idx
		}
		buf.WriteRune(r)
	}
	flush()
	return out.String()
}

// scanIndex 返回位置 i 在 tick 时刻所处的高亮档位。
func scanIndex(i, tick, n int) int {
	if n <= 1 {
		return 0
	}
	// 光带跨度约 26 个字符，每秒前进约 14 个字符：慢到不抢注意力，又看得出在动。
	const (
		frequency = 0.24
		speed     = 0.6
	)
	v := math.Sin((float64(i) - float64(tick)*speed) * frequency)
	t := v*0.5 + 0.5
	// 抬高对比，让亮带收窄，其余区域保持低调。
	t = math.Pow(t, 2.2)
	k := int(math.Round(t * float64(n-1)))
	if k < 0 {
		k = 0
	}
	if k >= n {
		k = n - 1
	}
	return k
}

/* ------------------------------- 渐变着色 ------------------------------- */

var palette = sync.OnceValue(func() []lipgloss.Style {
	colors := theme.Ramp(24, theme.PrimaryDark, theme.PrimaryDeep, theme.Primary, theme.PrimarySoft, theme.PrimaryPale)
	return theme.RampStyles(colors)
})

// Gradient 为字符串施加横向渐变着色，shift 用于产生流动效果。
// 输入必须是纯文本（不含 ANSI 序列）。
func Gradient(s string, shift int) string {
	return shade(s, shift, palette())
}

// shade 用给定的色带给字符串做横向渐变着色。
func shade(s string, shift int, pal []lipgloss.Style) string {
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
