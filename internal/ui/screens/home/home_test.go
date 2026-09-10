package home

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/unieditdept/ued-uninstaller/internal/config"
	"github.com/unieditdept/ued-uninstaller/internal/i18n"
	"github.com/unieditdept/ued-uninstaller/internal/ui"
	"github.com/unieditdept/ued-uninstaller/internal/ui/components"
)

// TestMain 强制真彩色，保证渲染路径与运行时一致。
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	os.Exit(m.Run())
}

func setupData(t *testing.T) {
	t.Helper()
	base := t.TempDir()
	appdata := filepath.Join(base, "roaming")
	t.Setenv("APPDATA", appdata)
	t.Setenv("LOCALAPPDATA", filepath.Join(base, "local"))
	t.Setenv("TEMP", filepath.Join(base, "temp"))

	dirs := []string{
		filepath.Join(appdata, "unieditdept", "NovaEditor"),
		filepath.Join(appdata, "unieditdept", "PixelForge"),
		filepath.Join(appdata, "unieditdept", "Zeta"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "data.bin"), make([]byte, 1024), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// ready 创建主屏幕并驱动到扫描完成，尺寸固定为 100x30。
func ready(t *testing.T) *Model {
	t.Helper()
	cfg := config.Default()
	cfg.Animate = false
	m := New(cfg, 100, 30)

	pending := []tea.Cmd{m.Init()}
	for i := 0; i < 5000 && len(pending) > 0; i++ {
		var next []tea.Cmd
		for _, c := range pending {
			if c == nil {
				continue
			}
			msg := c()
			if msg == nil {
				continue
			}
			if batch, ok := msg.(tea.BatchMsg); ok {
				next = append(next, batch...)
				continue
			}
			updated, cmd := m.Update(msg)
			m = updated.(*Model)
			if cmd != nil {
				next = append(next, cmd)
			}
		}
		pending = next
	}
	m.render() // 触发一次渲染，生成可点击区域
	return m
}

// render 渲染一帧并返回输出。
func (m *Model) render() string {
	m.Update(tea.WindowSizeMsg{Width: m.w, Height: m.h})
	return m.View()
}

func findHit(m *Model, kind hitKind, pred func(hitRegion) bool) (hitRegion, bool) {
	for _, r := range m.hits {
		if r.kind == kind && (pred == nil || pred(r)) {
			return r, true
		}
	}
	return hitRegion{}, false
}

func clickAt(m *Model, x, y int) (*Model, tea.Cmd) {
	updated, cmd := m.Update(tea.MouseMsg{
		X: x, Y: y,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	return updated.(*Model), cmd
}

// clickHit 点击某个命中区域的中心。
func clickHit(t *testing.T, m *Model, r hitRegion) (*Model, tea.Cmd) {
	t.Helper()
	m2, cmd := clickAt(m, r.x+r.w/2, r.y+r.h/2)
	m2.render()
	return m2, cmd
}

// TestMouseSelectRow 校验单击软件项会切换选中状态。
func TestMouseSelectRow(t *testing.T) {
	setupData(t)
	m := ready(t)

	row, ok := findHit(m, hitRow, nil)
	if !ok {
		t.Fatal("列表中没有可点击的行")
	}
	// 记录该行对应的软件名
	name := m.items[m.view[m.top+row.row]].Name

	m, _ = clickHit(t, m, row)
	if !m.items[indexOf(m, name)].Selected {
		t.Errorf("单击后 %s 应被选中", name)
	}

	row, _ = findHit(m, hitRow, func(r hitRegion) bool { return r.row == row.row })
	m, _ = clickHit(t, m, row)
	if m.items[indexOf(m, name)].Selected {
		t.Errorf("再次单击后 %s 应取消选中", name)
	}
}

// TestMouseClickUninstallShortcut 校验单击底部「卸载」提示等同按下 D。
func TestMouseClickUninstallShortcut(t *testing.T) {
	setupData(t)
	m := ready(t)

	// 先选中一项，否则点击 d 只会临时选中光标项。
	m, _ = press(m, " ")
	m.render()

	hit, ok := findHit(m, hitBinding, func(r hitRegion) bool { return r.click == "d" })
	if !ok {
		t.Fatal("底部没有可点击的卸载提示")
	}
	m, _ = clickHit(t, m, hit)
	if m.phase != phaseConfirm {
		t.Errorf("单击卸载提示后应进入待确认，实际 phase=%d", m.phase)
	}

	// 待确认阶段单击「其它键」应当取消。
	any, ok := findHit(m, hitBinding, func(r hitRegion) bool { return r.click == "any" })
	if !ok {
		t.Fatal("待确认阶段应提供可点击的取消提示")
	}
	m, _ = clickHit(t, m, any)
	if m.phase != phaseIdle {
		t.Errorf("单击取消提示后应回到空闲，实际 phase=%d", m.phase)
	}
}

// TestMouseClickQuit 校验单击「退出」提示会返回退出命令。
// 需要足够宽的终端，否则靠后的提示会被省略。
func TestMouseClickQuit(t *testing.T) {
	setupData(t)
	m := ready(t)
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 30})
	m.render()

	hit, ok := findHit(m, hitBinding, func(r hitRegion) bool { return r.click == "q" })
	if !ok {
		t.Fatal("底部没有可点击的退出提示")
	}
	_, cmd := clickHit(t, m, hit)
	if cmd == nil {
		t.Fatal("单击退出应当产生命令")
	}
	if _, ok := cmd().(ui.QuitMsg); !ok {
		t.Errorf("单击退出应返回 QuitMsg，实际 %T", cmd())
	}
}

// TestMouseClickLanguage 校验单击语言徽章会切换语言。
func TestMouseClickLanguage(t *testing.T) {
	setupData(t)
	m := ready(t)

	if m.lang != i18n.ZH {
		t.Fatalf("默认语言应为中文，实际 %s", m.lang)
	}

	en, ok := findHit(m, hitLang, func(r hitRegion) bool { return r.lang == i18n.EN })
	if !ok {
		t.Fatal("没有找到 English 徽章的可点击区域")
	}
	m, _ = clickHit(t, m, en)
	if m.lang != i18n.EN {
		t.Errorf("单击 English 后语言应为 en，实际 %s", m.lang)
	}

	// 单击 L 徽章切到下一个语言（此时是中文）。
	key, ok := findHit(m, hitLang, func(r hitRegion) bool { return r.next })
	if !ok {
		t.Fatal("没有找到 L 徽章的可点击区域")
	}
	m, _ = clickHit(t, m, key)
	if m.lang != i18n.ZH {
		t.Errorf("单击 L 徽章后语言应为 zh，实际 %s", m.lang)
	}

	zh, _ := findHit(m, hitLang, func(r hitRegion) bool { return r.lang == i18n.ZH })
	m, _ = clickHit(t, m, zh)
	if m.lang != i18n.ZH {
		t.Errorf("单击简体中文后语言应为 zh，实际 %s", m.lang)
	}
}

// TestMouseClickGitHub 校验单击 GitHub 入口会指向仓库地址。
// 这里刻意不执行返回的命令——那会真的拉起浏览器。
func TestMouseClickGitHub(t *testing.T) {
	setupData(t)
	m := ready(t)

	hit, ok := findHit(m, hitLink, nil)
	if !ok {
		t.Fatal("右上角没有 GitHub 入口")
	}
	if hit.link != githubURL {
		t.Errorf("链接应为 %s，实际 %s", githubURL, hit.link)
	}
	_, cmd := clickHit(t, m, hit)
	if cmd == nil {
		t.Fatal("单击 GitHub 应产生打开链接的命令")
	}
}

// TestHeaderShowsGitHub 校验右上角确实显示了 GitHub 字样。
func TestHeaderShowsGitHub(t *testing.T) {
	setupData(t)
	m := ready(t)
	view := components.StripANSI(m.View())
	if !strings.Contains(view, "GitHub") {
		t.Errorf("右上角应显示 GitHub 入口:\n%s", view)
	}
}

// TestMouseClicksAreInBounds 校验所有命中区域都落在终端范围内，且不重叠。
func TestMouseClicksAreInBounds(t *testing.T) {
	setupData(t)
	m := ready(t)

	if len(m.hits) == 0 {
		t.Fatal("应当存在可点击区域")
	}
	for _, r := range m.hits {
		if r.x < 0 || r.y < 0 || r.x+r.w > m.w || r.y+r.h > m.h {
			t.Errorf("命中区域越界：%+v（终端 %dx%d）", r, m.w, m.h)
		}
	}
	for i, a := range m.hits {
		for j, b := range m.hits {
			if i >= j {
				continue
			}
			if overlaps(a, b) {
				t.Errorf("命中区域重叠：%+v 与 %+v", a, b)
			}
		}
	}
}

func overlaps(a, b hitRegion) bool {
	return a.x < b.x+b.w && b.x < a.x+a.w && a.y < b.y+b.h && b.y < a.y+a.h
}

func indexOf(m *Model, name string) int {
	for i := range m.items {
		if m.items[i].Name == name {
			return i
		}
	}
	return -1
}

// press 模拟按下单个字符键。
func press(m *Model, key string) (*Model, tea.Cmd) {
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	return updated.(*Model), cmd
}
