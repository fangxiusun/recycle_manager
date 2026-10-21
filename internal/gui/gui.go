// Package gui 是 rbm 的图形外壳：参数区 + 分区表 + 明细表 + 三层删除闸门 + 日志。
// 分区、编号、保险丝、账本这些判定全在 internal/engine，这里只负责展示与确认，
// 因此 GUI 和命令行看到的是同一份快照、同一套编号。
package gui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"golang.org/x/sys/windows"

	"rbm/internal/engine"
	"rbm/internal/i18n"
	"rbm/internal/meta"
	"rbm/internal/parts"
	"rbm/internal/store"
	"rbm/internal/tty"
)

// x/sys 那版没导出 FreeConsole，直接按名字取 kernel32 的入口。
var procFreeConsole = syscall.NewLazyDLL("kernel32.dll").NewProc("FreeConsole")

// 数字参数默认值，与命令行 / Python 版一致。
const (
	defPrefixLen  = 3
	defMaxBuckets = 12
	defMaxParts   = 120
	defMinCount   = 5
	defMaxScan    = 50000
	// 删除进度回调的节奏：每这么多个条目刷一次进度条。
	progressEvery = 100
)

var (
	byValues    = []string{"dir", "prefix", "ext", "token"}
	levelValues = []string{"1", "2", "3", "full"}
	// bucketValues 是下拉每一项对应的写法，顺序和 bucketDisplay 一一对应。最后一项是「自定义」，
	// 它不给固定写法，而是把手填框里的文本原样交给 parts.ParseSpec（所以这里放空串占位）。
	// 这一组是 parts 认的 token，不是给人看的话，所以不跟语言变。
	bucketValues = []string{"", "gap:15m", "gap:1h", "gap:1d",
		"15m", "30m", "hourly", "daily", "3d", "weekly", "14d",
		"monthly", "3mo", "6mo", "auto", ""}
)

// 下面三组是下拉框里给人看的显示文本，取 i18n 目录，所以按语言给、切换语言时整组换掉。
// 顺序必须和上面各自的 values 一致（gui_test 里按长度对齐钉住）。

func byDisplay() []string {
	return []string{
		i18n.T("gui.by.dir"), i18n.T("gui.by.prefix"), i18n.T("gui.by.ext"), i18n.T("gui.by.token"),
	}
}

func levelDisplay() []string {
	return []string{
		i18n.T("gui.level.1"), i18n.T("gui.level.2"), i18n.T("gui.level.3"), i18n.T("gui.level.full"),
	}
}

func bucketDisplay() []string {
	return []string{
		i18n.T("gui.bucket.auto"),
		i18n.T("gui.bucket.gap15m"),
		i18n.T("gui.bucket.gap1h"),
		i18n.T("gui.bucket.gap1d"),
		i18n.T("gui.bucket.w15m"),
		i18n.T("gui.bucket.w30m"),
		i18n.T("gui.bucket.hourly"),
		i18n.T("gui.bucket.daily"),
		i18n.T("gui.bucket.w3d"),
		i18n.T("gui.bucket.weekly"),
		i18n.T("gui.bucket.w14d"),
		i18n.T("gui.bucket.monthly"),
		i18n.T("gui.bucket.w3mo"),
		i18n.T("gui.bucket.w6mo"),
		i18n.T("gui.bucket.pick-auto"),
		i18n.T("gui.bucket.custom"),
	}
}

// bucketIsCustom 说清下拉第 idx 项是不是「自定义」那一项（最后一项）。
// 按 bucketValues 判，不按显示文本：语言换了，档位个数不会换。
func bucketIsCustom(idx int) bool { return idx == len(bucketValues)-1 }

// 日志行的级别前缀：让「扫完了」「拦下了」「报错了」在纯文本日志里也能一眼分开。
const (
	logInfo = iota
	logOK
	logWarn
	logErr
)

// deleteButtonText 把「当前要删的对象」实时写在按钮上：没勾选时是个明显的禁用态文案，
// 勾了就把分区数和条目数报出来（评价 P0：永久删除按钮要够危险）。
func deleteButtonText(parts, items int) string {
	if parts == 0 {
		return i18n.T("gui.btn.delete-none")
	}
	return i18n.Tf("gui.btn.delete-n", parts, items)
}

// aliveFilterText 解释「只看未删完」这次切换到底改变了什么。这个筛选只隐藏「已经删完」
// 的分区，而刚扫完的快照里一个都没有 —— 不解释的话界面就像点了没反应（本机实测反馈）。
func aliveFilterText(on bool, total, shown int) string {
	hidden := total - shown
	state := i18n.T("gui.alive.off")
	if on {
		state = i18n.T("gui.alive.on")
	}
	if on && hidden == 0 {
		return i18n.Tf("gui.alive.nothing-hidden", state, total)
	}
	return i18n.Tf("gui.alive.line", state, total, hidden, shown)
}

// 使用说明正文在 i18n 目录的 gui.usage.body 里（中英各一份），见 catalog_gui.go。

// entry 是「全局 index + 条目」，明细表的一行。
type entry struct {
	index int
	item  *meta.Item
}

// 编译期钉住三个契约：walk 的 TableView 用 `model.(Sorter)` 决定点表头要不要排序、
// 用 `model.(ItemChecker)` 决定方框能不能点。少实现一个方法，界面不会报错，只是那点
// 下去没反应，所以这里让它编译不过。
var (
	_ walk.Sorter      = (*partModel)(nil)
	_ walk.Sorter      = (*itemModel)(nil)
	_ walk.ItemChecker = (*partModel)(nil)
)

// headerSort 是「点表头排序」的公共状态。walk 在 LVN_COLUMNCLICK 里只对实现了
// Sorter 的模型下手的（tableview.go 里的 `tv.model.(Sorter)` 断言），所以两个模型
// 各嵌一份；SortChanged 一发布，walk 就画排序箭头并重绘可见行。
type headerSort struct {
	walk.SorterBase
	col   int
	order walk.SortOrder
}

func (h *headerSort) SortedColumn() int         { return h.col }
func (h *headerSort) SortOrder() walk.SortOrder { return h.order }

// apply 记下排序列再重排。col 为 -1 时不重排，只发布事件当重绘用。
func (h *headerSort) apply(col int, order walk.SortOrder, resort func()) error {
	h.col, h.order = col, order
	if col >= 0 && resort != nil {
		resort()
	}
	return h.SorterBase.Sort(col, order)
}

func (h *headerSort) reset() { h.col, h.order = -1, walk.SortAscending }

func resortRows[T any](rows []T, col int, order walk.SortOrder, less func(a, b T, col int) bool) {
	if col < 0 {
		return
	}
	if order == walk.SortDescending {
		sort.SliceStable(rows, func(i, j int) bool { return less(rows[j], rows[i], col) })
		return
	}
	sort.SliceStable(rows, func(i, j int) bool { return less(rows[i], rows[j], col) })
}

// 主键相等时退到次键（编号或 index），同一列反复点不会乱跳。
func cmpStr(a, b string, tieA, tieB int) bool {
	la, lb := strings.ToLower(a), strings.ToLower(b)
	if la != lb {
		return la < lb
	}
	return tieA < tieB
}

func cmpInt(a, b int64, tieA, tieB int) bool {
	if a != b {
		return a < b
	}
	return tieA < tieB
}

func cmpF(a, b float64, tieA, tieB int) bool {
	if a != b {
		return a < b
	}
	return tieA < tieB
}

// partModel 是分区表的模型。勾选按「编号」记账而不是行号，所以点表头重排之后
// 勾选不会漂到别的分区上；复选框本身走 walk.ItemChecker。
type partModel struct {
	walk.TableModelBase
	headerSort
	owner   *app
	rows    []*parts.Part
	checked map[int]bool // 编号 -> 已勾选
	anchor  int          // shift 范围勾选的锚点编号，-1 = 没有锚点
}

func newPartModel(a *app) *partModel {
	return &partModel{owner: a, headerSort: headerSort{col: -1}, checked: map[int]bool{}, anchor: -1}
}

func (m *partModel) RowCount() int { return len(m.rows) }

func (m *partModel) ColumnCount() int { return 9 }

func (m *partModel) Value(row, col int) interface{} {
	if row < 0 || row >= len(m.rows) {
		return ""
	}
	p := m.rows[row]
	switch col {
	case 0:
		return strconv.Itoa(p.PID)
	case 1:
		return p.TimeRange
	case 2:
		return p.Label
	case 3:
		return strconv.Itoa(p.Count)
	case 4:
		return tty.Human(float64(p.Size))
	case 5:
		return fmt.Sprintf("%d-%d", p.IndexStart, p.IndexEnd)
	case 6:
		return p.Drives
	case 7:
		return fmt.Sprintf("%d/%d", p.Count-p.Deleted, p.Count)
	case 8:
		return p.TopLabels
	}
	return ""
}

// Checked / SetChecked 实现 walk.ItemChecker，复选框点的就是这两个方法。
func (m *partModel) Checked(row int) bool {
	if row < 0 || row >= len(m.rows) {
		return false
	}
	return m.checked[m.rows[row].PID]
}

// SetChecked 由 walk 在点方框（或选中后按空格）时调用。
func (m *partModel) SetChecked(row int, checked bool) error {
	if m.click(row, checked, walk.ShiftDown()) {
		// walk 只重绘它自己点过的那一行，区间里其余方框要自己触发重画。
		m.refresh()
	}
	m.owner.updateSelectionHint()
	return nil
}

// click 点一个方框；shift 为真时把锚点到这一行之间（含两端）全点成同一个状态。
// 返回是否动了不止一行，调用方据此决定要不要整表重画。
func (m *partModel) click(row int, checked, shift bool) bool {
	if row < 0 || row >= len(m.rows) {
		return false
	}
	pid := m.rows[row].PID
	m.mark(pid, checked)
	spanned := false
	if shift && m.anchor >= 0 && m.anchor != pid {
		if lo, hi, ok := m.rowSpan(m.anchor, pid); ok {
			for r := lo; r <= hi; r++ {
				m.mark(m.rows[r].PID, checked)
			}
			spanned = true
		}
	}
	m.anchor = pid
	return spanned
}

func (m *partModel) mark(pid int, on bool) {
	if on {
		m.checked[pid] = true
		return
	}
	delete(m.checked, pid)
}

// rowSpan 给两个编号所在行的区间（小的在前）；有一个不在当前视图里就不成立。
func (m *partModel) rowSpan(pidA, pidB int) (int, int, bool) {
	first, second := -1, -1
	for i, p := range m.rows {
		if p.PID != pidA && p.PID != pidB {
			continue
		}
		if first < 0 {
			first = i
		} else {
			second = i
		}
	}
	if second < 0 {
		return 0, 0, false
	}
	if first > second {
		first, second = second, first
	}
	return first, second, true
}

// checkedPIDs 返回勾选中的编号，升序。
func (m *partModel) checkedPIDs() []int {
	ids := make([]int, 0, len(m.checked))
	for pid := range m.checked {
		ids = append(ids, pid)
	}
	sort.Ints(ids)
	return ids
}

// selectAlive 是【全选】：勾上当前视图里所有还有未删条目的分区，已删完的跳过 ——
// 那些分区的 $R/$I 已经没了，勾上只会让引擎逐条报「找不到文件」。
func (m *partModel) selectAlive() (selected, skipped int) {
	m.checked = map[int]bool{}
	m.anchor = -1
	for _, p := range m.rows {
		if p.Deleted >= p.Count {
			skipped++
			continue
		}
		m.checked[p.PID] = true
		selected++
	}
	return selected, skipped
}

// clearChecks 是【清空勾选】，返回原本勾着的个数。
func (m *partModel) clearChecks() int {
	n := len(m.checked)
	m.checked = map[int]bool{}
	m.anchor = -1
	return n
}

// refresh 按当前排序再走一遍（col 为 -1 时不重排），让 walk 重读每行的 Checked。
func (m *partModel) refresh() {
	_ = m.Sort(m.col, m.order)
}

func (m *partModel) Sort(col int, order walk.SortOrder) error {
	return m.apply(col, order, m.resort)
}

func (m *partModel) resort() { resortRows(m.rows, m.col, m.order, partLess) }

func (m *partModel) setRows(rows []*parts.Part) {
	m.rows = rows
	m.checked = map[int]bool{}
	m.anchor = -1
	m.reset()
	m.PublishRowsReset()
}

// partLess 是分区表 9 列的升序比较；col 0 和兜底都按编号。
func partLess(a, b *parts.Part, col int) bool {
	switch col {
	case 1:
		return cmpF(a.RangeStart, b.RangeStart, a.PID, b.PID)
	case 2:
		return cmpStr(a.Label, b.Label, a.PID, b.PID)
	case 3:
		return cmpInt(int64(a.Count), int64(b.Count), a.PID, b.PID)
	case 4:
		return cmpInt(a.Size, b.Size, a.PID, b.PID)
	case 5:
		return cmpInt(int64(a.IndexStart), int64(b.IndexStart), a.PID, b.PID)
	case 6:
		return cmpStr(a.Drives, b.Drives, a.PID, b.PID)
	case 7:
		return cmpInt(int64(a.Count-a.Deleted), int64(b.Count-b.Deleted), a.PID, b.PID)
	case 8:
		return cmpStr(a.TopLabels, b.TopLabels, a.PID, b.PID)
	}
	return a.PID < b.PID
}

// itemModel 是明细表的模型。
type itemModel struct {
	walk.TableModelBase
	headerSort
	rows []entry
}

func newItemModel() *itemModel { return &itemModel{headerSort: headerSort{col: -1}} }

func (m *itemModel) RowCount() int { return len(m.rows) }

func (m *itemModel) ColumnCount() int { return 6 }

func (m *itemModel) Value(row, col int) interface{} {
	if row < 0 || row >= len(m.rows) {
		return ""
	}
	it := m.rows[row].item
	switch col {
	case 0:
		return strconv.Itoa(m.rows[row].index)
	case 1:
		return it.DeletedAt
	case 2:
		return tty.Human(float64(it.Size))
	case 3:
		return it.Drive
	case 4:
		return it.Display
	case 5:
		return it.OrigPath
	}
	return ""
}

func (m *itemModel) Sort(col int, order walk.SortOrder) error {
	return m.apply(col, order, m.resort)
}

func (m *itemModel) resort() { resortRows(m.rows, m.col, m.order, itemLess) }

func (m *itemModel) setRows(rows []entry) {
	m.rows = rows
	m.reset()
	m.PublishRowsReset()
}

// itemLess 是明细表 6 列的升序比较；相等时退回 index 顺序。
func itemLess(a, b entry, col int) bool {
	switch col {
	case 1:
		return cmpF(a.item.Epoch, b.item.Epoch, a.index, b.index)
	case 2:
		return cmpInt(a.item.Size, b.item.Size, a.index, b.index)
	case 3:
		return cmpStr(a.item.Drive, b.item.Drive, a.index, b.index)
	case 4:
		return cmpStr(a.item.Display, b.item.Display, a.index, b.index)
	case 5:
		return cmpStr(a.item.OrigPath, b.item.OrigPath, a.index, b.index)
	}
	return a.index < b.index
}

type app struct {
	mw *walk.MainWindow
	// ① 扫描配置：常调的项。「按时间分批」是下拉里的档位，只有选了「自定义」才露出手填框；
	// 「桶内分组」决定另外两项要不要显示：目录层级只在 dir 下用到，前缀长度只在 prefix / token
	// 下用到（见 parts.GroupKey）。
	bucket            *walk.ComboBox
	bucketCustomLabel *walk.TextLabel
	bucketCustom      *walk.LineEdit
	by                *walk.ComboBox
	dirLevelLabel     *walk.TextLabel
	dirLevel          *walk.ComboBox
	prefixLenLabel    *walk.TextLabel
	prefixLen         *walk.LineEdit
	// 高级参数：默认收起，点【▼ 高级参数】展开
	advBtn     *walk.PushButton
	advBox     *walk.Composite
	maxBuckets *walk.LineEdit
	maxParts   *walk.LineEdit
	minCount   *walk.LineEdit
	drives     *walk.LineEdit
	maxScan    *walk.LineEdit
	fuse       *walk.LineEdit
	stateDir   *walk.LineEdit
	// ③ 选择分区
	idsBox    *walk.LineEdit
	selectBtn *walk.PushButton
	idsClear  *walk.PushButton
	prefixBox *walk.LineEdit
	aliveOnly *walk.CheckBox
	// 表与文本区
	partsTV       *walk.TableView
	itemsTV       *walk.TableView
	partModel     *partModel
	itemModel     *itemModel
	partsHint     *walk.TextLabel
	detailTitle   *walk.TextLabel
	logBox        *walk.TextEdit
	logCount      *walk.TextLabel
	progress      *walk.ProgressBar
	progressLabel *walk.TextLabel
	scanBtn       *walk.PushButton
	previewBtn    *walk.PushButton
	deleteBtn     *walk.PushButton
	sbState       *walk.StatusBarItem
	sbSnapshot    *walk.StatusBarItem
	sbHint        *walk.StatusBarItem
	// ② 扫描结果概览：分区 / 对象 / 未删完 / 已勾选
	sumParts   *walk.TextLabel
	sumItems   *walk.TextLabel
	sumAlive   *walk.TextLabel
	sumChecked *walk.TextLabel
	// 状态
	st       *store.State
	busy     bool
	logLines int           // 日志区已有行数，给「0 行」计数用
	advOpen  bool          // 高级参数那一行是否展开
	lock     []walk.Window // 后台任务期间禁用的控件
	notice   *walk.Dialog  // 启动自动扫描期间的小提示窗，扫完自动关，用户也能自己关
	// 语言热切换：retexts 是「换语言时逐条重设」的登记表（见 retext.go）。
	retexts []func()
	// 下面几个字段记下「动态那几格现在显示的是什么事」，换语言时按同样的口径重算，
	// 而不是凭默认值重新拼一遍：忙到一半切语言不能把「扫描中…」变成「就绪」。
	busyKey      string        // beginBusy 用的是哪条文案（gui.sb.scanning / gui.sb.deleting）
	snapDeleted  bool          // 状态栏中间那格该按「删过东西」的口径显示
	detailRender func() string // ⑤ 明细表标题的算法，换语言时照着它重算一次
	progKind     int           // 进度那一行处于哪种状态（progIdle / progBusy / …）
	progElapsed  string        // progLastScan：上次扫描用时，已格式化好的文本
	progN        int           // progDeleteStep：已处理到第几个
	progTotal    int
	progDone     int // 成功数，progDeleteEnd 也用这两个
	progFailed   int
	// 分组框标题的指针：只给登记表用（①…⑥ 各一个）。
	gbConfig, gbResult, gbSelect, gbParts, gbItems, gbDelete *walk.GroupBox
	// 菜单项指针：文字和【界面语言】的勾都要跟着语言走
	menuFile, menuView, menuHelp, langMenu *walk.Action
	langZhAction, langEnAction             *walk.Action
}

// Run 打开主窗口并进入消息循环，窗口关闭后返回进程退出码。
func Run() int {
	a := &app{}
	if err := a.build(); err != nil {
		// 语言在 main.go 里已经按 --lang / RBM_LANG / settings.json 定好了，这里跟着它走。
		fmt.Fprintln(os.Stderr, i18n.Tf("gui.run.create-fail", err))
		return 1
	}
	// 这份 rbm.exe 是控制台子系统，系统先给它建了黑窗口；界面已经建好，这里把控制台甩掉，
	// 日志之后只走窗口里的日志区。命令行分支不经过这里；要黑框从头不出现就用 rbm-gui.exe。
	procFreeConsole.Call()
	a.mw.Run()
	return 0
}

func (a *app) build() error {
	a.partModel = newPartModel(a)
	a.itemModel = newItemModel()

	// 这些菜单项指针只在换语言时要用（登记表里的闭包解引用它们），声明成局部变量就够了。
	var actScan, actOpenDir, actQuit, actAdv, actAll, actClear, actUsage, actAbout *walk.Action

	spec := MainWindow{
		AssignTo: &a.mw,
		Title:    i18n.T("gui.title"),
		MinSize:  Size{Width: minWindowZh, Height: 680},
		Size:     Size{Width: 1400, Height: 900},
		Layout:   VBox{},
		MenuItems: []MenuItem{
			a.menu(&a.menuFile, "gui.menu.file", []MenuItem{
				a.actKey(&actScan, "gui.menu.scan", Shortcut{Key: walk.KeyF5}, func() { a.startScan() }),
				a.act(&actOpenDir, "gui.menu.open-dir", func() { a.openStateDir() }),
				Separator{},
				a.act(&actQuit, "gui.menu.quit", func() { a.mw.Close() }),
			}),
			a.menu(&a.menuView, "gui.menu.view", []MenuItem{
				a.act(&actAdv, "gui.menu.adv", func() { a.toggleAdvanced() }),
				Separator{},
				a.act(&actAll, "gui.menu.select-all", func() { a.selectAll() }),
				a.act(&actClear, "gui.menu.clear-checks", func() { a.clearChecked() }),
				Separator{},
				// 界面语言：点一下整窗换文字，并把选择写进 settings.json。
				// 勾跟着当前语言走，所以做成 Checkable —— walk 在触发时会自己把勾翻一次
				// （action.go:443-449），真正的状态由 setLanguage 里的 syncLangMenu 定。
				a.menu(&a.langMenu, "gui.menu.language", []MenuItem{
					Action{AssignTo: &a.langZhAction, Text: i18n.T("gui.menu.lang-zh"), Checkable: true,
						OnTriggered: func() { a.setLanguage(i18n.Zh) }},
					Action{AssignTo: &a.langEnAction, Text: i18n.T("gui.menu.lang-en"), Checkable: true,
						OnTriggered: func() { a.setLanguage(i18n.En) }},
				}),
			}),
			a.menu(&a.menuHelp, "gui.menu.help", []MenuItem{
				a.act(&actUsage, "gui.menu.usage", func() { a.showUsage() }),
				a.act(&actAbout, "gui.menu.about", func() { a.showAbout() }),
			}),
		},
		StatusBarItems: []StatusBarItem{
			a.sb(&a.sbState, "gui.sb.ready", 110, sbStateCap),
			a.sb(&a.sbSnapshot, "gui.sb.none", 470, sbSnapCap),
			a.sb(&a.sbHint, "gui.sb.hint", 360, sbHintCap),
		},
		Children: []Widget{
			a.configBox(), a.resultBox(), a.selectBox(), a.tables(), a.deleteBox(), a.bottom(),
		},
	}
	if err := spec.Create(); err != nil {
		return err
	}
	// 窗口标题不在控件树上，只能自己重设。
	a.onRetext(func() { a.mw.SetTitle(i18n.T("gui.title")) })
	// 英文起点（--lang en / RBM_LANG / settings.json）不能等到第一次切换语言才抬最小宽度。
	a.applyWindowMinSize()
	// 两个语言项是直接在声明里写的（要 Checkable），文字也登记一下。
	a.onRetext(func() {
		a.langZhAction.SetText(i18n.T("gui.menu.lang-zh"))
		a.langEnAction.SetText(i18n.T("gui.menu.lang-en"))
	})

	// 先给「桶内分组」设默认索引，再给「按时间分批」：两个下拉的 OnCurrentIndexChanged 都会调
	// applyParamVisibility，而它要读 a.by 的索引；walk 在设索引前 CurrentIndex() 是 -1。
	a.by.SetCurrentIndex(0)
	a.bucket.SetCurrentIndex(0)
	a.dirLevel.SetCurrentIndex(1)
	a.prefixLen.SetText(strconv.Itoa(defPrefixLen))
	a.maxBuckets.SetText(strconv.Itoa(defMaxBuckets))
	a.maxParts.SetText(strconv.Itoa(defMaxParts))
	a.minCount.SetText(strconv.Itoa(defMinCount))
	a.maxScan.SetText(strconv.Itoa(defMaxScan))
	a.fuse.SetText("0")
	dir := store.DefaultStateDir()
	a.stateDir.SetText(dir)

	// 高级参数默认收起：收起后不占位，① 只剩「按时间分批 / 桶内分组 / 目录层级 / 前缀长度」这一行。
	a.advBox.SetAlwaysConsumeSpace(false)
	a.advBox.SetVisible(false)
	a.applyParamVisibility()

	a.lock = []walk.Window{
		a.scanBtn, a.previewBtn, a.deleteBtn, a.bucket, a.bucketCustom, a.by, a.dirLevel, a.prefixLen,
		a.maxBuckets, a.maxParts, a.minCount, a.drives, a.maxScan, a.fuse, a.stateDir,
		a.idsBox, a.prefixBox, a.aliveOnly, a.selectBtn, a.idsClear, a.advBtn,
	}

	a.partsTV.CurrentIndexChanged().Attach(func() { a.showCurrentDetail() })

	// 换语言时两张表的列名和列宽也要换：英文列名更长，沿用中文的宽度会被截断。
	a.onRetext(func() { a.applyColumnTitles() })
	// 菜单 / 状态栏 / 进度 / 明细标题在 Create 之后先按当前语言对齐一次：
	// 声明期给的是初始语言的文案，这里补上「扫描前还没有内容可算」的那几格。
	a.syncLangMenu()

	a.updateSelectionHint()
	// 不载入上次的快照：界面要显示回收站此刻的内容，编号也按此刻内容重建。
	a.showStartupNotice()
	a.startScan()
	return nil
}

// ---------- 界面搭建 ----------

// configBox 是 ① 扫描配置：只有对当前选择生效的项才占位——「目录层级」只在 dir 分组下被
// parts.GroupKey 用到，「前缀长度」只在 prefix / token 下用到，「写法」手填框只在「自定义」那一项
// 出现，所以切下拉时这些标签+输入框会跟着显示或收起（applyParamVisibility）。
// 其余项和快照目录收在【▼ 高级参数】里。
func (a *app) configBox() Widget {
	// 这一行里只有 ① 的标题和几个标签需要「换语言时重设」，都用局部指针接住，
	// 声明期顺手登记（见 retext.go）。
	var lblBucket, lblBy, lblMaxBuckets, lblMaxParts, lblMinCount, lblDrives, lblMaxScan, lblFuse, lblStateDir *walk.TextLabel
	var openDirBtn *walk.PushButton
	return a.box(&a.gbConfig, "gui.box.config", VBox{}, []Widget{
		Composite{Layout: HBox{}, Children: []Widget{
			a.lab(&lblBucket, "gui.lab.bucket"),
			a.combo(&a.bucket, 250, bucketDisplay, "gui.tip.bucket", func() { a.applyParamVisibility() }),
			a.lab(&a.bucketCustomLabel, "gui.lab.bucket-spec"),
			// 「写法」手填框里是 daily / 30m / gap:5m 这类 token，不是给人读的话，所以不跟语言变。
			LineEdit{AssignTo: &a.bucketCustom, MaxSize: Size{Width: 170},
				CueBanner: "daily / 30m / 3mo / gap:5m / auto"},
			a.lab(&lblBy, "gui.lab.by"),
			a.combo(&a.by, 160, byDisplay, "", func() { a.applyParamVisibility() }),
			a.lab(&a.dirLevelLabel, "gui.lab.dir-level"),
			a.combo(&a.dirLevel, 200, levelDisplay, "", nil),
			a.lab(&a.prefixLenLabel, "gui.lab.prefix-len"),
			LineEdit{AssignTo: &a.prefixLen, MaxSize: Size{Width: 56}},
			HSpacer{},
			a.advToggleBtn(),
		}},
		Composite{AssignTo: &a.advBox, Layout: VBox{}, Children: []Widget{
			Composite{Layout: HBox{}, Children: []Widget{
				a.lab(&lblMaxBuckets, "gui.lab.max-buckets"),
				LineEdit{AssignTo: &a.maxBuckets, MaxSize: Size{Width: 56},
					ToolTipText: tipOf(a, &a.maxBuckets, "gui.tip.max-buckets")},
				a.lab(&lblMaxParts, "gui.lab.max-parts"),
				LineEdit{AssignTo: &a.maxParts, MaxSize: Size{Width: 64},
					ToolTipText: tipOf(a, &a.maxParts, "gui.tip.max-parts")},
				a.lab(&lblMinCount, "gui.lab.min-count"),
				LineEdit{AssignTo: &a.minCount, MaxSize: Size{Width: 56},
					ToolTipText: tipOf(a, &a.minCount, "gui.tip.min-count")},
				a.lab(&lblDrives, "gui.lab.drives"),
				LineEdit{AssignTo: &a.drives, MaxSize: Size{Width: 90},
					CueBanner: cueOf(a, &a.drives, "gui.cue.drives")},
				a.lab(&lblMaxScan, "gui.lab.max-scan"),
				LineEdit{AssignTo: &a.maxScan, MaxSize: Size{Width: 72},
					ToolTipText: tipOf(a, &a.maxScan, "gui.tip.max-scan")},
				a.lab(&lblFuse, "gui.lab.fuse"),
				LineEdit{AssignTo: &a.fuse, MaxSize: Size{Width: 72},
					CueBanner:   cueOf(a, &a.fuse, "gui.cue.fuse"),
					ToolTipText: tipOf(a, &a.fuse, "gui.tip.fuse")},
				HSpacer{},
			}},
			Composite{Layout: HBox{}, Children: []Widget{
				a.lab(&lblStateDir, "gui.lab.state-dir"),
				LineEdit{AssignTo: &a.stateDir, StretchFactor: 1,
					ToolTipText: tipOf(a, &a.stateDir, "gui.tip.state-dir")},
				a.btn(&openDirBtn, "gui.btn.open-dir", 92, "", func() { a.openStateDir() }),
				HSpacer{},
			}},
		}},
	})
}

// advToggleBtn 声明【▼ 展开高级参数】那个开关按钮。它不走 a.btn 的登记表：文字由「当前展没展开」
// 决定（▼ / ▲ 是两条文案），所以只登记一条按 a.advOpen 取键的闭包。
func (a *app) advToggleBtn() PushButton {
	a.onRetext(func() { a.setAdvBtnText() })
	key := advBtnKey(a.advOpen)
	return PushButton{
		AssignTo:    &a.advBtn,
		Text:        i18n.T(key),
		MaxSize:     Size{Width: widthFor(140, i18n.T(key))},
		ToolTipText: i18n.T("gui.tip.adv"),
		OnClicked:   func() { a.toggleAdvanced() },
	}
}

func advBtnKey(open bool) string {
	if open {
		return "gui.btn.adv-close"
	}
	return "gui.btn.adv-open"
}

// setAdvBtnText 按「当前展开状态 + 当前语言」重设按钮的文字、宽度和气泡提示。
func (a *app) setAdvBtnText() {
	text := i18n.T(advBtnKey(a.advOpen))
	_ = a.advBtn.SetText(text)
	_ = a.advBtn.SetMinMaxSize(walk.Size{}, walk.Size{Width: widthFor(140, text)})
	_ = a.advBtn.SetToolTipText(i18n.T("gui.tip.adv"))
}

// paramsShow 说清「桶内分组」取 by 时哪两项真正被用到：parts.GroupKey 只在 dir 读 dirLevel，
// 只在 prefix / token 读 prefixLen（ext 两个都不读）。界面据此决定显示哪一项。
func paramsShow(by string) (level bool, prefixLen bool) {
	return by == "dir", by == "prefix" || by == "token"
}

// bucketToken 把下拉选项换成 parts.ParseSpec 认的写法。「自定义」那一项（末尾）取手填框里的
// 文本（去掉首尾空格，留空就等于默认的自动空档）；越界时按默认。
func bucketToken(idx int, custom string) string {
	if bucketIsCustom(idx) {
		return strings.TrimSpace(custom)
	}
	if idx >= 0 && idx < len(bucketValues) {
		return bucketValues[idx]
	}
	return ""
}

// comboValue 按下拉索引取那一项对应的写法。索引没就绪（-1：列表刚被重建，或还没设过默认项）
// 就回落到默认值，别拿它去下标 —— 那会 panic 并把整个窗口带走。
func comboValue(items []string, idx int, def string) string {
	if idx >= 0 && idx < len(items) {
		return items[idx]
	}
	return def
}

// applyParamVisibility 按「桶内分组」显示/隐藏目录层级与前缀长度，按「按时间分批」显示/隐藏
// 自定义写法框。隐藏时设成不占位，否则 ① 会留空档。值本身留在控件里，切回来就按原值生效；
// collectOptions 也照旧读它们。
func (a *app) applyParamVisibility() {
	// walk 在下拉列表被重建的那一刻会先把索引打回 -1（ComboBox.SetModel → resetItems →
	// SetCurrentIndex(-1)，并且这一跳就会发布 CurrentIndexChanged），此时按索引取组名会
	// 直接 panic。换语言正是走这条路（combo 的登记表里 SetModel 之后才把索引复位），
	// 所以索引没就绪就什么都不做：显示维持上一次的样子，索引复位后还会再走一遍。
	idx := a.by.CurrentIndex()
	if idx < 0 || idx >= len(byValues) {
		return
	}
	showLevel, showPrefixLen := paramsShow(byValues[idx])
	custom := bucketIsCustom(a.bucket.CurrentIndex())
	for _, pair := range []struct {
		w     walk.Widget
		shows bool
	}{
		{a.dirLevelLabel, showLevel}, {a.dirLevel, showLevel},
		{a.prefixLenLabel, showPrefixLen}, {a.prefixLen, showPrefixLen},
		{a.bucketCustomLabel, custom}, {a.bucketCustom, custom},
	} {
		pair.w.SetAlwaysConsumeSpace(pair.shows)
		pair.w.SetVisible(pair.shows)
	}
}

// resultBox 是 ② 扫描结果：扫描按钮 + 四个总体数字，不用看表格也知道此刻的状态。
func (a *app) resultBox() Widget {
	var sumPartsLabel, sumItemsLabel, sumAliveLabel, sumCheckedLabel *walk.TextLabel
	return a.box(&a.gbResult, "gui.box.result", VBox{}, []Widget{
		Composite{Layout: HBox{}, Children: []Widget{
			// Min 与 Max 同值才叫锁宽：layout.go 的 minSizeEffective 先取 max(文字理想宽度, MinSize)
			// 再按 MaxSize 收，只写 MaxSize 时按钮只拿到文字宽度，两侧就没有留白。
			a.btnLock(&a.scanBtn, "gui.btn.scan", 120, "", func() { a.startScan() }),
			HSpacer{},
		}},
		Composite{Layout: HBox{}, Children: []Widget{
			a.lab(&sumPartsLabel, "gui.lab.parts"),
			TextLabel{AssignTo: &a.sumParts, MinSize: Size{Width: 150}, TextColor: walk.RGB(0, 80, 160)},
			a.lab(&sumItemsLabel, "gui.lab.objects"),
			TextLabel{AssignTo: &a.sumItems, MinSize: Size{Width: 170}, TextColor: walk.RGB(0, 80, 160)},
			a.lab(&sumAliveLabel, "gui.lab.alive"),
			TextLabel{AssignTo: &a.sumAlive, MinSize: Size{Width: 170}, TextColor: walk.RGB(0, 80, 160)},
			a.lab(&sumCheckedLabel, "gui.lab.checked"),
			TextLabel{AssignTo: &a.sumChecked, MinSize: Size{Width: 230}, TextColor: walk.RGB(0, 80, 160)},
			HSpacer{},
		}},
	})
}

// selectBox 是 ③ 选择分区：全选 / 清空勾选在最前，然后是按编号勾选、前缀过滤和只看未删完。
func (a *app) selectBox() Widget {
	var lblIDs, lblPrefix *walk.TextLabel
	var applyBtn *walk.PushButton
	return a.box(&a.gbSelect, "gui.box.select", HBox{}, []Widget{
		a.btn(&a.selectBtn, "gui.btn.select-all", 72, "gui.tip.select-all", func() { a.selectAll() }),
		a.btn(&a.idsClear, "gui.btn.clear-checks", 92, "", func() { a.clearChecked() }),
		a.lab(&lblIDs, "gui.lab.by-ids"),
		LineEdit{AssignTo: &a.idsBox, MaxSize: Size{Width: 160},
			CueBanner: cueOf(a, &a.idsBox, "gui.cue.ids")},
		a.btn(&applyBtn, "gui.btn.apply", 64, "", func() { a.applyIDs() }),
		a.lab(&lblPrefix, "gui.lab.prefix-filter"),
		LineEdit{AssignTo: &a.prefixBox, MaxSize: Size{Width: 160},
			CueBanner:         cueOf(a, &a.prefixBox, "gui.cue.prefix"),
			OnEditingFinished: func() { a.showCurrentDetail(); a.updateSelectionHint() }},
		a.check(&a.aliveOnly, "gui.cb.alive", func() { a.onAliveToggled() }),
		HSpacer{},
	})
}

// deleteBox 是 ⑥ 删除操作，放在选完、看完明细之后：预览在前，永久删除在最后并且随时报出规模。
// 进度条和这两个按钮同一行（原来单独占一行，纵向太吃空间），文字说明它在量什么。
func (a *app) deleteBox() Widget {
	// 删除按钮不走 a.btn：它的文字和宽度都由「勾了多少」决定（setDeleteButton →
	// deleteButtonWidths），换语言时 applyLanguage 末尾的 updateSelectionHint 会重算，
	// 所以这里只声明初值，不登记固定的那条键。
	delText := deleteButtonText(0, 0)
	delMin, delMax := deleteButtonWidths(delText)
	return a.box(&a.gbDelete, "gui.box.delete", HBox{}, []Widget{
		a.btnLock(&a.previewBtn, "gui.btn.preview", 165, "gui.tip.preview", func() { a.previewDelete() }),
		PushButton{
			AssignTo:    &a.deleteBtn,
			Text:        delText,
			MinSize:     Size{Width: delMin},
			MaxSize:     Size{Width: delMax},
			ToolTipText: i18n.T("gui.tip.delete"),
			OnClicked:   func() { a.deleteSelected() },
		},
		ProgressBar{AssignTo: &a.progress, MinValue: 0, MaxValue: 100, StretchFactor: 1,
			MinSize: Size{Width: 140}},
		TextLabel{AssignTo: &a.progressLabel, MinSize: Size{Width: 270},
			Text: i18n.T("gui.progress-word") + i18n.T("gui.progress.idle")},
	})
}

func (a *app) tables() Widget {
	// 这行概览标签和右边明细表的标题行同高，两张表的表头才能对齐。
	gbLeft := a.box(&a.gbParts, "gui.box.parts", VBox{}, []Widget{
		TextLabel{AssignTo: &a.partsHint, Text: i18n.T("gui.parts-hint.none")},
		a.partsTVSpec(),
	})
	gbLeft.StretchFactor = 3
	gbRight := a.box(&a.gbItems, "gui.box.items", VBox{}, []Widget{
		TextLabel{AssignTo: &a.detailTitle, Text: i18n.T("gui.detail.ask")},
		a.itemsTVSpec(),
	})
	gbRight.StretchFactor = 2
	return HSplitter{
		Children:      []Widget{gbLeft, gbRight},
		StretchFactor: 3,
	}
}

// partsTVSpec / itemsTVSpec 的列名与列宽从 partColumns / itemColumns 取，
// 和换语言时走的是同一份表（见 applyColumnTitles）。
func (a *app) partsTVSpec() TableView {
	cols := a.partColumns()
	decl := make([]TableViewColumn, len(cols))
	for i, c := range cols {
		decl[i] = TableViewColumn{Title: c.title, Width: widthFor(c.zhW, c.title), Alignment: c.align}
	}
	return TableView{
		AssignTo:            &a.partsTV,
		Model:               a.partModel,
		CheckBoxes:          true,
		MultiSelection:      true,
		AlternatingRowBG:    true,
		LastColumnStretched: true,
		MinSize:             Size{Width: 360, Height: 180},
		OnItemActivated:     func() { a.showCurrentDetail() },
		StyleCell: func(style *walk.CellStyle) {
			p := a.partRow(style.Row())
			if p != nil && p.Deleted >= p.Count {
				style.TextColor = walk.RGB(150, 150, 150)
			}
		},
		Columns: decl,
	}
}

func (a *app) itemsTVSpec() TableView {
	cols := itemColumns()
	decl := make([]TableViewColumn, len(cols))
	for i, c := range cols {
		decl[i] = TableViewColumn{Title: c.title, Width: widthFor(c.zhW, c.title), Alignment: c.align}
	}
	return TableView{
		AssignTo:            &a.itemsTV,
		Model:               a.itemModel,
		AlternatingRowBG:    true,
		LastColumnStretched: true,
		MinSize:             Size{Width: 420, Height: 180},
		Columns:             decl,
	}
}

// bottom 是操作日志：进度条已经并进 ⑥ 那一行，这里只剩标题行 + 日志区。
// 日志区带【清空】【复制】，行首有时间和级别。
func (a *app) bottom() Widget {
	var lblLog *walk.TextLabel
	var clearLogBtn, copyLogBtn *walk.PushButton
	return Composite{
		Layout: VBox{},
		Children: []Widget{
			Composite{Layout: HBox{}, Children: []Widget{
				a.lab(&lblLog, "gui.lab.log"),
				TextLabel{AssignTo: &a.logCount, Text: i18n.Tf("gui.log.lines", 0)},
				a.btn(&clearLogBtn, "gui.btn.clear-log", 64, "", func() { a.clearLog() }),
				a.btn(&copyLogBtn, "gui.btn.copy-log", 64, "gui.tip.copy-log", func() { a.copyLog() }),
				HSpacer{},
			}},
			TextEdit{AssignTo: &a.logBox, ReadOnly: true, VScroll: true, MinSize: Size{Height: 120},
				Font: Font{Family: "Consolas", PointSize: 9}, StretchFactor: 1},
		},
	}
}

// ---------- 通用 ----------

// levelMark 是日志行的级别前缀。
func levelMark(kind int) string {
	switch kind {
	case logOK:
		return "✓"
	case logWarn:
		return "!"
	case logErr:
		return "×"
	}
	return "·"
}

// emit 往日志区追加一条带时间戳和级别前缀的记录。多行文本要转成 CRLF，Windows 编辑控件才换行；
// 续行缩进对齐到正文列，扫表那种多行输出读起来才不歪。
func (a *app) emit(kind int, text string) {
	stamp := time.Now().Format("15:04:05")
	pad := strings.Repeat(" ", len(stamp)+len(levelMark(kind))+2)
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var b strings.Builder
	for i, ln := range lines {
		if i == 0 {
			fmt.Fprintf(&b, "%s %s %s", stamp, levelMark(kind), ln)
		} else {
			b.WriteString(pad + ln)
		}
		if i != len(lines)-1 {
			b.WriteString("\r\n")
		}
	}
	a.logBox.AppendText(b.String() + "\r\n")
	// walk 的 AppendText 追加完会把选区还原成追加前的位置（textedit.go:240-245），光标不在末尾，
	// 所以要先自己把光标挪到最后一条再滚过去，否则每次都滚回顶部。
	n := a.logBox.TextLength()
	a.logBox.SetTextSelection(n, n)
	a.logBox.ScrollToCaret()
	a.logLines += len(lines)
	a.logCount.SetText(i18n.Tf("gui.log.lines", a.logLines))
}

// log 追加一条普通记录（信息级）；ok/warn/fail 是成功、提醒、失败三档，
// 用在扫描完成、闸门拦下、报错这些关键节点上。
//
// 这几个方法都收「已经拼好的整句话」：文案一律走 i18n.T / i18n.Tf 先格式化，
// 不能再把译文当格式串传进来（路径和文件名里带 % 会被当成动词）。
func (a *app) log(text string) { a.emit(logInfo, text) }

func (a *app) ok(text string)   { a.emit(logOK, text) }
func (a *app) warn(text string) { a.emit(logWarn, text) }
func (a *app) fail(text string) { a.emit(logErr, text) }

func (a *app) clearLog() {
	a.logBox.SetText("")
	a.logLines = 0
	a.logCount.SetText(i18n.Tf("gui.log.lines", 0))
}

// copyLog 把整段日志放进剪贴板。空日志不写剪贴板，免得把用户先前复制的内容冲掉。
func (a *app) copyLog() {
	text := a.logBox.Text()
	if strings.TrimSpace(text) == "" {
		a.log(i18n.T("gui.log.empty-copy"))
		return
	}
	if err := walk.Clipboard().SetText(text); err != nil {
		a.fail(i18n.Tf("gui.log.copy-fail", err))
		return
	}
	a.ok(i18n.Tf("gui.log.copied", strings.Count(text, "\r\n")+1))
}

// toggleAdvanced 展开/收起「桶数上限…删除保险丝 + 快照目录」那两行。SetVisible 自己会请求重排，
// 收起时又设了不占位，所以① 会缩回一行高。
func (a *app) toggleAdvanced() {
	a.advOpen = !a.advOpen
	a.advBox.SetVisible(a.advOpen)
	a.setAdvBtnText()
	if !a.advOpen {
		a.log(i18n.T("gui.log.adv-collapsed"))
	}
}

func (a *app) partRow(row int) *parts.Part {
	if row < 0 || row >= len(a.partModel.rows) {
		return nil
	}
	return a.partModel.rows[row]
}

// beginBusy 后台任务期间锁住参数与动作，状态栏和进度文字都换成「在做什么」。
// key 是目录里的键（gui.sb.scanning / gui.sb.deleting），不是现成的句子：
// 忙到一半换语言时，updateStateLine / updateProgressLine 要照着它重算。
func (a *app) beginBusy(key string) {
	a.busy = true
	a.busyKey = key
	a.progKind = progBusy
	for _, w := range a.lock {
		w.SetEnabled(false)
	}
	a.progress.SetValue(0)
	a.updateStateLine()
	a.updateProgressLine()
	a.setHint(i18n.T(key))
}

func (a *app) endBusy() {
	for _, w := range a.lock {
		w.SetEnabled(true)
	}
	a.busy = false
	a.busyKey = ""
	a.progKind = progIdle
	a.updateStateLine()
	a.updateProgressLine()
	a.updateSelectionHint()
}

// showStartupNotice 在启动自动扫描期间开一个非模态小提示窗（扫完自动关，用户也能先自己关掉）。
// 非模态是必须的：Modal 会把消息循环堵住，扫描 goroutine 回来更新的 Synchronize 就永远排不上。
func (a *app) showStartupNotice() {
	var dlg *walk.Dialog
	closeText := i18n.T("gui.btn.close")
	err := Dialog{
		AssignTo:  &dlg,
		Title:     i18n.T("gui.notice.title"),
		FixedSize: true,
		MinSize:   Size{Width: 360},
		Layout:    HBox{},
		Children: []Widget{
			TextLabel{Text: i18n.T("gui.notice.text")},
			HSpacer{},
			PushButton{Text: closeText,
				MaxSize:   Size{Width: widthFor(64, closeText)},
				OnClicked: func() { dlg.Close(0) }},
		},
	}.Create(a.mw)
	if err != nil {
		a.log(i18n.Tf("gui.notice.create-fail", err))
		return
	}
	// 用户点 X 或【关闭】之后这个 Dialog 已经被销毁，句柄不能再碰，所以先清掉引用。
	dlg.Closing().Attach(func(canceled *bool, reason walk.CloseReason) { a.notice = nil })
	a.notice = dlg
	dlg.Show()
}

func (a *app) closeNotice() {
	if a.notice == nil {
		return
	}
	d := a.notice
	a.notice = nil
	d.Close(0)
}

func (a *app) dir() string {
	d := strings.TrimSpace(a.stateDir.Text())
	if d == "" {
		return store.DefaultStateDir()
	}
	return d
}

// intValue 读一个数字输入框；留空取默认值，填错返回可直接打印的错误。
// nameKey 是「这项参数叫什么」的目录键，错误文案里要按当前语言念出它的名字。
func intValue(le *walk.LineEdit, nameKey string, def int) (int, error) {
	s := strings.TrimSpace(le.Text())
	if s == "" {
		return def, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, errors.New(i18n.Tf("gui.err.int", i18n.T(nameKey), s))
	}
	if n < 0 {
		return 0, errors.New(i18n.Tf("gui.err.negative", i18n.T(nameKey), n))
	}
	return n, nil
}

// collectOptions 把界面上的参数拼成引擎的入参。
func (a *app) collectOptions() (engine.Options, int, error) {
	prefixLen, err := intValue(a.prefixLen, "gui.lab.prefix-len", defPrefixLen)
	if err != nil {
		return engine.Options{}, 0, err
	}
	maxBuckets, err := intValue(a.maxBuckets, "gui.lab.max-buckets", defMaxBuckets)
	if err != nil {
		return engine.Options{}, 0, err
	}
	maxParts, err := intValue(a.maxParts, "gui.lab.max-parts", defMaxParts)
	if err != nil {
		return engine.Options{}, 0, err
	}
	minCount, err := intValue(a.minCount, "gui.lab.min-count", defMinCount)
	if err != nil {
		return engine.Options{}, 0, err
	}
	maxScan, err := intValue(a.maxScan, "gui.lab.max-scan", defMaxScan)
	if err != nil {
		return engine.Options{}, 0, err
	}
	fuse, err := intValue(a.fuse, "gui.lab.fuse", 0)
	if err != nil {
		return engine.Options{}, 0, err
	}
	drivesSpec := strings.TrimSpace(a.drives.Text())
	o := engine.Options{
		Bucket:     bucketToken(a.bucket.CurrentIndex(), a.bucketCustom.Text()),
		By:         comboValue(byValues, a.by.CurrentIndex(), "dir"),
		DirLevel:   comboValue(levelValues, a.dirLevel.CurrentIndex(), "2"),
		PrefixLen:  prefixLen,
		MaxBuckets: maxBuckets,
		MaxParts:   maxParts,
		MinCount:   minCount,
		Drives:     store.SplitDrives(drivesSpec),
		DrivesSpec: drivesSpec,
		MaxScan:    maxScan,
		StateDir:   a.dir(),
	}
	if o.By == "prefix" || o.By == "token" {
		if o.PrefixLen < 1 {
			return o, fuse, errors.New(i18n.Tf("gui.err.prefix-len", o.By))
		}
	}
	return o, fuse, nil
}

// overflowText 复刻命令行的超限提示：截断必须显式说清，不能静默丢掉条目。
func overflowText(ov *meta.Overflow) string {
	var b strings.Builder
	b.WriteString(i18n.Tf("gui.overflow.head", ov.Total, ov.Taken, ov.Taken, ov.Total-ov.Taken))
	drives := make([]string, 0, len(ov.ByDrive))
	for d := range ov.ByDrive {
		drives = append(drives, d)
	}
	sort.Strings(drives)
	cells := make([]string, len(drives))
	for i, d := range drives {
		v := ov.ByDrive[d]
		cells[i] = fmt.Sprintf("%s %d/%d", d, v[0], v[1])
	}
	b.WriteString(i18n.Tf("gui.overflow.drives", strings.Join(cells, ", ")))
	b.WriteString(i18n.T("gui.overflow.two-ways"))
	b.WriteString(i18n.Tf("gui.overflow.option-a", (ov.Total/10000+1)*10000))
	b.WriteString(i18n.T("gui.overflow.option-b"))
	return b.String()
}

func (a *app) setState(st *store.State) {
	a.st = st
	a.snapDeleted = false
	a.refreshParts(false)
	a.updateSnapshotLine()
}

func totalSize(st *store.State) int64 {
	var n int64
	for _, p := range st.Parts {
		n += p.Size
	}
	return n
}

// 换数据一律走这两个：模型只发布 RowsReset，而 walk 收到它只把行数塞进 ListView，带的还是
// LVSICF_NOINVALIDATEALL（walk tableview.go:936 setItemCount）——行数不变或变少时一格都不重画。
// 所以删除完的置灰、「剩余」列和明细表尾巴会停在旧像素上，要等鼠标逐行划过才显形。
// 勾选状态没这问题：那条走 partModel.refresh()，发布 SortChanged，walk 会重画可见行。
func (a *app) setPartRows(rows []*parts.Part) {
	a.partModel.setRows(rows)
	_ = a.partsTV.Invalidate()
}

func (a *app) setItemRows(rows []entry) {
	a.itemModel.setRows(rows)
	_ = a.itemsTV.Invalidate()
}

// refreshParts 按「只看未删完」重画分区表；显示顺序交给点表头，所以这里固定用快照自身的
// 时间顺序。视图一变，勾选一律清空：行号和勾选的对应关系变了，留着旧勾选可能删错批次。
// explain 为 true 时说明这次筛选改变了什么 —— 刚扫完的快照没有「已删完」的分区，勾上这个框
// 一行也不会少，不解释看上去就像点了没反应。
func (a *app) refreshParts(explain bool) {
	had := len(a.partModel.checked)
	if a.st == nil {
		a.setPartRows(nil)
		a.updateSelectionHint()
		return
	}
	rows := engine.SortedParts(a.st, "time", a.aliveOnly.Checked())
	a.setPartRows(rows)
	// 第 3 列的列名跟着快照的分组维度走（路径组 / 前缀组），中英文各有一套写法。
	a.applyColumnTitles()
	if explain {
		a.log(aliveFilterText(a.aliveOnly.Checked(), len(a.st.Parts), len(rows)))
	}
	if had > 0 {
		a.warn(i18n.Tf("gui.alive.checks-lost", had))
	}
	a.updateSelectionHint()
	// PublishRowsReset 把当前行清成 -1，右栏会跟着空掉；视图非空时回到第一行。
	if a.partsTV.CurrentIndex() < 0 && len(rows) > 0 {
		a.partsTV.SetCurrentIndex(0)
	}
	a.showCurrentDetail()
}

// onAliveToggled 是「只看未删完」的处理：刷视图，并把这次到底隐藏了几个说清楚。
func (a *app) onAliveToggled() { a.refreshParts(true) }

// ---------- 动作 ----------

func (a *app) startScan() {
	if a.busy {
		a.log(i18n.T("gui.select.busy"))
		a.closeNotice()
		return
	}
	o, _, err := a.collectOptions()
	if err != nil {
		a.log(err.Error())
		a.closeNotice()
		return
	}
	drives := o.DrivesSpec
	if drives == "" {
		drives = i18n.T("gui.scan.all-drives")
	}
	tier := o.Bucket
	if tier == "" {
		tier = i18n.Tf("gui.scan.auto-tier", o.MaxBuckets)
	}
	idx := a.bucket.CurrentIndex()
	if bucketIsCustom(idx) && o.Bucket == "" {
		a.log(i18n.T("gui.scan.custom-empty"))
	}
	label := tier
	list := bucketDisplay()
	if !bucketIsCustom(idx) && idx >= 0 && idx < len(list) {
		label = list[idx]
	}
	a.log(i18n.Tf("gui.scan.start", drives, label, limitText(o.MaxScan)))
	a.beginBusy("gui.sb.scanning")
	dir := o.StateDir
	go func() {
		start := time.Now()
		snap, err := engine.Scan(o)
		var listPath string
		if err == nil {
			tWrite := time.Now()
			err = store.Write(dir, snap.State)
			snap.Stats.Write = time.Since(tWrite)
			if err == nil {
				tCSV := time.Now()
				listPath, err = store.WriteListCSV(dir, snap.State)
				snap.Stats.CSV = time.Since(tCSV)
			}
		}
		elapsed := time.Since(start)
		a.mw.Synchronize(func() {
			a.endBusy()
			a.closeNotice()
			if err != nil {
				a.fail(i18n.Tf("gui.scan.fail", err))
				return
			}
			a.setState(snap.State)
			st := snap.State
			spent := elapsed.Round(10 * time.Millisecond)
			a.progKind = progLastScan
			a.progElapsed = spent.String()
			a.updateProgressLine()
			if len(st.Parts) == 0 {
				// 回收站本来就是空的：这是扫描完成，不是失败，别让它读起来像出错。
				a.ok(i18n.Tf("gui.scan.empty", spent))
			} else {
				a.ok(i18n.Tf("gui.scan.done", spent, len(st.Items),
					tty.Human(float64(totalSize(st))), len(st.Parts), len(st.Parts)))
			}
			a.log(snap.Stats.TimingText())
			if snap.Overflow != nil {
				a.warn(overflowText(snap.Overflow))
			}
			if len(st.Issues) > 0 {
				a.warn(i18n.Tf("gui.scan.issues", len(st.Issues)))
				for i, line := range st.Issues {
					if i == 5 {
						break
					}
					a.log("  - " + line)
				}
			}
			a.log(i18n.Tf("gui.scan.written", store.Path(dir), listPath))
			if len(st.Parts) == 0 {
				a.log(i18n.T("gui.scan.nothing"))
			} else {
				a.log(i18n.T("gui.scan.next"))
			}
		})
	}()
}

func limitText(n int) string {
	if n == 0 {
		return i18n.T("gui.limit.no-limit")
	}
	return strconv.Itoa(n)
}

// entriesOf 取某编号分区在（可选前缀过滤）之下的元素，走的是和删除同一条收窄逻辑。
func (a *app) entriesOf(pid int, prefix string) ([]entry, *parts.Part) {
	chosen := engine.SelectedParts(a.st, []int{pid}, prefix)
	if len(chosen) == 0 {
		return nil, nil
	}
	p := chosen[0]
	rows := make([]entry, 0, len(p.Positions))
	for _, pos := range p.Positions {
		rows = append(rows, entry{pos + 1, a.st.Items[pos]})
	}
	return rows, p
}

func (a *app) showCurrentDetail() {
	if a.st == nil {
		a.setItemRows(nil)
		a.setDetailTitle(func() string { return i18n.T("gui.detail.none") })
		return
	}
	row := a.partsTV.CurrentIndex()
	p := a.partRow(row)
	if p == nil {
		a.setItemRows(nil)
		// 重画表格会把当前行清成 -1，所以「有行但没当前行」和「视图真的空了」要分开说，
		// 否则点开【全选】之后右栏会怪到「只看未删完」头上。
		if n := len(a.partModel.rows); n > 0 {
			a.setDetailTitle(func() string { return i18n.Tf("gui.detail.pick-row", n) })
		} else {
			a.setDetailTitle(func() string { return i18n.T("gui.detail.empty-view") })
		}
		return
	}
	prefix := strings.TrimSpace(a.prefixBox.Text())
	rows, eff := a.entriesOf(p.PID, prefix)
	if eff == nil {
		a.setItemRows(nil)
		pid, label := p.PID, p.Label
		a.setDetailTitle(func() string { return i18n.Tf("gui.detail.no-match-prefix", pid, label, prefix) })
		return
	}
	// 标题的算法和命令行的 show 同一条键（cli.show.header）：两边的口径要一致，
	// 英文那条有 %s 槽，所以按 i18n.Noun 补单复数。
	count, size, pid := eff.Count, eff.Size, eff.PID
	timeRange, label, drives := eff.TimeRange, eff.Label, eff.Drives
	iStart, iEnd := eff.IndexStart, eff.IndexEnd
	a.setItemRows(rows)
	a.setDetailTitle(func() string {
		return i18n.Tf("cli.show.header", pid, timeRange, label,
			count, i18n.Noun(count, "个", "item", "items"), tty.Human(float64(size)),
			iStart, iEnd, drives)
	})
}

// checkedSelection 汇总勾选的分区（含前缀收窄）。返回的 ids 用于提示文案。
func (a *app) checkedSelection() (ids []int, chosen []*parts.Part, prefix string) {
	ids = a.partModel.checkedPIDs()
	prefix = strings.TrimSpace(a.prefixBox.Text())
	if len(ids) == 0 {
		return ids, nil, prefix
	}
	return ids, engine.SelectedParts(a.st, ids, prefix), prefix
}

func noMatchText(ids []int, prefix string) string {
	label := strings.Join(intsToStrings(ids), ",")
	if prefix == "" {
		prefix = i18n.T("gui.del.no-match-blank")
	}
	return i18n.Tf("gui.del.no-match", label, prefix)
}

func intsToStrings(xs []int) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = strconv.Itoa(x)
	}
	return out
}

// briefIDs 只列出前 max 个编号：【全选】120 个分区时把整串塞进确认框，
// 单行标签会把对话框撑得比屏幕还宽。
func briefIDs(ids []int, max int) string {
	if len(ids) <= max {
		return strings.Join(intsToStrings(ids), ",")
	}
	return i18n.Tf("gui.del.brief-more", strings.Join(intsToStrings(ids[:max]), ","), len(ids))
}

// previewDelete 是第一层闸门：只列清单，不碰文件（等价 --dry-run）。
func (a *app) previewDelete() {
	if a.st == nil {
		a.log(i18n.T("gui.select.no-snapshot"))
		return
	}
	ids, chosen, prefix := a.checkedSelection()
	if len(ids) == 0 {
		a.log(i18n.T("gui.del.no-tick"))
		return
	}
	if len(chosen) == 0 {
		a.log(noMatchText(ids, prefix))
		return
	}
	a.log(a.deleteSummary(chosen))
	rows := []entry{}
	for _, p := range chosen {
		for _, pos := range p.Positions {
			rows = append(rows, entry{pos + 1, a.st.Items[pos]})
		}
	}
	a.setItemRows(rows)
	partCount, itemCount := len(chosen), len(rows)
	a.setDetailTitle(func() string { return i18n.Tf("gui.del.preview-title", partCount, itemCount) })
	a.log(i18n.T("gui.del.preview-after"))
}

// summaryCols 是【预览删除】那张选中统计表的表头键与列宽。列宽沿用命令行手调的像素，
// 表头随语言取目录：英文表头一旦超过列宽就会被 tty.Truncate 削成「Time ra…」，
// gui_test 按这个成对表把两侧都钉住。
var summaryCols = []struct {
	key   string
	width int
}{
	{"cli.th.pid", 4},
	{"cli.th.range", 24},
	{"cli.th.group2", 44},
	{"cli.th.count", 7},
	{"cli.th.size", 9},
	{"cli.th.index", 12},
}

// deleteSummary 复刻命令行 delete 的选中统计（表头/口径一致，所以连键都共用）。
func (a *app) deleteSummary(chosen []*parts.Part) string {
	count := 0
	for _, p := range chosen {
		count += p.Count
	}
	ids := make([]string, len(chosen))
	rows := make([][]string, 0, len(chosen))
	for i, p := range chosen {
		ids[i] = strconv.Itoa(p.PID)
		rows = append(rows, []string{strconv.Itoa(p.PID), p.TimeRange, p.Label,
			strconv.Itoa(p.Count), tty.Human(float64(p.Size)),
			fmt.Sprintf("%d-%d", p.IndexStart, p.IndexEnd)})
	}
	headers := make([]string, len(summaryCols))
	widths := make([]int, len(summaryCols))
	for i, c := range summaryCols {
		headers[i], widths[i] = i18n.T(c.key), c.width
	}
	var b strings.Builder
	b.WriteString(i18n.Tf("cli.del.selected", len(chosen),
		i18n.Noun(len(chosen), "个", "partition", "partitions"), strings.Join(ids, ",")))
	b.WriteString("\n")
	b.WriteString(tty.Table(headers, rows, widths))
	b.WriteString("\n" + i18n.Tf("cli.del.will", count,
		i18n.Noun(count, "个条目", "item", "items"), tty.Human(float64(engine.TotalSize(chosen)))))
	return b.String()
}

// confirmPermanent 是第二层闸门：把规模、编号、快照路径和「不可恢复」一次列清，
// 点【确认永久删除】才动手（不再要求手打确认词）。不用 walk.MsgBox 是因为它只有「是/否」
// 两个等价的按钮，看不出哪一个是危险的；这里警示行用红字、规模用粗体，且不设默认按钮，
// 回车什么都不做，只有点按钮这条路。
func (a *app) confirmPermanent(partCount, itemCount int, size int64, ids []int, prefix string) bool {
	var dlg *walk.Dialog
	var cancelBtn, confirmBtn *walk.PushButton
	scale := i18n.Tf("gui.confirm.scale", partCount, itemCount, tty.Human(float64(size)))
	numbered := i18n.Tf("gui.confirm.ids", briefIDs(ids, 20))
	if prefix != "" {
		numbered += i18n.Tf("gui.confirm.prefix", prefix)
	}
	cancelText := i18n.T("gui.btn.cancel")
	confirmText := i18n.T("gui.btn.confirm-delete")
	err := Dialog{
		AssignTo:  &dlg,
		Title:     i18n.T("gui.confirm.title"),
		FixedSize: true,
		MinSize:   Size{Width: 520},
		Layout:    VBox{},
		Children: []Widget{
			TextLabel{Text: i18n.T("gui.confirm.head"), TextColor: walk.RGB(192, 0, 0)},
			TextLabel{Text: scale, Font: Font{Bold: true}},
			TextLabel{Text: numbered},
			TextLabel{Text: i18n.T("gui.confirm.warn"), TextColor: walk.RGB(192, 0, 0)},
			TextLabel{Text: i18n.Tf("gui.confirm.snapshot", store.Path(a.dir()))},
			TextLabel{Text: i18n.T("gui.confirm.ledger")},
			Composite{Layout: HBox{}, Children: []Widget{
				TextLabel{Text: i18n.T("gui.confirm.hint")},
				HSpacer{},
				PushButton{AssignTo: &cancelBtn, Text: cancelText,
					MaxSize:   Size{Width: widthFor(88, cancelText)},
					OnClicked: func() { dlg.Cancel() }},
				PushButton{AssignTo: &confirmBtn, Text: confirmText,
					MaxSize:   Size{Width: widthFor(120, confirmText)},
					OnClicked: func() { dlg.Accept() }},
			}},
		},
	}.Create(a.mw)
	if err != nil {
		a.fail(i18n.Tf("gui.confirm.create-fail", err))
		return false
	}
	defer dlg.Dispose()
	return dlg.Run() == walk.DlgCmdOK
}

// deleteSelected 走第二层（弹窗确认，等价 --yes）和第三层（保险丝，等价 --max）。
func (a *app) deleteSelected() {
	if a.st == nil {
		a.log(i18n.T("gui.select.no-snapshot"))
		return
	}
	if a.busy {
		a.log(i18n.T("gui.select.busy"))
		return
	}
	_, fuse, err := a.collectOptions()
	if err != nil {
		a.log(err.Error())
		return
	}
	ids, chosen, prefix := a.checkedSelection()
	if len(ids) == 0 {
		a.log(i18n.T("gui.del.no-tick"))
		return
	}
	if len(chosen) == 0 {
		a.log(noMatchText(ids, prefix))
		return
	}
	count := 0
	for _, p := range chosen {
		count += p.Count
	}
	size := engine.TotalSize(chosen)
	if !a.confirmPermanent(len(chosen), count, size, ids, prefix) {
		a.log(i18n.T("gui.del.cancelled"))
		return
	}

	dir := a.dir()
	a.log(i18n.Tf("gui.del.start", count, progressEvery))
	a.beginBusy("gui.sb.deleting")
	a.progress.SetRange(0, count)
	st := a.st
	go func() {
		rep, err := engine.RunDelete(st, chosen, dir, fuse, progressEvery, false,
			func(n, total, done, failed int) {
				a.mw.Synchronize(func() {
					if !a.busy {
						return
					}
					a.progress.SetValue(n)
					a.progN, a.progTotal, a.progDone, a.progFailed = n, total, done, failed
					a.progKind = progDeleteStep
					a.updateProgressLine()
					a.setHint(i18n.Tf("gui.sb.delete-progress", n, total, done, failed))
				})
			})
		a.mw.Synchronize(func() { a.finishDelete(rep, err, dir) })
	}()
}

func (a *app) finishDelete(rep *engine.Report, err error, dir string) {
	a.endBusy()
	if err != nil && rep == nil {
		// 保险丝拦下或账本打不开：一条都没动。
		a.warn(err.Error())
		a.warn(i18n.T("gui.del.nothing-done"))
		return
	}
	if err != nil {
		a.fail(i18n.Tf("gui.del.interrupted", err))
	}
	if rep != nil {
		a.progKind = progDeleteEnd
		a.progDone, a.progFailed = rep.Done, rep.Failed
		a.updateProgressLine()
		ledger := filepath.Join(dir, store.LedgerName)
		if rep.Failed == 0 {
			a.ok(i18n.Tf("gui.del.ok", rep.Done, rep.Failed, ledger))
		} else {
			a.warn(i18n.Tf("gui.del.partial", rep.Done, rep.Failed, ledger))
		}
		for i, line := range rep.Errors {
			if i == 20 {
				a.warn(i18n.Tf("gui.del.more-errors", len(rep.Errors)-i))
				break
			}
			a.warn(line)
		}
	}
	a.log(i18n.Tf("gui.del.state-updated", store.Path(dir)))
	a.log(i18n.T("gui.del.rescan-hint"))
	a.snapDeleted = true
	a.updateSnapshotLine()
	a.refreshParts(false)
}

func (a *app) applyIDs() {
	if a.st == nil {
		a.log(i18n.T("gui.select.no-snapshot"))
		return
	}
	spec := strings.TrimSpace(a.idsBox.Text())
	if spec == "" {
		a.log(i18n.T("gui.ids.help"))
		return
	}
	ids, err := tty.ParseIDs(spec, len(a.st.Parts))
	if err != nil {
		a.log(err.Error())
		return
	}
	want := map[int]bool{}
	for _, id := range ids {
		want[id] = true
	}
	checked := map[int]bool{}
	for _, p := range a.partModel.rows {
		if want[p.PID] {
			checked[p.PID] = true
		}
	}
	missing := []string{}
	for _, id := range ids {
		if !checked[id] {
			missing = append(missing, strconv.Itoa(id))
		}
	}
	a.partModel.checked = checked
	a.partModel.anchor = -1
	a.partModel.refresh()
	a.updateSelectionHint()
	if len(missing) > 0 {
		a.log(i18n.Tf("gui.ids.missing", len(checked), strings.Join(missing, ",")))
		return
	}
	a.log(i18n.Tf("gui.ids.applied", len(checked)))
}

// selectAll 是【全选】：勾上当前视图里所有还有未删条目的分区。
// 已删完的跳过 —— 它们的 $R/$I 已经不在了，勾上只会让引擎逐条报「找不到文件」。
func (a *app) selectAll() {
	if a.st == nil {
		a.log(i18n.T("gui.select.no-snapshot"))
		return
	}
	if a.partModel.RowCount() == 0 {
		a.log(i18n.T("gui.select.empty-view"))
		return
	}
	selected, skipped := a.partModel.selectAlive()
	a.partModel.refresh()
	a.updateSelectionHint()
	if selected == 0 {
		a.log(i18n.Tf("gui.select.all-deleted", skipped))
		return
	}
	if skipped > 0 {
		a.warn(i18n.Tf("gui.select.skipped", selected, skipped))
		return
	}
	a.ok(i18n.Tf("gui.select.ok", selected))
}

func (a *app) clearChecked() {
	n := a.partModel.clearChecks()
	a.partModel.refresh()
	a.updateSelectionHint()
	if n > 0 {
		a.log(i18n.Tf("gui.clear.done", n))
	}
}

// updateSelectionHint 把「此刻有多少东西」和「勾了要删多少」一次刷齐四处：
// ② 的四个总体数字、⑥ 的删除按钮文案、④ 的概览行、状态栏右侧。
// 这四处都是「按当前状态现算」的，所以换语言也走这里（applyLanguage 末尾调一次）。
func (a *app) updateSelectionHint() {
	if a.st == nil {
		a.setHint(i18n.T("gui.sb.hint"))
		_ = a.partsHint.SetText(i18n.T("gui.detail.none"))
		a.sumParts.SetText("—")
		a.sumItems.SetText("—")
		a.sumAlive.SetText("—")
		a.sumChecked.SetText(i18n.T("gui.sum.none"))
		a.setDeleteButton(0, 0)
		return
	}
	ids, chosen, _ := a.checkedSelection()
	count := 0
	size := int64(0)
	for _, p := range chosen {
		count += p.Count
		size += p.Size
	}
	aliveParts := 0
	for _, p := range a.st.Parts {
		if p.Deleted < p.Count {
			aliveParts++
		}
	}
	st := a.st
	total := totalSize(st)
	_ = a.sumParts.SetText(i18n.Tf("gui.sum.parts", len(st.Parts), len(st.Parts)))
	_ = a.sumItems.SetText(i18n.Tf("gui.sum.items", len(st.Items), tty.Human(float64(total))))
	_ = a.sumAlive.SetText(i18n.Tf("gui.sum.alive", st.AliveLeft(), aliveParts))
	_ = a.sumChecked.SetText(i18n.Tf("gui.sum.checked", len(chosen), count))
	a.setDeleteButton(len(chosen), count)
	_ = a.partsHint.SetText(i18n.Tf("gui.parts-hint.line",
		a.partModel.RowCount(), len(st.Parts), len(st.Parts),
		st.AliveLeft(), aliveParts, len(chosen), count, tty.Human(float64(size))))
	if len(ids) == 0 {
		a.setHint(i18n.Tf("gui.sb.no-tick", st.AliveLeft()))
		return
	}
	a.setHint(i18n.Tf("gui.sb.ticked", len(ids), len(chosen), count, tty.Human(float64(size))))
}

func (a *app) openStateDir() {
	dir := a.dir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		a.log(i18n.Tf("gui.open.mkdir-fail", err))
		return
	}
	verb, err := windows.UTF16PtrFromString("open")
	if err != nil {
		a.log(i18n.Tf("gui.open.fail", err))
		return
	}
	path, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		a.log(i18n.Tf("gui.open.fail", err))
		return
	}
	if err := windows.ShellExecute(windows.Handle(a.mw.Handle()), verb, path, nil, nil, windows.SW_SHOWNORMAL); err != nil {
		a.log(i18n.Tf("gui.open.fail", err))
		return
	}
	a.log(i18n.Tf("gui.opened", dir))
}

// showUsage 把说明装进一个可缩放、文字可框选的只读文本框。原来用 MsgBox：使用说明有 35 行
// （正文在 catalog_gui.go 的 gui.usage.body 里，中英各一份），弹窗既拉不高也没法整段复制。
// 【关闭】挂成 CancelButton，所以 Esc 也关得掉。
func (a *app) showUsage() {
	var dlg *walk.Dialog
	var closeBtn *walk.PushButton
	// 编辑控件只认 CRLF，先把目录里的 \n 统一一遍（同 emit 的处理）。
	body := strings.ReplaceAll(strings.ReplaceAll(i18n.T("gui.usage.body"), "\r\n", "\n"), "\n", "\r\n")
	closeText := i18n.T("gui.btn.close")
	err := Dialog{
		AssignTo:     &dlg,
		CancelButton: &closeBtn,
		Title:        i18n.T("gui.usage.title"),
		// 有布局的 Dialog 在 Show 时把窗口尺寸改成 max(内容最小尺寸, MinSize)（walk dialog.go:165-188），
		// 声明里的 Size 会被丢掉，所以初始大小只能写在 MinSize 上；拖边框放大照旧。
		MinSize: Size{Width: 760, Height: 520},
		Layout:  VBox{},
		Children: []Widget{
			TextEdit{
				ReadOnly: true, VScroll: true, HScroll: true, StretchFactor: 1,
				Font: Font{Family: "Consolas", PointSize: 9},
				Text: body,
			},
			Composite{Layout: HBox{}, Children: []Widget{
				TextLabel{Text: i18n.T("gui.usage.hint")},
				HSpacer{},
				PushButton{AssignTo: &closeBtn, Text: closeText,
					MinSize:   Size{Width: widthFor(88, closeText)},
					MaxSize:   Size{Width: widthFor(88, closeText)},
					OnClicked: func() { dlg.Cancel() }},
			}},
		},
	}.Create(a.mw)
	if err != nil {
		a.fail(i18n.Tf("gui.usage.create-fail", err))
		return
	}
	defer dlg.Dispose()
	// 焦点要在 Show 之前自己定好：walk 首次布局后会走 focusFirstCandidateDescendant
	// （container.go:448-465），给第一个 tab 停靠控件设焦点，紧接着延迟 1ms 调
	// SetTextSelection(0, -1)，于是整篇说明一打开就处于「全选高亮」状态。layout.go:325-333
	// 那段只在「焦点不在本对话框内」时才动手，所以这里先把焦点交给【关闭】：打开时没有选中，
	// Esc/空格照旧关窗，要复制就点进文字区框选再 Ctrl+C（实测把焦点给文本框反而会让第一次
	// 点击拖出一大段选中，故不这么做）。
	closeBtn.SetFocus()
	dlg.Run()
}

func (a *app) showAbout() {
	bin := meta.LocalDrives()
	walk.MsgBox(a.mw, i18n.T("gui.about.title"),
		i18n.Tf("gui.about.body", len(bin), strings.Join(bin, ",")),
		walk.MsgBoxOK|walk.MsgBoxIconInformation)
}
