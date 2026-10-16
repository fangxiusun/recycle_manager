package engine

import (
	"bytes"
	"encoding/binary"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"rbm/internal/meta"
	"rbm/internal/parts"
	"rbm/internal/store"
	"rbm/internal/winfs"
)

// ---- 造一个临时假回收站：只碰 t.TempDir() 里自建的 $I/$R ----

func writeI(t *testing.T, dir, stem, origPath string, size int64, when time.Time) {
	t.Helper()
	units := utf16.Encode([]rune(origPath + "\x00"))
	payload := make([]byte, 2*len(units))
	for i, u := range units {
		binary.LittleEndian.PutUint16(payload[2*i:], u)
	}
	head := make([]byte, 28)
	binary.LittleEndian.PutUint64(head[0:], 2)
	binary.LittleEndian.PutUint64(head[8:], uint64(size))
	binary.LittleEndian.PutUint64(head[16:], uint64(when.UTC().UnixNano()/100+winfs.FiletimeEpochTicks))
	binary.LittleEndian.PutUint32(head[24:], uint32(len(units)))
	raw := append(head, payload...)
	if err := os.WriteFile(filepath.Join(dir, "$I"+stem), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeEntry 造一个条目：$I + $R（$R 可以是文件，也可以是带子项的目录）。
func writeEntry(t *testing.T, dir, stem, origPath string, size int64, when time.Time, asDir bool) {
	t.Helper()
	writeI(t, dir, stem, origPath, size, when)
	rPath := filepath.Join(dir, "$R"+stem)
	if asDir {
		sub := filepath.Join(rPath, "nested")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sub, "leaf.txt"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if err := os.WriteFile(rPath, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fakeSnapshot 扫临时目录并建快照，返回快照、临时目录与状态目录。
func fakeSnapshot(t *testing.T, count int) (*store.State, string, string) {
	t.Helper()
	bin := t.TempDir()
	stateDir := filepath.Join(t.TempDir(), ".rbm")
	base := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	for i := 1; i <= count; i++ {
		stem := fmt.Sprintf("%05dfile%d.bin", i, i)
		writeEntry(t, bin, stem, fmt.Sprintf(`Z:\work\build%d\out\file%d.bin`, i%2+1, i),
			int64(1000*i), base.Add(time.Duration(i)*time.Minute), i%3 == 0)
	}
	items, issues, _ := meta.Scan([]meta.BinDir{{Drive: "Z", Path: bin}}, 0)
	st, err := BuildState(items, issues, Options{Bucket: "gap:5m", MaxBuckets: 12, By: "dir",
		PrefixLen: 3, MaxParts: 120, MinCount: 1, DirLevel: "2", StateDir: stateDir, DrivesSpec: "Z"})
	if err != nil {
		t.Fatal(err)
	}
	return st, bin, stateDir
}

// ---- BuildState ----

func TestBuildStateParamText(t *testing.T) {
	st, _, stateDir := fakeSnapshot(t, 12)
	if st.Params.Bucket != "gap:5m" || st.Params.Drives != "Z" {
		t.Errorf("参数没写进快照: %+v", st.Params)
	}
	if st.Created == "" {
		t.Error("快照缺少创建时间")
	}
	total := 0
	for _, p := range st.Parts {
		if p.IndexStart != total+1 {
			t.Fatalf("分区 %d 的 index 不连续", p.PID)
		}
		total = p.IndexEnd
	}
	if total != 12 {
		t.Errorf("分区覆盖 %d 条，期望 12", total)
	}
	// 没有异常时 issues 也得是空数组：Python 版写出来是 []，nil 会变成 null，口径就变了。
	if st.Issues == nil {
		t.Error("空 issues 应当是空切片而不是 nil")
	}
	if err := store.Write(stateDir, st); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(stateDir, store.StateName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"issues":[]`) {
		t.Errorf("空 issues 落盘应当是 []，实得片段 %q", string(raw)[min(200, len(raw)):])
	}
}

func TestBuildStateEmptyAndDefaults(t *testing.T) {
	if _, err := BuildState(nil, nil, Options{}); err == nil || !strings.Contains(err.Error(), "条目为 0") {
		t.Errorf("空条目应当报错: %v", err)
	}
	item := &meta.Item{Display: "a.txt", MetaVersion: 2, Epoch: 100, DeletedAt: "1970-01-01 08:01:40",
		Size: 1, Drive: "C", OrigPath: `C:\a.txt`}
	st, err := BuildState([]*meta.Item{item}, []string{"一条异常"}, Options{MaxBuckets: 7, By: "dir", DirLevel: "2"})
	if err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf("gap:自动断点(<= %d 桶)", 7); st.Params.Bucket != want {
		t.Errorf("bucket 留空时的说明应为 %q，实得 %q", want, st.Params.Bucket)
	}
	if st.Params.Drives != "all" {
		t.Errorf("drives 留空时应记 all，实得 %q", st.Params.Drives)
	}
	if len(st.Issues) != 1 {
		t.Errorf("异常记录没带进快照: %v", st.Issues)
	}
}

// ---- SortedParts / SelectedParts ----

func stateWithParts(ps ...*parts.Part) *store.State {
	return &store.State{Parts: ps}
}

func TestSortedParts(t *testing.T) {
	st := stateWithParts(
		&parts.Part{PID: 1, Count: 5, Size: 300},
		&parts.Part{PID: 2, Count: 9, Size: 100},
		&parts.Part{PID: 3, Count: 9, Size: 900},
		&parts.Part{PID: 4, Count: 2, Size: 2, Deleted: 2}, // 已删完
	)
	if got := len(SortedParts(st, "time", false)); got != 4 {
		t.Errorf("time 排序不该丢行: %d", got)
	}
	if got := SortedParts(st, "time", true); len(got) != 3 || got[2].PID != 3 {
		// alive=true 会把已删完的 4 号剔除，剩下的仍按编号序
		t.Errorf("time 顺序应保持编号序: %v", pids(got))
	}
	bySize := SortedParts(st, "size", true)
	if bySize[0].PID != 3 || bySize[1].PID != 1 || bySize[2].PID != 2 {
		t.Errorf("size 降序不符: %v", pids(bySize))
	}
	byCount := SortedParts(st, "count", true)
	if byCount[0].PID != 2 || byCount[1].PID != 3 { // 元素数相同按编号小者优先
		t.Errorf("count 排序不符: %v", pids(byCount))
	}
	if got := SortedParts(st, "未知口径", true); len(got) != 3 {
		t.Errorf("未知排序口径应当退回编号序，实得 %d 行", len(got))
	}
}

func pids(ps []*parts.Part) []int {
	out := make([]int, len(ps))
	for i, p := range ps {
		out[i] = p.PID
	}
	return out
}

func TestSelectedPartsPrefix(t *testing.T) {
	items := []*meta.Item{
		{Display: "esbuild-a.js", Size: 10},
		{Display: "ESBUILD-b.js", Size: 20}, // 前缀匹配忽略大小写
		{Display: "notes.md", Size: 30},
	}
	st := &store.State{Items: items, Parts: []*parts.Part{
		{PID: 1, Count: 3, Size: 60, Positions: []int{0, 1, 2}, Label: `C:\x`},
		{PID: 2, Count: 1, Size: 5, Positions: []int{0}, Label: `C:\y`},
	}}
	all := SelectedParts(st, []int{1, 2}, "")
	if len(all) != 2 || all[0].Count != 3 || all[1].Count != 1 {
		t.Fatalf("按编号取分区不符: %+v", all)
	}
	if TotalSize(all) != 70 {
		// SelectedParts 按 positions 逐条重算大小，不信任快照里声明的 Size：60 + 10
		t.Errorf("TotalSize = %d，期望 70", TotalSize(all))
	}
	got := SelectedParts(st, []int{1}, "esbuild")
	if len(got) != 1 || got[0].Count != 2 || got[0].Size != 30 {
		t.Fatalf("前缀过滤不符: %+v", got)
	}
	if got[0].Label != `C:\x/esbuild*` {
		t.Errorf("过滤后的标签应标明前缀，实得 %q", got[0].Label)
	}
	if st.Parts[0].Count != 3 || len(st.Parts[0].Positions) != 3 {
		t.Error("SelectedParts 改动了原快照的分区")
	}
	if none := SelectedParts(st, []int{1}, "zzz"); len(none) != 0 {
		t.Errorf("没有命中应当返回空，实得 %d 个", len(none))
	}
	if len(SelectedParts(st, nil, "")) != 0 {
		t.Error("空编号列表应当返回空")
	}
}

func TestMatchPrefix(t *testing.T) {
	it := &meta.Item{Display: "Vue.config.js"}
	for _, c := range []struct {
		needle string
		want   bool
	}{
		{"vue", true}, {"VUE", true}, {"vue.config", true}, {"config", false}, {"", true},
	} {
		if got := MatchPrefix(it, c.needle); got != c.want {
			t.Errorf("MatchPrefix(%q) = %v，期望 %v", c.needle, got, c.want)
		}
	}
}

// ---- DeleteOne ----

func TestDeleteOneFileAndTree(t *testing.T) {
	dir := t.TempDir()
	when := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	writeEntry(t, dir, "00001a.bin", `Z:\work\a.bin`, 10, when, false)
	writeEntry(t, dir, "00002b.bin", `Z:\work\b`, 20, when, true)

	itemA := &meta.Item{IPath: filepath.Join(dir, "$I00001a.bin"), RPath: filepath.Join(dir, "$R00001a.bin")}
	ok, note := DeleteOne(itemA)
	if !ok || note != "已删" {
		t.Errorf("删文件条目失败: %v %s", ok, note)
	}
	if winfs.Exists(itemA.IPath) || winfs.Exists(itemA.RPath) {
		t.Error("$I 与 $R 应当都不在了")
	}
	itemB := &meta.Item{IPath: filepath.Join(dir, "$I00002b.bin"), RPath: filepath.Join(dir, "$R00002b.bin")}
	ok, note = DeleteOne(itemB)
	if !ok {
		t.Errorf("删目录条目失败: %v %s", ok, note)
	}
	if winfs.Exists(filepath.Join(itemB.RPath, "nested", "leaf.txt")) || winfs.Exists(itemB.RPath) {
		t.Error("目录树没有被递归删干净")
	}
	// 已经不在了也算成功（重跑、或用户手动清空过）。
	ok, note = DeleteOne(itemA)
	if !ok || note != "已删" {
		t.Errorf("重复删除应当按成功处理: %v %s", ok, note)
	}
}

// ---- RunDelete：保险丝、账本、快照更新、进度节流 ----

func TestRunDeleteFuseAbortsEverything(t *testing.T) {
	st, bin, stateDir := fakeSnapshot(t, 9)
	chosen := SelectedParts(st, []int{1}, "")
	before := countI(t, bin)
	rep, err := RunDelete(st, chosen, stateDir, 1, 100, false, nil)
	if err == nil || !strings.Contains(err.Error(), "超过 --max") {
		t.Fatalf("保险丝该拦下来: %v", err)
	}
	if rep != nil {
		t.Error("保险丝触发时不该有半成品报告")
	}
	if countI(t, bin) != before {
		t.Error("保险丝触发时一个文件都不该动")
	}
	if _, err := os.Stat(filepath.Join(stateDir, store.LedgerName)); !os.IsNotExist(err) {
		t.Error("保险丝触发时不该开账本")
	}
	if _, err := os.Stat(filepath.Join(stateDir, store.StateName)); !os.IsNotExist(err) {
		t.Error("保险丝触发时不该写快照")
	}
}

func TestRunDeleteMarksStateAndLedger(t *testing.T) {
	st, bin, stateDir := fakeSnapshot(t, 20)
	chosen := SelectedParts(st, []int{1}, "")
	want := chosen[0].Count
	before := countI(t, bin)

	var seen []int
	rep, err := RunDelete(st, chosen, stateDir, 0, 2, false, func(n, total, done, failed int) {
		seen = append(seen, n)
		if n > total {
			t.Errorf("进度 n=%d 超过 total=%d", n, total)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Done != want || rep.Failed != 0 {
		t.Errorf("删除结果 %+v，期望 Done=%d Failed=0", rep, want)
	}
	if got := countI(t, bin); got != before-want {
		t.Errorf("$I 数量 %d，期望 %d", got, before-want)
	}
	// every=2：第 2、4…条与最后一条回调。
	wantSeen := []int{}
	for n := 2; n < want; n += 2 {
		wantSeen = append(wantSeen, n)
	}
	wantSeen = append(wantSeen, want)
	if strings.Join(ints(seen), ",") != strings.Join(ints(wantSeen), ",") {
		t.Errorf("进度回调节流不符: %v，期望 %v", seen, wantSeen)
	}
	if st.Parts[0].Deleted != want {
		t.Errorf("快照 deleted 标记 %d，期望 %d", st.Parts[0].Deleted, want)
	}
	for i, p := range st.Parts {
		if p.PID != i+1 {
			t.Fatalf("删除后编号变了: 第 %d 个 pid=%d", i+1, p.PID)
		}
	}
	if st.LastDelete == "" {
		t.Error("没有记录最近删除时间")
	}
	if got := st.AliveLeft(); got != len(st.Items)-want {
		t.Errorf("未删完条目数 %d，期望 %d", got, len(st.Items)-want)
	}
	rows := readCSV(t, filepath.Join(stateDir, store.LedgerName))
	if len(rows) != 1+want {
		t.Errorf("账本 %d 行，期望 %d 行（含表头）", len(rows), 1+want)
	}
	if rows[0][0] != "操作时间" || rows[1][1] != "1" || rows[1][8] != "已删" {
		t.Errorf("账本内容不符: %v / %v", rows[0], rows[1])
	}
	// 快照落盘且能读回来，deleted 标记跟着持久化。
	back, err := store.Read(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if back.Parts[0].Deleted != want || len(back.Items) != len(st.Items) {
		t.Errorf("重读快照不符: %+v", back.Parts[0])
	}
	// 再删同一批：条目已经不在，按成功处理但不会重复计数。
	rep2, err := RunDelete(st, SelectedParts(st, []int{1}, ""), stateDir, 0, 100, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep2.Done != want {
		t.Errorf("重跑同一批 Done=%d，期望 %d", rep2.Done, want)
	}
	if st.Parts[0].Deleted != 2*want {
		t.Errorf("重跑后 deleted 累加到 %d（口径：按次计数，不重复扣减）", st.Parts[0].Deleted)
	}
}

func TestRunDeleteVerboseReportsEveryItem(t *testing.T) {
	st, _, stateDir := fakeSnapshot(t, 6)
	chosen := SelectedParts(st, []int{1}, "")
	n := 0
	if _, err := RunDelete(st, chosen, stateDir, 0, 5, true, func(int, int, int, int) { n++ }); err != nil {
		t.Fatal(err)
	}
	if n != chosen[0].Count {
		t.Errorf("verbose 下每条都该回调: %d vs %d", n, chosen[0].Count)
	}
}

func TestRunDeleteFuseNotTrippedAtExactLimit(t *testing.T) {
	st, _, stateDir := fakeSnapshot(t, 8)
	chosen := SelectedParts(st, []int{1}, "")
	if _, err := RunDelete(st, chosen, stateDir, chosen[0].Count, 100, false, nil); err != nil {
		t.Errorf("正好等于保险丝上限时应当放行: %v", err)
	}
}

func countI(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "$I") {
			n++
		}
	}
	return n
}

func readCSV(t *testing.T, path string) [][]string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	r := csv.NewReader(bytes.NewReader(raw))
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatalf("%s 解析失败: %v", path, err)
	}
	return rows
}

func ints(xs []int) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = fmt.Sprint(x)
	}
	return out
}
