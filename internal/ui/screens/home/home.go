// Package home 是程序唯一的主屏幕：左侧软件列表，右侧卸载任务进度。
//
// 进入后立即枚举出全部软件（几乎无耗时），占用统计在后台异步完成并逐个回填；
// 卸载通过「按 D 进入待确认 → 再按 D 确认」的两段式交互完成，
// 待确认期间条目与按键指示会闪烁，超时或按下其它键均视为取消。
package home

import (
	"context"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/unieditdept/ued-uninstaller/internal/config"
	"github.com/unieditdept/ued-uninstaller/internal/core"
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

// Model 是主屏幕。
type Model struct {
	cfg   config.Config
	roots []core.Root
	items []core.Software

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
	return &Model{
		cfg:      cfg,
		roots:    core.ResolveRoots(cfg.Namespace),
		index:    make(map[string]int),
		sortMode: core.SortBySize,
		autoSort: true,
		phase:    phaseScan,
		w:        width,
		h:        height,
	}
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
		if m.filtering || m.phase == phaseConfirm || m.phase == phaseDelete {
			return m, nil
		}
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.move(-1)
		case tea.MouseButtonWheelDown:
			m.move(1)
		}
		return m, nil

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
			m.cancelConfirm("已取消：超时未确认")
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
		m.setMessage(ev.Err.Error())

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

// handleKey 处理按键。
func (m *Model) handleKey(msg tea.KeyMsg) (ui.Screen, tea.Cmd) {
	key := msg.String()

	// 待确认阶段：只有确认键生效，其它任何键都视为取消。
	if m.phase == phaseConfirm {
		if len(key) == 1 && strings.ContainsRune(confirmKeys, rune(key[0])) {
			return m.startDelete()
		}
		m.cancelConfirm("已取消卸载")
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

// View 渲染整个界面。
func (m *Model) View() string {
	width, height := m.w, m.h
	if width < 40 {
		width = 40
	}
	if height < 10 {
		height = 10
	}

	rule := ascii.Rule(width, m.tick)
	header := components.Header("UNIEDITDEPT 卸载程序", "", width, rule)
	right := m.footerRight()
	footer := components.Footer(m.footerLeft(width, components.Width(right)), right, width)

	bodyHeight := height - 5
	if bodyHeight < 5 {
		bodyHeight = 5
	}

	var body string
	switch {
	case m.showHelp:
		body = components.Box{Title: "快捷键", Width: width, Height: bodyHeight}.Render(strings.Join(m.helpLines(), "\n"))
	case width >= 78:
		detailW := 30
		if width >= 100 {
			detailW = 36
		}
		if width >= 120 {
			detailW = 40
		}
		listW := width - detailW - 1
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

	// 用 Place 强制输出固定尺寸，避免 bubbletea 行级 diff 错位导致旧内容残留。
	view := header + "\n\n" + body + "\n\n" + footer
	if m.h > 0 && m.w > 0 {
		return lipgloss.Place(m.w, m.h, lipgloss.Left, lipgloss.Top, view)
	}
	return view
}

func (m *Model) listTitle() string {
	title := "软件（" + core.HumanCount(len(m.view)) + "）"
	if m.filter != "" {
		title += " · 过滤 " + m.filter
	}
	if m.filtering {
		title += " ▌"
	}
	return title
}

func (m *Model) taskTitle() string {
	switch m.phase {
	case phaseScan:
		return "统计进度"
	case phaseConfirm:
		return "确认卸载"
	case phaseDelete:
		return "卸载进度"
	case phaseDone:
		return "卸载结果"
	default:
		return "卸载任务"
	}
}

// footerRight 固定显示版本号（构建日期）。
// 已选数量与体积由右侧任务面板承担，不必在底部重复。
func (m *Model) footerRight() string { return version.String() }

// footerLeft 渲染底部左侧内容。reserved 是右侧统计信息已占用的宽度。
func (m *Model) footerLeft(width, reserved int) string {
	return components.Help(m.bindings(), m.tick, width-reserved-2)
}

// bindings 返回当前状态下可用的快捷键，由 Help 按宽度取舍。
func (m *Model) bindings() []components.Binding {
	switch {
	case m.filtering:
		return []components.Binding{
			{Keys: []string{"enter"}, Desc: "确认过滤"},
			{Keys: []string{"esc"}, Desc: "清空"},
		}
	case m.phase == phaseConfirm:
		return []components.Binding{
			{Keys: []string{"D"}, Desc: "确认卸载 " + core.HumanCount(core.SelectedCount(m.items)) + " 项", Alert: true},
			{Keys: []string{"其它键"}, Desc: "取消"},
		}
	case m.phase == phaseDelete:
		return []components.Binding{{Keys: []string{"ctrl+c"}, Desc: "中断"}}
	case m.phase == phaseDone:
		// 卸载已完成，列表可能已空，只留仍然有意义的两个键。
		return []components.Binding{
			{Keys: []string{"r"}, Desc: "重扫"},
			{Keys: []string{"q"}, Desc: "退出"},
		}
	}

	// 按重要程度排列，放不下时从末尾开始舍弃。
	return []components.Binding{
		{Keys: []string{"↑", "↓"}, Desc: "移动"},
		{Keys: []string{"space"}, Desc: "选择"},
		{Keys: []string{"a"}, Desc: "全选"},
		{Keys: []string{"d"}, Desc: "卸载"},
		{Keys: []string{"/"}, Desc: "搜索"},
		{Keys: []string{"s"}, Desc: "排序:" + m.sortMode.String()},
		{Keys: []string{"i"}, Desc: "反选"},
		{Keys: []string{"r"}, Desc: "重扫"},
		{Keys: []string{"?"}, Desc: "帮助"},
		{Keys: []string{"q"}, Desc: "退出"},
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
	sb.WriteString(faint(components.Pad("名称", l.nameW)))
	sb.WriteString(" " + faint(components.PadLeft("占用", l.sizeW)))
	if l.dateW > 0 {
		sb.WriteString(" " + faint(components.PadLeft("创建日期", l.dateW)))
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
		sb.WriteString(components.PadLeft("计算中", l.sizeW))
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
	text := "没有发现任何软件"
	switch {
	case !m.loaded:
		text = "正在读取软件列表…"
	case len(m.items) == 0 && m.phase == phaseDone:
		text = "已全部卸载"
	case len(m.items) > 0:
		text = "没有匹配 “" + m.filter + "” 的软件"
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
		add(" " + ascii.Spinner(m.tick) + " " + st.Strong.Render("正在统计占用"))
		add("")
		add(" " + components.Bar(inner-2, ratio, m.tick))
		add(" " + st.Muted.Render(core.HumanCount(m.scanDone)+" / "+core.HumanCount(m.scanTotal)+" 个目录"))
		add(" " + st.Faint.Render("已统计 "+core.HumanSize(m.scanBytes)))

	case phaseConfirm:
		add(" " + st.Danger.Render("待确认"))
		add("")
		add(" " + st.Base.Render(core.HumanCount(core.SelectedCount(m.items))+" 个软件"))
		add(" " + st.Muted.Render("预计释放 ") + st.Subtle.Render(core.HumanSize(core.SelectedSize(m.items))))
		add("")
		add(" " + st.Warn.Render(strconv.Itoa(m.confirmTTL/ui.FrameRate+1)+" 秒后自动取消"))

	case phaseDelete:
		ratio := 0.0
		if m.del.total > 0 {
			ratio = float64(m.del.done) / float64(m.del.total)
		}
		verb := "正在卸载"
		if m.cfg.DryRun {
			verb = "正在演练"
		}
		add(" " + ascii.Spinner(m.tick) + " " + st.Strong.Render(verb))
		add("")
		add(" " + components.Bar(inner-2, ratio, m.tick))
		add(" " + st.Muted.Render(core.HumanCount(m.del.done)+" / "+core.HumanCount(m.del.total)+" 个目录"))
		add(" " + st.Faint.Render("已释放 "+core.HumanSize(m.del.shown)))
		add("")
		lines = append(lines, m.taskLines(inner, rows-len(lines))...)

	case phaseDone:
		res := m.del.result
		headline := "✓ 卸载完成"
		if m.cfg.DryRun {
			headline = "✓ 演练完成"
		}
		if len(res.Failures) > 0 {
			headline = "! 存在失败项"
		}
		add(" " + st.OK.Render(headline))
		add("")
		add(" " + st.Muted.Render("删除 ") + st.Base.Render(core.HumanCount(res.Deleted)+" 个目录"))
		add(" " + st.Muted.Render("释放 ") + st.Subtle.Render(core.HumanSize(res.Freed)))
		add(" " + st.Muted.Render("耗时 ") + st.Base.Render(res.Elapsed.Round(time.Millisecond).String()))
		if len(res.Pruned) > 0 {
			add(" " + st.Faint.Render("清理空目录 "+core.HumanCount(len(res.Pruned))+" 个"))
		}
		if len(res.Failures) > 0 {
			add("")
			add(" " + st.Danger.Render("失败 "+core.HumanCount(len(res.Failures))+" 项"))
			for i, f := range res.Failures {
				if i >= 4 {
					add(" " + st.Faint.Render("… 另有 "+core.HumanCount(len(res.Failures)-i)+" 项"))
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
		add(" " + st.Strong.Render(core.HumanCount(count)+" 个软件"))
		add(" " + st.Muted.Render("预计释放 ") + st.Subtle.Render(core.HumanSize(core.SelectedSize(m.items))))
		add("")
		add(" " + st.Faint.Render("按 D 卸载"))
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

func (m *Model) helpLines() []string {
	st := theme.S()
	groups := [][2]string{
		{"↑ / ↓ · k / j", "上下移动光标"},
		{"PgUp / PgDn", "翻页"},
		{"Home / End", "跳到首项 / 末项"},
		{"Space", "选择或取消选择当前软件"},
		{"A", "全选 / 取消全选当前列表"},
		{"I", "反选当前列表"},
		{"S", "切换排序（占用 / 名称 / 日期）"},
		{"/", "按名称搜索，Esc 清空"},
		{"D", "卸载：进入待确认，再按一次 D 执行"},
		{"R", "重新扫描"},
		{"?", "显示或隐藏本帮助"},
		{"Q / Ctrl+C", "退出程序"},
	}
	lines := make([]string, 0, len(groups)+4)
	lines = append(lines, st.Subtle.Render("创建日期为三个位置中最早创建的那个目录"))
	lines = append(lines, "")
	for _, g := range groups {
		lines = append(lines, "  "+st.Key.Render(g[0])+"  "+st.KeyDesc.Render(g[1]))
	}
	lines = append(lines, "")
	lines = append(lines, st.Faint.Render("  待确认状态下按其它任意键即取消，超时也会自动取消。"))
	return lines
}
