// Package gui 是 rbm 的图形外壳：参数区 + 分区表 + 明细表 + 三层删除闸门 + 日志。
// 分区、编号、保险丝、账本这些判定全在 internal/engine，这里只负责展示与确认，
// 因此 GUI 和命令行看到的是同一份快照、同一套编号。
package gui

import (
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
	"github.com/lxn/win"
	"golang.org/x/sys/windows"

	"rbm/internal/engine"
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
	byValues     = []string{"dir", "prefix", "ext", "token"}
	byDisplay    = []string{"目录 dir（默认）", "字符前缀 prefix", "扩展名 ext", "文件名首段 token"}
	levelValues  = []string{"1", "2", "3", "full"}
	levelDisplay = []string{"1 只看所在目录名", "2 上级目录+目录（默认）", "3 更具体", "full 完整路径"}
	sortValues   = []string{"time", "size", "count"}
	sortDisplay  = []string{"按时间（默认）", "按大小", "按元素数"}
)

const usageText = `rbm 图形界面 = 命令行内核的一层薄壳，两边共用同一份快照文件。

流程（和命令树一一对应）:
  1) 窗口一打开自动扫描一次；【扫描 / 重扫】≈ rbm scan —— 按「时间桶 × 原路径
     分组」给条目编号，写出 state.json 与 scan_list.csv。改了上面的参数就重扫。
  2) 单击分区表的某一行 ≈ rbm show <编号> —— 右下方明细表列出该分区的内容。
  3) 在分区最左列点方框（或选中后按空格）勾选要删的分区 ≈ delete 1-5,8-10,16。
     按住 Shift 再点另一个方框，两点之间（含两端）一起勾选；
     【按编号勾选】输入 1-5,8-10,16 可以一次勾上一批。
  4) 【预览删除】≈ rbm delete <编号> --dry-run —— 只把清单打进明细表和日志，不动文件。
  5) 【永久删除】≈ rbm delete <编号> --yes —— 弹窗确认后真删，逐笔记入 ledger.csv。
     「删除保险丝」填 N：选中条目数超过 N 就整批不动（等价 --max N）。

两张表都可以点列名排序：点一下升序，再点同一列降序（表头有箭头）。
排序只动显示顺序，编号和勾选都不变。

前缀过滤 = 命令行的 --prefix：同时作用于明细表和删除选中的分区（在分区内再按文件名前缀收窄）。
只看未删完 = --alive，排序 = --sort time|size|count。

注意：删除是永久删除，不进回收站、不可恢复；编号只在当前这份快照里有效，
重新扫描会按当前内容重新编号。`

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
	// 分区参数
	bucket     *walk.LineEdit
	by         *walk.ComboBox
	dirLevel   *walk.ComboBox
	prefixLen  *walk.LineEdit
	maxBuckets *walk.LineEdit
	maxParts   *walk.LineEdit
	minCount   *walk.LineEdit
	drives     *walk.LineEdit
	maxScan    *walk.LineEdit
	fuse       *walk.LineEdit
	stateDir   *walk.LineEdit
	// 选择与视图
	idsBox    *walk.LineEdit
	prefixBox *walk.LineEdit
	aliveOnly *walk.CheckBox
	sortBy    *walk.ComboBox
	// 表与文本区
	partsTV     *walk.TableView
	itemsTV     *walk.TableView
	partModel   *partModel
	itemModel   *itemModel
	partsHint   *walk.TextLabel
	detailTitle *walk.TextLabel
	logBox      *walk.TextEdit
	progress    *walk.ProgressBar
	scanBtn     *walk.PushButton
	previewBtn  *walk.PushButton
	deleteBtn   *walk.PushButton
	sbSnapshot  *walk.StatusBarItem
	sbHint      *walk.StatusBarItem
	// 状态
	st   *store.State
	busy bool
	lock []walk.Window // 后台任务期间禁用的控件
}

// Run 打开主窗口并进入消息循环，窗口关闭后返回进程退出码。
func Run() int {
	a := &app{}
	if err := a.build(); err != nil {
		fmt.Fprintln(os.Stderr, "图形界面创建失败:", err)
		return 1
	}
	// 双击打开的是控制台子系统的 exe，系统会先给一个黑窗口；界面已经建好了，
	// 这里把控制台甩掉，之后的日志只走窗口里的日志区。命令行分支不经过这里。
	procFreeConsole.Call()
	a.mw.Run()
	return 0
}

func (a *app) build() error {
	a.partModel = newPartModel(a)
	a.itemModel = newItemModel()

	spec := MainWindow{
		AssignTo: &a.mw,
		Title:    "回收站分批管理 rbm",
		MinSize:  Size{Width: 1080, Height: 680},
		Size:     Size{Width: 1400, Height: 860},
		Layout:   VBox{},
		MenuItems: []MenuItem{
			Menu{Text: "文件(&F)", Items: []MenuItem{
				Action{Text: "扫描 / 重扫", Shortcut: Shortcut{Key: walk.KeyF5}, OnTriggered: func() { a.startScan() }},
				Action{Text: "打开快照目录", OnTriggered: func() { a.openStateDir() }},
				Separator{},
				Action{Text: "退出", OnTriggered: func() { a.mw.Close() }},
			}},
			Menu{Text: "帮助(&H)", Items: []MenuItem{
				Action{Text: "使用说明", OnTriggered: func() { a.showUsage() }},
				Action{Text: "关于 rbm", OnTriggered: func() { a.showAbout() }},
			}},
		},
		StatusBarItems: []StatusBarItem{
			{AssignTo: &a.sbSnapshot, Text: "尚未扫描", Width: 520},
			{AssignTo: &a.sbHint, Text: "F5 扫描；勾选分区后先预览再删除", Width: 360},
		},
		Children: []Widget{a.paramsBox(), a.actionRow(), a.filterRow(), a.tables(), a.bottom()},
	}
	if err := spec.Create(); err != nil {
		return err
	}

	a.by.SetCurrentIndex(0)
	a.dirLevel.SetCurrentIndex(1)
	a.sortBy.SetCurrentIndex(0)
	a.prefixLen.SetText(strconv.Itoa(defPrefixLen))
	a.maxBuckets.SetText(strconv.Itoa(defMaxBuckets))
	a.maxParts.SetText(strconv.Itoa(defMaxParts))
	a.minCount.SetText(strconv.Itoa(defMinCount))
	a.maxScan.SetText(strconv.Itoa(defMaxScan))
	a.fuse.SetText("0")
	dir := store.DefaultStateDir()
	a.stateDir.SetText(dir)

	a.lock = []walk.Window{
		a.scanBtn, a.previewBtn, a.deleteBtn, a.bucket, a.by, a.dirLevel, a.prefixLen,
		a.maxBuckets, a.maxParts, a.minCount, a.drives, a.maxScan, a.fuse, a.stateDir,
		a.idsBox, a.prefixBox, a.aliveOnly, a.sortBy,
	}

	a.partsTV.CurrentIndexChanged().Attach(func() { a.showCurrentDetail() })

	a.log("rbm 图形界面：窗口一打开就按当前回收站自动扫描一次，参数改完点【扫描 / 重扫】(F5)。")
	a.updateSelectionHint()
	// 不载入上次的快照：界面要显示回收站此刻的内容，编号也按此刻内容重建。
	a.startScan()
	return nil
}

// ---------- 界面搭建 ----------

func (a *app) paramsBox() Widget {
	return GroupBox{
		Title:  "分区参数（改完点【扫描 / 重扫】生效，数字留空按默认值）",
		Layout: VBox{},
		Children: []Widget{
			Composite{Layout: HBox{}, Children: []Widget{
				TextLabel{Text: "时间桶"},
				LineEdit{AssignTo: &a.bucket, MaxSize: Size{Width: 330},
					CueBanner: "留空=自动断点 gap；可填 daily/hourly/monthly/3mo/30m/gap:5m/auto"},
				TextLabel{Text: "桶内分组"},
				ComboBox{AssignTo: &a.by, Model: byDisplay, MaxSize: Size{Width: 160}},
				TextLabel{Text: "目录层级"},
				ComboBox{AssignTo: &a.dirLevel, Model: levelDisplay, MaxSize: Size{Width: 200}},
				TextLabel{Text: "前缀长度"},
				LineEdit{AssignTo: &a.prefixLen, MaxSize: Size{Width: 56}},
				HSpacer{},
			}},
			Composite{Layout: HBox{}, Children: []Widget{
				TextLabel{Text: "桶数上限"},
				LineEdit{AssignTo: &a.maxBuckets, MaxSize: Size{Width: 56}},
				TextLabel{Text: "分区上限"},
				LineEdit{AssignTo: &a.maxParts, MaxSize: Size{Width: 64}},
				TextLabel{Text: "小组并入"},
				LineEdit{AssignTo: &a.minCount, MaxSize: Size{Width: 56}},
				TextLabel{Text: "限定盘符"},
				LineEdit{AssignTo: &a.drives, MaxSize: Size{Width: 90}, CueBanner: "留空=全部"},
				TextLabel{Text: "单次扫描上限"},
				LineEdit{AssignTo: &a.maxScan, MaxSize: Size{Width: 72}},
				TextLabel{Text: "删除保险丝"},
				LineEdit{AssignTo: &a.fuse, MaxSize: Size{Width: 72}, CueBanner: "0=不限"},
				HSpacer{},
			}},
			Composite{Layout: HBox{}, Children: []Widget{
				TextLabel{Text: "快照目录"},
				LineEdit{AssignTo: &a.stateDir, StretchFactor: 1,
					ToolTipText: "state.json / scan_list.csv / ledger.csv 所在目录，命令行 --state-dir 指同一个目录"},
			}},
		},
	}
}

func (a *app) actionRow() Widget {
	return Composite{Layout: HBox{}, Children: []Widget{
		PushButton{AssignTo: &a.scanBtn, Text: "扫描 / 重扫 (F5)", MaxSize: Size{Width: 150},
			OnClicked: func() { a.startScan() }},
		PushButton{AssignTo: &a.previewBtn, Text: "预览删除（不动文件）", MaxSize: Size{Width: 180},
			OnClicked: func() { a.previewDelete() }},
		PushButton{AssignTo: &a.deleteBtn, Text: "永久删除勾选的分区", MaxSize: Size{Width: 190},
			ToolTipText: "真删：不进回收站、不可恢复。会先弹确认框，并受「删除保险丝」约束。",
			OnClicked:   func() { a.deleteSelected() }},
		HSpacer{},
	}}
}

func (a *app) filterRow() Widget {
	return Composite{Layout: HBox{}, Children: []Widget{
		TextLabel{Text: "按编号勾选"},
		LineEdit{AssignTo: &a.idsBox, MaxSize: Size{Width: 160}, CueBanner: "如 1-5,8-10,16"},
		PushButton{Text: "应用", MaxSize: Size{Width: 64}, OnClicked: func() { a.applyIDs() }},
		PushButton{Text: "清空勾选", MaxSize: Size{Width: 92}, OnClicked: func() { a.clearChecked() }},
		TextLabel{Text: "文件名前缀过滤"},
		LineEdit{AssignTo: &a.prefixBox, MaxSize: Size{Width: 160}, CueBanner: "空=不过滤（同 --prefix）",
			OnEditingFinished: func() { a.showCurrentDetail() }},
		CheckBox{AssignTo: &a.aliveOnly, Text: "只看未删完", OnCheckedChanged: func() { a.refreshParts() }},
		TextLabel{Text: "排序"},
		ComboBox{AssignTo: &a.sortBy, Model: sortDisplay, MaxSize: Size{Width: 130},
			OnCurrentIndexChanged: func() { a.refreshParts() }},
		HSpacer{},
	}}
}

func (a *app) tables() Widget {
	return HSplitter{
		Children: []Widget{
			GroupBox{
				Title:  "分区统计（点方框勾选=要删的分区，按住 Shift 点另一个可以连选一段；单击行看明细）",
				Layout: VBox{},
				// 这行标签和右边明细表的标题行同高，两张表的表头才能对齐。
				Children: []Widget{
					TextLabel{AssignTo: &a.partsHint, Text: "（还没有快照）"},
					TableView{
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
						Columns: []TableViewColumn{
							{Title: "编号", Width: 44, Alignment: AlignFar},
							{Title: "时间范围", Width: 168},
							{Title: "路径组", Width: 156},
							{Title: "元素数", Width: 52, Alignment: AlignFar},
							{Title: "大小", Width: 64, Alignment: AlignFar},
							{Title: "index区间", Width: 74, Alignment: AlignFar},
							{Title: "盘", Width: 32},
							{Title: "剩余", Width: 52, Alignment: AlignFar},
							{Title: "组内构成/示例", Width: 118},
						},
					}},
				StretchFactor: 3,
			},
			GroupBox{
				Title:  "元素明细",
				Layout: VBox{},
				Children: []Widget{
					TextLabel{AssignTo: &a.detailTitle, Text: "（单击左侧某一行看它的元素清单）"},
					TableView{
						AssignTo:            &a.itemsTV,
						Model:               a.itemModel,
						AlternatingRowBG:    true,
						LastColumnStretched: true,
						MinSize:             Size{Width: 420, Height: 180},
						Columns: []TableViewColumn{
							{Title: "index", Width: 48, Alignment: AlignFar},
							{Title: "删除时间", Width: 124},
							{Title: "大小", Width: 60, Alignment: AlignFar},
							{Title: "盘", Width: 32},
							{Title: "文件名", Width: 130},
							{Title: "原路径", Width: 112},
						},
					},
				},
				StretchFactor: 2,
			},
		},
		StretchFactor: 3,
	}
}

func (a *app) bottom() Widget {
	return Composite{
		Layout: VBox{},
		Children: []Widget{
			ProgressBar{AssignTo: &a.progress, MinValue: 0, MaxValue: 100},
			TextEdit{AssignTo: &a.logBox, ReadOnly: true, VScroll: true, MinSize: Size{Height: 120},
				Font: Font{Family: "Consolas", PointSize: 9}, StretchFactor: 1},
		},
	}
}

// ---------- 通用 ----------

// log 往日志区追加一行。多行文本要转成 CRLF，Windows 编辑控件才换行。
func (a *app) log(text string) {
	a.logBox.AppendText(strings.ReplaceAll(text, "\n", "\r\n") + "\r\n")
	a.logBox.ScrollToCaret()
}

func (a *app) logf(format string, args ...interface{}) { a.log(fmt.Sprintf(format, args...)) }

func (a *app) partRow(row int) *parts.Part {
	if row < 0 || row >= len(a.partModel.rows) {
		return nil
	}
	return a.partModel.rows[row]
}

func (a *app) beginBusy(text string) {
	a.busy = true
	for _, w := range a.lock {
		w.SetEnabled(false)
	}
	a.progress.SetValue(0)
	a.sbHint.SetText(text)
}

func (a *app) endBusy() {
	for _, w := range a.lock {
		w.SetEnabled(true)
	}
	a.busy = false
	a.updateSelectionHint()
}

func (a *app) dir() string {
	d := strings.TrimSpace(a.stateDir.Text())
	if d == "" {
		return store.DefaultStateDir()
	}
	return d
}

// intValue 读一个数字输入框；留空取默认值，填错返回可直接打印的错误。
func intValue(le *walk.LineEdit, name string, def int) (int, error) {
	s := strings.TrimSpace(le.Text())
	if s == "" {
		return def, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("「%s」要填整数，现在填的是 %q", name, s)
	}
	if n < 0 {
		return 0, fmt.Errorf("「%s」不能是负数，现在填的是 %d", name, n)
	}
	return n, nil
}

// collectOptions 把界面上的参数拼成引擎的入参。
func (a *app) collectOptions() (engine.Options, int, error) {
	prefixLen, err := intValue(a.prefixLen, "前缀长度", defPrefixLen)
	if err != nil {
		return engine.Options{}, 0, err
	}
	maxBuckets, err := intValue(a.maxBuckets, "桶数上限", defMaxBuckets)
	if err != nil {
		return engine.Options{}, 0, err
	}
	maxParts, err := intValue(a.maxParts, "分区上限", defMaxParts)
	if err != nil {
		return engine.Options{}, 0, err
	}
	minCount, err := intValue(a.minCount, "小组并入", defMinCount)
	if err != nil {
		return engine.Options{}, 0, err
	}
	maxScan, err := intValue(a.maxScan, "单次扫描上限", defMaxScan)
	if err != nil {
		return engine.Options{}, 0, err
	}
	fuse, err := intValue(a.fuse, "删除保险丝", 0)
	if err != nil {
		return engine.Options{}, 0, err
	}
	drivesSpec := strings.TrimSpace(a.drives.Text())
	o := engine.Options{
		Bucket:     strings.TrimSpace(a.bucket.Text()),
		By:         byValues[a.by.CurrentIndex()],
		DirLevel:   levelValues[a.dirLevel.CurrentIndex()],
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
			return o, fuse, fmt.Errorf("「前缀长度」在 %s 分组下至少 1 个字符", o.By)
		}
	}
	return o, fuse, nil
}

// overflowText 复刻命令行的超限提示：截断必须显式说清，不能静默丢掉条目。
func overflowText(ov *meta.Overflow) string {
	var b strings.Builder
	fmt.Fprintf(&b, "! 回收站共有 %d 个条目，超过本次「单次扫描上限」%d，快照只建了其中最老的 %d 个"+
		"（按 $I 写入时间排序，约等于删除时间），另有 %d 个未纳入。\n",
		ov.Total, ov.Taken, ov.Taken, ov.Total-ov.Taken)
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
	fmt.Fprintf(&b, "  各盘 已扫/总数: %s\n", strings.Join(cells, ", "))
	b.WriteString("  两种继续办法:\n")
	fmt.Fprintf(&b, "    A) 一次看全 —— 把「单次扫描上限」调到 %d 以上再重扫\n", (ov.Total/10000+1)*10000)
	b.WriteString("    B) 分批推进（推荐）—— 先删掉本快照里的分区，再重新扫描，剩下的条目会自动进新快照并重新编号")
	return b.String()
}

func (a *app) setState(st *store.State) {
	a.st = st
	a.refreshParts()
	a.sbSnapshot.SetText(fmt.Sprintf("快照 %s｜条目 %d 个 / %s｜分区 %d 个（编号 1..%d）｜未删完 %d",
		st.Created, len(st.Items), tty.Human(float64(totalSize(st))), len(st.Parts), len(st.Parts), st.AliveLeft()))
	if a.partModel.RowCount() > 0 {
		a.partsTV.SetCurrentIndex(0)
	}
	a.showCurrentDetail()
}

func totalSize(st *store.State) int64 {
	var n int64
	for _, p := range st.Parts {
		n += p.Size
	}
	return n
}

// refreshParts 按「只看未删完 / 排序」重画分区表。视图一变，勾选一律清空：
// 行号和勾选的对应关系变了，留着旧勾选可能删错批次。
func (a *app) refreshParts() {
	if a.st == nil {
		a.partModel.setRows(nil)
		return
	}
	a.partModel.setRows(engine.SortedParts(a.st, sortValues[a.sortBy.CurrentIndex()], a.aliveOnly.Checked()))
	if len(a.st.Parts) > 0 {
		if err := a.partsTV.Columns().At(2).SetTitle(a.st.GroupCol()); err != nil {
			a.logf("改列名失败: %v", err)
		}
	}
	a.log("（分区表已刷新，勾选已清空，请重新勾选要删的分区）")
	a.updateSelectionHint()
	a.showCurrentDetail()
}

// ---------- 动作 ----------

func (a *app) startScan() {
	if a.busy {
		a.log("上一次任务还在跑，请等它结束。")
		return
	}
	o, _, err := a.collectOptions()
	if err != nil {
		a.log(err.Error())
		return
	}
	drives := o.DrivesSpec
	if drives == "" {
		drives = "全部盘"
	}
	a.log(fmt.Sprintf("开始扫描（%s，单次上限 %s）…", drives, limitText(o.MaxScan)))
	a.beginBusy("扫描中…")
	dir := o.StateDir
	go func() {
		start := time.Now()
		snap, err := engine.Scan(o)
		var listPath string
		if err == nil {
			if err = store.Write(dir, snap.State); err == nil {
				listPath, err = store.WriteListCSV(dir, snap.State)
			}
		}
		elapsed := time.Since(start)
		a.mw.Synchronize(func() {
			a.endBusy()
			if err != nil {
				a.logf("扫描失败: %v", err)
				return
			}
			a.setState(snap.State)
			st := snap.State
			a.log(fmt.Sprintf("扫描完成（用时 %s）：条目 %d 个 / %s，分区 %d 个，编号 1..%d。",
				elapsed.Round(10*time.Millisecond), len(st.Items), tty.Human(float64(totalSize(st))),
				len(st.Parts), len(st.Parts)))
			if snap.Overflow != nil {
				a.log(overflowText(snap.Overflow))
			}
			if len(st.Issues) > 0 {
				a.log(fmt.Sprintf("异常记录 %d 条（全量在 scan_list.csv），示例:", len(st.Issues)))
				for i, line := range st.Issues {
					if i == 5 {
						break
					}
					a.log("  - " + line)
				}
			}
			a.log(fmt.Sprintf("快照已写出: %s\n清单已写出: %s", store.Path(dir), listPath))
			a.log("下一步：勾选分区 →【预览删除】看清单 →【永久删除】确认。")
		})
	}()
}

func limitText(n int) string {
	if n == 0 {
		return "不限"
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
		a.itemModel.setRows(nil)
		a.detailTitle.SetText("（还没有快照：点【扫描 / 重扫】）")
		return
	}
	row := a.partsTV.CurrentIndex()
	p := a.partRow(row)
	if p == nil {
		a.itemModel.setRows(nil)
		a.detailTitle.SetText("（当前视图里没有可选的分区；换掉「只看未删完」或重新扫描）")
		return
	}
	prefix := strings.TrimSpace(a.prefixBox.Text())
	rows, eff := a.entriesOf(p.PID, prefix)
	if eff == nil {
		a.itemModel.setRows(nil)
		a.detailTitle.SetText(fmt.Sprintf("分区 %d｜%s｜前缀过滤 %q 之后 0 个", p.PID, p.Label, prefix))
		return
	}
	title := fmt.Sprintf("分区 %d｜%s｜%s｜%d 个 / %s｜index %d-%d｜盘 %s",
		eff.PID, eff.TimeRange, eff.Label, eff.Count, tty.Human(float64(eff.Size)),
		eff.IndexStart, eff.IndexEnd, eff.Drives)
	a.itemModel.setRows(rows)
	a.detailTitle.SetText(title)
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
		prefix = "(空)"
	}
	return fmt.Sprintf("没有匹配项：编号 %s 加上前缀过滤 %s 后为空，未做任何改动。", label, prefix)
}

func intsToStrings(xs []int) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = strconv.Itoa(x)
	}
	return out
}

// previewDelete 是第一层闸门：只列清单，不碰文件（等价 --dry-run）。
func (a *app) previewDelete() {
	if a.st == nil {
		a.log("还没有快照，先点【扫描 / 重扫】。")
		return
	}
	ids, chosen, prefix := a.checkedSelection()
	if len(ids) == 0 {
		a.log("没有勾选分区：在分区表最左列点方框（或选中后按空格）勾上要删的分区，也可以用【按编号勾选】。")
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
	a.itemModel.setRows(rows)
	a.detailTitle.SetText(fmt.Sprintf("预览：选中 %d 个分区 / %d 个条目（明细列在下面，未做任何改动）",
		len(chosen), len(rows)))
	a.log("预览未做任何改动。要真删请点【永久删除】并在弹窗里确认。")
}

// deleteSummary 复刻命令行 delete 的选中统计（表头/口径一致）。
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
	var b strings.Builder
	fmt.Fprintf(&b, "选中分区 %d 个: %s\n", len(chosen), strings.Join(ids, ","))
	b.WriteString(tty.Table([]string{"编号", "时间范围", "路径组/前缀组", "元素数", "大小", "index区间"},
		rows, []int{4, 24, 44, 7, 9, 12}))
	fmt.Fprintf(&b, "\n本次将永久删除 %d 个条目，约 %s（不可恢复）", count,
		tty.Human(float64(engine.TotalSize(chosen))))
	return b.String()
}

// deleteSelected 走第二层（弹窗确认，等价 --yes）和第三层（保险丝，等价 --max）。
func (a *app) deleteSelected() {
	if a.st == nil {
		a.log("还没有快照，先点【扫描 / 重扫】。")
		return
	}
	if a.busy {
		a.log("上一次任务还在跑，请等它结束。")
		return
	}
	_, fuse, err := a.collectOptions()
	if err != nil {
		a.log(err.Error())
		return
	}
	ids, chosen, prefix := a.checkedSelection()
	if len(ids) == 0 {
		a.log("没有勾选分区：在分区表最左列点方框（或选中后按空格）勾上要删的分区，也可以用【按编号勾选】。")
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
	msg := fmt.Sprintf("将永久删除 %d 个条目，约 %s。\n\n"+
		"这些内容直接从 $Recycle.Bin 抹掉，不进回收站、无法恢复。\n"+
		"选中分区编号：%s%s\n快照：%s\n\n建议先点【预览删除】核对清单。现在确定要删吗？",
		count, tty.Human(float64(size)), strings.Join(intsToStrings(ids), ","),
		func() string {
			if prefix == "" {
				return ""
			}
			return "（前缀 " + prefix + "*）"
		}(),
		store.Path(a.dir()))
	if walk.MsgBox(a.mw, "永久删除确认", msg,
		walk.MsgBoxYesNo|walk.MsgBoxIconWarning|walk.MsgBoxDefButton2) != win.IDYES {
		a.log("已取消，未做任何改动。")
		return
	}

	dir := a.dir()
	a.log(fmt.Sprintf("开始删除 %d 个条目…（每 %d 个刷一次进度）", count, progressEvery))
	a.beginBusy("删除中…")
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
					a.sbHint.SetText(fmt.Sprintf("删除中 %d/%d  已删 %d  失败 %d", n, total, done, failed))
				})
			})
		a.mw.Synchronize(func() { a.finishDelete(rep, err, dir) })
	}()
}

func (a *app) finishDelete(rep *engine.Report, err error, dir string) {
	a.endBusy()
	a.progress.SetValue(a.progress.MaxValue())
	if err != nil && rep == nil {
		// 保险丝拦下或账本打不开：一条都没动。
		a.log(err.Error())
		a.log("未做任何改动。")
		return
	}
	if err != nil {
		a.logf("删除中断: %v", err)
	}
	if rep != nil {
		a.log(fmt.Sprintf("\n完成：删除 %d 个，失败 %d 个，账本 %s",
			rep.Done, rep.Failed, filepath.Join(dir, store.LedgerName)))
		for i, line := range rep.Errors {
			if i == 20 {
				a.logf("  ! …另有 %d 条失败记录，见账本", len(rep.Errors)-i)
				break
			}
			a.log("  ! " + line)
		}
	}
	a.log(fmt.Sprintf("快照已更新（%s）：标记了本次删掉的条目，编号不变。", store.Path(dir)))
	a.log("要看回收站最新状态请重新扫描：编号按当前内容重建。")
	if a.st != nil {
		a.sbSnapshot.SetText(fmt.Sprintf("快照 %s｜未删完 %d｜最近删除 %s",
			a.st.Created, a.st.AliveLeft(), a.st.LastDelete))
	}
	a.refreshParts()
}

func (a *app) applyIDs() {
	if a.st == nil {
		a.log("还没有快照，先点【扫描 / 重扫】。")
		return
	}
	spec := strings.TrimSpace(a.idsBox.Text())
	if spec == "" {
		a.log("「按编号勾选」里填编号写法，例如 1-5,8-10,16。")
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
		a.logf("已勾选 %d 个分区；编号 %s 不在当前视图里（被「只看未删完」或分区上限挡着了），需要的话先取消筛选。",
			len(checked), strings.Join(missing, ","))
		return
	}
	a.logf("已勾选 %d 个分区。", len(checked))
}

func (a *app) clearChecked() {
	a.partModel.checked = map[int]bool{}
	a.partModel.anchor = -1
	a.partModel.refresh()
	a.updateSelectionHint()
}

// updateSelectionHint 同时刷两处：左表上方的概览行（也是和明细表标题配对的等高一行）
// 和状态栏的勾选统计。
func (a *app) updateSelectionHint() {
	if a.st == nil {
		a.sbHint.SetText("F5 扫描；勾选分区后先预览再删除")
		a.partsHint.SetText("（还没有快照：点【扫描 / 重扫】）")
		return
	}
	ids, chosen, _ := a.checkedSelection()
	count := 0
	size := int64(0)
	for _, p := range chosen {
		count += p.Count
		size += p.Size
	}
	a.partsHint.SetText(fmt.Sprintf("当前视图 %d 行｜快照共 %d 个分区（编号 1..%d）｜未删完 %d 个｜勾选 %d 个 / %d 个条目 %s",
		a.partModel.RowCount(), len(a.st.Parts), len(a.st.Parts), a.st.AliveLeft(),
		len(chosen), count, tty.Human(float64(size))))
	if len(ids) == 0 {
		a.sbHint.SetText(fmt.Sprintf("未勾选分区｜未删完 %d 个", a.st.AliveLeft()))
		return
	}
	a.sbHint.SetText(fmt.Sprintf("已勾选 %d 个编号 / %d 个分区 / %d 个条目 %s",
		len(ids), len(chosen), count, tty.Human(float64(size))))
}

func (a *app) openStateDir() {
	dir := a.dir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		a.logf("建快照目录失败: %v", err)
		return
	}
	verb, err := windows.UTF16PtrFromString("open")
	if err != nil {
		a.logf("打开目录失败: %v", err)
		return
	}
	path, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		a.logf("打开目录失败: %v", err)
		return
	}
	if err := windows.ShellExecute(windows.Handle(a.mw.Handle()), verb, path, nil, nil, windows.SW_SHOWNORMAL); err != nil {
		a.logf("打开目录失败: %v", err)
		return
	}
	a.logf("已在资源管理器里打开: %s", dir)
}

func (a *app) showUsage() {
	walk.MsgBox(a.mw, "rbm 使用说明", usageText, walk.MsgBoxOK|walk.MsgBoxIconInformation)
}

func (a *app) showAbout() {
	bin := meta.LocalDrives()
	walk.MsgBox(a.mw, "关于 rbm",
		fmt.Sprintf("回收站分批管理工具 rbm（图形界面）\n\n"+
			"内核：$Recycle.Bin\\<当前用户SID> 的 $I 元数据直读 → 时间桶 × 原路径分组 → 编号 → 分批永久删除。\n"+
			"本机盘符 %d 个（%s）；命令行等价用法：rbm scan / parts / show / delete / shell。\n"+
			"删除不可恢复，请配合【预览删除】和「删除保险丝」使用。",
			len(bin), strings.Join(bin, ",")),
		walk.MsgBoxOK|walk.MsgBoxIconInformation)
}
