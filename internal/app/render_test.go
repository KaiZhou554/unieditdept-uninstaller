package app

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/unieditdept/ued-uninstaller/internal/config"
	"github.com/unieditdept/ued-uninstaller/internal/ui"
	"github.com/unieditdept/ued-uninstaller/internal/ui/components"
	"github.com/unieditdept/ued-uninstaller/internal/ui/screens/home"
	"github.com/unieditdept/ued-uninstaller/internal/version"
)

// TestMain 强制真彩色：布局与 ANSI 完整性断言只有在真正产生转义序列时才有效。
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	os.Exit(m.Run())
}

// setupData 在临时目录中构造三个根位置的软件数据。
func setupData(t *testing.T) (dirs []string) {
	t.Helper()
	base := t.TempDir()
	appdata := filepath.Join(base, "roaming")
	local := filepath.Join(base, "local")
	temp := filepath.Join(base, "temp")
	t.Setenv("APPDATA", appdata)
	t.Setenv("LOCALAPPDATA", local)
	t.Setenv("TEMP", temp)

	dirs = []string{
		filepath.Join(appdata, "unieditdept", "NovaEditor"),
		filepath.Join(local, "unieditdept", "NovaEditor"),
		filepath.Join(temp, "unieditdept", "PixelForge"),
		filepath.Join(appdata, "unieditdept", "一个名字非常非常长的软件用来测试截断行为abcdefghijklmnop"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		payload := make([]byte, 4096)
		if err := os.WriteFile(filepath.Join(dir, "data.bin"), payload, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dirs
}

// driveUntilIdle 反复执行命令并回灌消息，直到没有待执行的命令。
func driveUntilIdle(t *testing.T, s ui.Screen, first tea.Cmd, limit int) ui.Screen {
	t.Helper()
	pending := []tea.Cmd{first}
	for i := 0; i < limit; i++ {
		if len(pending) == 0 {
			return s
		}
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
			var cmd tea.Cmd
			s, cmd = s.Update(msg)
			if cmd != nil {
				next = append(next, cmd)
			}
		}
		pending = next
	}
	t.Fatal("命令驱动超过上限仍未结束")
	return s
}

// newScannedHome 创建一个已完成扫描的主界面。
func newScannedHome(t *testing.T) ui.Screen {
	t.Helper()
	screen := ui.Screen(home.New(testConfig(), 0, 0))
	return driveUntilIdle(t, screen, screen.Init(), 5000)
}

func testConfig() config.Config {
	cfg := config.Default()
	cfg.Animate = false
	return cfg
}

func press(s ui.Screen, key string) (ui.Screen, tea.Cmd) {
	return s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
}

func pressKey(s ui.Screen, k tea.KeyType) (ui.Screen, tea.Cmd) {
	return s.Update(tea.KeyMsg{Type: k})
}

// TestEnterHasNoAction 校验 Enter 不再具备任何含义。
// 它曾经同时兼任「开始卸载」「确认卸载」「退出」等多种角色，容易误操作。
func TestEnterHasNoAction(t *testing.T) {
	setupData(t)
	screen := newScannedHome(t)

	screen, _ = pressKey(screen, tea.KeyEnter)
	view := plainView(screen, 100, 30)
	if strings.Contains(view, "待确认") {
		t.Error("Enter 不应进入待确认状态")
	}
	if strings.Contains(view, "[✓]") {
		t.Error("Enter 不应选中任何软件")
	}

	// 进入待确认后按 Enter，应当与「其它任意键」一样取消，而不是执行卸载。
	screen, _ = press(screen, "d")
	screen, _ = pressKey(screen, tea.KeyEnter)
	view = plainView(screen, 100, 30)
	if !strings.Contains(view, "已取消") {
		t.Errorf("待确认状态下按 Enter 应视同其它键取消\n%s", view)
	}
}

func render(s ui.Screen, w, h int) []string {
	next, _ := s.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return strings.Split(next.View(), "\n")
}

// plainView 渲染并剥离 ANSI，便于做文本断言。
// （扫描高亮会在字符之间插入转义序列，直接匹配子串会失败。）
func plainView(s ui.Screen, w, h int) string {
	return components.StripANSI(strings.Join(render(s, w, h), "\n"))
}

// checkFits 校验界面在指定尺寸下不溢出。
func checkFits(t *testing.T, name string, s ui.Screen, w, h int) {
	t.Helper()
	lines := render(s, w, h)
	if len(lines) > h {
		t.Errorf("%s 在 %dx%d 下渲染了 %d 行，超出高度", name, w, h, len(lines))
	}
	for i, line := range lines {
		if got := components.Width(line); got > w {
			t.Errorf("%s 在 %dx%d 下第 %d 行宽度为 %d，超出宽度:\n%s", name, w, h, i+1, got, line)
		}
	}
}

var sizes = [][2]int{{80, 24}, {100, 30}, {120, 40}, {160, 50}, {200, 60}}

// TestHomeLayout 校验主界面在常见尺寸下不溢出。
func TestHomeLayout(t *testing.T) {
	setupData(t)
	screen := newScannedHome(t)
	for _, size := range sizes {
		checkFits(t, "idle", screen, size[0], size[1])
	}

	// 待确认状态
	confirming, _ := press(screen, "d")
	for _, size := range sizes {
		checkFits(t, "confirm", confirming, size[0], size[1])
	}

	// 帮助覆盖层
	help, _ := press(screen, "?")
	for _, size := range sizes {
		checkFits(t, "help", help, size[0], size[1])
	}

	// 过滤状态
	filtering, _ := press(screen, "/")
	filtering, _ = press(filtering, "e")
	for _, size := range sizes {
		checkFits(t, "filter", filtering, size[0], size[1])
	}
}

// sgrPattern 匹配完整的 SGR 序列，用于检测残留的转义序列。
var sgrPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// hasBrokenANSI 判断字符串中是否残留未闭合的 ANSI 转义序列。
// 未闭合的序列会让终端吞掉后续内容，表现为半行文字凭空消失。
func hasBrokenANSI(s string) bool {
	return strings.Contains(sgrPattern.ReplaceAllString(s, ""), "\x1b")
}

// TestNoBrokenANSISequences 校验各种宽度下都不会产生未闭合的转义序列。
func TestNoBrokenANSISequences(t *testing.T) {
	setupData(t)
	screen := newScannedHome(t)
	for w := 40; w <= 200; w++ {
		for _, line := range render(screen, w, 24) {
			if hasBrokenANSI(line) {
				t.Fatalf("宽度 %d 出现未闭合的 ANSI 序列：%q", w, line)
			}
		}
	}
}

// TestFooterKeepsFirstBinding 校验底部提示在各宽度下都完整保留首项。
//
// 这条断言曾经失败过：footer 因为宽度算错而误判超宽，触发了一次会切断 ANSI
// 序列的截断，结果底部只剩 "↑/" 而其余内容被终端吞掉。
func TestFooterKeepsFirstBinding(t *testing.T) {
	setupData(t)
	screen := newScannedHome(t)
	// 选中两项，让右侧统计信息变长，制造最容易触发截断的情况。
	screen, _ = press(screen, " ")
	screen, _ = press(screen, "j")
	screen, _ = press(screen, " ")

	for _, s := range [][2]int{{40, 24}, {60, 24}, {80, 24}, {100, 30}, {126, 22}, {126, 24}, {140, 40}, {200, 60}} {
		w, h := s[0], s[1]
		lines := render(screen, w, h)
		footer := components.StripANSI(lines[len(lines)-1])
		if !strings.Contains(footer, "↑/↓") {
			t.Errorf("宽度 %d 下底部提示不完整，实际：%q", w, footer)
		}
		// 版本号已移到标题栏，底栏应彻底让给快捷键提示。
		if strings.Contains(footer, version.String()) {
			t.Errorf("宽度 %d 下版本号不应占用底栏，实际：%q", w, footer)
		}
		// 标题栏足够宽时，版本号应出现在 GitHub 入口左侧。
		if w < 80 {
			continue
		}
		header := components.StripANSI(lines[0])
		idxVer := strings.Index(header, version.String())
		idxGH := strings.Index(header, "GitHub")
		if idxVer < 0 || idxGH < 0 {
			t.Errorf("宽度 %d 下标题栏应同时含版本号与 GitHub 入口，实际：%q", w, header)
		} else if idxVer > idxGH {
			t.Errorf("宽度 %d 下版本号应在 GitHub 左侧，实际：%q", w, header)
		}
	}
}

// driveApp 驱动根模型，直到没有待执行的命令。
func driveApp(t *testing.T, m *Model, first tea.Cmd) *Model {
	t.Helper()
	pending := []tea.Cmd{first}
	for i := 0; i < 5000; i++ {
		if len(pending) == 0 {
			return m
		}
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
	t.Fatal("驱动根模型超过上限仍未结束")
	return m
}

// TestRescanKeepsTerminalSize 校验重新扫描后仍沿用当前终端尺寸。
//
// 曾经这里会退回构造时的默认尺寸，表现为「按 R 重扫后界面突然缩小，
// 必须改变窗口大小才能恢复」——因为新建的屏幕并不知道终端有多大。
func TestRescanKeepsTerminalSize(t *testing.T) {
	setupData(t)
	m := New(testConfig(), 0, 0)

	// bubbletea 启动时会下发一次真实尺寸。
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = updated.(*Model)
	m = driveApp(t, m, m.Init())

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	m = updated.(*Model)
	if cmd == nil {
		t.Fatal("按 R 应触发重新扫描")
	}
	msg := cmd()
	if _, ok := msg.(ui.RescanMsg); !ok {
		t.Fatalf("期望 ui.RescanMsg，实际 %T", msg)
	}
	updated, _ = m.Update(msg)
	m = updated.(*Model)

	lines := strings.Split(m.View(), "\n")
	if len(lines) != 30 {
		t.Errorf("重扫后渲染高度应为 30，实际 %d", len(lines))
	}
	// 分隔线行必定铺满整宽，用它来校验宽度。
	if got := components.Width(lines[1]); got != 120 {
		t.Errorf("重扫后渲染宽度应为 120，实际 %d", got)
	}
}

// TestLanguageSwitch 校验按 L 能在中英之间切换，且切换器始终展示两种语言。
func TestLanguageSwitch(t *testing.T) {
	setupData(t)
	screen := newScannedHome(t)

	// 切换器形如「L 简体中文 | English」（徽章样式带去色的内层留白）。
	checkSwitcher := func(name, view string) {
		t.Helper()
		zhAt, enAt := strings.Index(view, "简体中文"), strings.Index(view, "English")
		if zhAt < 0 || enAt < 0 {
			t.Errorf("%s：右上角应同时展示两种语言:\n%s", name, view)
			return
		}
		if zhAt > enAt {
			t.Errorf("%s：语言顺序应为「简体中文 | English」", name)
		}
		if !strings.Contains(view[:max(zhAt, 0)], "L") {
			t.Errorf("%s：切换器应带 L 键提示", name)
		}
	}

	zh := plainView(screen, 100, 30)
	if !strings.Contains(zh, "软件（") {
		t.Fatalf("默认应为中文:\n%s", zh)
	}
	checkSwitcher("默认", zh)

	screen, _ = press(screen, "L")
	en := plainView(screen, 100, 30)
	if !strings.Contains(en, "Apps (") {
		t.Errorf("切换后应显示英文列表标题:\n%s", en)
	}
	if strings.Contains(en, "软件（") {
		t.Errorf("切换后不应残留中文:\n%s", en)
	}
	checkSwitcher("英文", en)

	screen, _ = press(screen, "L")
	if back := plainView(screen, 100, 30); !strings.Contains(back, "软件（") {
		t.Errorf("再按一次应切回中文:\n%s", back)
	}
}

// TestExternalAppRemoval 端到端：非 UniEditDept 的软件也能整目录卸载。
func TestExternalAppRemoval(t *testing.T) {
	setupData(t)
	ext := filepath.Join(os.Getenv("APPDATA"), "OtherTool.exe")
	if err := os.MkdirAll(filepath.Join(ext, "EBWebView"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ext, "EBWebView", "blob.bin"), make([]byte, 4096), 0o644); err != nil {
		t.Fatal(err)
	}

	screen := newScannedHome(t)
	view := plainView(screen, 100, 30)
	if !strings.Contains(view, "OtherTool") {
		t.Fatalf("应识别并列出外部软件：\n%s", view)
	}
	if strings.Contains(view, "OtherTool.exe") {
		t.Errorf("列出时不应带 .exe 后缀：\n%s", view)
	}

	// 全选（把外部软件也一起选上）→ 待确认 → 确认 → 跑完整个事件流。
	screen, _ = press(screen, "a")
	screen, _ = press(screen, "d")
	screen, cmd := press(screen, "d")
	_ = driveUntilIdle(t, screen, cmd, 5000)

	if _, err := os.Stat(ext); !os.IsNotExist(err) {
		t.Error("外部软件的文件夹应被整体删除")
	}
}

// TestEnglishSingularCounts 校验英文计数文案在数量为 1 时不会退化成 "1 apps"。
//
// 英文没有能同时读通 0/1/N 的复数形式，所以计数名词一律写成 "app(s)"；
// 这条断言守住那个约定，避免以后又改回裸复数。
func TestEnglishSingularCounts(t *testing.T) {
	setupData(t)
	screen := newScannedHome(t)
	screen, _ = press(screen, "L") // 切到英文

	// 只选中一项，然后进入待确认，让面板显示计数。
	screen, _ = press(screen, " ")
	screen, _ = press(screen, "d")

	view := plainView(screen, 100, 30)
	if !strings.Contains(view, "1 app(s)") {
		t.Errorf("英文确认面板应显示 “1 app(s)”：\n%s", view)
	}
	for _, bad := range []string{"1 apps", "1 app "} {
		if strings.Contains(view, bad) {
			t.Errorf("英文文案不应出现 %q：\n%s", bad, view)
		}
	}

	// 底栏的确认提示同理。
	if !strings.Contains(view, "confirm 1 app(s)") {
		t.Errorf("英文底栏应显示 “confirm 1 app(s)”：\n%s", view)
	}
}

// TestEnglishLayoutFits 校验英文文案（通常更长）在各尺寸下同样不溢出。
func TestEnglishLayoutFits(t *testing.T) {
	setupData(t)
	screen := newScannedHome(t)
	screen, _ = press(screen, "L") // 切到英文

	for _, size := range sizes {
		checkFits(t, "en-idle", screen, size[0], size[1])
	}
	confirming, _ := press(screen, "d")
	for _, size := range sizes {
		checkFits(t, "en-confirm", confirming, size[0], size[1])
	}
	help, _ := press(screen, "?")
	for _, size := range sizes {
		checkFits(t, "en-help", help, size[0], size[1])
	}
}

// TestFooterCompleteAfterRemoval 校验卸载完成后底部仍是完整的按键列表，
// 而不是被裁剪成只剩「重扫 / 退出」。
func TestFooterCompleteAfterRemoval(t *testing.T) {
	setupData(t)
	screen := newScannedHome(t)
	screen, _ = press(screen, "a")
	screen, _ = press(screen, "d")
	screen, cmd := press(screen, "d")
	screen = driveUntilIdle(t, screen, cmd, 5000)

	lines := render(screen, 180, 30)
	footer := components.StripANSI(lines[len(lines)-1])
	for _, want := range []string{"移动", "选择", "卸载", "重扫", "退出"} {
		if !strings.Contains(footer, want) {
			t.Errorf("卸载完成后底部应保留 %q，实际：%q", want, footer)
		}
	}
}

// TestHelpPageTitle 校验帮助页用的是自己的标题，而不是「卸载任务」。
func TestHelpPageTitle(t *testing.T) {
	setupData(t)
	screen := newScannedHome(t)
	help, _ := press(screen, "?")

	view := plainView(help, 100, 30)
	if !strings.Contains(view, "╭─ 快捷键 ") {
		t.Errorf("帮助页标题应为「快捷键」:\n%s", view)
	}
	if strings.Contains(view, "创建日期为三个位置") {
		t.Error("帮助页不应再解释创建日期")
	}
}

// TestHomeSmallTerminal 校验极小终端下不崩溃。
func TestHomeSmallTerminal(t *testing.T) {
	setupData(t)
	screen := newScannedHome(t)
	_ = render(screen, 30, 8)
	_ = render(screen, 40, 10)
}

// TestHomeConfirmCancel 校验二次确认被其它键取消后不会删除任何文件，并恢复进入前的选中。
func TestHomeConfirmCancel(t *testing.T) {
	dirs := setupData(t)
	screen := newScannedHome(t)

	// 进入列表后未做选择 → 按 D 会自动选中光标项 → 取消后应恢复为未选中。
	screen, _ = press(screen, "d")
	view := plainView(screen, 100, 30)
	if !strings.Contains(view, "待确认") {
		t.Error("待确认状态下应出现“待确认”提示")
	}
	screen, _ = press(screen, "x")
	lines := render(screen, 100, 30)
	view = components.StripANSI(strings.Join(lines, "\n"))
	if !strings.Contains(view, "已取消") {
		t.Error("取消后应在右侧面板给出提示")
	}
	// 提示只出现在右侧面板，底部状态栏应恢复成按键说明。
	if footer := components.StripANSI(lines[len(lines)-1]); strings.Contains(footer, "已取消") {
		t.Errorf("取消提示不应占用底部状态栏：%q", footer)
	}
	if strings.Contains(view, "[✓]") {
		t.Errorf("取消后应清空临时选中的项\n%s", view)
	}
	for _, dir := range dirs {
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("取消后目录不应被删除: %s", dir)
		}
	}
}

// TestHomeConfirmPreservesUserSelection 校验用户主动选中的软件在取消后保留。
func TestHomeConfirmPreservesUserSelection(t *testing.T) {
	dirs := setupData(t)
	screen := newScannedHome(t)

	// 用户主动选中 2 个软件（不动光标 = 第一个；↓ + space = 第二个）。
	screen, _ = press(screen, " ")
	screen, _ = press(screen, "j")
	screen, _ = press(screen, " ")
	view := plainView(screen, 100, 30)
	if n := strings.Count(view, "[✓]"); n != 2 {
		t.Errorf("手动选中 2 项失败（实际 %d 项）:\n%s", n, view)
	}

	screen, _ = press(screen, "d") // 待确认
	view = plainView(screen, 100, 30)
	if !strings.Contains(view, "待确认") {
		t.Errorf("应进入待确认\n%s", view)
	}
	if !strings.Contains(view, "2 项") {
		t.Errorf("待确认界面应显示 2 项")
	}

	screen, _ = press(screen, "x") // 取消
	view = plainView(screen, 100, 30)
	if !strings.Contains(view, "已取消") {
		t.Errorf("应显示已取消\n%s", view)
	}
	if n := strings.Count(view, "[✓]"); n != 2 {
		t.Errorf("用户手动选中的 2 项应保留（实际 %d 项）:\n%s", n, view)
	}
	for _, dir := range dirs {
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("取消后目录不应被删除: %s", dir)
		}
	}
}

// TestHomeDeleteFlow 校验「D → D」两段式确认后真的执行卸载。
func TestHomeDeleteFlow(t *testing.T) {
	dirs := setupData(t)
	screen := newScannedHome(t)

	screen, _ = press(screen, "a") // 全选
	screen, _ = press(screen, "d") // 待确认
	screen, cmd := press(screen, "d")
	if cmd == nil {
		t.Fatal("确认后应启动卸载任务")
	}
	screen = driveUntilIdle(t, screen, cmd, 5000)

	view := strings.Join(render(screen, 100, 30), "\n")
	if !strings.Contains(view, "卸载完成") {
		t.Errorf("卸载完成后应显示结果，实际界面为:\n%s", view)
	}
	for _, dir := range dirs {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("目录应已被删除: %s", dir)
		}
	}
}

// TestHomeDeleteNothingSelected 校验没有选中项时 D 只作用于光标所在软件。
func TestHomeDeleteNothingSelected(t *testing.T) {
	setupData(t)
	screen := newScannedHome(t)
	screen, _ = press(screen, "d")
	screen, cmd := press(screen, "d")
	if cmd == nil {
		t.Fatal("应启动卸载任务")
	}
	screen = driveUntilIdle(t, screen, cmd, 5000)
	view := strings.Join(render(screen, 100, 30), "\n")
	if !strings.Contains(view, "卸载完成") {
		t.Errorf("应完成卸载，实际界面为:\n%s", view)
	}
}

// TestDumpHome 在设置 UED_DUMP 时导出界面快照，便于人工检查。
func TestDumpHome(t *testing.T) {
	out := os.Getenv("UED_DUMP")
	if out == "" {
		t.Skip("未设置 UED_DUMP，跳过导出")
	}
	setupData(t)
	// 造一个「其它软件」的数据目录，便于检查分区、配色与提示换行。
	ext := filepath.Join(os.Getenv("APPDATA"), "SomeOtherApp.exe")
	if err := os.MkdirAll(filepath.Join(ext, "EBWebView"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ext, "EBWebView", "blob.bin"), make([]byte, 3<<20), 0o644); err != nil {
		t.Fatal(err)
	}

	screen := ui.Screen(home.New(testConfig(), 0, 0))

	var sb strings.Builder
	dump := func(name string, s ui.Screen) {
		for _, size := range [][2]int{{80, 24}, {100, 30}} {
			s.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			s.Update(ui.FrameMsg{})
			s.Update(ui.FrameMsg{})
			s.Update(ui.FrameMsg{})
			sb.WriteString("=== " + name + " (" + strconv.Itoa(size[0]) + "x" + strconv.Itoa(size[1]) + ") ===\n")
			sb.WriteString(components.StripANSI(s.View()))
			sb.WriteString("\n\n")
		}
	}

	dump("启动（枚举完成，统计进行中）", screen)
	screen = driveUntilIdle(t, screen, screen.Init(), 5000)
	dump("空闲", screen)

	screen, _ = press(screen, "L")
	dump("English idle", screen)
	screen, _ = press(screen, "L")

	screen, _ = press(screen, "a")
	dump("已全选", screen)

	// 英文下的待确认面板：计数写成 "app(s)"，便于目视检查单复数与换行。
	screen, _ = press(screen, "L")
	screen, _ = press(screen, "d")
	dump("English confirm", screen)
	screen, _ = press(screen, "x")
	screen, _ = press(screen, "L")

	screen, _ = press(screen, "?")
	dump("帮助", screen)
	screen, _ = press(screen, "?")

	screen, _ = press(screen, "d")
	dump("待确认", screen)

	screen, cmd := press(screen, "d")
	screen = driveUntilIdle(t, screen, cmd, 5000)
	dump("卸载结果", screen)

	if err := os.WriteFile(out, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}
