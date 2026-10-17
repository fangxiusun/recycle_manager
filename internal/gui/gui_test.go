package gui

import (
	"strings"
	"testing"

	"github.com/lxn/walk"

	"rbm/internal/i18n"
	"rbm/internal/meta"
	"rbm/internal/parts"
	"rbm/internal/tty"
)

func partFixture(pids ...int) *partModel {
	m := newPartModel(nil)
	m.rows = make([]*parts.Part, 0, len(pids))
	for _, pid := range pids {
		m.rows = append(m.rows, &parts.Part{
			PID: pid, Count: pid * 2, Size: int64(pid) * 10,
			Label: string(rune('a' + pid)), RangeStart: float64(pid),
		})
	}
	return m
}

func TestShiftClickChecksInRange(t *testing.T) {
	m := partFixture(1, 2, 3, 4, 5)
	m.click(1, true, false) // 第 2 行 = 编号 2，成为锚点
	if !m.click(4, true, true) {
		t.Fatal("按住 Shift 点第 5 行应当连选一段")
	}
	for _, pid := range []int{2, 3, 4, 5} {
		if !m.checked[pid] {
			t.Errorf("编号 %d 应被勾上", pid)
		}
	}
	if m.checked[1] {
		t.Error("锚点之前的编号 1 不该被勾")
	}
	if got := m.checkedPIDs(); len(got) != 4 {
		t.Errorf("checkedPIDs = %v，要 4 个", got)
	}
	// 锚点移到第 5 行后再按住 Shift 取消，区间同样跟着取消。
	if !m.click(3, false, true) {
		t.Fatal("Shift 取消勾选也应覆盖区间")
	}
	if m.checked[4] || m.checked[5] {
		t.Errorf("编号 4、5 应被取消：%v", m.checkedPIDs())
	}
	if !m.checked[2] || !m.checked[3] {
		t.Error("区间外的编号 2、3 不该被动到")
	}
}

func TestPlainClickDoesNotSpan(t *testing.T) {
	m := partFixture(1, 2, 3)
	m.click(0, true, false) // 勾编号 1，它成为锚点
	if m.click(2, true, false) {
		t.Fatal("没按 Shift 不该整段勾选")
	}
	if m.checked[2] {
		t.Error("锚点与第二点之间的编号 2 不该被连选")
	}
	if !m.checked[1] || !m.checked[3] {
		t.Errorf("两点自己该勾上：%v", m.checkedPIDs())
	}
}

func TestChecksSurviveHeaderSort(t *testing.T) {
	m := partFixture(1, 2, 3, 4)
	m.click(0, true, false)                                // 勾编号 1
	m.click(3, true, false)                                // 勾编号 4
	if err := m.Sort(4, walk.SortDescending); err != nil { // 按大小降序 = 编号倒过来
		t.Fatal(err)
	}
	if m.rows[0].PID != 4 || m.rows[3].PID != 1 {
		t.Fatalf("排序结果不对：%d %d %d %d", m.rows[0].PID, m.rows[1].PID, m.rows[2].PID, m.rows[3].PID)
	}
	if !m.Checked(0) || !m.Checked(3) {
		t.Error("勾选是按编号记的，重排后应还在原来那两个分区上")
	}
	if m.Checked(1) || m.Checked(2) {
		t.Error("中间两行不该被勾上")
	}
	if got := m.checkedPIDs(); len(got) != 2 || got[0] != 1 || got[1] != 4 {
		t.Errorf("checkedPIDs = %v，要 [1 4]", got)
	}
}

func TestPartSortByColumn(t *testing.T) {
	m := partFixture(3, 1, 2)
	if err := m.Sort(0, walk.SortAscending); err != nil {
		t.Fatal(err)
	}
	if m.rows[0].PID != 1 || m.rows[2].PID != 3 {
		t.Errorf("按编号升序不对：%d %d %d", m.rows[0].PID, m.rows[1].PID, m.rows[2].PID)
	}
	if err := m.Sort(4, walk.SortDescending); err != nil {
		t.Fatal(err)
	}
	if m.rows[0].Size != 30 || m.rows[2].Size != 10 {
		t.Errorf("按大小降序不对：%d %d %d", m.rows[0].Size, m.rows[1].Size, m.rows[2].Size)
	}
	if m.SortedColumn() != 4 || m.SortOrder() != walk.SortDescending {
		t.Errorf("排序状态没记住：col %d order %d", m.SortedColumn(), m.SortOrder())
	}
	// 相等的主键退到编号，反复点同一列顺序不会漂。
	a, b := partFixture(1, 2), partFixture(1, 2)
	a.rows[0].Size, a.rows[1].Size = 5, 5
	b.rows[0].Size, b.rows[1].Size = 5, 5
	a.Sort(4, walk.SortDescending)
	b.Sort(4, walk.SortDescending)
	for i := range a.rows {
		if a.rows[i].PID != b.rows[i].PID {
			t.Fatalf("同样数据排两次结果不一致：%d vs %d", a.rows[i].PID, b.rows[i].PID)
		}
	}
}

func TestSetRowsClearsChecksAndSort(t *testing.T) {
	m := partFixture(1, 2, 3)
	m.click(1, true, false)
	m.Sort(0, walk.SortDescending)
	m.setRows([]*parts.Part{{PID: 9, Count: 1}})
	if len(m.checked) != 0 || m.anchor != -1 {
		t.Errorf("换视图后勾选该清空：%v / 锚点 %d", m.checked, m.anchor)
	}
	if m.SortedColumn() != -1 {
		t.Errorf("换视图后表头排序该复位，现在是列 %d", m.SortedColumn())
	}
	if !m.ColumnSortable(0) {
		t.Error("ColumnSortable 要返回 true，否则 walk 不调 Sort")
	}
}

func TestItemSortByColumn(t *testing.T) {
	m := newItemModel()
	mk := func(index int, size int64, name, drive string) entry {
		return entry{index: index, item: &meta.Item{
			Size: size, Display: name, Drive: drive, Epoch: float64(index),
			OrigPath: drive + "\\" + name,
		}}
	}
	m.rows = []entry{mk(30, 500, "b.txt", "D"), mk(10, 500, "a.txt", "C"), mk(20, 900, "c.txt", "C")}
	if err := m.Sort(2, walk.SortDescending); err != nil { // 大小
		t.Fatal(err)
	}
	if m.rows[0].index != 20 || m.rows[1].index != 30 || m.rows[2].index != 10 {
		t.Errorf("按大小降序（相等退到 index）不对：%d %d %d", m.rows[0].index, m.rows[1].index, m.rows[2].index)
	}
	if err := m.Sort(4, walk.SortAscending); err != nil { // 文件名
		t.Fatal(err)
	}
	if m.rows[0].item.Display != "a.txt" || m.rows[2].item.Display != "c.txt" {
		t.Errorf("按文件名升序不对：%s %s %s", m.rows[0].item.Display, m.rows[1].item.Display, m.rows[2].item.Display)
	}
	if err := m.Sort(0, walk.SortDescending); err != nil { // index
		t.Fatal(err)
	}
	if m.rows[0].index != 30 {
		t.Errorf("按 index 降序不对：%d", m.rows[0].index)
	}
	m.setRows(nil)
	if m.SortedColumn() != -1 {
		t.Error("换分区后明细表的排序该复位")
	}
}

func TestSelectAliveSkipsEmptiedParts(t *testing.T) {
	m := partFixture(1, 2, 3)
	m.rows[1].Deleted = m.rows[1].Count // 编号 2 已经删完
	m.click(0, true, false)             // 先前勾过一个，全选要重新开始而不是叠加
	selected, skipped := m.selectAlive()
	if selected != 2 || skipped != 1 {
		t.Errorf("selectAlive = %d 选 / %d 跳，要 2 选 1 跳", selected, skipped)
	}
	if !m.checked[1] || !m.checked[3] {
		t.Errorf("编号 1、3 该勾上：%v", m.checkedPIDs())
	}
	if m.checked[2] {
		t.Error("已删完的编号 2 不该勾（勾上只会逐条报找不到文件）")
	}
	if m.anchor != -1 {
		t.Errorf("全选后锚点该复位，现在是 %d", m.anchor)
	}
}

func TestClearChecksReportsPreviousSelection(t *testing.T) {
	m := partFixture(1, 2, 3)
	m.click(0, true, false)
	m.click(2, true, false)
	if n := m.clearChecks(); n != 2 {
		t.Errorf("clearChecks = %d，要报原先勾着的 2 个", n)
	}
	if len(m.checked) != 0 || m.anchor != -1 {
		t.Errorf("没清干净：%v / 锚点 %d", m.checked, m.anchor)
	}
	if n := m.clearChecks(); n != 0 {
		t.Errorf("本来就空的时候该报 0，得到 %d", n)
	}
}

func TestParamsShowFollowGroupKey(t *testing.T) {
	want := map[string][2]bool{"dir": {true, false}, "prefix": {false, true},
		"ext": {false, false}, "token": {false, true}}
	for i, by := range byValues {
		level, prefixLen := paramsShow(by)
		got := [2]bool{level, prefixLen}
		if want[by] != got {
			t.Errorf("分组 %q（下拉第 %d 项）该显示 %+v，界面按 %+v 显示", by, i, want[by], got)
		}
	}
}

func TestDeleteButtonTextCarriesScale(t *testing.T) {
	if got := deleteButtonText(0, 0); !strings.Contains(got, "未勾选") {
		t.Errorf("没勾选时文案要把状态说出来，得到 %q", got)
	}
	got := deleteButtonText(294, 20255)
	for _, want := range []string{"⚠", "294", "20255"} {
		if !strings.Contains(got, want) {
			t.Errorf("文案缺 %q：%q", want, got)
		}
	}
}

func TestBriefIDsKeepsDialogWidthSane(t *testing.T) {
	ids := make([]int, 0, 120)
	for i := 1; i <= 120; i++ {
		ids = append(ids, i)
	}
	got := briefIDs(ids, 20)
	if strings.Contains(got, "21") {
		t.Errorf("第 21 个编号不该整串写出来：%q", got)
	}
	for _, want := range []string{"1,2,3", "…", "共 120 个"} {
		if !strings.Contains(got, want) {
			t.Errorf("缺 %q：%q", want, got)
		}
	}
	if short := briefIDs([]int{3, 7}, 20); short != "3,7" {
		t.Errorf("没超过 max 时原样列出，得到 %q", short)
	}
}

func TestBucketDropdownMatchesKernelGrammar(t *testing.T) {
	// 下拉里每一项的写法都必须是 parts.ParseSpec 认得的；界面不能给出一个扫不动的选项。
	list := bucketDisplay()
	if len(bucketValues) != len(list) {
		t.Fatalf("写法表和文字表长度不一致: %d vs %d", len(bucketValues), len(list))
	}
	for i, v := range bucketValues {
		if bucketIsCustom(i) {
			continue // 「自定义」那项不给固定写法，值来自手填框
		}
		if _, err := parts.ParseSpec(v); err != nil {
			t.Errorf("下拉第 %d 项 %q（%s）内核认不得: %v", i, v, list[i], err)
		}
	}
	if !bucketIsCustom(len(list) - 1) {
		t.Errorf("最后一项得是「自定义」，否则专业用户没地方手填：%q", list[len(list)-1])
	}
	if got := bucketToken(0, ""); got != "" {
		t.Errorf("默认那项该传空串（= 自动找删除空档），实得 %q", got)
	}
	if got := bucketToken(len(list)-1, "  daily "); got != "daily" {
		t.Errorf("自定义该取手填文本并去掉首尾空格，实得 %q", got)
	}
	if got := bucketToken(len(list)-1, "   "); got != "" {
		t.Errorf("自定义留空该回落到默认，实得 %q", got)
	}
	if got := bucketToken(-1, "daily"); got != "" {
		t.Errorf("控件还没建好（index = -1）时该按默认，实得 %q", got)
	}
}

func TestDisplayListsFollowLanguage(t *testing.T) {
	before := i18n.Get()
	t.Cleanup(func() { i18n.Set(before) })

	// 显示文本表可以随语言整组换掉，但顺序和个数必须跟 values 表对齐，
	// 否则下拉第 i 项会把「每 15 分钟」的文案配到「每 1 天」的写法上。
	for _, lang := range []i18n.Lang{i18n.Zh, i18n.En} {
		i18n.Set(lang)
		if len(byDisplay()) != len(byValues) {
			t.Errorf("%s：分组显示表 %d 项 / 写法表 %d 项", lang, len(byDisplay()), len(byValues))
		}
		if len(levelDisplay()) != len(levelValues) {
			t.Errorf("%s：层级显示表 %d 项 / 写法表 %d 项", lang, len(levelDisplay()), len(levelValues))
		}
		if len(bucketDisplay()) != len(bucketValues) {
			t.Errorf("%s：分批显示表 %d 项 / 写法表 %d 项", lang, len(bucketDisplay()), len(bucketValues))
		}
		for _, d := range append(append(byDisplay(), levelDisplay()...), bucketDisplay()...) {
			if strings.TrimSpace(d) == "" {
				t.Errorf("%s：显示表里有空白项", lang)
			}
		}
	}

	i18n.Set(i18n.Zh)
	zhBucket, zhBy := bucketDisplay(), byDisplay()
	i18n.Set(i18n.En)
	enBucket, enBy := bucketDisplay(), byDisplay()
	if zhBy[0] == enBy[0] {
		t.Errorf("换语言后分组文字没变：%q", enBy[0])
	}
	for i := range zhBucket {
		if zhBucket[i] == enBucket[i] {
			t.Errorf("第 %d 项分批文字两侧相同：%q", i, enBucket[i])
		}
	}
	// 档位个数不随语言变，「自定义」始终落在最后一项。
	if !bucketIsCustom(len(enBucket) - 1) {
		t.Error("英文下最后一项不再是「自定义」")
	}
}

func TestAliveFilterTextExplainsWhyRowsDidNotChange(t *testing.T) {
	// 刚扫完的快照没有已删完分区：开启后行数不变，文案必须把原因讲清楚。
	on := aliveFilterText(true, 120, 120)
	for _, want := range []string{"已开启", "还没有「已删完」的分区", "视图行数不变"} {
		if !strings.Contains(on, want) {
			t.Errorf("开启且无变化时缺 %q：%q", want, on)
		}
	}
	hidden := aliveFilterText(true, 120, 100)
	for _, want := range []string{"隐藏已删完 20 个", "视图 100 行"} {
		if !strings.Contains(hidden, want) {
			t.Errorf("真隐藏了分区时缺 %q：%q", want, hidden)
		}
	}
	off := aliveFilterText(false, 120, 120)
	for _, want := range []string{"已关闭", "隐藏已删完 0 个"} {
		if !strings.Contains(off, want) {
			t.Errorf("关闭时缺 %q：%q", want, off)
		}
	}
}

// 下面这一组是双语改造的守卫：界面切成英文之后，「数字还在不在」是唯一的硬指标。
// 文案措辞可以改，但 120 个分区、50000 上限这类规模一旦在翻译里丢了，
// 用户就看不出这一轮到底动了多少东西。

func withLang(t *testing.T, l i18n.Lang, run func()) {
	t.Helper()
	before := i18n.Get()
	t.Cleanup(func() { i18n.Set(before) })
	i18n.Set(l)
	run()
}

func mustHave(t *testing.T, label, text string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(text, w) {
			t.Errorf("%s 缺 %q：%q", label, w, text)
		}
	}
}

func TestScaleNumbersSurviveInBothLanguages(t *testing.T) {
	for _, l := range []i18n.Lang{i18n.Zh, i18n.En} {
		withLang(t, l, func() {
			none := deleteButtonText(0, 0)
			if !strings.Contains(none, "⚠") {
				t.Errorf("%s：没勾选时删除按钮要把危险态说出来：%q", l, none)
			}
			// 「未勾选 / nothing ticked」两侧都得有，否则空勾选时看着像能删。
			if none == deleteButtonText(2, 3) {
				t.Errorf("%s：没勾选和勾了 2 个的按钮文案相同：%q", l, none)
			}
			some := deleteButtonText(294, 20255)
			mustHave(t, string(l)+"/删除按钮", some, "294", "20255")

			mustHave(t, string(l)+"/筛选-无变化", aliveFilterText(true, 120, 120), "120")
			mustHave(t, string(l)+"/筛选-有隐藏", aliveFilterText(true, 120, 100), "120", "20", "100")
			mustHave(t, string(l)+"/筛选-关闭", aliveFilterText(false, 120, 120), "120")

			long := briefIDs(makeIDs(120), 20)
			mustHave(t, string(l)+"/编号摘要", long, "1", "20", "120")
			if strings.Contains(long, "21") {
				t.Errorf("%s：第 21 个编号不该整串写出来：%q", l, long)
			}
			mustHave(t, string(l)+"/超限提示", overflowText(&meta.Overflow{
				Total: 60000, Taken: 50000,
				ByDrive: map[string][2]int{"C:": {30000, 40000}, "D:": {20000, 20000}},
			}), "60000", "50000", "10000", "70000", "C:", "D:")
		})
	}
}

func makeIDs(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i + 1
	}
	return out
}

func TestSummaryHeadersFitTheirColumns(t *testing.T) {
	// tty.Table 会把每格截到列宽，表头也一样：英文表头一长就变成「Time ra…」，
	// 日志里的统计表读起来像坏了。列宽是手调的中文字数，英文必须自己适配。
	for _, l := range []i18n.Lang{i18n.Zh, i18n.En} {
		withLang(t, l, func() {
			for _, c := range summaryCols {
				h := i18n.T(c.key)
				if tty.WLen(h) > c.width {
					t.Errorf("%s：表头 %q（宽 %d）超出列宽 %d，会被削成 %q",
						l, h, tty.WLen(h), c.width, tty.Truncate(h, c.width))
				}
			}
		})
	}
}

func TestDeleteSummaryChineseUnchanged(t *testing.T) {
	// 中文这一侧要逐字保持改造前的输出：表头、量词、分隔竖线一个都不许动，
	// 否则老用户和黄金样例都会看到陌生的字。
	withLang(t, i18n.Zh, func() {
		chosen := []*parts.Part{
			{PID: 3, Count: 2, Size: 2048, Label: "文档", TimeRange: "2026-10-01 10:00 ~ 10:15", IndexStart: 0, IndexEnd: 1},
			{PID: 7, Count: 3, Size: 4096, Label: "其他", TimeRange: "2026-10-02 09:00 ~ 09:30", IndexStart: 2, IndexEnd: 4},
		}
		rows := [][]string{
			{"3", "2026-10-01 10:00 ~ 10:15", "文档", "2", tty.Human(2048), "0-1"},
			{"7", "2026-10-02 09:00 ~ 09:30", "其他", "3", tty.Human(4096), "2-4"},
		}
		want := "选中分区 2 个: 3,7\n" +
			tty.Table([]string{"编号", "时间范围", "路径组/前缀组", "元素数", "大小", "index区间"},
				rows, []int{4, 24, 44, 7, 9, 12}) +
			"\n本次将永久删除 5 个条目，约 " + tty.Human(6144) + "（不可恢复）"
		if got := (&app{}).deleteSummary(chosen); got != want {
			t.Errorf("中文选中统计变了样：\n实得 %q\n要  %q", got, want)
		}
	})
}

func TestDeleteSummaryEnglishCarriesScale(t *testing.T) {
	withLang(t, i18n.En, func() {
		got := (&app{}).deleteSummary([]*parts.Part{
			{PID: 3, Count: 1, Size: 2048, Label: "其他", TimeRange: "2026-10-01", IndexStart: 0, IndexEnd: 0},
			{PID: 7, Count: 3, Size: 4096, Label: "文档", TimeRange: "2026-10-02", IndexStart: 1, IndexEnd: 3},
		})
		mustHave(t, "en/选中统计", got, "Selected 2 partitions: 3,7", "Time range",
			"Path/prefix group", "Index range", "This will", "permanently delete 4 items")
		if strings.Contains(got, "个") {
			t.Errorf("英文输出里漏了中文量词：%q", got)
		}
	})
}
