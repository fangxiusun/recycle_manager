package gui

import (
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
