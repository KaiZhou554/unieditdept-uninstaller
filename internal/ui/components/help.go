package components

import (
	"strings"

	"github.com/unieditdept/ued-uninstaller/internal/ui/ascii"
	"github.com/unieditdept/ued-uninstaller/internal/ui/theme"
)

// Binding 描述一条快捷键说明。
type Binding struct {
	Keys  []string
	Desc  string
	Alert bool
	// Click 是点击这条提示时应当模拟按下的键，为空表示不可点击。
	// 特殊值："any" 表示任意键（用于「其它键取消」），
	// "enter" / "esc" 表示对应功能键，其余按单个字符处理。
	Click string
}

// Span 描述某一条提示在整行中的水平位置，供鼠标点击命中判断使用。
type Span struct {
	Index int // 对应传入 bindings 的下标
	Start int // 起始显示列（相对整行开头）
	Width int // 显示宽度
}

// itemSep 是相邻两条快捷键之间的间隔宽度。
const itemSep = 2

// Help 把快捷键渲染成一行说明文本，并返回各条提示的位置。
//
// maxWidth > 0 时按键宽度取舍：先按纯文本宽度计算，放不下就整条丢弃，
// 绝不对已着色的字符串做截断——那会在 ANSI 序列中间切断，导致残行。
// tick 用于驱动告警项的闪烁。
func Help(bindings []Binding, tick, maxWidth int) (string, []Span) {
	st := theme.S()
	spans := make([]Span, 0, len(bindings))

	var sb strings.Builder
	width := 0
	for i, b := range bindings {
		keys := strings.Join(b.Keys, "/")
		desc := b.Desc

		sepW := 0
		if i > 0 {
			sepW = itemSep
		}

		// itemWidth 给出这一条渲染后的真实宽度：
		// 普通项的按键两侧各留一格（让背景色块不贴字），告警项同样左右各加一格。
		itemWidth := func() int {
			if b.Alert {
				w := Width(keys)
				if desc != "" {
					w += 1 + Width(desc)
				}
				return w + 2
			}
			return Width(keys) + 2 + 1 + Width(desc)
		}

		if maxWidth > 0 && width+sepW+itemWidth() > maxWidth {
			if i > 0 {
				break // 后续条目整体舍弃
			}
			// 连首条都放不下时退化为只显示按键。
			desc = ""
			keys = Truncate(keys, maxWidth-2, "…")
			if width+sepW+itemWidth() > maxWidth {
				break
			}
		}

		if sepW > 0 {
			sb.WriteString(strings.Repeat(" ", sepW))
			width += sepW
		}
		spans = append(spans, Span{Index: i, Start: width, Width: itemWidth()})
		switch {
		case b.Alert:
			text := keys
			if desc != "" {
				text += " " + desc
			}
			// 用缓慢扫过的光带代替闪烁。
			sb.WriteString(ascii.ScanHighlight(" "+text+" ", tick))
		case desc == "":
			sb.WriteString(st.Key.Render(" " + keys + " "))
		default:
			sb.WriteString(st.Key.Render(" "+keys+" ") + " " + st.KeyDesc.Render(desc))
		}
		width += itemWidth()
	}
	return sb.String(), spans
}

// Header 渲染顶部标题栏。
//
// icon 是标题前的装饰符号（需自带样式），title 会套用标题样式。
// right 同样需自带样式；超宽时会做 ANSI 安全的裁剪，保证整行不超过 w。
func Header(icon, title, right string, w int, rule string) string {
	st := theme.S()
	leftW := Width(icon) + 1 + Width(title)
	if right != "" {
		maxRight := w - leftW - 1
		if maxRight < 8 {
			right = "" // 放不下就不显示，好过把标题挤掉
		} else if Width(right) > maxRight {
			right = Truncate(right, maxRight, "…")
		}
	}

	line := icon + " " + st.Title.Render(title)
	if right != "" {
		gap := w - leftW - Width(right)
		if gap < 1 {
			gap = 1
		}
		line += strings.Repeat(" ", gap) + right
	}
	if rule == "" {
		return line
	}
	return line + "\n" + rule
}

// Footer 渲染底部状态栏：左侧为快捷键，右侧为统计信息。
//
// left 应当由 Help 预先裁剪到合适宽度；此处只负责补空格对齐，
// 并在极端情况下做一次 ANSI 安全的兜底裁剪。
func Footer(left, right string, w int) string {
	st := theme.S()
	rightW := Width(right)

	maxLeft := w - rightW - 1
	if maxLeft < 0 {
		maxLeft = 0
	}
	if Width(left) > maxLeft {
		left = Truncate(left, maxLeft, "…")
	}

	gap := w - Width(left) - rightW
	if gap < 1 {
		gap = 1
	}
	// 右侧通常是版本号一类的次要信息，用中性灰，不抢注意力。
	return left + strings.Repeat(" ", gap) + st.Neutral.Render(right)
}
