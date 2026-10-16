package gui

import (
	"strings"
	"testing"

	"github.com/lxn/walk"

	"rbm/internal/meta"
	"rbm/internal/parts"
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
	if len(bucketValues) != len(bucketDisplay) {
		t.Fatalf("写法表和文字表长度不一致: %d vs %d", len(bucketValues), len(bucketDisplay))
	}
	for i, v := range bucketValues {
		if bucketIsCustom(i) {
			continue // 「自定义」那项不给固定写法，值来自手填框
		}
		if _, err := parts.ParseSpec(v); err != nil {
			t.Errorf("下拉第 %d 项 %q（%s）内核认不得: %v", i, v, bucketDisplay[i], err)
		}
	}
	if !bucketIsCustom(len(bucketDisplay) - 1) {
		t.Errorf("最后一项得是「自定义」，否则专业用户没地方手填：%q", bucketDisplay[len(bucketDisplay)-1])
	}
	if got := bucketToken(0, ""); got != "" {
		t.Errorf("默认那项该传空串（= 自动找删除空档），实得 %q", got)
	}
	if got := bucketToken(len(bucketDisplay)-1, "  daily "); got != "daily" {
		t.Errorf("自定义该取手填文本并去掉首尾空格，实得 %q", got)
	}
	if got := bucketToken(len(bucketDisplay)-1, "   "); got != "" {
		t.Errorf("自定义留空该回落到默认，实得 %q", got)
	}
	if got := bucketToken(-1, "daily"); got != "" {
		t.Errorf("控件还没建好（index = -1）时该按默认，实得 %q", got)
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
