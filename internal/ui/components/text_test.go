package components

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// TestMain 强制使用真彩色，避免 lipgloss 在无 TTY 环境下退化为无色输出，
// 让「截断不破坏 ANSI 序列」这类断言真正生效。
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	os.Exit(m.Run())
}

// TestStyledOutputIsColored 确认测试环境确实产生了着色输出。
func TestStyledOutputIsColored(t *testing.T) {
	if !strings.ContainsRune(lipgloss.NewStyle().Foreground(lipgloss.Color("#ff6699")).Render("x"), 0x1b) {
		t.Fatal("未产生 ANSI 着色，后续断言将失去意义")
	}
}

// TestWidthMatchesLipgloss 是本包最关键的约束：
// 我们用来排版算宽度的函数，必须和 lipgloss/bubbletea 渲染时认为的宽度完全一致。
//
// 历史上这里踩过坑：core 层曾用 mattn/go-runewidth 计算宽度，而它在中文 Windows 上
// 会把 EastAsianWidth 置为 true，导致 ↑ ↓ · ▸ 这类 ambiguous 字符被算成 2 列；
// 而 lipgloss 按 1 列渲染。两套宽度不一致会让每行多算若干列，最终在 ANSI 转义序列
// 中间截断，表现为「底部提示条只剩 ↑/」。
func TestWidthMatchesLipgloss(t *testing.T) {
	samples := []string{
		"↑/↓  移动",
		"◆ UNIEDITDEPT 卸载程序",
		"▸ [✓] taffy_team",
		"已选 2 项 · 18.8 MB",
		"· · · · ·",
		"space  选择   a  全选",
		"纯中文标题测试",
		"mixed 中英 abc 123",
	}
	for _, s := range samples {
		if got, want := Width(s), lipgloss.Width(s); got != want {
			t.Errorf("Width(%q) = %d，但 lipgloss 渲染为 %d 列", s, got, want)
		}
		if got, want := Width(s), ansi.StringWidth(s); got != want {
			t.Errorf("Width(%q) = %d，但 ansi.StringWidth 为 %d", s, got, want)
		}
	}
}

// TestTruncateKeepsANSIIntact 校验截断含样式的字符串不会留下未闭合的转义序列。
func TestTruncateKeepsANSIIntact(t *testing.T) {
	styles := []lipgloss.Style{
		lipgloss.NewStyle().Foreground(lipgloss.Color("#ff6699")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#ffdce8")).Background(lipgloss.Color("#c94f79")).Bold(true),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5d73")).Background(lipgloss.Color("#2b0710")).Bold(true),
	}
	for _, st := range styles {
		styled := st.Render("↑/↓") + "/" + st.Render("↓") + " " + st.Render("移动   space  选择")
		for w := 1; w <= Width(styled)+4; w++ {
			got := Truncate(styled, w, "…")
			if Width(got) > w {
				t.Errorf("Truncate 到 %d 后宽度为 %d", w, Width(got))
			}
			// 剥离所有合法 ANSI 序列后不应再有残留的 ESC——有则说明序列被切断了。
			if rest := stripAll(got); strings.ContainsRune(rest, 0x1b) {
				t.Errorf("宽度 %d 截断后残留未闭合的转义序列：%q", w, got)
			}
		}
	}
}

// stripAll 移除所有 ANSI 转义序列；未被识别的残留 ESC 会被保留下来供断言检查。
func stripAll(s string) string {
	var sb strings.Builder
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		if runes[i] != 0x1b {
			sb.WriteRune(runes[i])
			continue
		}
		// ESC 序列：ESC [ ... final(0x40~0x7e)
		if i+1 < len(runes) && runes[i+1] == '[' {
			j := i + 2
			for j < len(runes) && (runes[j] < 0x40 || runes[j] > 0x7e) {
				j++
			}
			if j < len(runes) {
				i = j // 合法序列，整段丢弃
				continue
			}
		}
		sb.WriteRune(runes[i]) // 未闭合，保留 ESC 让断言发现
	}
	return sb.String()
}
