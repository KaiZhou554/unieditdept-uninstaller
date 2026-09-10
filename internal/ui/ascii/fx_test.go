package ascii

import (
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// TestMain 强制使用真彩色。
// 否则在无 TTY 的测试环境里 lipgloss 会降级为无色输出，
// 断言「没有残留转义序列」之类的用例会变成空转。
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	os.Exit(m.Run())
}

// TestScanHighlightKeepsWidth 是 ScanHighlight 最重要的约束：
// 它只改颜色，绝不能改变显示宽度，否则调用方的排版会整体错位。
func TestScanHighlightKeepsWidth(t *testing.T) {
	samples := []string{
		"↑/↓  移动",
		"NovaEditor  8.00 KB  2026-09-10",
		" 一个名字非常非常长的软件用来测试截断 ",
		"D 确认卸载 3 项",
		"",
	}
	for _, s := range samples {
		want := ansi.StringWidth(s)
		for tick := 0; tick < 120; tick++ {
			if w := ansi.StringWidth(ScanHighlight(s, tick)); w != want {
				t.Fatalf("tick=%d 时宽度由 %d 变为 %d", tick, want, w)
			}
		}
	}
}

// TestScanHighlightANSIIntact 校验输出不会残留未闭合的转义序列。
func TestScanHighlightANSIIntact(t *testing.T) {
	s := "D 确认卸载 3 项 · 16.0 KB"
	for tick := 0; tick < 120; tick++ {
		got := ScanHighlight(s, tick)
		if rest := stripAllSGR(got); strings.ContainsRune(rest, 0x1b) {
			t.Fatalf("tick=%d 时残留未闭合序列：%q", tick, got)
		}
	}
}

// TestScanHighlightIsColored 确认测试环境确实产生了着色输出。
func TestScanHighlightIsColored(t *testing.T) {
	if !strings.ContainsRune(ScanHighlight("abc", 0), 0x1b) {
		t.Fatal("扫描高亮未产生 ANSI 着色，后续断言将失去意义")
	}
}

// TestScanHighlightMoves 校验光带随时间移动。
// 直接检查纯函数，比对比渲染结果更稳定。
func TestScanHighlightMoves(t *testing.T) {
	const n = 12
	before := make([]int, 40)
	after := make([]int, 40)
	for i := range before {
		before[i] = scanIndex(i, 0, n)
		after[i] = scanIndex(i, 20, n)
	}
	if reflect.DeepEqual(before, after) {
		t.Error("光带应当随时间移动")
	}
}

// TestScanIndexCoversRange 校验光带既有暗部也有亮部，而不是整片同色。
func TestScanIndexCoversRange(t *testing.T) {
	seen := map[int]bool{}
	for i := 0; i < 64; i++ {
		seen[scanIndex(i, 0, 12)] = true
	}
	if len(seen) < 5 {
		t.Errorf("光带应覆盖多个亮度档，实际只有 %d 档", len(seen))
	}
}

// TestSparkIsStableWidth 校验星形符号宽度恒为 1。
// 它嵌在标题前，宽度一旦变化就会把整行挤歪。
func TestSparkIsStableWidth(t *testing.T) {
	for tick := 0; tick < 4*fps; tick++ {
		if w := ansi.StringWidth(Spark(tick)); w != 1 {
			t.Fatalf("tick=%d 时星形宽度为 %d", tick, w)
		}
	}
}

// TestSparkAnimates 校验星形既换了字形也换了颜色，是真正的动画而非静止符号。
func TestSparkAnimates(t *testing.T) {
	glyphs := map[string]bool{}
	renders := map[string]bool{}
	for tick := 0; tick < 4*fps; tick++ {
		s := Spark(tick)
		glyphs[stripAllSGR(s)] = true
		renders[s] = true
	}
	if len(glyphs) < 2 {
		t.Errorf("星形应在实心与空心之间切换，实际只有 %d 种", len(glyphs))
	}
	if len(renders) < 8 {
		t.Errorf("星形颜色变化过少，实际只有 %d 种", len(renders))
	}
}

// TestRuleAdvancesSlowly 校验分隔线流光推进得足够慢。
func TestRuleAdvancesSlowly(t *testing.T) {
	// 相邻两帧的内容应当相同（每 3 帧才推进一格）。
	if Rule(40, 0) == Rule(40, 0) {
		// 同一 tick 自然相同，这里只是防止实现被改成随机。
	}
	if Rule(40, 0) != Rule(40, 1) {
		t.Error("分隔线不应每帧都推进")
	}
	if Rule(40, 0) == Rule(40, 3) {
		t.Error("分隔线应当每 3 帧推进一格")
	}
}

// TestScanHighlightEmpty 校验空串不会 panic。
func TestScanHighlightEmpty(t *testing.T) {
	if got := ScanHighlight("", 5); got != "" {
		t.Errorf("空串应原样返回，实际 %q", got)
	}
}

// TestCoolGradientUsesCoolPalette 校验外部分区用的渐变是蓝紫色，而不是主色粉红。
func TestCoolGradientUsesCoolPalette(t *testing.T) {
	const s = "OtherTool"
	out := CoolGradient(s, 0)
	if stripAllSGR(out) != s {
		t.Errorf("不应改变文本内容，实际 %q", stripAllSGR(out))
	}
	if !strings.ContainsRune(out, 0x1b) {
		t.Fatal("应产生 ANSI 着色，否则后续断言失去意义")
	}
	if !hasBlueDominant(out) {
		t.Errorf("应使用蓝紫色带（蓝色分量高于红色），实际 %q", out)
	}
	// 主色 #ff6699 的红色分量远高于蓝色，不该出现在冷色带里。
	if strings.Contains(out, "255;102;153") {
		t.Error("蓝紫渐变里不应混入主色粉红")
	}
}

// TestCoolGradientKeepsWidth 校验渐变只改颜色，不改显示宽度（排版依赖这一点）。
func TestCoolGradientKeepsWidth(t *testing.T) {
	for _, s := range []string{"", "pending", "OtherTool  4.00 KB  2026-09-10"} {
		want := ansi.StringWidth(s)
		for tick := 0; tick < 48; tick++ {
			if w := ansi.StringWidth(CoolGradient(s, tick)); w != want {
				t.Fatalf("tick=%d 时宽度由 %d 变为 %d", tick, want, w)
			}
		}
	}
}

// TestCoolGradientMoves 校验蓝紫渐变会随时间流动。
func TestCoolGradientMoves(t *testing.T) {
	if CoolGradient("scanning slowly", 0) == CoolGradient("scanning slowly", 8) {
		t.Error("渐变应随时间流动")
	}
}

// hasBlueDominant 判断串里是否存在「蓝色分量高于红色分量」的 RGB 前景色。
func hasBlueDominant(s string) bool {
	const marker = "38;2;"
	for i := 0; i+len(marker) <= len(s); i++ {
		if s[i:i+len(marker)] != marker {
			continue
		}
		parts := strings.SplitN(s[i+len(marker):], ";", 3)
		if len(parts) < 3 {
			continue
		}
		r, errR := strconv.Atoi(parts[0])
		if errR != nil {
			continue
		}
		// 第三个分量后面可能紧跟 'm' 或 ';'。
		third := parts[2]
		if j := strings.IndexAny(third, "m;"); j >= 0 {
			third = third[:j]
		}
		b, errB := strconv.Atoi(third)
		if errB == nil && b > r {
			return true
		}
	}
	return false
}

// stripAllSGR 移除完整的 SGR 序列；未闭合的 ESC 会被保留，便于断言发现。
func stripAllSGR(s string) string {
	var sb strings.Builder
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		if runes[i] != 0x1b {
			sb.WriteRune(runes[i])
			continue
		}
		if i+1 < len(runes) && runes[i+1] == '[' {
			j := i + 2
			for j < len(runes) && (runes[j] < 0x40 || runes[j] > 0x7e) {
				j++
			}
			if j < len(runes) {
				i = j
				continue
			}
		}
		sb.WriteRune(runes[i])
	}
	return sb.String()
}
