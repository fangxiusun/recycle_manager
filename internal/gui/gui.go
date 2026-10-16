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
	// bucketValues 与 bucketDisplay 一一对应，最后一项是「自定义」，它不给固定写法，
	// 而是把手填框里的文本原样交给 parts.ParseSpec（所以这里放空串占位）。
	bucketValues = []string{"", "gap:15m", "gap:1h", "gap:1d",
		"15m", "30m", "hourly", "daily", "3d", "weekly", "14d",
		"monthly", "3mo", "6mo", "auto", ""}
	bucketDisplay = []string{
		"自动找删除空档（默认）",
		"只在 ≥15 分钟的空档切开（gap:15m）",
		"只在 ≥1 小时的空档切开（gap:1h）",
		"只在 ≥1 天的空档切开（gap:1d）",
		"每 15 分钟一批（15m）",
		"每 30 分钟一批（30m）",
		"每 1 小时一批（hourly）",
		"每 1 天一批（daily）",
		"每 3 天一批（3d）",
		"每 7 天一批（weekly）",
		"每 14 天一批（14d）",
		"每 1 个自然月一批（monthly）",
		"每 3 个自然月一批（3mo）",
		"每 6 个自然月一批（6mo）",
		"自动挑最合适的固定粒度（auto）",
		"自定义：手填写法",
	}
)

// bucketIsCustom 说清下拉第 idx 项是不是「自定义」那一项（最后一项）。
func bucketIsCustom(idx int) bool { return idx == len(bucketDisplay)-1 }

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
		return "⚠ 永久删除（未勾选分区）"
	}
	return fmt.Sprintf("⚠ 永久删除 %d 个分区 / %d 个条目", parts, items)
}

// aliveFilterText 解释「只看未删完」这次切换到底改变了什么。这个筛选只隐藏「已经删完」
// 的分区，而刚扫完的快照里一个都没有 —— 不解释的话界面就像点了没反应（本机实测反馈）。
func aliveFilterText(on bool, total, shown int) string {
	hidden := total - shown
	state := "已关闭（显示全部编号）"
	if on {
		state = "已开启（只显示还有未删条目的分区）"
	}
	if on && hidden == 0 {
		return fmt.Sprintf("只看未删完：%s。本快照里还没有「已删完」的分区（%d 个分区全部还有内容），"+
			"所以视图行数不变。删完一批之后再开，就能看到已清空的分区从列表里消失。", state, total)
	}
	return fmt.Sprintf("只看未删完：%s。快照 %d 个分区，隐藏已删完 %d 个，视图 %d 行。",
		state, total, hidden, shown)
}

const usageText = `rbm 图形界面 = 命令行内核的一层薄壳，两边共用同一份快照文件。

界面按工作流分成六段，从上往下依次操作：

  ① 扫描配置 —— 按工作流只有三样常用项：
     「按时间分批」= 原来的时间桶，下拉里是「自动找删除空档（默认）/ 每 15 分钟一批 / 每天一批 /
     每 7 天一批 / 每 3 个自然月一批 …」，最后一项「自定义」才露出手填框（写 daily、30m、3mo、
     gap:5m、auto 这种原始写法）。
     「桶内分组」= 每批里按什么归堆，四项：目录 dir（默认，按原来所在目录）/ 字符前缀 prefix
     （文件名前几个字符）/ 扩展名 ext / 文件名首段 token（下划线、连字符、点之前那段）。
     只有对当前分组生效的参数才显示：「目录层级」在 目录 dir 下用，「前缀长度」在 字符前缀 prefix /
     文件名首段 token 下用，切分组时另一项会收起。
     其余六项（桶数上限、分区上限、小组并入、限定盘符、单次扫描上限、删除保险丝）
     和快照目录一起收在【▼ 高级参数】里，点开展开、点回收起；收起时值照旧生效。
  ② 扫描结果 —— 【扫描 / 重扫】≈ rbm scan：先按【按时间分批】切批、再按【桶内分组】归堆，
     然后给条目编号，写出 state.json 与 scan_list.csv；右边四个数字是此刻的总体状态
     （分区数 / 对象数 / 未删完 / 已勾选）。改了参数就重扫。
  ③ 选择分区 —— 【全选】勾上当前视图里所有还有内容的分区，【清空勾选】撤销；
     也可以在分区表最左列点方框（选中后按空格也行），按住 Shift 点另一个连选一段，
     或在【按编号勾选】填 1-5,8-10,16 一次勾上一批 ≈ delete 1-5,8-10,16。
     文件名前缀过滤 = 命令行的 --prefix；只看未删完 = --alive，只隐藏「已删完」的分区，
     刚扫完的快照没有已删完分区时它不会改变行数，日志会说明原因。
  ④ 分区列表 —— 单击某一行 ≈ rbm show <编号>，右下方列出该分区的元素。
  ⑤ 元素明细 —— 没选分区时是空的，选了就给 index / 删除时间 / 大小 / 盘 / 文件名 / 原路径。
  ⑥ 删除操作 —— 【预览删除】≈ rbm delete <编号> --dry-run，只列清单不动文件；
     【⚠ 永久删除 N 个分区 / M 个条目】≈ rbm delete <编号> --yes，
     要在这个弹窗里点【确认永久删除】才会真删，逐笔记入 ledger.csv。
     「删除保险丝」填 N：选中条目数超过 N 就整批不动（等价 --max N）。

两张表都能点列名排序：点一下升序，再点同一列降序（表头有箭头）。排序只动显示顺序，
编号和勾选都不变，所以原来的「排序」下拉框已经去掉。
换「只看未删完」会清空勾选（视图变了，留着旧勾选可能删错批次）；点表头排序不清空。

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
		Size:     Size{Width: 1400, Height: 900},
		Layout:   VBox{},
		MenuItems: []MenuItem{
			Menu{Text: "文件(&F)", Items: []MenuItem{
				Action{Text: "扫描 / 重扫", Shortcut: Shortcut{Key: walk.KeyF5}, OnTriggered: func() { a.startScan() }},
				Action{Text: "打开快照目录", OnTriggered: func() { a.openStateDir() }},
				Separator{},
				Action{Text: "退出", OnTriggered: func() { a.mw.Close() }},
			}},
			Menu{Text: "视图(&V)", Items: []MenuItem{
				Action{Text: "展开 / 收起高级参数", OnTriggered: func() { a.toggleAdvanced() }},
				Separator{},
				Action{Text: "全选当前视图的分区", OnTriggered: func() { a.selectAll() }},
				Action{Text: "清空勾选", OnTriggered: func() { a.clearChecked() }},
			}},
			Menu{Text: "帮助(&H)", Items: []MenuItem{
				Action{Text: "使用说明", OnTriggered: func() { a.showUsage() }},
				Action{Text: "关于 rbm", OnTriggered: func() { a.showAbout() }},
			}},
		},
		StatusBarItems: []StatusBarItem{
			{AssignTo: &a.sbState, Text: "● 就绪", Width: 110},
			{AssignTo: &a.sbSnapshot, Text: "尚未扫描", Width: 470},
			{AssignTo: &a.sbHint, Text: "F5 扫描；勾选分区后先预览再删除", Width: 360},
		},
		Children: []Widget{
			a.configBox(), a.resultBox(), a.selectBox(), a.tables(), a.deleteBox(), a.bottom(),
		},
	}
	if err := spec.Create(); err != nil {
		return err
	}

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
	return GroupBox{
		Title:  "① 扫描配置（改完点【扫描 / 重扫】生效，数字留空按默认值）",
		Layout: VBox{},
		Children: []Widget{
			Composite{Layout: HBox{}, Children: []Widget{
				TextLabel{Text: "按时间分批"},
				ComboBox{AssignTo: &a.bucket, Model: bucketDisplay, MaxSize: Size{Width: 250},
					ToolTipText: "先按删除时间把条目切成几批：默认在删除记录的空档处切，也可以按固定粒度（每 15 分钟 / 每天 / 每月…），" +
						"或选「自定义」手填写法。这一项只管切批，每批里再按【桶内分组】分块。",
					OnCurrentIndexChanged: func() { a.applyParamVisibility() }},
				TextLabel{AssignTo: &a.bucketCustomLabel, Text: "写法"},
				LineEdit{AssignTo: &a.bucketCustom, MaxSize: Size{Width: 170},
					CueBanner: "daily / 30m / 3mo / gap:5m / auto"},
				TextLabel{Text: "桶内分组"},
				ComboBox{AssignTo: &a.by, Model: byDisplay, MaxSize: Size{Width: 160},
					OnCurrentIndexChanged: func() { a.applyParamVisibility() }},
				TextLabel{AssignTo: &a.dirLevelLabel, Text: "目录层级"},
				ComboBox{AssignTo: &a.dirLevel, Model: levelDisplay, MaxSize: Size{Width: 200}},
				TextLabel{AssignTo: &a.prefixLenLabel, Text: "前缀长度"},
				LineEdit{AssignTo: &a.prefixLen, MaxSize: Size{Width: 56}},
				HSpacer{},
				PushButton{AssignTo: &a.advBtn, Text: "▼ 展开高级参数", MaxSize: Size{Width: 140},
					ToolTipText: "桶数上限 / 分区上限 / 小组并入 / 限定盘符 / 单次扫描上限 / 删除保险丝 / 快照目录",
					OnClicked:   func() { a.toggleAdvanced() }},
			}},
			Composite{AssignTo: &a.advBox, Layout: VBox{}, Children: []Widget{
				Composite{Layout: HBox{}, Children: []Widget{
					TextLabel{Text: "桶数上限"},
					LineEdit{AssignTo: &a.maxBuckets, MaxSize: Size{Width: 56},
						ToolTipText: "时间切成几个桶；超过就把最接近的桶并起来"},
					TextLabel{Text: "分区上限"},
					LineEdit{AssignTo: &a.maxParts, MaxSize: Size{Width: 64},
						ToolTipText: "分区总数上限；调小会把小组并成「其他」"},
					TextLabel{Text: "小组并入"},
					LineEdit{AssignTo: &a.minCount, MaxSize: Size{Width: 56},
						ToolTipText: "元素数不到这个数的组合进「其他」"},
					TextLabel{Text: "限定盘符"},
					LineEdit{AssignTo: &a.drives, MaxSize: Size{Width: 90}, CueBanner: "留空=全部"},
					TextLabel{Text: "单次扫描上限"},
					LineEdit{AssignTo: &a.maxScan, MaxSize: Size{Width: 72},
						ToolTipText: "超过就只取最老的这些建快照，日志会说明；0 = 不限"},
					TextLabel{Text: "删除保险丝"},
					LineEdit{AssignTo: &a.fuse, MaxSize: Size{Width: 72}, CueBanner: "0=不限",
						ToolTipText: "选中条目数超过这个数就整批不动（等价 --max）"},
					HSpacer{},
				}},
				Composite{Layout: HBox{}, Children: []Widget{
					TextLabel{Text: "快照目录"},
					LineEdit{AssignTo: &a.stateDir, StretchFactor: 1,
						ToolTipText: "state.json / scan_list.csv / ledger.csv 所在目录，命令行 --state-dir 指同一个目录"},
					PushButton{Text: "打开目录", MaxSize: Size{Width: 92}, OnClicked: func() { a.openStateDir() }},
					HSpacer{},
				}},
			}},
		},
	}
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

// applyParamVisibility 按「桶内分组」显示/隐藏目录层级与前缀长度，按「按时间分批」显示/隐藏
// 自定义写法框。隐藏时设成不占位，否则 ① 会留空档。值本身留在控件里，切回来就按原值生效；
// collectOptions 也照旧读它们。
func (a *app) applyParamVisibility() {
	showLevel, showPrefixLen := paramsShow(byValues[a.by.CurrentIndex()])
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
	return GroupBox{
		Title:  "② 扫描结果",
		Layout: VBox{},
		Children: []Widget{
			Composite{Layout: HBox{}, Children: []Widget{
				PushButton{AssignTo: &a.scanBtn, Text: "扫描 / 重扫 (F5)", MaxSize: Size{Width: 150},
					OnClicked: func() { a.startScan() }},
				HSpacer{},
			}},
			Composite{Layout: HBox{}, Children: []Widget{
				TextLabel{Text: "分区"},
				TextLabel{AssignTo: &a.sumParts, MinSize: Size{Width: 150}, TextColor: walk.RGB(0, 80, 160)},
				TextLabel{Text: "对象"},
				TextLabel{AssignTo: &a.sumItems, MinSize: Size{Width: 170}, TextColor: walk.RGB(0, 80, 160)},
				TextLabel{Text: "未删完"},
				TextLabel{AssignTo: &a.sumAlive, MinSize: Size{Width: 170}, TextColor: walk.RGB(0, 80, 160)},
				TextLabel{Text: "已勾选"},
				TextLabel{AssignTo: &a.sumChecked, MinSize: Size{Width: 230}, TextColor: walk.RGB(0, 80, 160)},
				HSpacer{},
			}},
		},
	}
}

// selectBox 是 ③ 选择分区：全选 / 清空勾选在最前，然后是按编号勾选、前缀过滤和只看未删完。
func (a *app) selectBox() Widget {
	return GroupBox{
		Title:  "③ 选择分区（勾选 = 要删的批次）",
		Layout: HBox{},
		Children: []Widget{
			PushButton{AssignTo: &a.selectBtn, Text: "全选", MaxSize: Size{Width: 72},
				ToolTipText: "勾上当前视图里所有还有未删条目的分区（已删完的跳过）",
				OnClicked:   func() { a.selectAll() }},
			PushButton{AssignTo: &a.idsClear, Text: "清空勾选", MaxSize: Size{Width: 92},
				OnClicked: func() { a.clearChecked() }},
			TextLabel{Text: "按编号勾选"},
			LineEdit{AssignTo: &a.idsBox, MaxSize: Size{Width: 160}, CueBanner: "如 1-5,8-10,16"},
			PushButton{Text: "应用", MaxSize: Size{Width: 64}, OnClicked: func() { a.applyIDs() }},
			TextLabel{Text: "文件名前缀过滤"},
			LineEdit{AssignTo: &a.prefixBox, MaxSize: Size{Width: 160}, CueBanner: "空=不过滤（同 --prefix）",
				OnEditingFinished: func() { a.showCurrentDetail(); a.updateSelectionHint() }},
			CheckBox{AssignTo: &a.aliveOnly, Text: "只看未删完", OnCheckedChanged: func() { a.onAliveToggled() }},
			HSpacer{},
		},
	}
}

// deleteBox 是 ⑥ 删除操作，放在选完、看完明细之后：预览在前，永久删除在最后并且随时报出规模。
// 进度条和这两个按钮同一行（原来单独占一行，纵向太吃空间），文字说明它在量什么。
func (a *app) deleteBox() Widget {
	return GroupBox{
		Title:  "⑥ 删除操作（三层闸门：预览 → 弹窗确认 → 保险丝）",
		Layout: HBox{},
		Children: []Widget{
			PushButton{AssignTo: &a.previewBtn, Text: "预览删除（不动文件）", MaxSize: Size{Width: 190},
				ToolTipText: "只把选中统计和明细打进日志/右表，等价 rbm delete <编号> --dry-run",
				OnClicked:   func() { a.previewDelete() }},
			PushButton{AssignTo: &a.deleteBtn, Text: deleteButtonText(0, 0), MaxSize: Size{Width: 280},
				ToolTipText: "真删：不进回收站、不可恢复。要在弹窗里点【确认永久删除】，并受「删除保险丝」约束。",
				OnClicked:   func() { a.deleteSelected() }},
			ProgressBar{AssignTo: &a.progress, MinValue: 0, MaxValue: 100, StretchFactor: 1,
				MinSize: Size{Width: 140}},
			TextLabel{AssignTo: &a.progressLabel, MinSize: Size{Width: 270}, Text: "进度 空闲"},
		},
	}
}

func (a *app) tables() Widget {
	return HSplitter{
		Children: []Widget{
			GroupBox{
				Title:  "④ 分区列表（点方框勾选=要删的分区，按住 Shift 点另一个可以连选一段；单击行看明细，点列名排序）",
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
				Title:  "⑤ 元素明细",
				Layout: VBox{},
				Children: []Widget{
					TextLabel{AssignTo: &a.detailTitle, Text: "请选择一个分区：在④里单击任意一行，这里列出它的文件清单"},
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

// bottom 是操作日志：进度条已经并进 ⑥ 那一行，这里只剩标题行 + 日志区。
// 日志区带【清空】【复制】，行首有时间和级别。
func (a *app) bottom() Widget {
	return Composite{
		Layout: VBox{},
		Children: []Widget{
			Composite{Layout: HBox{}, Children: []Widget{
				TextLabel{Text: "操作日志"},
				TextLabel{AssignTo: &a.logCount, Text: "0 行"},
				PushButton{Text: "清空", MaxSize: Size{Width: 64}, OnClicked: func() { a.clearLog() }},
				PushButton{Text: "复制", MaxSize: Size{Width: 64},
					ToolTipText: "把整段日志放进剪贴板，方便贴给别人排查",
					OnClicked:   func() { a.copyLog() }},
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
	a.logBox.ScrollToCaret()
	a.logLines += len(lines)
	a.logCount.SetText(fmt.Sprintf("%d 行", a.logLines))
}

// log 追加一条普通记录（信息级）。
func (a *app) log(text string) { a.emit(logInfo, text) }

// ok/warn/fail 是成功、提醒、失败三档，用在扫描完成、闸门拦下、报错这些关键节点上。
func (a *app) ok(text string)   { a.emit(logOK, text) }
func (a *app) warn(text string) { a.emit(logWarn, text) }
func (a *app) fail(text string) { a.emit(logErr, text) }

func (a *app) logf(format string, args ...interface{}) {
	a.emit(logInfo, fmt.Sprintf(format, args...))
}

func (a *app) okf(format string, args ...interface{}) {
	a.emit(logOK, fmt.Sprintf(format, args...))
}

func (a *app) warnf(format string, args ...interface{}) {
	a.emit(logWarn, fmt.Sprintf(format, args...))
}

func (a *app) failf(format string, args ...interface{}) {
	a.emit(logErr, fmt.Sprintf(format, args...))
}

func (a *app) clearLog() {
	a.logBox.SetText("")
	a.logLines = 0
	a.logCount.SetText("0 行")
}

// copyLog 把整段日志放进剪贴板。空日志不写剪贴板，免得把用户先前复制的内容冲掉。
func (a *app) copyLog() {
	text := a.logBox.Text()
	if strings.TrimSpace(text) == "" {
		a.log("日志区是空的，没有可复制的内容。")
		return
	}
	if err := walk.Clipboard().SetText(text); err != nil {
		a.failf("复制到剪贴板失败: %v", err)
		return
	}
	a.okf("已复制 %d 行日志到剪贴板。", strings.Count(text, "\r\n")+1)
}

// toggleAdvanced 展开/收起「桶数上限…删除保险丝 + 快照目录」那两行。SetVisible 自己会请求重排，
// 收起时又设了不占位，所以① 会缩回一行高。
func (a *app) toggleAdvanced() {
	a.advOpen = !a.advOpen
	a.advBox.SetVisible(a.advOpen)
	if a.advOpen {
		a.advBtn.SetText("▲ 收起高级参数")
		return
	}
	a.advBtn.SetText("▼ 展开高级参数")
	a.log("高级参数已收起：桶数上限/分区上限/小组并入/限定盘符/单次扫描上限/删除保险丝/快照目录 仍按里面的值生效。")
}

func (a *app) partRow(row int) *parts.Part {
	if row < 0 || row >= len(a.partModel.rows) {
		return nil
	}
	return a.partModel.rows[row]
}

// beginBusy 后台任务期间锁住参数与动作，状态栏和进度文字都换成「在做什么」。
func (a *app) beginBusy(text string) {
	a.busy = true
	for _, w := range a.lock {
		w.SetEnabled(false)
	}
	a.progress.SetValue(0)
	a.sbState.SetText("● " + text)
	a.progressLabel.SetText("进度 " + text)
	a.sbHint.SetText(text)
}

func (a *app) endBusy() {
	for _, w := range a.lock {
		w.SetEnabled(true)
	}
	a.busy = false
	a.sbState.SetText("● 就绪")
	a.progressLabel.SetText("进度 空闲")
	a.updateSelectionHint()
}

// showStartupNotice 在启动自动扫描期间开一个非模态小提示窗（扫完自动关，用户也能先自己关掉）。
// 非模态是必须的：Modal 会把消息循环堵住，扫描 goroutine 回来更新的 Synchronize 就永远排不上。
func (a *app) showStartupNotice() {
	var dlg *walk.Dialog
	err := Dialog{
		AssignTo:  &dlg,
		Title:     "正在扫描回收站",
		FixedSize: true,
		MinSize:   Size{Width: 360},
		Layout:    HBox{},
		Children: []Widget{
			TextLabel{Text: "正在扫描，请等待完成…"},
			HSpacer{},
			PushButton{Text: "关闭", MaxSize: Size{Width: 64}, OnClicked: func() { dlg.Close(0) }},
		},
	}.Create(a.mw)
	if err != nil {
		a.log("启动提示窗创建失败: " + err.Error())
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
		Bucket:     bucketToken(a.bucket.CurrentIndex(), a.bucketCustom.Text()),
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
	a.refreshParts(false)
	a.sbSnapshot.SetText(fmt.Sprintf("快照 %s｜条目 %d 个 / %s｜分区 %d 个（编号 1..%d）",
		st.Created, len(st.Items), tty.Human(float64(totalSize(st))), len(st.Parts), len(st.Parts)))
}

func totalSize(st *store.State) int64 {
	var n int64
	for _, p := range st.Parts {
		n += p.Size
	}
	return n
}

// refreshParts 按「只看未删完」重画分区表；显示顺序交给点表头，所以这里固定用快照自身的
// 时间顺序。视图一变，勾选一律清空：行号和勾选的对应关系变了，留着旧勾选可能删错批次。
// explain 为 true 时说明这次筛选改变了什么 —— 刚扫完的快照没有「已删完」的分区，勾上这个框
// 一行也不会少，不解释看上去就像点了没反应。
func (a *app) refreshParts(explain bool) {
	had := len(a.partModel.checked)
	if a.st == nil {
		a.partModel.setRows(nil)
		a.updateSelectionHint()
		return
	}
	rows := engine.SortedParts(a.st, "time", a.aliveOnly.Checked())
	a.partModel.setRows(rows)
	if len(a.st.Parts) > 0 {
		if err := a.partsTV.Columns().At(2).SetTitle(a.st.GroupCol()); err != nil {
			a.failf("改列名失败: %v", err)
		}
	}
	if explain {
		a.log(aliveFilterText(a.aliveOnly.Checked(), len(a.st.Parts), len(rows)))
	}
	if had > 0 {
		a.warnf("视图换了，原先勾选的 %d 个分区已经取消勾选（留着旧勾选可能删错批次），请重新勾选。", had)
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
		a.log("上一次任务还在跑，请等它结束。")
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
		drives = "全部盘"
	}
	tier := o.Bucket
	if tier == "" {
		tier = fmt.Sprintf("自动找空档（不超过 %d 批）", o.MaxBuckets)
	}
	idx := a.bucket.CurrentIndex()
	if bucketIsCustom(idx) && o.Bucket == "" {
		a.log("「自定义」的写法留空了，这一轮按默认的自动找删除空档切批。")
	}
	label := tier
	if !bucketIsCustom(idx) && idx >= 0 && idx < len(bucketDisplay) {
		label = bucketDisplay[idx]
	}
	a.log(fmt.Sprintf("开始扫描（%s，按%s分批，单次上限 %s）…", drives, label, limitText(o.MaxScan)))
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
			a.closeNotice()
			if err != nil {
				a.failf("扫描失败: %v", err)
				return
			}
			a.setState(snap.State)
			st := snap.State
			spent := elapsed.Round(10 * time.Millisecond)
			a.progressLabel.SetText(fmt.Sprintf("进度 空闲｜上次扫描用时 %s", spent))
			if len(st.Parts) == 0 {
				// 回收站本来就是空的：这是扫描完成，不是失败，别让它读起来像出错。
				a.okf("扫描完成（用时 %s）：条目 0 个、分区 0 个——回收站里当前用户没有对象。"+
					"这是正常结果，不是错误；没有可删的分区。", spent)
			} else {
				a.okf("扫描完成（用时 %s）：条目 %d 个 / %s，分区 %d 个，编号 1..%d。",
					spent, len(st.Items), tty.Human(float64(totalSize(st))),
					len(st.Parts), len(st.Parts))
			}
			if snap.Overflow != nil {
				a.warn(overflowText(snap.Overflow))
			}
			if len(st.Issues) > 0 {
				a.warnf("异常记录 %d 条（全量在 scan_list.csv），示例:", len(st.Issues))
				for i, line := range st.Issues {
					if i == 5 {
						break
					}
					a.log("  - " + line)
				}
			}
			a.log(fmt.Sprintf("快照已写出: %s\n清单已写出: %s", store.Path(dir), listPath))
			if len(st.Parts) == 0 {
				a.log("没有可删的分区。回收站清空之后重新扫描会得到同样的 0。")
			} else {
				a.log("下一步：③ 里【全选】或点方框勾选 → ④ 单击看明细 → ⑥【预览删除】核对 → 再点【永久删除】并在弹窗里确认。")
			}
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
		// 重画表格会把当前行清成 -1，所以「有行但没当前行」和「视图真的空了」要分开说，
		// 否则点开【全选】之后右栏会怪到「只看未删完」头上。
		if n := len(a.partModel.rows); n > 0 {
			a.detailTitle.SetText(fmt.Sprintf("（还没点行：视图里有 %d 个分区，单击任一行看它的条目明细）", n))
		} else {
			a.detailTitle.SetText("（视图里 0 个分区：去掉「只看未删完」或重新扫描）")
		}
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

// briefIDs 只列出前 max 个编号：【全选】120 个分区时把整串塞进确认框，
// 单行标签会把对话框撑得比屏幕还宽。
func briefIDs(ids []int, max int) string {
	if len(ids) <= max {
		return strings.Join(intsToStrings(ids), ",")
	}
	return fmt.Sprintf("%s…（共 %d 个，完整清单在【预览删除】的日志里）",
		strings.Join(intsToStrings(ids[:max]), ","), len(ids))
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
	a.log("预览未做任何改动。要真删请点【⚠ 永久删除】，再在弹窗里点【确认永久删除】。")
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

// confirmPermanent 是第二层闸门：把规模、编号、快照路径和「不可恢复」一次列清，
// 点【确认永久删除】才动手（不再要求手打确认词）。不用 walk.MsgBox 是因为它只有「是/否」
// 两个等价的按钮，看不出哪一个是危险的；这里警示行用红字、规模用粗体，且不设默认按钮，
// 回车什么都不做，只有点按钮这条路。
func (a *app) confirmPermanent(partCount, itemCount int, size int64, ids []int, prefix string) bool {
	var dlg *walk.Dialog
	scale := fmt.Sprintf("%d 个分区 / %d 个条目，约 %s", partCount, itemCount, tty.Human(float64(size)))
	numbered := "选中编号：" + briefIDs(ids, 20)
	if prefix != "" {
		numbered += fmt.Sprintf("（再按前缀 %q* 收窄）", prefix)
	}
	err := Dialog{
		AssignTo:  &dlg,
		Title:     "永久删除确认（不可恢复）",
		FixedSize: true,
		MinSize:   Size{Width: 520},
		Layout:    VBox{},
		Children: []Widget{
			TextLabel{Text: "⚠ 即将永久删除", TextColor: walk.RGB(192, 0, 0)},
			TextLabel{Text: scale, Font: Font{Bold: true}},
			TextLabel{Text: numbered},
			TextLabel{Text: "这些内容直接从 $Recycle.Bin 抹掉，不进回收站、无法恢复。", TextColor: walk.RGB(192, 0, 0)},
			TextLabel{Text: "快照：" + store.Path(a.dir())},
			TextLabel{Text: "逐笔记入同目录的 ledger.csv，删完能核对。"},
			Composite{Layout: HBox{}, Children: []Widget{
				TextLabel{Text: "没核对清单的话先【取消】，回④/⑤看明细。"},
				HSpacer{},
				PushButton{Text: "取消", MaxSize: Size{Width: 88}, OnClicked: func() { dlg.Cancel() }},
				PushButton{Text: "确认永久删除", MaxSize: Size{Width: 120}, OnClicked: func() { dlg.Accept() }},
			}},
		},
	}.Create(a.mw)
	if err != nil {
		a.failf("确认窗口创建失败: %v", err)
		return false
	}
	defer dlg.Dispose()
	return dlg.Run() == walk.DlgCmdOK
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
	if !a.confirmPermanent(len(chosen), count, size, ids, prefix) {
		a.log("已取消，未做任何改动。")
		return
	}

	dir := a.dir()
	a.logf("开始删除 %d 个条目…（每 %d 个刷一次进度）", count, progressEvery)
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
					a.progressLabel.SetText(fmt.Sprintf("进度 删除中 %d/%d（成功 %d，失败 %d）", n, total, done, failed))
					a.sbHint.SetText(fmt.Sprintf("删除中 %d/%d  已删 %d  失败 %d", n, total, done, failed))
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
		a.warn("未做任何改动。")
		return
	}
	if err != nil {
		a.failf("删除中断: %v", err)
	}
	if rep != nil {
		a.progressLabel.SetText(fmt.Sprintf("进度 删除结束：成功 %d，失败 %d", rep.Done, rep.Failed))
		if rep.Failed == 0 {
			a.okf("删除完成：成功 %d 个，失败 %d 个，账本 %s",
				rep.Done, rep.Failed, filepath.Join(dir, store.LedgerName))
		} else {
			a.warnf("删除结束：成功 %d 个，失败 %d 个，账本 %s",
				rep.Done, rep.Failed, filepath.Join(dir, store.LedgerName))
		}
		for i, line := range rep.Errors {
			if i == 20 {
				a.warnf("…另有 %d 条失败记录，见账本", len(rep.Errors)-i)
				break
			}
			a.warn(line)
		}
	}
	a.log(fmt.Sprintf("快照已更新（%s）：标记了本次删掉的条目，编号不变。", store.Path(dir)))
	a.log("要看回收站最新状态请重新扫描：编号按当前内容重建。")
	if a.st != nil {
		a.sbSnapshot.SetText(fmt.Sprintf("快照 %s｜未删完条目 %d｜最近删除 %s",
			a.st.Created, a.st.AliveLeft(), a.st.LastDelete))
	}
	a.refreshParts(false)
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

// selectAll 是【全选】：勾上当前视图里所有还有未删条目的分区。
// 已删完的跳过 —— 它们的 $R/$I 已经不在了，勾上只会让引擎逐条报「找不到文件」。
func (a *app) selectAll() {
	if a.st == nil {
		a.log("还没有快照，先点【扫描 / 重扫】。")
		return
	}
	if a.partModel.RowCount() == 0 {
		a.log("当前视图是空的（多半是「只看未删完」把分区都筛掉了，或还没扫描），没有可全选的分区。")
		return
	}
	selected, skipped := a.partModel.selectAlive()
	a.partModel.refresh()
	a.updateSelectionHint()
	if selected == 0 {
		a.logf("视图里 %d 行全都已经删完了，一个也没勾（要清掉这些空分区，重新扫描即可让它们消失）。", skipped)
		return
	}
	if skipped > 0 {
		a.warnf("已勾选当前视图的 %d 个分区；另外 %d 个已经没有未删条目，跳过了。", selected, skipped)
		return
	}
	a.okf("已勾选当前视图的 %d 个分区。", selected)
}

func (a *app) clearChecked() {
	n := a.partModel.clearChecks()
	a.partModel.refresh()
	a.updateSelectionHint()
	if n > 0 {
		a.logf("已清空勾选（原先勾着 %d 个分区）。", n)
	}
}

// updateSelectionHint 把「此刻有多少东西」和「勾了要删多少」一次刷齐四处：
// ② 的四个总体数字、⑥ 的删除按钮文案、④ 的概览行、状态栏右侧。
func (a *app) updateSelectionHint() {
	if a.st == nil {
		a.sbHint.SetText("F5 扫描；勾选分区后先预览再删除")
		a.partsHint.SetText("（还没有快照：点【扫描 / 重扫】）")
		a.sumParts.SetText("—")
		a.sumItems.SetText("—")
		a.sumAlive.SetText("—")
		a.sumChecked.SetText("未勾选")
		a.deleteBtn.SetText(deleteButtonText(0, 0))
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
	a.sumParts.SetText(fmt.Sprintf("%d 个（编号 1..%d）", len(a.st.Parts), len(a.st.Parts)))
	a.sumItems.SetText(fmt.Sprintf("%d 个 / %s", len(a.st.Items), tty.Human(float64(totalSize(a.st)))))
	a.sumAlive.SetText(fmt.Sprintf("%d 个条目 / %d 个分区", a.st.AliveLeft(), aliveParts))
	a.sumChecked.SetText(fmt.Sprintf("%d 个分区 / %d 个条目", len(chosen), count))
	a.deleteBtn.SetText(deleteButtonText(len(chosen), count))
	a.partsHint.SetText(fmt.Sprintf("当前视图 %d 行｜快照共 %d 个分区（编号 1..%d）｜未删完条目 %d 个（在 %d 个分区里）｜勾选 %d 个分区 / %d 个条目 %s",
		a.partModel.RowCount(), len(a.st.Parts), len(a.st.Parts),
		a.st.AliveLeft(), aliveParts, len(chosen), count, tty.Human(float64(size))))
	if len(ids) == 0 {
		a.sbHint.SetText(fmt.Sprintf("未勾选分区｜未删完条目 %d 个", a.st.AliveLeft()))
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
