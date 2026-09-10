package components

import "testing"

func sampleBindings() []Binding {
	return []Binding{
		{Keys: []string{"↑", "↓"}, Desc: "move"},
		{Keys: []string{"space"}, Desc: "select"},
		{Keys: []string{"a"}, Desc: "all"},
		{Keys: []string{"d"}, Desc: "remove"},
		{Keys: []string{"/"}, Desc: "search"},
		{Keys: []string{"s"}, Desc: "sort:size"},
		{Keys: []string{"i"}, Desc: "invert"},
		{Keys: []string{"r"}, Desc: "rescan"},
		{Keys: []string{"?"}, Desc: "help"},
		{Keys: []string{"q"}, Desc: "quit"},
	}
}

// TestHelpRespectsMaxWidth 校验 Help 的输出宽度绝不超过 maxWidth。
//
// 这里曾经算错过：分隔符被重复计入宽度，导致估算值虚高、提前丢弃条目，
// 剩下的半条又被调用方兜底截断，界面上出现「sort:si…」这种残字。
func TestHelpRespectsMaxWidth(t *testing.T) {
	for _, max := range []int{6, 10, 16, 24, 32, 48, 64, 80, 120, 200} {
		if got := Width(Help(sampleBindings(), 0, max)); got > max {
			t.Errorf("maxWidth=%d 时输出宽度为 %d", max, got)
		}
	}
}

// TestHelpDropsWholeItems 校验放不下的条目是整条舍弃，而不是截断出半条。
func TestHelpDropsWholeItems(t *testing.T) {
	bindings := sampleBindings()
	for _, max := range []int{20, 30, 40, 50, 60} {
		got := StripANSI(Help(bindings, 0, max))
		if len(got) == 0 {
			continue
		}
		// 只要出现省略号，说明某条被截断了——应当改为整条舍弃。
		if containsEllipsis(got) {
			t.Errorf("maxWidth=%d 时出现被截断的条目：%q", max, got)
		}
	}
}

// TestHelpAlertFitsWidth 校验带告警样式的条目同样遵守宽度上限。
func TestHelpAlertFitsWidth(t *testing.T) {
	bindings := []Binding{
		{Keys: []string{"D"}, Desc: "confirm 3 apps", Alert: true},
		{Keys: []string{"any key"}, Desc: "cancel"},
	}
	for _, max := range []int{12, 20, 30, 40, 60} {
		if got := Width(Help(bindings, 0, max)); got > max {
			t.Errorf("maxWidth=%d 时告警条目输出宽度为 %d", max, got)
		}
	}
}

func containsEllipsis(s string) bool {
	for _, r := range s {
		if r == '…' {
			return true
		}
	}
	return false
}
