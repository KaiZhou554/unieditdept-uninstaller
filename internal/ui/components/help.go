package components

import (
	"strings"

	"github.com/unieditdept/ued-uninstaller/internal/ui/ascii"
	"github.com/unieditdept/ued-uninstaller/internal/ui/theme"
)

// Binding 描述一条快捷键说明。Alert 为 true 时该项会以醒目配色闪烁。
type Binding struct {
	Keys  []string
	Desc  string
	Alert bool
}

// itemSep 是相邻两条快捷键之间的间隔宽度。
const itemSep = 2

// Help 把快捷键渲染成一行说明文本。
//
// maxWidth > 0 时按键宽度取舍：先按纯文本宽度计算，放不下就整条丢弃，
// 绝不对已着色的字符串做截断——那会在 ANSI 序列中间切断，导致残行。
// tick 用于驱动告警项的闪烁。
func Help(bindings []Binding, tick, maxWidth int) string {
	st := theme.S()

	var sb strings.Builder
	width := 0
	for i, b := range bindings {
		keys := strings.Join(b.Keys, "/")
		text := keys + " " + b.Desc

		need := Width(text)
		if i > 0 {
			need += itemSep
		}
		if maxWidth > 0 {
			if i == 0 {
				// 首条放不下时在纯文本上裁剪（此时还没有着色，是安全的）。
				if need > maxWidth {
					text = Truncate(text, maxWidth, "…")
				}
			} else if width+need > maxWidth {
				break // 后续条目整体舍弃
			}
		}

		if i > 0 {
			sb.WriteString(strings.Repeat(" ", itemSep))
			width += itemSep
		}
		if b.Alert {
			// 用缓慢扫过的光带代替闪烁。
			sb.WriteString(ascii.ScanHighlight(" "+text+" ", tick))
		} else {
			sb.WriteString(st.Key.Render(keys) + " " + st.KeyDesc.Render(b.Desc))
		}
		width += need
	}
	return sb.String()
}

// Header 渲染顶部标题栏。
// right 超宽时会在纯文本阶段裁剪，保证整行不超过 w。
func Header(title, right string, w int, rule string) string {
	st := theme.S()
	leftPlain := "◆ " + title
	if right != "" {
		maxRight := w - Width(leftPlain) - 1
		if maxRight < 8 {
			right = "" // 放不下就不显示，好过把标题挤掉
		} else if Width(right) > maxRight {
			right = Truncate(right, maxRight, "…")
		}
	}

	line := st.Accent.Bold(true).Render("◆ ") + st.Title.Render(title)
	if right != "" {
		gap := w - Width(leftPlain) - Width(right)
		if gap < 1 {
			gap = 1
		}
		line += strings.Repeat(" ", gap) + st.Muted.Render(right)
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
	return left + strings.Repeat(" ", gap) + st.Muted.Render(right)
}
