// Package theme 定义全局配色与可复用的 lipgloss 样式。
package theme

import (
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
)

// 主色板：以 #ff6699 为主色展开。
var (
	Primary     = lipgloss.Color("#ff6699")
	PrimaryAlt  = lipgloss.Color("#ff85ad")
	PrimarySoft = lipgloss.Color("#ffb3cb")
	PrimaryPale = lipgloss.Color("#ffe0ea")
	PrimaryDeep = lipgloss.Color("#c94f79")
	PrimaryDark = lipgloss.Color("#5e1f38")

	Background = lipgloss.Color("#0d0710")
	Surface    = lipgloss.Color("#170d14")
	SurfaceAlt = lipgloss.Color("#22121c")
	Track      = lipgloss.Color("#3a1c2b")
	Border     = lipgloss.Color("#4a2438")

	Text      = lipgloss.Color("#ffedf3")
	TextDim   = lipgloss.Color("#c497a9")
	TextFaint = lipgloss.Color("#7d5668")

	// Neutral 是低饱和的中性灰，用于不抢注意力的次要信息（如右下角版本号）。
	Neutral = lipgloss.Color("#68626d")

	// 右上角语言切换器用的中性点缀色（小徽章）。
	ChipBg     = lipgloss.Color("#3d3945")
	ChipInk    = lipgloss.Color("#b6b0be")
	ChipOnBg   = lipgloss.Color("#57525f")
	ChipOnInk  = lipgloss.Color("#f2eff5")
	ChipOffInk = lipgloss.Color("#635d6a")

	OK     = lipgloss.Color("#7ce0b0")
	Warn   = lipgloss.Color("#ffc46b")
	Danger = lipgloss.Color("#ff5d73")

	// 二次确认时的光带配色：暗端压得足够低，光带扫过时才不刺眼。
	FlashOn  = lipgloss.Color("#ff5d73")
	FlashOff = lipgloss.Color("#5e1f38")
	FlashInk = lipgloss.Color("#ffe0ea")
)

// Styles 聚合界面所需的全部样式。
type Styles struct {
	Base    lipgloss.Style
	Title   lipgloss.Style
	Subtle  lipgloss.Style
	Muted   lipgloss.Style
	Faint   lipgloss.Style
	Neutral lipgloss.Style
	Accent  lipgloss.Style
	Strong  lipgloss.Style
	Danger  lipgloss.Style
	OK      lipgloss.Style
	Warn    lipgloss.Style
	Key     lipgloss.Style
	KeyDesc lipgloss.Style
	Badge   lipgloss.Style
	BadgeOn lipgloss.Style
	Row     lipgloss.Style
	RowSel  lipgloss.Style
	Logo    lipgloss.Style

	// Chip / ChipOn / ChipOff 用于右上角语言切换器这类小徽章。
	// Chip 是按键提示，ChipOn 是当前选项（都带底色），ChipOff 是未选中的选项。
	Chip    lipgloss.Style
	ChipOn  lipgloss.Style
	ChipOff lipgloss.Style
}

var styles = sync.OnceValue(func() *Styles {
	// 注意：这里不要用 Padding。带 padding 的样式会让实际渲染宽度比字符串宽度
	// 多出若干格，而排版处的宽度估算是按字符串算的，两者不一致就会溢出或误截断。
	// 按键两侧的空隙由调用方显式书写。
	key := lipgloss.NewStyle().
		Foreground(PrimaryPale).
		Background(PrimaryDeep).
		Bold(true)
	return &Styles{
		Base:    lipgloss.NewStyle().Foreground(Text),
		Title:   lipgloss.NewStyle().Foreground(Primary).Bold(true),
		Subtle:  lipgloss.NewStyle().Foreground(PrimarySoft),
		Muted:   lipgloss.NewStyle().Foreground(TextDim),
		Faint:   lipgloss.NewStyle().Foreground(TextFaint),
		Neutral: lipgloss.NewStyle().Foreground(Neutral),
		Accent:  lipgloss.NewStyle().Foreground(Primary),
		Strong:  lipgloss.NewStyle().Foreground(PrimaryPale).Bold(true),
		Danger:  lipgloss.NewStyle().Foreground(Danger).Bold(true),
		OK:      lipgloss.NewStyle().Foreground(OK),
		Warn:    lipgloss.NewStyle().Foreground(Warn),
		Key:     key,
		KeyDesc: lipgloss.NewStyle().Foreground(TextDim),
		Badge:   lipgloss.NewStyle().Foreground(TextFaint),
		BadgeOn: lipgloss.NewStyle().Foreground(Primary).Bold(true),
		Row:     lipgloss.NewStyle().Foreground(Text),
		RowSel:  lipgloss.NewStyle().Foreground(PrimaryPale).Background(PrimaryDeep).Bold(true),
		Logo:    lipgloss.NewStyle().Foreground(Primary),

		Chip:    lipgloss.NewStyle().Foreground(ChipInk).Background(ChipBg).Bold(true),
		ChipOn:  lipgloss.NewStyle().Foreground(ChipOnInk).Background(ChipOnBg).Bold(true),
		ChipOff: lipgloss.NewStyle().Foreground(ChipOffInk),
	}
})

// S 返回全局样式集合（惰性构造）。
func S() *Styles { return styles() }

// Mix 在两个颜色之间线性插值。t 为 0 时返回 a，为 1 时返回 b。
func Mix(a, b lipgloss.Color, t float64) lipgloss.Color {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	ar, ag, ab := rgb(a)
	br, bg, bb := rgb(b)
	mix := func(x, y float64) uint8 {
		v := math.Round(x + (y-x)*t)
		if v < 0 {
			v = 0
		}
		if v > 255 {
			v = 255
		}
		return uint8(v)
	}
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", mix(ar, br), mix(ag, bg), mix(ab, bb)))
}

// Ramp 生成包含 n 个颜色的渐变色带。
func Ramp(n int, stops ...lipgloss.Color) []lipgloss.Color {
	if n <= 0 {
		return nil
	}
	if len(stops) == 0 {
		stops = []lipgloss.Color{PrimaryDark, Primary, PrimaryPale}
	}
	if len(stops) == 1 {
		out := make([]lipgloss.Color, n)
		for i := range out {
			out[i] = stops[0]
		}
		return out
	}
	out := make([]lipgloss.Color, 0, n)
	segments := len(stops) - 1
	for i := 0; i < n; i++ {
		pos := float64(i) / float64(n-1) * float64(segments)
		seg := int(pos)
		if seg >= segments {
			seg = segments - 1
		}
		out = append(out, Mix(stops[seg], stops[seg+1], pos-float64(seg)))
	}
	return out
}

// RampStyles 把色带转换为样式切片，便于逐段着色。
func RampStyles(colors []lipgloss.Color) []lipgloss.Style {
	out := make([]lipgloss.Style, len(colors))
	for i, c := range colors {
		out[i] = lipgloss.NewStyle().Foreground(c)
	}
	return out
}

func rgb(c lipgloss.Color) (float64, float64, float64) {
	s := strings.TrimPrefix(strings.TrimSpace(string(c)), "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	var r, g, b int
	_, err := fmt.Sscanf(s, "%02x%02x%02x", &r, &g, &b)
	if err != nil {
		return 255, 102, 153 // 回退到主色
	}
	return float64(r), float64(g), float64(b)
}
