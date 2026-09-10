package home

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/unieditdept/ued-uninstaller/internal/config"
	"github.com/unieditdept/ued-uninstaller/internal/core"
	"github.com/unieditdept/ued-uninstaller/internal/i18n"
	"github.com/unieditdept/ued-uninstaller/internal/ui"
	"github.com/unieditdept/ued-uninstaller/internal/ui/components"
	"github.com/unieditdept/ued-uninstaller/internal/ui/theme"
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

// addExternalApp 在 %AppData% 根目录下造一个「其它软件」的数据文件夹，
// 返回文件夹路径。名字会自动补上 .exe 后缀（展示时应被去掉）。
func addExternalApp(t *testing.T, name string, size int) string {
	t.Helper()
	dir := filepath.Join(os.Getenv("APPDATA"), name+".exe")
	if err := os.MkdirAll(filepath.Join(dir, "EBWebView"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "EBWebView", "blob.bin"), make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// pressKey 模拟按下特殊键（方向键等）。
func pressKey(m *Model, k tea.KeyType) *Model {
	updated, _ := m.Update(tea.KeyMsg{Type: k})
	return updated.(*Model)
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

// findTopmost 返回满足条件的、位置最靠上的命中区域。
func findTopmost(m *Model, pred func(hitRegion) bool) (hitRegion, bool) {
	var best hitRegion
	found := false
	for _, r := range m.hits {
		if !pred(r) {
			continue
		}
		if !found || r.y < best.y || (r.y == best.y && r.x < best.x) {
			best, found = r, true
		}
	}
	return best, found
}

// TestExternalSection 校验「其它软件」分区：提示夹在两类软件之间，条目去掉 .exe 后缀。
func TestExternalSection(t *testing.T) {
	setupData(t)
	addExternalApp(t, "OtherTool", 4096)
	m := ready(t)

	view := components.StripANSI(m.View())
	appAt := strings.Index(view, "NovaEditor")
	noticeAt := strings.Index(view, m.txt.ExternalNotice)
	extAt := strings.Index(view, "OtherTool")
	if appAt < 0 || noticeAt < 0 || extAt < 0 {
		t.Fatalf("三部分都应出现：app=%d notice=%d external=%d\n%s", appAt, noticeAt, extAt, view)
	}
	if !(appAt < noticeAt && noticeAt < extAt) {
		t.Errorf("顺序应为 软件 < 提示 < 其它软件，实际 %d / %d / %d", appAt, noticeAt, extAt)
	}
	if strings.Contains(view, "OtherTool.exe") {
		t.Errorf("条目名不应带 .exe 后缀：\n%s", view)
	}
	if !strings.Contains(view, "4.00 KB") {
		t.Errorf("外部软件的占用应被异步统计出来：\n%s", view)
	}

	// 外部条目是淡灰色，说明文字再压深一档；这块整体要比主色系低调。
	if !strings.Contains(m.View(), rgb(theme.ExternalText)) {
		t.Error("外部条目应以淡灰色显示")
	}
	if !strings.Contains(m.View(), rgb(theme.ExternalNote)) {
		t.Error("说明文字应以深灰色显示")
	}
}

// TestOpenGitHubKey 校验顶部有可点击的 P 提示、按 P 会打开仓库。
func TestOpenGitHubKey(t *testing.T) {
	setupData(t)
	m := ready(t)

	// 按 P 应产生打开仓库的命令。刻意不执行它，免得测试真的弹出浏览器。
	if _, cmd := press(m, "p"); cmd == nil {
		t.Error("按 P 应产生打开仓库的命令")
	}
	if _, cmd := press(m, "P"); cmd == nil {
		t.Error("大写 P 同样应生效")
	}

	// 顶部应有一枚可点击的 P 提示，位于 GitHub 入口左侧。
	pHit, ok := findHit(m, hitBinding, func(r hitRegion) bool { return r.click == "p" })
	if !ok {
		t.Fatal("顶部应有 P 提示")
	}
	if pHit.y != 0 {
		t.Errorf("P 提示应在标题栏，实际 y=%d", pHit.y)
	}
	ghHit, ok := findHit(m, hitLink, nil)
	if !ok {
		t.Fatal("顶部应有 GitHub 入口")
	}
	if pHit.x >= ghHit.x {
		t.Errorf("P 提示应在 GitHub 入口左侧，实际 p.x=%d gh.x=%d", pHit.x, ghHit.x)
	}

	// 与 L 徽章同款：渲染串应当就是 Chip 样式套上 " P "。
	st := theme.S()
	if !strings.Contains(m.View(), st.Chip.Render(" P ")) {
		t.Error("P 提示应使用与 L 徽章相同的 Chip 样式")
	}
	if !strings.Contains(m.View(), st.Chip.Render(" L ")) {
		t.Error("L 徽章仍应是 Chip 样式")
	}

	// 帮助页里 P 同样可用（顶部入口在帮助页也看得见），且列表里有这一条。
	help, _ := press(m, "?")
	if !help.showHelp {
		t.Fatal("应进入帮助页")
	}
	if _, cmd := press(help, "p"); cmd == nil {
		t.Error("帮助页里按 P 也应生效")
	}
	listed := false
	for _, e := range help.txt.HelpEntries {
		if e[0] == "P" {
			listed = true
			break
		}
	}
	if !listed {
		t.Error("帮助页的快捷键列表应包含 P")
	}
}

// TestExternalNoticeSpacing 校验提示与下面的条目之间只空一行。
// 提示区行数曾经写死成四行，宽窗口下文案只占一行，于是多出两个空行。
func TestExternalNoticeSpacing(t *testing.T) {
	setupData(t)
	addExternalApp(t, "OtherTool", 4096)
	m := ready(t)

	lines := strings.Split(components.StripANSI(m.View()), "\n")
	noticeIdx := -1
	for i, line := range lines {
		if strings.Contains(line, m.txt.ExternalNotice) {
			noticeIdx = i
			break
		}
	}
	if noticeIdx < 0 {
		t.Fatalf("应显示外部软件提示：\n%s", strings.Join(lines, "\n"))
	}
	i := noticeIdx
	for i < len(lines) && strings.Contains(lines[i], m.txt.ExternalNotice) {
		i++ // 跳过折行的后续行
	}
	// 提示之后应当恰好隔一行（那一行在列表区里是空的）就接到外部条目。
	if strings.Contains(lines[i], "OtherTool") {
		t.Fatalf("提示与外部条目之间应空一行，实际紧挨着：%q", lines[i])
	}
	if !strings.Contains(lines[i+1], "OtherTool") {
		t.Errorf("提示之后应只空一行，实际第 %d 行是 %q、第 %d 行是 %q",
			i, lines[i], i+1, lines[i+1])
	}
}

// TestConfirmIgnoresClicks 校验待确认期间单击列表不再改变选择。
func TestConfirmIgnoresClicks(t *testing.T) {
	setupData(t)
	m := ready(t)

	m = pressKey(m, tea.KeyDown)
	m, _ = press(m, " ")
	if !m.items[m.view[1]].Selected {
		t.Fatal("前置条件：第二项应已选中")
	}
	m, _ = press(m, "d")
	if m.phase != phaseConfirm {
		t.Fatal("应进入待确认状态")
	}

	hit, ok := findHit(m, hitRow, func(r hitRegion) bool { return r.row == 0 })
	if !ok {
		t.Fatal("第一行应可点击")
	}
	before := m.items[m.view[0]].Selected
	m, _ = clickHit(t, m, hit)
	if m.items[m.view[0]].Selected != before {
		t.Error("待确认期间单击不应改变选择")
	}
	if m.phase != phaseConfirm {
		t.Errorf("待确认期间单击不应退出确认，实际 phase=%d", m.phase)
	}
}

// TestConfirmHidesCursor 校验待确认期间光标行不再高亮 ——
// 否则会让人以为光标停的那一项也在删除范围内。
func TestConfirmHidesCursor(t *testing.T) {
	setupData(t)
	m := ready(t)

	// 光标停回第一项，但只选中第三项。
	m = pressKey(m, tea.KeyDown)
	m = pressKey(m, tea.KeyDown)
	m, _ = press(m, " ")
	m = pressKey(m, tea.KeyUp)
	m = pressKey(m, tea.KeyUp)
	if m.cursor != 0 {
		t.Fatalf("前置条件：光标应在首行，实际 %d", m.cursor)
	}
	cursorRow := m.items[m.view[m.cursor]].Name
	if m.items[m.view[m.cursor]].Selected {
		t.Fatal("前置条件：光标所在项不应被选中")
	}

	m, _ = press(m, "d")
	if m.phase != phaseConfirm {
		t.Fatal("应进入待确认状态")
	}

	for _, line := range strings.Split(m.View(), "\n") {
		if !strings.Contains(components.StripANSI(line), cursorRow) {
			continue
		}
		if strings.Contains(line, "48;2;"+rgb(theme.PrimaryDeep)) {
			t.Errorf("待确认期间未选中的光标行不应高亮：%q", line)
		}
	}
}

// TestExternalConfirmUsesCoolScan 校验待确认时外部条目扫过的是蓝紫光带。
// UniEditDept 的条目是粉红光带，两者必须能一眼分开。
func TestExternalConfirmUsesCoolScan(t *testing.T) {
	setupData(t)
	addExternalApp(t, "OtherTool", 4096)
	m := ready(t)

	for m.cursor < len(m.view)-1 {
		m = pressKey(m, tea.KeyDown)
	}
	m, _ = press(m, " ")
	m, _ = press(m, "d")
	if m.phase != phaseConfirm {
		t.Fatal("应进入待确认状态")
	}
	// 逐行看：底栏的「确认卸载」提示本来就闪粉红，不能拿整屏来判断。
	found := false
	for _, line := range strings.Split(m.View(), "\n") {
		if !strings.Contains(components.StripANSI(line), "OtherTool") {
			continue
		}
		found = true
		if !strings.Contains(line, rgb(theme.CoolScanPale)) {
			t.Errorf("外部条目应扫过蓝紫色光带：%q", line)
		}
		if strings.Contains(line, rgb(theme.FlashOn)) {
			t.Errorf("外部条目的光带不应是粉红色：%q", line)
		}
	}
	if !found {
		t.Fatal("没有找到外部条目所在的行")
	}
}

// rgb 把主题色转换成 lipgloss 在真彩色下输出的 "r;g;b" 片段，便于断言。
func rgb(c lipgloss.Color) string {
	s := strings.TrimPrefix(string(c), "#")
	r, _ := strconv.ParseInt(s[0:2], 16, 32)
	g, _ := strconv.ParseInt(s[2:4], 16, 32)
	b, _ := strconv.ParseInt(s[4:6], 16, 32)
	return strconv.Itoa(int(r)) + ";" + strconv.Itoa(int(g)) + ";" + strconv.Itoa(int(b))
}

// TestExternalRowHighlight 校验外部条目作为光标行时反白（白底深字）。
func TestExternalRowHighlight(t *testing.T) {
	setupData(t)
	addExternalApp(t, "OtherTool", 4096)
	m := ready(t)

	for m.cursor < len(m.view)-1 {
		m = pressKey(m, tea.KeyDown)
	}
	if idx := m.view[m.cursor]; !m.items[idx].External {
		t.Fatalf("光标应停在最后一条外部软件上，实际 %q", m.items[idx].Name)
	}
	if !strings.Contains(m.View(), "48;2;"+rgb(theme.ExternalRow)) {
		t.Error("外部条目作为光标行时应反白")
	}
}

// TestCursorAndClickReachExternalRows 校验光标与鼠标都能正确落到外部分区的条目上。
func TestCursorAndClickReachExternalRows(t *testing.T) {
	setupData(t)
	addExternalApp(t, "OtherTool", 4096)
	m := ready(t)

	// 光标能一路走到外部分区（提示区占了几行，别把光标算错位）。
	before := m.cursor
	for m.cursor < len(m.view)-1 {
		m = pressKey(m, tea.KeyDown)
	}
	if m.cursor <= before {
		t.Fatal("方向键应能向下移动光标")
	}
	if idx := m.view[m.cursor]; !m.items[idx].External {
		t.Fatalf("光标应到达外部软件，实际 %q", m.items[idx].Name)
	}

	// 点击外部分区的行，应恰好切换那一条。
	hit, ok := findHit(m, hitRow, func(r hitRegion) bool {
		return m.items[m.view[r.row]].External
	})
	if !ok {
		t.Fatal("外部条目应可点击")
	}
	want := m.items[m.view[hit.row]].Name
	m, _ = clickHit(t, m, hit)
	if !m.items[m.view[hit.row]].Selected {
		t.Errorf("单击外部条目 %q 应选中它", want)
	}
	for _, sw := range m.items {
		if !sw.External && sw.Selected {
			t.Errorf("内部软件 %q 不应被牵连选中", sw.Name)
		}
	}
}

// TestHelpPageSwallowsListKeys 校验帮助页只看不按：列表相关按键一律不生效。
//
// 这条曾经是真实缺陷：showHelp 只参与了渲染，没参与按键分发，
// 于是帮助页开着也能选择、卸载、重扫。
func TestHelpPageSwallowsListKeys(t *testing.T) {
	setupData(t)
	m := ready(t)

	m, _ = press(m, "?")
	m.render()
	if !m.showHelp {
		t.Fatal("应先进入帮助页")
	}

	selected := core.SelectedCount(m.items)
	for _, k := range []string{" ", "a", "i", "d", "s", "/", "r"} {
		m, cmd := press(m, k)
		if !m.showHelp {
			t.Fatalf("按下 %q 不应离开帮助页", k)
		}
		if cmd != nil {
			t.Errorf("按下 %q 不应产生命令", k)
		}
	}
	if got := core.SelectedCount(m.items); got != selected {
		t.Errorf("帮助页内不应改变选中数：%d → %d", selected, got)
	}
	if m.phase != phaseIdle {
		t.Errorf("帮助页内不应切换阶段，实际 %v", m.phase)
	}

	// 移动键同样要被吞掉。
	if m.cursor != 0 {
		t.Fatalf("前置条件：光标应停在首行，实际 %d", m.cursor)
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(*Model)
	if m.cursor != 0 {
		t.Errorf("帮助页内方向键不应移动光标，实际 %d", m.cursor)
	}

	// 底栏提示要同步收窄，不能还列着按不动的键。
	m.render()
	footer := components.StripANSI(strings.Split(m.View(), "\n")[m.h-1])
	if strings.Contains(footer, m.txt.KeySelectAll) || strings.Contains(footer, m.txt.KeyUninstall) {
		t.Errorf("帮助页底栏不应仍列出列表快捷键：%q", footer)
	}

	// 该保留的仍然可用：切换语言、返回。
	m, _ = press(m, "l")
	if m.lang != i18n.EN {
		t.Errorf("帮助页内应仍可切换语言，实际 %q", m.lang)
	}
	if !m.showHelp {
		t.Error("切换语言后应仍停留在帮助页")
	}
	m, _ = press(m, "?")
	if m.showHelp {
		t.Error("再按 ? 应关闭帮助")
	}
}

// TestHelpBackButton 校验帮助页顶部的返回按钮能关闭帮助。
func TestHelpBackButton(t *testing.T) {
	setupData(t)
	m := ready(t)

	m, _ = press(m, "?")
	m.render()
	if !m.showHelp {
		t.Fatal("按 ? 应打开帮助")
	}

	back, ok := findTopmost(m, func(r hitRegion) bool {
		return r.kind == hitBinding && r.click == "?"
	})
	if !ok {
		t.Fatal("帮助页应有可点击的返回入口")
	}
	if back.y >= m.h-1 {
		t.Errorf("返回按钮应在内容区顶部而不是底栏，实际 y=%d", back.y)
	}

	// 命中区域必须正好盖住按钮文字（含左箭头），否则点上去会落空。
	rows := strings.Split(components.StripANSI(m.View()), "\n")
	row := []rune(rows[back.y])
	if back.x+back.w > len(row) {
		t.Fatalf("命中区域越界：x=%d w=%d 行长=%d", back.x, back.w, len(row))
	}
	seg := string(row[back.x : back.x+back.w])
	if !strings.Contains(seg, "?") || !strings.Contains(seg, "←") ||
		!strings.Contains(seg, m.txt.HelpBack) {
		t.Errorf("返回按钮命中区域与文字不对齐：%q", seg)
	}

	m, _ = clickHit(t, m, back)
	if m.showHelp {
		t.Error("单击返回后应关闭帮助")
	}
}

// TestHelpListLayout 校验「?」排在快捷键最前，且其后空一行。
func TestHelpListLayout(t *testing.T) {
	setupData(t)
	m := ready(t)

	if got := m.txt.HelpEntries[0][0]; got != "?" {
		t.Fatalf("「?」应当是第一条快捷键，实际 %q", got)
	}
	lines, back := m.helpLines()
	if back.line != 1 {
		t.Errorf("返回按钮应在内容区第 1 行，实际 %d", back.line)
	}
	plain := func(i int) string {
		if i >= len(lines) {
			t.Fatalf("帮助内容只有 %d 行，缺少第 %d 行", len(lines), i)
		}
		return components.StripANSI(lines[i])
	}
	if !strings.Contains(plain(3), "?") {
		t.Errorf("第 3 行应为「?」条目，实际 %q", plain(3))
	}
	if plain(4) != "" {
		t.Errorf("「?」条目之后应空一行，实际 %q", plain(4))
	}
}

// TestClickTitleGoesHome 校验任何状态下单击顶部标题都回到主界面。
func TestClickTitleGoesHome(t *testing.T) {
	setupData(t)
	m := ready(t)

	title, ok := findHit(m, hitTitle, nil)
	if !ok {
		t.Fatal("应存在标题命中区域")
	}

	// 帮助页 → 主界面
	m, _ = press(m, "?")
	m.render()
	m, _ = clickHit(t, m, title)
	if m.showHelp {
		t.Error("在帮助页点击标题应关闭帮助")
	}

	// 待确认 → 取消
	m, _ = press(m, " ")
	m, _ = press(m, "d")
	m.render()
	if m.phase != phaseConfirm {
		t.Fatalf("应进入待确认，实际 phase=%d", m.phase)
	}
	m, _ = clickHit(t, m, title)
	if m.phase != phaseIdle {
		t.Errorf("待确认时点击标题应取消，实际 phase=%d", m.phase)
	}

	// 卸载结果 → 回到空闲展示
	m.phase = phaseDone
	m.render()
	m, _ = clickHit(t, m, title)
	if m.phase != phaseIdle {
		t.Errorf("结果页点击标题应回到空闲，实际 phase=%d", m.phase)
	}

	// 卸载进行中不打断
	m.phase = phaseDelete
	m.render()
	m, _ = clickHit(t, m, title)
	if m.phase != phaseDelete {
		t.Errorf("卸载进行中不应被标题点击打断，实际 phase=%d", m.phase)
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
