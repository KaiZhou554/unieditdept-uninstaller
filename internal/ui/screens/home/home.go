// Package home 是程序唯一的主屏幕：左侧软件列表，右侧卸载任务进度。
//
// 进入后立即枚举出全部软件（几乎无耗时），占用统计在后台异步完成并逐个回填；
// 卸载通过「按 D 进入待确认 → 再按 D 确认」的两段式交互完成，
// 待确认期间条目与按键指示会闪烁，超时或按下其它键均视为取消。
package home

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/unieditdept/ued-uninstaller/internal/config"
	"github.com/unieditdept/ued-uninstaller/internal/core"
	"github.com/unieditdept/ued-uninstaller/internal/i18n"
	"github.com/unieditdept/ued-uninstaller/internal/platform"
	"github.com/unieditdept/ued-uninstaller/internal/ui"
	"github.com/unieditdept/ued-uninstaller/internal/ui/ascii"
	"github.com/unieditdept/ued-uninstaller/internal/ui/components"
	"github.com/unieditdept/ued-uninstaller/internal/ui/theme"
	"github.com/unieditdept/ued-uninstaller/internal/version"
)

// phase 表示界面当前所处的阶段。
type phase int

const (
	phaseScan    phase = iota // 正在后台统计占用
	phaseIdle                 // 空闲
	phaseConfirm              // 待二次确认
	phaseDelete               // 正在卸载
	phaseDone                 // 卸载完成
)

// confirmFrames 是二次确认的倒计时帧数（约 6 秒）。
const confirmFrames = 6 * ui.FrameRate

const confirmKeys = "dDyY"

// taskItem 是卸载任务中的一个目录。
type taskItem struct {
	name string
	path string
	size int64
	err  error
}

// deleteState 保存卸载任务的进度。
type deleteState struct {
	items  []taskItem
	done   int
	total  int
	freed  int64
	shown  int64
	result core.DeleteResult
}

// hitKind 区分可点击区域的类型。
type hitKind int

const (
	hitRow     hitKind = iota // 列表中的软件行
	hitBinding                // 底部按键提示
	hitLang                   // 右上角语言徽章
	hitLink                   // 右上角链接
	hitTitle                  // 顶部标题（回到主界面）
)

// githubURL 是右上角 GitHub 入口跳转的地址。
const githubURL = "https://github.com/KaiZhou554/unieditdept-uninstaller"

// hitRegion 是一块可点击区域，坐标以终端左上角为原点。
type hitRegion struct {
	kind  hitKind
	x, y  int
	w, h  int
	row   int       // hitRow：数据行序号（相对可视区首行）
	click string    // hitBinding：点击时模拟按下的键
	lang  i18n.Lang // hitLang：目标语言
	next  bool      // hitLang：点击的是 L 徽章
	link  string    // hitLink：要打开的链接
}

// headerHit 是右上角一枚徽章的位置（相对该行的起始列）。
type headerHit struct {
	start, width int
	kind         hitKind
	lang         i18n.Lang
	isKey        bool
	link         string
}

// Model 是主屏幕。
type Model struct {
	cfg   config.Config
	roots []core.Root
	items []core.Software

	lang i18n.Lang     // 当前语言
	txt  *i18n.Strings // 当前语言的文案

	view   []int // 过滤后的 items 索引
	index  map[string]int
	cursor int
	top    int

	sortMode core.SortMode
	autoSort bool // 统计完成后是否自动按占用排序

	filter    string
	filtering bool
	showHelp  bool

	phase phase
	tick  int

	scanDone  int
	scanTotal int
	scanBytes int64
	loaded    bool // 是否已收到枚举结果

	// savedSelected 记录进入待确认前用户已选中的软件，cancel 时恢复。
	savedSelected map[string]bool

	events     <-chan core.Event
	cancel     context.CancelFunc
	delCancel  context.CancelFunc
	del        deleteState
	confirmTTL int

	message    string
	messageTTL int

	// hits 在每次渲染时重建，记录当前帧的可点击区域。
	hits []hitRegion

	w, h int
}

type eventMsg struct{ ev core.Event }
type streamClosedMsg struct{}

// New 创建主屏幕。
// width / height 为当前终端尺寸；传入 0 时退回默认值，等待 WindowSizeMsg 校正。
func New(cfg config.Config, width, height int) *Model {
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	lang := i18n.Parse(cfg.Lang)
	return &Model{
		cfg:      cfg,
		roots:    core.ResolveRoots(cfg.Namespace),
		items:    nil,
		index:    make(map[string]int),
		lang:     lang,
		txt:      i18n.Get(lang),
		sortMode: core.SortBySize,
		autoSort: true,
		phase:    phaseScan,
		w:        width,
		h:        height,
	}
}

// setLang 切换界面语言。
func (m *Model) setLang(l i18n.Lang) {
	m.lang = l
	m.txt = i18n.Get(l)
}

// Init 立即开始扫描（枚举 + 后台统计）。
func (m *Model) Init() tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	sc := core.NewScanner(m.cfg.Namespace)
	m.roots = sc.Roots
	m.events = sc.Stream(ctx)

	cmds := []tea.Cmd{waitEvent(m.events)}
	if m.cfg.Animate {
		cmds = append(cmds, ui.NextFrame())
	}
	return tea.Batch(cmds...)
}

// Close 结束后台的扫描 / 卸载任务。
func (m *Model) Close() {
	if m.cancel != nil {
		m.cancel()
	}
	if m.delCancel != nil {
		m.delCancel()
	}
}

func waitEvent(ch <-chan core.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return streamClosedMsg{}
		}
		return eventMsg{ev: ev}
	}
}

// Update 处理消息。
func (m *Model) Update(msg tea.Msg) (ui.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		return m, nil

	case ui.FrameMsg:
		m.tick++
		m.tickTimers()
		return m, ui.NextFrame()

	case eventMsg:
		return m.handleEvent(msg.ev)

	case streamClosedMsg:
		if m.phase == phaseDelete {
			m.finishDelete()
		}
		return m, nil

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// tickTimers 推进二次确认倒计时与提示信息的存活时间。
func (m *Model) tickTimers() {
	if m.phase == phaseConfirm {
		m.confirmTTL--
		if m.confirmTTL <= 0 {
			m.cancelConfirm(m.txt.CanceledByTime)
		}
	}
	if m.messageTTL > 0 {
		m.messageTTL--
		if m.messageTTL == 0 {
			m.message = ""
		}
	}
	if m.phase == phaseDelete && m.del.shown < m.del.freed {
		delta := (m.del.freed - m.del.shown) / 4
		if delta < 1 {
			delta = 1
		}
		m.del.shown += delta
		if m.del.shown > m.del.freed {
			m.del.shown = m.del.freed
		}
	}
}

// setMessage 在右侧面板显示一条临时提示。
func (m *Model) setMessage(text string) {
	m.message = text
	m.messageTTL = 3 * ui.FrameRate
}

// handleEvent 处理扫描与卸载事件。
func (m *Model) handleEvent(ev core.Event) (ui.Screen, tea.Cmd) {
	switch ev := ev.(type) {
	case core.ScanStarted:
		m.items = ev.Items
		m.scanTotal = ev.Total
		m.scanDone = 0
		m.roots = ev.Roots
		m.loaded = true
		if len(m.items) == 0 {
			m.phase = phaseIdle
		}
		m.rebuild()

	case core.SoftwareMeasured:
		m.applyMeasured(ev)
		m.rebuild()

	case core.ScanError:
		m.setMessage(fmt.Sprintf(m.txt.ScanFailed, ev.Err.Error()))

	case core.ScanCompleted:
		m.applyCompleted(ev)
		return m, nil // 扫描事件流已结束，不再等待

	case core.DeleteStarted:
		m.del.total = ev.Total

	case core.ItemDeleted:
		m.del.done = ev.Done
		if idx := ev.Done - 1; idx >= 0 && idx < len(m.del.items) {
			m.del.items[idx].err = ev.Err
		}
		if ev.Err == nil {
			m.del.freed += ev.Freed
		}

	case core.DeleteCompleted:
		m.del.result = ev.Result
		m.finishDelete()
		return m, nil
	}
	return m, waitEvent(m.events)
}

// applyMeasured 把一个目录的统计结果回填到列表中。
func (m *Model) applyMeasured(ev core.SoftwareMeasured) {
	i, ok := m.index[ev.Name]
	if !ok {
		return
	}
	installs := m.items[i].Installs
	for k := range installs {
		if installs[k].Root == ev.Install.Root {
			installs[k] = ev.Install
			break
		}
	}
	m.scanDone = ev.Done
	m.scanBytes += ev.Install.Size
}

// applyCompleted 用最终统计结果替换列表，并保留用户已做的选择。
func (m *Model) applyCompleted(ev core.ScanCompleted) {
	selected := make(map[string]bool, len(m.items))
	for _, it := range m.items {
		if it.Selected {
			selected[it.Name] = true
		}
	}
	items := ev.Items
	for i := range items {
		if selected[items[i].Name] {
			items[i].Selected = true
		}
	}
	m.items = items
	m.roots = ev.Roots
	m.phase = phaseIdle
	if m.autoSort {
		m.sortMode = core.SortBySize
	}
	m.rebuild()
	if m.cancel != nil {
		m.cancel()
	}
}

// handleMouse 处理鼠标：滚轮滚动，左键点击命中可交互区域。
func (m *Model) handleMouse(msg tea.MouseMsg) (ui.Screen, tea.Cmd) {
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		if m.filtering {
			return m, nil
		}
		if msg.Button == tea.MouseButtonWheelUp {
			m.move(-1)
		} else {
			m.move(1)
		}
		return m, nil
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}
	for _, r := range m.hits {
		if msg.X >= r.x && msg.X < r.x+r.w && msg.Y >= r.y && msg.Y < r.y+r.h {
			return m.activateHit(r)
		}
	}
	return m, nil
}

// activateHit 执行点击区域对应的操作。
func (m *Model) activateHit(r hitRegion) (ui.Screen, tea.Cmd) {
	switch r.kind {
	case hitLang:
		if r.next {
			m.setLang(m.lang.Next())
		} else {
			m.setLang(r.lang)
		}
		return m, nil

	case hitRow:
		// 单击软件项：光标移过去并切换选中状态。
		m.cursor = m.top + r.row
		m.clamp()
		m.toggle()
		return m, nil

	case hitBinding:
		return m.handleKey(keyMsgFor(r.click))

	case hitLink:
		if r.link == "" {
			return m, nil
		}
		url := r.link
		return m, func() tea.Msg {
			if err := platform.OpenURL(url); err != nil {
				slog.Warn("打开链接失败", "url", url, "err", err)
			}
			return nil
		}

	case hitTitle:
		m.goHome()
		return m, nil
	}
	return m, nil
}

// goHome 回到主界面：关闭帮助、退出待确认、结束结果展示。
// 卸载进行中不打断，避免误点导致状态错乱。
func (m *Model) goHome() {
	if m.phase == phaseDelete {
		return
	}
	m.showHelp = false
	m.filtering = false
	m.message = ""
	m.messageTTL = 0
	switch m.phase {
	case phaseConfirm:
		m.cancelConfirm(m.txt.Canceled)
	case phaseDone:
		m.phase = phaseIdle
	}
}

// keyMsgFor 把 Binding.Click 转换成对应的按键消息。
func keyMsgFor(click string) tea.KeyMsg {
	switch click {
	case "":
		return tea.KeyMsg{}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "any":
		// 任意一个不承担功能的键，用于「其它键取消」。
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(click)}
	}
}

// handleKey 处理按键。
func (m *Model) handleKey(msg tea.KeyMsg) (ui.Screen, tea.Cmd) {
	key := msg.String()

	// 待确认阶段：只有确认键生效，其它任何键都视为取消。
	if m.phase == phaseConfirm {
		if len(key) == 1 && strings.ContainsRune(confirmKeys, rune(key[0])) {
			return m.startDelete()
		}
		m.cancelConfirm(m.txt.Canceled)
		return m, nil
	}

	if m.phase == phaseDelete {
		return m, nil // 卸载过程中不接受其它操作
	}

	if m.filtering {
		return m.handleFilterKey(msg), nil
	}

	switch key {
	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case "pgup":
		m.move(-m.viewportRows())
	case "pgdown":
		m.move(m.viewportRows())
	case "home":
		m.cursor = 0
		m.clamp()
	case "end":
		m.cursor = len(m.view) - 1
		m.clamp()
	case " ":
		m.toggle()
	case "a", "A":
		m.toggleAll()
	case "i", "I":
		m.invert()
	case "s", "S":
		m.autoSort = false
		m.sortMode = m.sortMode.Next()
		m.rebuild()
	case "/":
		m.filtering = true
	case "?":
		m.showHelp = !m.showHelp
	case "l", "L":
		m.setLang(m.lang.Next())
	case "r", "R":
		return m, ui.Rescan()
	case "d", "D":
		m.requestDelete()
	case "q", "Q", "esc":
		return m, ui.Quit()
	}
	return m, nil
}

// handleFilterKey 处理过滤输入。
func (m *Model) handleFilterKey(msg tea.KeyMsg) *Model {
	switch msg.Type {
	case tea.KeyEsc:
		m.filtering = false
		m.filter = ""
		m.rebuild()
	case tea.KeyEnter:
		m.filtering = false
		m.rebuild()
	case tea.KeyBackspace:
		if len(m.filter) > 0 {
			runes := []rune(m.filter)
			m.filter = string(runes[:len(runes)-1])
			m.rebuild()
		}
	case tea.KeyRunes, tea.KeySpace:
		m.filter += string(msg.Runes)
		if msg.Type == tea.KeySpace {
			m.filter += " "
		}
		m.rebuild()
	}
	return m
}

// requestDelete 进入待确认状态。
// 先把当前已选中的软件记录下来，取消时会恢复——避免"按 D 自动选中光标项、取消后又忘记"造成的状态错觉。
func (m *Model) requestDelete() {
	if len(m.view) == 0 {
		return
	}
	m.savedSelected = make(map[string]bool, len(m.items))
	for i := range m.items {
		if m.items[i].Selected {
			m.savedSelected[m.items[i].Name] = true
		}
	}
	if core.SelectedCount(m.items) == 0 {
		m.toggle() // 没有选中项时，默认卸载光标所在的软件
	}
	m.phase = phaseConfirm
	m.confirmTTL = confirmFrames
	m.message = ""
	m.messageTTL = 0
}

// cancelConfirm 取消待确认状态，恢复到进入前的选中情况。
func (m *Model) cancelConfirm(msg string) {
	if m.savedSelected != nil {
		for i := range m.items {
			m.items[i].Selected = m.savedSelected[m.items[i].Name]
		}
		m.savedSelected = nil
	}
	m.phase = phaseIdle
	m.setMessage(msg)
}

// startDelete 真正开始卸载。
func (m *Model) startDelete() (ui.Screen, tea.Cmd) {
	tasks := make([]taskItem, 0, 8)
	for _, sw := range m.items {
		if !sw.Selected {
			continue
		}
		for _, in := range sw.Installs {
			tasks = append(tasks, taskItem{name: sw.Name, path: in.Path, size: in.Size})
		}
	}
	if len(tasks) == 0 {
		m.phase = phaseIdle
		return m, nil
	}

	m.del = deleteState{items: tasks, total: len(tasks)}
	m.phase = phaseDelete
	m.confirmTTL = 0
	m.savedSelected = nil
	m.message = ""
	m.messageTTL = 0

	ctx, cancel := context.WithCancel(context.Background())
	m.delCancel = cancel
	remover := &core.Remover{DryRun: m.cfg.DryRun, Prune: m.cfg.Prune}
	m.events = remover.Stream(ctx, m.items, m.roots)
	return m, waitEvent(m.events)
}

// finishDelete 卸载结束：移除已删除的软件，保留失败项。
func (m *Model) finishDelete() {
	res := m.del.result
	res.Freed = m.del.freed
	if res.Deleted == 0 {
		res.Deleted = m.del.done
	}
	m.del.result = res

	if !m.cfg.DryRun && len(res.Removed) > 0 {
		removed := make(map[string]bool, len(res.Removed))
		for _, name := range res.Removed {
			removed[name] = true
		}
		kept := m.items[:0]
		for _, it := range m.items {
			if removed[it.Name] {
				continue
			}
			it.Selected = false
			kept = append(kept, it)
		}
		m.items = kept
	}
	for i := range m.items {
		m.items[i].Selected = false
	}

	if m.delCancel != nil {
		m.delCancel()
	}
	m.phase = phaseDone
	m.rebuild()
}

/* ---------------------------------- 列表操作 --------------------------------- */

func (m *Model) move(delta int) {
	m.cursor += delta
	m.clamp()
}

func (m *Model) clamp() {
	if len(m.view) == 0 {
		m.cursor = 0
		m.top = 0
		return
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.view) {
		m.cursor = len(m.view) - 1
	}
	rows := m.viewportRows()
	if m.cursor < m.top {
		m.top = m.cursor
	}
	if m.cursor >= m.top+rows {
		m.top = m.cursor - rows + 1
	}
	if m.top > len(m.view)-rows {
		m.top = len(m.view) - rows
	}
	if m.top < 0 {
		m.top = 0
	}
}

func (m *Model) toggle() {
	if len(m.view) == 0 {
		return
	}
	idx := m.view[m.cursor]
	next := !m.items[idx].Selected
	for i := range m.items {
		if m.items[i].Name == m.items[idx].Name {
			m.items[i].Selected = next
		}
	}
}

func (m *Model) toggleAll() {
	all := true
	for _, i := range m.view {
		if !m.items[i].Selected {
			all = false
			break
		}
	}
	for _, i := range m.view {
		m.items[i].Selected = !all
	}
}

func (m *Model) invert() {
	for _, i := range m.view {
		m.items[i].Selected = !m.items[i].Selected
	}
}

// rebuild 重新排序、应用过滤并重建索引。
func (m *Model) rebuild() {
	mode := m.sortMode
	// 统计尚未完成时占用都是 0，先按名称排序，避免列表乱跳。
	if m.phase == phaseScan && m.autoSort {
		mode = core.SortByName
	}
	core.Sort(m.items, mode)

	m.view = m.view[:0]
	m.index = make(map[string]int, len(m.items))
	q := strings.ToLower(strings.TrimSpace(m.filter))
	for i := range m.items {
		m.index[m.items[i].Name] = i
		if q == "" || strings.Contains(strings.ToLower(m.items[i].Name), q) {
			m.view = append(m.view, i)
		}
	}
	m.clamp()
}

// viewportRows 返回列表可容纳的数据行数（已扣除表头）。
func (m *Model) viewportRows() int {
	rows := m.h - 8
	if rows < 3 {
		rows = 3
	}
	return rows
}

/* ----------------------------------- 渲染 ----------------------------------- */

// detailWidth 返回右侧任务面板的宽度；0 表示当前宽度下不显示面板。
func detailWidth(width int) int {
	switch {
	case width >= 120:
		return 40
	case width >= 100:
		return 36
	case width >= 78:
		return 30
	default:
		return 0
	}
}

// View 渲染整个界面，并顺带记录本帧的可点击区域。
func (m *Model) View() string {
	width, height := m.w, m.h
	if width < 40 {
		width = 40
	}
	if height < 10 {
		height = 10
	}

	icon := ascii.Spark(m.tick)
	rightText, headerHits := m.headerRight()
	rule := ascii.Rule(width, m.tick)
	header := components.Header(icon, m.txt.AppTitle, rightText, width, rule)

	bindings := m.bindings()
	// 版本号已移到顶部标题栏，底栏整行留给快捷键提示。
	left, spans := components.Help(bindings, m.tick, width)
	footer := components.Footer(left, "", width)

	bodyHeight := height - 5
	if bodyHeight < 5 {
		bodyHeight = 5
	}

	detailW := detailWidth(width)
	listW := width
	if detailW > 0 {
		listW = width - detailW - 1
	}

	var body string
	var back helpBack
	switch {
	case m.showHelp:
		lines, b := m.helpLines()
		back = b
		body = components.Box{Title: m.txt.TaskHelp, Width: width, Height: bodyHeight}.
			Render(strings.Join(lines, "\n"))
	case detailW > 0:
		body = lipgloss.JoinHorizontal(lipgloss.Top,
			components.Box{Title: m.listTitle(), Width: listW, Height: bodyHeight, Active: true}.
				Render(m.renderList(listW, bodyHeight)),
			" ",
			components.Box{Title: m.taskTitle(), Width: detailW, Height: bodyHeight}.
				Render(m.renderTask(detailW, bodyHeight)),
		)
	default:
		body = components.Box{Title: m.listTitle(), Width: width, Height: bodyHeight, Active: true}.
			Render(m.renderList(width, bodyHeight))
	}

	m.rebuildHits(hitContext{
		width:      width,
		height:     height,
		listW:      listW,
		bodyHeight: bodyHeight,
		iconW:      components.Width(icon),
		headW:      components.Width(rightText),
		headHits:   headerHits,
		spans:      spans,
		bindings:   bindings,
		showHelp:   m.showHelp,
		helpBack:   back,
	})

	// 用 Place 强制输出固定尺寸，避免 bubbletea 行级 diff 错位导致旧内容残留。
	view := header + "\n\n" + body + "\n\n" + footer
	if m.h > 0 && m.w > 0 {
		return lipgloss.Place(m.w, m.h, lipgloss.Left, lipgloss.Top, view)
	}
	return view
}

// hitContext 是重建可点击区域时需要知道的布局参数。
type hitContext struct {
	width, height int
	listW         int
	bodyHeight    int
	iconW, headW  int
	headHits      []headerHit
	spans         []components.Span
	bindings      []components.Binding
	showHelp      bool
	helpBack      helpBack
}

// 布局常量：标题 2 行 + 空行之后是内容区，最后一行是底栏。
const (
	bodyTop   = 3 // 内容区首行（0 标题、1 分隔线、2 空行）
	listHead  = 2 // 边框 1 行 + 列头 1 行
	boxInnerX = 2 // Box 内内容的起始列：左边框 1 列 + 内边距 1 列
)

// rebuildHits 依据当前帧的布局重建可点击区域。
func (m *Model) rebuildHits(ctx hitContext) {
	m.hits = m.hits[:0]

	// 顶部标题：任何情况下点一下都能回到主界面。
	m.hits = append(m.hits, hitRegion{
		kind: hitTitle, x: 0, y: 0,
		w: ctx.iconW + 1 + components.Width(m.txt.AppTitle), h: 1,
	})

	// 帮助页顶部的返回按钮（等同按下 ?）。
	if ctx.showHelp {
		m.hits = append(m.hits, hitRegion{
			kind:  hitBinding,
			x:     boxInnerX + ctx.helpBack.start,
			y:     bodyTop + 1 + ctx.helpBack.line,
			w:     ctx.helpBack.width,
			h:     1,
			click: "?",
		})
	}

	// 右上角徽章（第 0 行，右对齐）。标题太挤时 Header 会隐藏右侧，这里同步跳过。
	if leftW := ctx.iconW + 1 + components.Width(m.txt.AppTitle); ctx.width-leftW-1 >= 8 &&
		ctx.headW <= ctx.width-leftW-1 {
		start := ctx.width - ctx.headW
		for _, h := range ctx.headHits {
			m.hits = append(m.hits, hitRegion{
				kind: h.kind, x: start + h.start, y: 0, w: h.width, h: 1,
				lang: h.lang, next: h.isKey, link: h.link,
			})
		}
	}

	// 软件列表的每一行。
	if !m.showHelp && len(m.view) > 0 && ctx.bodyHeight > listHead {
		rows := ctx.bodyHeight - listHead
		for n := 0; n < rows; n++ {
			if m.top+n >= len(m.view) {
				break
			}
			m.hits = append(m.hits, hitRegion{
				kind: hitRow, x: 0, y: bodyTop + listHead + n,
				w: ctx.listW, h: 1, row: n,
			})
		}
	}

	// 底部按键提示（最后一个行）。
	footerY := ctx.height - 1
	for _, s := range ctx.spans {
		if s.Index >= len(ctx.bindings) || ctx.bindings[s.Index].Click == "" {
			continue
		}
		m.hits = append(m.hits, hitRegion{
			kind: hitBinding, x: s.Start, y: footerY,
			w: s.Width, h: 1, click: ctx.bindings[s.Index].Click,
		})
	}
}

func (m *Model) listTitle() string {
	count := core.HumanCount(len(m.view))
	title := fmt.Sprintf(m.txt.ListTitle, count)
	if m.filter != "" {
		title = fmt.Sprintf(m.txt.ListFiltered, count, m.filter)
	}
	if m.filtering {
		title += " ▌"
	}
	return title
}

func (m *Model) taskTitle() string {
	switch m.phase {
	case phaseScan:
		return m.txt.TaskScanning
	case phaseConfirm:
		return m.txt.TaskConfirm
	case phaseDelete:
		return m.txt.TaskRemoving
	case phaseDone:
		return m.txt.TaskResult
	default:
		return m.txt.TaskIdle
	}
}

// headerRight 渲染右上角内容：版本号 + GitHub 入口 + 语言切换提示。
// 形如：260910   GitHub   L 简体中文 | English
//
// 语言部分采用中性灰的「小徽章」样式：L 键提示与当前语言都带底色（当前语言底色略亮），
// 未选中的语言不带底色。同时返回各枚徽章的位置，供鼠标点击命中。
func (m *Model) headerRight() (string, []headerHit) {
	st := theme.S()
	hits := make([]headerHit, 0, len(i18n.Order)+2)

	var sb strings.Builder
	col := 0
	write := func(s string) int {
		w := components.Width(s)
		sb.WriteString(s)
		col += w
		return w
	}

	// 版本号（构建日期）：中性灰，与右侧其余元素同一调性，不抢注意力。
	write(st.Neutral.Render(version.String()) + "   ")

	// GitHub 入口：与未选中的语言同色，带下划线暗示可点击。
	gh := st.ChipOff.Underline(true).Render(m.txt.LinkGitHub)
	hits = append(hits, headerHit{start: col, width: components.Width(gh), kind: hitLink, link: githubURL})
	write(gh + "   ")

	key := st.Chip.Render(" L ")
	hits = append(hits, headerHit{start: col, width: components.Width(key), kind: hitLang, isKey: true})
	write(key)

	for i, l := range i18n.Order {
		if i > 0 {
			write(st.ChipOff.Render("|"))
		}
		style := st.ChipOff
		if l == m.lang {
			style = st.ChipOn
		}
		chip := style.Render(" " + l.Label() + " ")
		hits = append(hits, headerHit{start: col, width: components.Width(chip), kind: hitLang, lang: l})
		write(chip)
	}
	return sb.String(), hits
}

// sortName 返回当前排序方式在该语言下的名称。
func (m *Model) sortName() string {
	switch m.sortMode {
	case core.SortByName:
		return m.txt.SortName
	case core.SortByCreated:
		return m.txt.SortDate
	default:
		return m.txt.SortSize
	}
}

// bindings 返回当前状态下可用的快捷键，由 Help 按宽度取舍。
func (m *Model) bindings() []components.Binding {
	switch {
	case m.filtering:
		return []components.Binding{
			{Keys: []string{"enter"}, Desc: m.txt.KeyConfirmFilter, Click: "enter"},
			{Keys: []string{"esc"}, Desc: m.txt.KeyClear, Click: "esc"},
		}
	case m.phase == phaseConfirm:
		return []components.Binding{
			{Keys: []string{"D"}, Desc: fmt.Sprintf(m.txt.KeyConfirmN, core.HumanCount(core.SelectedCount(m.items))), Alert: true, Click: "d"},
			{Keys: []string{m.txt.KeyAnyKey}, Desc: m.txt.KeyCancel, Click: "any"},
		}
	case m.phase == phaseDelete:
		// 卸载中不接受操作，ctrl+c 也不适合用鼠标模拟。
		return []components.Binding{{Keys: []string{"ctrl+c"}, Desc: m.txt.KeyAbort}}
	}

	// 按重要程度排列，放不下时从末尾开始舍弃。
	// 方向键用鼠标拖动更自然，因此不做点击绑定。
	return []components.Binding{
		{Keys: []string{"↑", "↓"}, Desc: m.txt.KeyMove},
		{Keys: []string{"space"}, Desc: m.txt.KeySelect, Click: " "},
		{Keys: []string{"a"}, Desc: m.txt.KeySelectAll, Click: "a"},
		{Keys: []string{"d"}, Desc: m.txt.KeyUninstall, Click: "d"},
		{Keys: []string{"/"}, Desc: m.txt.KeySearch, Click: "/"},
		{Keys: []string{"s"}, Desc: fmt.Sprintf(m.txt.KeySort, m.sortName()), Click: "s"},
		{Keys: []string{"i"}, Desc: m.txt.KeyInvert, Click: "i"},
		{Keys: []string{"r"}, Desc: m.txt.KeyRescan, Click: "r"},
		{Keys: []string{"?"}, Desc: m.txt.KeyHelp, Click: "?"},
		{Keys: []string{"q"}, Desc: m.txt.KeyQuit, Click: "q"},
	}
}

// rowLayout 描述列表各列的宽度分配。
// 名称列可伸缩，占用与创建日期列为固定宽度，空间不足时依次缩短、舍弃日期列。
type rowLayout struct {
	nameW      int
	sizeW      int
	dateW      int
	dateLayout string
}

func (m *Model) rowLayout(width int) rowLayout {
	l := rowLayout{sizeW: 8, dateW: 10, dateLayout: "2006-01-02"}
	switch {
	case width >= 40:
	case width >= 32:
		l.dateW, l.dateLayout = 8, "06-01-02"
	default:
		l.dateW, l.dateLayout = 0, ""
	}
	reserved := 2 + 4 + 1 + l.sizeW // 光标 + 选择框 + 间隔 + 占用
	if l.dateW > 0 {
		reserved += 1 + l.dateW
	}
	l.nameW = width - reserved
	if l.nameW < 8 {
		l.nameW = 8
	}
	return l
}

// renderList 渲染左侧软件列表（首行为列头）。
func (m *Model) renderList(width, height int) string {
	innerW := width - 4
	rows := height - 2
	if rows < 1 {
		rows = 1
	}
	m.clamp()

	if len(m.view) == 0 {
		return m.emptyState(innerW)
	}

	lines := make([]string, 0, rows)
	lines = append(lines, m.renderHeader(innerW))
	for n := 0; n < rows-1; n++ {
		idx := m.top + n
		if idx >= len(m.view) {
			lines = append(lines, "")
			continue
		}
		lines = append(lines, m.renderRow(m.items[m.view[idx]], idx == m.cursor, innerW))
	}
	return strings.Join(lines, "\n")
}

// renderHeader 渲染列表的列头。
func (m *Model) renderHeader(width int) string {
	st := theme.S()
	l := m.rowLayout(width)
	faint := st.Faint.Render

	var sb strings.Builder
	sb.WriteString("    ") // 与光标 + 选择框对齐
	sb.WriteString(faint(components.Pad(m.txt.ColName, l.nameW)))
	sb.WriteString(" " + faint(components.PadLeft(m.txt.ColSize, l.sizeW)))
	if l.dateW > 0 {
		sb.WriteString(" " + faint(components.PadLeft(m.txt.ColCreated, l.dateW)))
	}
	return components.Fit(components.StripANSI(sb.String()), width)
}

// renderRow 渲染一行软件：选择框、名称、占用、创建日期。
func (m *Model) renderRow(sw core.Software, active bool, width int) string {
	st := theme.S()
	l := m.rowLayout(width)

	cursor := "  "
	if active {
		cursor = st.Accent.Render("▸ ")
	}
	box := st.Faint.Render("[ ]")
	if sw.Selected {
		box = st.Accent.Render("[") + st.Strong.Render("✓") + st.Accent.Render("]")
	}

	var sb strings.Builder
	sb.WriteString(cursor)
	sb.WriteString(box)
	sb.WriteString(" ")
	sb.WriteString(components.Pad(components.Truncate(sw.Name, l.nameW, "…"), l.nameW))
	sb.WriteString(" ")
	if m.measured(sw) {
		sb.WriteString(components.PadLeft(core.HumanSize(sw.TotalSize()), l.sizeW))
	} else {
		sb.WriteString(components.PadLeft(m.txt.Calculating, l.sizeW))
	}
	if l.dateW > 0 {
		sb.WriteString(" " + m.dateText(sw.Created(), l))
	}

	plain := components.Fit(components.StripANSI(sb.String()), width)
	switch {
	case m.phase == phaseConfirm && sw.Selected:
		// 光带缓缓扫过，表示「这一条正等着你确认」。
		return ascii.ScanHighlight(plain, m.tick)
	case active:
		return st.RowSel.Render(plain)
	default:
		return sb.String()
	}
}

// measured 判断该软件的占用是否已统计完成。
func (m *Model) measured(sw core.Software) bool {
	if m.phase != phaseScan {
		return true
	}
	for _, in := range sw.Installs {
		if in.Size == 0 && in.Files == 0 && in.Dirs == 0 {
			return false
		}
	}
	return true
}

// dateText 按布局渲染一个日期列。
func (m *Model) dateText(t time.Time, l rowLayout) string {
	st := theme.S()
	if t.IsZero() {
		return st.Faint.Render(components.PadLeft("-", l.dateW))
	}
	return st.Muted.Render(components.PadLeft(t.Format(l.dateLayout), l.dateW))
}

// emptyState 渲染列表的空状态，只保留一句说明。
func (m *Model) emptyState(width int) string {
	st := theme.S()
	text := m.txt.NoApps
	switch {
	case !m.loaded:
		text = m.txt.Loading
	case len(m.items) == 0 && m.phase == phaseDone:
		text = m.txt.AllRemoved
	case len(m.items) > 0:
		text = fmt.Sprintf(m.txt.NoMatch, m.filter)
	}
	return "\n\n" + components.Center(st.Subtle.Render(text), width)
}

// renderTask 渲染右侧任务面板。
// 只保留当前阶段真正有用的信息：操作提示交给底部状态栏，动画仅用于表达「进行中」。
func (m *Model) renderTask(width, height int) string {
	inner := width - 4
	rows := height - 2
	st := theme.S()

	lines := make([]string, 0, rows)
	add := func(s string) { lines = append(lines, s) }

	// 临时提示（例如「已取消卸载」）占用面板几秒，随后自动让位给正常内容。
	if m.message != "" {
		add(" " + st.Subtle.Render(components.Truncate(m.message, inner-2, "…")))
		return strings.Join(lines, "\n")
	}

	switch m.phase {
	case phaseScan:
		ratio := 0.0
		if m.scanTotal > 0 {
			ratio = float64(m.scanDone) / float64(m.scanTotal)
		}
		add(" " + ascii.Spinner(m.tick) + " " + st.Strong.Render(m.txt.ScanningUsage))
		add("")
		add(" " + components.Bar(inner-2, ratio, m.tick))
		add(" " + st.Muted.Render(fmt.Sprintf(m.txt.FolderProgress,
			core.HumanCount(m.scanDone), core.HumanCount(m.scanTotal))))
		add(" " + st.Faint.Render(fmt.Sprintf(m.txt.FolderMeasured, core.HumanSize(m.scanBytes))))

	case phaseConfirm:
		// 配色收敛成三层，避免红 / 白 / 粉 / 橙四种颜色并排显得杂乱：
		// 红（状态警示）→ 亮粉白（要卸的东西，作为一个整体）→ 暗灰（辅助的倒计时）。
		add(" " + st.Danger.Render(m.txt.ConfirmPending))
		add("")
		add(" " + st.Strong.Render(fmt.Sprintf(m.txt.AppCount, core.HumanCount(core.SelectedCount(m.items)))))
		add(" " + st.Strong.Render(fmt.Sprintf(m.txt.WillFree, core.HumanSize(core.SelectedSize(m.items)))))
		add("")
		add(" " + st.Faint.Render(fmt.Sprintf(m.txt.AutoCancelIn,
			strconv.Itoa(m.confirmTTL/ui.FrameRate+1))))

	case phaseDelete:
		ratio := 0.0
		if m.del.total > 0 {
			ratio = float64(m.del.done) / float64(m.del.total)
		}
		verb := m.txt.Removing
		if m.cfg.DryRun {
			verb = m.txt.DryRunning
		}
		add(" " + ascii.Spinner(m.tick) + " " + st.Strong.Render(verb))
		add("")
		add(" " + components.Bar(inner-2, ratio, m.tick))
		add(" " + st.Muted.Render(fmt.Sprintf(m.txt.FolderProgress,
			core.HumanCount(m.del.done), core.HumanCount(m.del.total))))
		add(" " + st.Faint.Render(fmt.Sprintf(m.txt.Freed, core.HumanSize(m.del.shown))))
		add("")
		lines = append(lines, m.taskLines(inner, rows-len(lines))...)

	case phaseDone:
		res := m.del.result
		headline := m.txt.ResultDone
		if m.cfg.DryRun {
			headline = m.txt.ResultDoneDry
		}
		if len(res.Failures) > 0 {
			headline = m.txt.ResultWithErrors
		}
		add(" " + st.OK.Render(headline))
		add("")
		add(" " + st.Muted.Render(fmt.Sprintf(m.txt.RemovedDirs, core.HumanCount(res.Deleted))))
		add(" " + st.Muted.Render(fmt.Sprintf(m.txt.Freed, core.HumanSize(res.Freed))))
		add(" " + st.Muted.Render(fmt.Sprintf(m.txt.Elapsed, res.Elapsed.Round(time.Millisecond).String())))
		if len(res.Pruned) > 0 {
			add(" " + st.Faint.Render(fmt.Sprintf(m.txt.PrunedDirs, core.HumanCount(len(res.Pruned)))))
		}
		if len(res.Failures) > 0 {
			add("")
			add(" " + st.Danger.Render(fmt.Sprintf(m.txt.FailedCount, core.HumanCount(len(res.Failures)))))
			for i, f := range res.Failures {
				if i >= 4 {
					add(" " + st.Faint.Render(fmt.Sprintf(m.txt.MoreItems, core.HumanCount(len(res.Failures)-i))))
					break
				}
				add(" " + st.Base.Render(components.Truncate(f.Name, inner-4, "…")))
				add("   " + st.Faint.Render(components.Truncate(f.Err.Error(), inner-5, "…")))
			}
		}

	default:
		// 未选择软件时面板留白；有选择时只给出这次卸载的关键数字。
		count := core.SelectedCount(m.items)
		if count == 0 {
			break
		}
		add(" " + st.Strong.Render(fmt.Sprintf(m.txt.AppCount, core.HumanCount(count))))
		add(" " + st.Subtle.Render(fmt.Sprintf(m.txt.WillFree, core.HumanSize(core.SelectedSize(m.items)))))
		add("")
		add(" " + st.Faint.Render(m.txt.PressToRemove))
	}

	if len(lines) > rows {
		lines = lines[:rows]
	}
	return strings.Join(lines, "\n")
}

// taskLines 渲染卸载任务的分项列表。
func (m *Model) taskLines(inner, rows int) []string {
	st := theme.S()
	if rows <= 0 {
		return nil
	}
	lines := make([]string, 0, rows)
	start := 0
	if len(m.del.items) > rows {
		start = m.del.done - rows/2
		if start < 0 {
			start = 0
		}
		if start > len(m.del.items)-rows {
			start = len(m.del.items) - rows
		}
	}
	for n := 0; n < rows; n++ {
		idx := start + n
		if idx >= len(m.del.items) {
			break
		}
		it := m.del.items[idx]
		var icon string
		switch {
		case idx < m.del.done && it.err != nil:
			icon = st.Danger.Render("✗")
		case idx < m.del.done:
			icon = st.OK.Render("✓")
		case idx == m.del.done:
			icon = ascii.Spinner(m.tick)
		default:
			icon = st.Faint.Render("·")
		}
		nameW := inner - 12
		if nameW < 6 {
			nameW = 6
		}
		body := components.Pad(components.Truncate(it.name, nameW, "…"), nameW) + " " +
			components.PadLeft(core.HumanSize(it.size), 8)
		// 正在处理的那条用擦除动画表现。
		if idx == m.del.done && m.cfg.Animate {
			lines = append(lines, " "+icon+" "+ascii.Dissolve(body, float64(m.tick%24)/24))
			continue
		}
		lines = append(lines, " "+icon+" "+st.Base.Render(body))
	}
	return lines
}

// helpBack 是帮助页「返回」按钮在内容区中的位置（行号相对内容区首行）。
type helpBack struct {
	line, start, width int
}

// helpLines 渲染帮助页内容，并给出返回按钮的位置。
// 返回按钮放在最上方：帮助页最容易让人迷路，出口要给得显眼。
func (m *Model) helpLines() ([]string, helpBack) {
	st := theme.S()
	entries := m.txt.HelpEntries

	lines := make([]string, 0, len(entries)+6)
	lines = append(lines, "")

	// 返回按钮借用右上角语言切换器的徽章配色（灰底 + 亮字），
	// 与下方不带底色的快捷键列表明显区分开，一眼就能认出这是可点的。
	key := " ? "
	label := " ← " + m.txt.HelpBack + " "
	lines = append(lines, "   "+st.Chip.Render(key)+st.ChipOn.Render(label))
	back := helpBack{
		line:  len(lines) - 1,
		start: 3,
		width: 3 + components.Width(key) + components.Width(label),
	}
	lines = append(lines, "")

	for i, e := range entries {
		// 按键用普通白字，不加底色：帮助页是逐条阅读的，块状高亮只会显得吵。
		lines = append(lines, "  "+st.Base.Render(components.Pad(e[0], 14))+"  "+st.KeyDesc.Render(e[1]))
		if i == 0 {
			// 把「关闭帮助」与其余快捷键隔开，避免看成一整片。
			lines = append(lines, "")
		}
	}
	lines = append(lines, "")
	lines = append(lines, st.Faint.Render("  "+m.txt.HelpCancelNote))
	return lines, back
}
