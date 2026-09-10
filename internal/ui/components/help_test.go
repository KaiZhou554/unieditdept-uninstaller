package components

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func sampleBindings() []Binding {
	return []Binding{
		{Keys: []string{"up", "down"}, Desc: "move"},
		{Keys: []string{"space"}, Desc: "select", Click: " "},
		{Keys: []string{"a"}, Desc: "all", Click: "a"},
		{Keys: []string{"d"}, Desc: "remove", Click: "d"},
		{Keys: []string{"/"}, Desc: "search", Click: "/"},
		{Keys: []string{"s"}, Desc: "sort:size", Click: "s"},
		{Keys: []string{"i"}, Desc: "invert", Click: "i"},
		{Keys: []string{"r"}, Desc: "rescan", Click: "r"},
		{Keys: []string{"?"}, Desc: "help", Click: "?"},
		{Keys: []string{"q"}, Desc: "quit", Click: "q"},
	}
}

// TestHelpRespectsMaxWidth 校验 Help 的输出宽度绝不超过 maxWidth。
//
// 这里曾经算错过：分隔符被重复计入宽度、按键样式的内边距未计入，
// 导致估算值与实际渲染不符，条目被误丢弃又被兜底截断，出现「sort:si…」这种残字。
func TestHelpRespectsMaxWidth(t *testing.T) {
	for _, max := range []int{6, 10, 16, 24, 32, 48, 64, 80, 120, 200} {
		text, _ := Help(sampleBindings(), 0, max)
		if got := Width(text); got > max {
			t.Errorf("maxWidth=%d 时输出宽度为 %d", max, got)
		}
	}
}

// TestHelpDropsWholeItems 校验放不下的条目是整条舍弃，而不是截断出半条。
func TestHelpDropsWholeItems(t *testing.T) {
	bindings := sampleBindings()
	for _, max := range []int{20, 30, 40, 50, 60} {
		text, _ := Help(bindings, 0, max)
		got := StripANSI(text)
		if len(got) == 0 {
			continue
		}
		if strings.ContainsRune(got, '…') {
			t.Errorf("maxWidth=%d 时出现被截断的条目：%q", max, got)
		}
	}
}

// TestHelpAlertFitsWidth 校验带告警样式的条目同样遵守宽度上限。
func TestHelpAlertFitsWidth(t *testing.T) {
	bindings := []Binding{
		{Keys: []string{"D"}, Desc: "confirm 3 apps", Alert: true, Click: "d"},
		{Keys: []string{"any key"}, Desc: "cancel", Click: "any"},
	}
	for _, max := range []int{12, 20, 30, 40, 60} {
		text, _ := Help(bindings, 0, max)
		if got := Width(text); got > max {
			t.Errorf("maxWidth=%d 时告警条目输出宽度为 %d", max, got)
		}
	}
}

// TestHelpSpansMatchRender 校验返回的 Span 位置与真实渲染位置一致。
// 鼠标点击依赖这些坐标，一旦偏离就会点错条目。
func TestHelpSpansMatchRender(t *testing.T) {
	bindings := sampleBindings()
	text, spans := Help(bindings, 0, 0)
	plain := StripANSI(text)

	if len(spans) != len(bindings) {
		t.Fatalf("应当为每条提示都给出一段位置，实际 %d / %d", len(spans), len(bindings))
	}
	for _, s := range spans {
		seg := ansi.Cut(plain, s.Start, s.Start+s.Width)
		b := bindings[s.Index]
		if !strings.Contains(seg, b.Keys[0]) {
			t.Errorf("第 %d 段位置不对：切出 %q，期望包含按键 %q", s.Index, seg, b.Keys[0])
		}
		if got := Width(seg); got != s.Width {
			t.Errorf("第 %d 段宽度为 %d，Span 记录 %d", s.Index, got, s.Width)
		}
	}
}

// TestHelpSpansSkipDropped 校验被舍弃的条目不会留下 Span。
func TestHelpSpansSkipDropped(t *testing.T) {
	bindings := sampleBindings()
	text, spans := Help(bindings, 0, 40)
	if len(spans) >= len(bindings) {
		t.Fatalf("窄宽度下应当舍弃部分条目，实际保留了 %d 条", len(spans))
	}
	if Width(text) > 40 {
		t.Errorf("输出宽度 %d 超出上限", Width(text))
	}
}
