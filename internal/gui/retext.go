// 语言热切换：把「换语言时怎么重设这个控件」逐条登记进 a.retexts，切换时按登记顺序执行一遍，
// 整窗换文字、不重启、也不重建控件（重建会丢勾选、丢滚动位置、丢排序）。
//
// 登记都发生在声明期，用的都是 AssignTo 的同一个指针，所以一条文案只写一次键名：
// 改文字不会漏掉热切换。闭包真正执行是在 MainWindow.Create() 之后，那时指针已经赋值好了。
package gui

import (
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"

	"rbm/internal/i18n"
	"rbm/internal/tty"
)

// onRetext 登记一条「换语言时执行」的闭包。
func (a *app) onRetext(f func()) { a.retexts = append(a.retexts, f) }

// applyLanguage 把整窗文字换成当前语言。动态那几格（② 的四个数字、⑥ 的删除按钮、④ 的概览行、
// 状态栏三格、进度文字）不逐条登记，统一交给自己的刷新函数按当前状态重算。
// 日志区里已经写下的行保持当时的语言，之后新写的行跟新语言。
func (a *app) applyLanguage() {
	for _, f := range a.retexts {
		f()
	}
	a.updateSelectionHint()
	a.updateStateLine()
	a.updateProgressLine()
	a.updateSnapshotLine()
	a.rerenderDetailTitle()
	a.logCount.SetText(i18n.Tf("gui.log.lines", a.logLines))
	a.applyWindowMinSize()
	a.syncLangMenu()
	// 菜单名改完必须自己重画菜单栏：walk 改菜单文字走 SetMenuItemInfo（menu.go:180），
	// 它只换模型不触发重绘，只有增删菜单项时 walk 才补 DrawMenuBar（menu.go:286）。
	// 于是英文比中文窄时，最后一个顶级菜单的旧中文会留在屏幕上（真机截图：Help 右边挂着「帮助(H)」。
	a.redrawMenuBar()
}

// redrawMenuBar 让 Windows 重画一次菜单栏。
func (a *app) redrawMenuBar() {
	if a.mw == nil {
		return
	}
	win.DrawMenuBar(a.mw.Handle())
}

// setLanguage 是【视图】→【界面语言】的处理：换全局语言、整窗重设文字、把选择写进
// %LOCALAPPDATA%\rbm\settings.json（下次启动沿用）。写失败不影响本次窗口，日志里说清楚。
func (a *app) setLanguage(l i18n.Lang) {
	if l == i18n.Get() {
		a.syncLangMenu()
		return
	}
	i18n.Set(l)
	a.applyLanguage()
	path, err := i18n.WriteSettings(l)
	if err != nil {
		// 走 a.warn 而不是 a.warnf：文案里已经带过格式化了，再当格式串解析会把
		// 错误文本里的 %（路径、百分号文件名）当成动词。
		a.warn(i18n.Tf("gui.lang.write-fail", err))
		return
	}
	key := "gui.lang.changed-zh"
	if l == i18n.En {
		key = "gui.lang.changed-en"
	}
	a.ok(i18n.Tf(key, path))
}

// syncLangMenu 让两个语言项的勾跟着当前语言走。walk 对 Checkable 的菜单项是「点一下就自己翻
// 状态」（action.go:443-449 raiseTriggered），所以这里在触发之后按事实重设，不靠它自己猜。
func (a *app) syncLangMenu() {
	en := i18n.IsEn()
	if a.langZhAction != nil {
		_ = a.langZhAction.SetChecked(!en)
	}
	if a.langEnAction != nil {
		_ = a.langEnAction.SetChecked(en)
	}
}

// textPx 估一条文本需要的像素宽。口径： tty.WLen 的列数（半角 1、全角 2）× 7px，
// 再加控件左右各 14px 留白 —— 9pt 比例字体下半角约 6.5px、全角约 13px，取值略偏宽。
// 只用来给英文算宽度：中文的宽度是原来一个个调好的定值，保持不动。
// 偏宽的代价只是按钮两侧多留白，偏窄就会被 walk 按 MaxSize 截断文字（见 resultBox 的锁宽注释）。
func textPx(text string) int { return tty.WLen(text)*7 + 28 }

// widthFor 给一个控件取当前语言的宽度：中文返回原来调好的 zh，
// 英文返回按这条文本估出来的宽度（不会比 zh 窄）。
func widthFor(zh int, text string) int {
	if !i18n.IsEn() {
		return zh
	}
	if n := textPx(text); n > zh {
		return n
	}
	return zh
}

// longest 取一组显示文本里最长的那条，用来算下拉框的宽度。
func longest(items []string) string {
	best := ""
	for _, s := range items {
		if tty.WLen(s) > tty.WLen(best) {
			best = s
		}
	}
	return best
}

// texter / tipped / cued 是「能被一个 string 重设的一类控件」。
// 泛型 helper 靠它们把 *walk.ComboBox、*walk.LineEdit 这些具体类型收进同一个签名。
type texter interface{ SetText(string) error }
type tipped interface{ SetToolTipText(string) error }
type cued interface{ SetCueBanner(string) error }

// lab 声明一个文本标签，文字跟着语言换。
func (a *app) lab(dst **walk.TextLabel, key string) TextLabel {
	a.onRetext(func() { (*dst).SetText(i18n.T(key)) })
	return TextLabel{AssignTo: dst, Text: i18n.T(key)}
}

// box 声明分组框，标题跟着语言换。
func (a *app) box(dst **walk.GroupBox, key string, layout Layout, children []Widget) GroupBox {
	a.onRetext(func() { (*dst).SetTitle(i18n.T(key)) })
	return GroupBox{AssignTo: dst, Title: i18n.T(key), Layout: layout, Children: children}
}

// btn 声明按钮：文字、MaxSize、气泡提示都跟着语言换。zhW 是中文下已经调好的宽度，
// tipKey 传空串表示这一件没有气泡提示。
func (a *app) btn(dst **walk.PushButton, key string, zhW int, tipKey string, onClick func()) PushButton {
	a.onRetext(func() {
		text := i18n.T(key)
		(*dst).SetText(text)
		(*dst).SetMinMaxSize(walk.Size{}, walk.Size{Width: widthFor(zhW, text)})
		if tipKey != "" {
			(*dst).SetToolTipText(i18n.T(tipKey))
		}
	})
	text := i18n.T(key)
	p := PushButton{AssignTo: dst, Text: text, MaxSize: Size{Width: widthFor(zhW, text)}, OnClicked: onClick}
	if tipKey != "" {
		p.ToolTipText = i18n.T(tipKey)
	}
	return p
}

// btnLock 声明 Min/Max 同值的锁宽按钮（两侧留白，见 resultBox 的注释）。
// 中文的两个宽度沿用原来调好的数字；英文按「这条文案」估宽，至少不和按钮挤在一起 ——
// 英文标签更长，沿用中文的宽度就是截断（动态文案的按钮由调用方自己管宽度，见 updateSelectionHint）。
func (a *app) btnLock(dst **walk.PushButton, key string, zhW int, tipKey string, onClick func()) PushButton {
	a.onRetext(func() {
		text := i18n.T(key)
		(*dst).SetText(text)
		lock := widthFor(zhW, text)
		(*dst).SetMinMaxSize(walk.Size{Width: lock}, walk.Size{Width: lock})
		if tipKey != "" {
			(*dst).SetToolTipText(i18n.T(tipKey))
		}
	})
	text := i18n.T(key)
	lock := widthFor(zhW, text)
	p := PushButton{
		AssignTo: dst, Text: text,
		MinSize: Size{Width: lock}, MaxSize: Size{Width: lock},
		OnClicked: onClick,
	}
	if tipKey != "" {
		p.ToolTipText = i18n.T(tipKey)
	}
	return p
}

// combo 声明下拉框：模型文本、MaxSize 跟着语言换，选中项按索引原地不动
// （SetModel 会重建列表，所以先记下索引再复位；复位触发的 OnCurrentIndexChanged 只是重算显示/隐藏，
// 参数值本身没变）。tipKey 传空串表示没有气泡提示。
func (a *app) combo(dst **walk.ComboBox, zhMax int, items func() []string, tipKey string, onChange func()) ComboBox {
	a.onRetext(func() {
		list := items()
		idx := (*dst).CurrentIndex()
		(*dst).SetModel(list)
		(*dst).SetMinMaxSize(walk.Size{}, walk.Size{Width: widthFor(zhMax, longest(list))})
		if idx >= 0 {
			(*dst).SetCurrentIndex(idx)
		}
		if tipKey != "" {
			(*dst).SetToolTipText(i18n.T(tipKey))
		}
	})
	list := items()
	c := ComboBox{
		AssignTo:              dst,
		Model:                 list,
		MaxSize:               Size{Width: widthFor(zhMax, longest(list))},
		OnCurrentIndexChanged: onChange,
	}
	if tipKey != "" {
		c.ToolTipText = i18n.T(tipKey)
	}
	return c
}

// check 声明复选框，文字跟着语言换。
func (a *app) check(dst **walk.CheckBox, key string, onChanged func()) CheckBox {
	a.onRetext(func() { (*dst).SetText(i18n.T(key)) })
	return CheckBox{AssignTo: dst, Text: i18n.T(key), OnCheckedChanged: onChanged}
}

// tipOf 用在声明里：返回当前语言的气泡提示文字，并登记「换语言时重设它」。
// dst 收的是「控件指针的地址」（&a.maxBuckets 这种 **walk.LineEdit）：声明期控件还没建出来，
// 只有二级指针能在 Create 之后再解到真控件。Go 不允许泛型方法，所以这是个包级函数。
func tipOf[P tipped](a *app, dst *P, key string) string {
	a.onRetext(func() { _ = (*dst).SetToolTipText(i18n.T(key)) })
	return i18n.T(key)
}

// cueOf 同 tipOf，重设的是输入框里的占位文字（CueBanner）。
func cueOf[P cued](a *app, dst *P, key string) string {
	a.onRetext(func() { _ = (*dst).SetCueBanner(i18n.T(key)) })
	return i18n.T(key)
}

// sb 声明状态栏的一格。文字与宽度都交给各自的刷新函数按当前状态给（updateStateLine、
// updateSnapshotLine、setHint、updateProgressLine），所以这里不登记热切换：
// 登记了反而会在换语言时把「扫描中…」写回「● 就绪」。
func (a *app) sb(dst **walk.StatusBarItem, key string, zhWidth, capW int) StatusBarItem {
	text := i18n.T(key)
	return StatusBarItem{AssignTo: dst, Text: text, Width: statusWidth(zhWidth, capW, text)}
}

// act 声明一个菜单项：文字跟着语言换。
func (a *app) act(dst **walk.Action, key string, onTriggered func()) Action {
	return a.actKey(dst, key, Shortcut{}, onTriggered)
}

// actKey 同 act，只是带快捷键。
func (a *app) actKey(dst **walk.Action, key string, sc Shortcut, onTriggered func()) Action {
	a.onRetext(func() { (*dst).SetText(i18n.T(key)) })
	return Action{AssignTo: dst, Text: i18n.T(key), Shortcut: sc, OnTriggered: onTriggered}
}

// menu 声明一个顶级菜单：菜单名跟着语言换（菜单名挂在它自己的 Action 上，所以用 AssignActionTo 取）。
func (a *app) menu(dst **walk.Action, key string, items []MenuItem) Menu {
	a.onRetext(func() { (*dst).SetText(i18n.T(key)) })
	return Menu{AssignActionTo: dst, Text: i18n.T(key), Items: items}
}

// ---------- 状态栏三格 ----------

// 状态栏的三格是按右边界一格一格累加排开的（walk 的 updateParts 把各格宽度送进 SB_SETPARTS），
// 加起来一旦超过状态栏自己的宽度，最右边那格就整个跑到可视区外面去了。
// 中文保持原来的 110 / 470 / 360 不动；英文按这条文本估宽，再各自封在下面的额度里
// （140 + 520 + 380 = 1040，仍在最窄 1080 的窗口内）。
const (
	sbStateCap = 140
	sbSnapCap  = 520
	sbHintCap  = 380
)

// statusWidth 给状态栏的一格取宽度：中文返回原来的定值，英文取「文本估宽」但不超过额度。
func statusWidth(zh, capW int, text string) int {
	if !i18n.IsEn() {
		return zh
	}
	w := textPx(text)
	if w < zh {
		return zh
	}
	if w > capW {
		return capW
	}
	return w
}

func (a *app) setHint(text string) {
	_ = a.sbHint.SetText(text)
	_ = a.sbHint.SetWidth(statusWidth(360, sbHintCap, text))
}

// updateStateLine 重画状态栏最左那格：忙的时候写 beginBusy 记下的那条（扫描中 / 删除中），
// 空闲就回到「● 就绪」。忙到一半换语言不能把它写成就绪，所以按 busyKey 重算。
func (a *app) updateStateLine() {
	text := i18n.T("gui.sb.ready")
	if a.busyKey != "" {
		text = "● " + i18n.T(a.busyKey)
	}
	_ = a.sbState.SetText(text)
	_ = a.sbState.SetWidth(statusWidth(110, sbStateCap, text))
}

// updateSnapshotLine 重画中间那格。snapDeleted 说清这次该按哪种口径显示：
// 刚扫完报「条目 / 分区」，删过东西报「未删完 / 最近删除」。
func (a *app) updateSnapshotLine() {
	if a.st == nil {
		a.setSnapshot(i18n.T("gui.sb.none"))
		return
	}
	st := a.st
	if a.snapDeleted {
		a.setSnapshot(i18n.Tf("gui.sb.snapshot-after", st.Created, st.AliveLeft(), st.LastDelete))
		return
	}
	a.setSnapshot(i18n.Tf("gui.sb.snapshot", st.Created, len(st.Items),
		tty.Human(float64(totalSize(st))), len(st.Parts), len(st.Parts)))
}

func (a *app) setSnapshot(text string) {
	_ = a.sbSnapshot.SetText(text)
	_ = a.sbSnapshot.SetWidth(statusWidth(470, sbSnapCap, text))
}

// ---------- 进度那一行 ----------

// 进度文字的五种状态。换语言时按「现在处于哪一种 + 当时的数字」重算，
// 不能凭默认值重拼：删除跑到一半换语言，进度不能变回「空闲」。
const (
	progIdle = iota
	progBusy
	progLastScan
	progDeleteStep
	progDeleteEnd
)

func (a *app) updateProgressLine() {
	var text string
	switch a.progKind {
	case progBusy:
		text = i18n.T("gui.progress-word") + i18n.T(a.busyKey)
	case progLastScan:
		text = i18n.Tf("gui.progress.last-scan", a.progElapsed)
	case progDeleteStep:
		text = i18n.Tf("gui.progress.delete-step", a.progN, a.progTotal, a.progDone, a.progFailed)
	case progDeleteEnd:
		text = i18n.Tf("gui.progress.delete-end", a.progDone, a.progFailed)
	default:
		text = i18n.T("gui.progress-word") + i18n.T("gui.progress.idle")
	}
	_ = a.progressLabel.SetText(text)
}

// ---------- ⑤ 明细表标题 ----------

// setDetailTitle 记下「这行标题怎么算」，再显示一次。记下算法而不是记下成品文本，
// 是因为换语言时要把同一份内容用新语言重算一遍，而不能退回「请选择一个分区」的默认提示。
func (a *app) setDetailTitle(render func() string) {
	a.detailRender = render
	_ = a.detailTitle.SetText(render())
}

func (a *app) rerenderDetailTitle() {
	if a.detailRender != nil {
		_ = a.detailTitle.SetText(a.detailRender())
	}
}

// ---------- 两张表的列名 ----------

// guiColumn 是「一列的标题 + 中文下已经调好的宽度 + 对齐」。声明期和换语言时都从这里取，
// 列名列宽只写一遍，不会改了声明漏了热切换。
type guiColumn struct {
	title string
	zhW   int
	align Alignment1D
}

// partColumns 是 ④ 分区表 9 列。第 3 列（路径组 / 前缀组）的标题跟着快照的分组维度走，
// 由 store.State.GroupCol() 按当前语言给；还没快照时按目录那种写法兜底
// —— 那时列宽和标题都用默认值，和改造前写死的「路径组 / 156」一致。
func (a *app) partColumns() []guiColumn {
	group := i18n.T("store.group-col.dir")
	if a.st != nil {
		group = a.st.GroupCol()
	}
	return []guiColumn{
		{i18n.T("cli.th.pid"), 44, AlignFar},
		{i18n.T("cli.th.range"), 168, AlignDefault},
		{group, 156, AlignDefault},
		{i18n.T("cli.th.count"), 52, AlignFar},
		{i18n.T("cli.th.size"), 64, AlignFar},
		{i18n.T("cli.th.index"), 74, AlignFar},
		{i18n.T("cli.th.drive"), 32, AlignDefault},
		{i18n.T("cli.th.left"), 52, AlignFar},
		{i18n.T("cli.th.top"), 118, AlignDefault},
	}
}

// itemColumns 是 ⑤ 明细表 6 列。第 1 列的表头就是全局 index 这个技术名，中英同形，
// 所以和命令行一样不走目录（见 cli.show 的表头写法）。
func itemColumns() []guiColumn {
	return []guiColumn{
		{"index", 48, AlignFar},
		{i18n.T("cli.th.deleted"), 124, AlignDefault},
		{i18n.T("cli.th.size"), 60, AlignFar},
		{i18n.T("cli.th.drive"), 32, AlignDefault},
		{i18n.T("cli.th.name"), 130, AlignDefault},
		{i18n.T("cli.th.orig"), 112, AlignDefault},
	}
}

// applyColumnTitles 把两张表的列名和列宽按当前语言重设：英文列名更长，沿用中文的宽度会被截断。
func (a *app) applyColumnTitles() {
	set := func(tv *walk.TableView, cols []guiColumn) {
		list := tv.Columns()
		for i, c := range cols {
			col := list.At(i)
			if col == nil {
				continue
			}
			if err := col.SetTitle(c.title); err != nil {
				a.fail(i18n.Tf("gui.err.column-title", err))
			}
			if err := col.SetWidth(widthFor(c.zhW, c.title)); err != nil {
				a.fail(i18n.Tf("gui.err.column-title", err))
			}
		}
	}
	set(a.partsTV, a.partColumns())
	set(a.itemsTV, itemColumns())
}

// ---------- ⑥ 永久删除按钮 ----------

// deleteButtonWidths 给删除按钮的 Min / Max。中文沿用原来手调的 240 / 280；
// 英文按「这条文案」估最小宽度，最大宽度留到「编号和条目数都写满位」时的那一条 ——
// 数字比它更长时直接用最小宽度（= 当前文案的估宽），宁可按钮宽一点，也不能被 MaxSize 截掉字。
func deleteButtonWidths(text string) (int, int) {
	if !i18n.IsEn() {
		return 240, 280
	}
	minW := widthFor(240, text)
	maxW := textPx(deleteButtonText(999, 999999))
	if maxW < minW {
		return minW, minW
	}
	return minW, maxW
}

// setDeleteButton 把「勾了多少」写在按钮上，同时按这条文案调宽度。
func (a *app) setDeleteButton(parts, items int) {
	text := deleteButtonText(parts, items)
	minW, maxW := deleteButtonWidths(text)
	_ = a.deleteBtn.SetText(text)
	_ = a.deleteBtn.SetMinMaxSize(walk.Size{Width: minW}, walk.Size{Width: maxW})
}

// ---------- 窗口最小宽度 ----------

// 同一条文案英文占的字符更多，① 那一行和状态栏就更吃紧，所以换语言时把窗口的最小宽度
// 一起调一次：中文维持原来的 1080，英文抬到 1280。默认 1400 的窗口不受影响，
// 只是不让用户再往更窄里拖；1280 够不够要由真机取证那一步来定，别把它当成已验证的数字。
const (
	minWindowZh = 1080
	minWindowEn = 1280
)

func (a *app) applyWindowMinSize() {
	w := minWindowZh
	if i18n.IsEn() {
		w = minWindowEn
	}
	// 设不上只影响「能不能把窗口拖得更窄」，不动功能，所以不报错。
	_ = a.mw.SetMinMaxSize(walk.Size{Width: w, Height: 680}, walk.Size{})
}
