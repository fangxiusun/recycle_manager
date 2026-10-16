package store

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"rbm/internal/meta"
	"rbm/internal/parts"
)

func copyFixture(t *testing.T, dest string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "py_state.json"))
	if err != nil {
		t.Fatalf("读 fixture 失败: %v", err)
	}
	if err := os.WriteFile(dest, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestReadsPythonState 用 Python 版真正写出的 state.json 验证跨语言可读：
// 字段名、类型（dir_level 是字符串）、编号与 index 区间都必须对得上。
func TestReadsPythonState(t *testing.T) {
	dir := t.TempDir()
	copyFixture(t, filepath.Join(dir, StateName))
	st, err := Read(dir)
	if err != nil {
		t.Fatalf("读 python 写的快照失败: %v", err)
	}
	if len(st.Items) != 150 {
		t.Errorf("条目数 %d，期望 150", len(st.Items))
	}
	if len(st.Parts) != 18 {
		t.Errorf("分区数 %d，期望 18", len(st.Parts))
	}
	if st.Params.By != "dir" || st.Params.DirLevel != "2" || st.Params.Bucket != "gap:5m" ||
		st.Params.PrefixLen != 3 || st.Params.MaxBuckets != 12 || st.Params.MaxParts != 120 ||
		st.Params.MinCount != 5 || st.Params.Drives != "Z" {
		t.Errorf("参数读错: %+v", st.Params)
	}
	if len(st.Issues) != 1 || !strings.Contains(st.Issues[0], "$R 数据不存在") {
		t.Errorf("异常记录没读出来: %v", st.Issues)
	}
	if st.Created == "" || len(st.Bounds) < 2 {
		t.Errorf("created/bounds 缺失: %q %d", st.Created, len(st.Bounds))
	}
	total := 0
	for i, p := range st.Parts {
		if p.PID != i+1 {
			t.Fatalf("编号不连续: 第 %d 个分区 pid=%d", i+1, p.PID)
		}
		if p.IndexStart != total+1 || p.IndexEnd != total+p.Count {
			t.Errorf("分区 %d 的 index 区间与累计不符: %d-%d vs %d", p.PID, p.IndexStart, p.IndexEnd, total)
		}
		total = p.IndexEnd
		if p.TimeRange == "" || p.Drives != "Z" {
			t.Errorf("分区 %d 的展示字段缺失: %q %q", p.PID, p.TimeRange, p.Drives)
		}
		if p.Deleted != 0 {
			t.Errorf("新快照的 deleted 应当是 0，实得 %d", p.Deleted)
		}
	}
	if total != len(st.Items) {
		t.Errorf("分区覆盖 %d 条，条目 %d 条", total, len(st.Items))
	}
	for _, it := range st.Items {
		if it.Drive != "Z" || !strings.HasPrefix(it.IPath, `Z:\$Recycle.Bin\`) {
			t.Fatalf("条目的盘符/路径没读对: %+v", it)
		}
	}
}

func sampleState() *State {
	return &State{
		Created: "2026-10-15 12:00:00",
		Params:  Params{Bucket: "gap:5m", By: "dir", PrefixLen: 3, DirLevel: "2", MaxBuckets: 12, MaxParts: 120, MinCount: 5, Drives: "all"},
		Bounds:  []float64{1, 2, 3},
		Items: []*meta.Item{
			{Display: "a<b>.txt", MetaVersion: 2, Epoch: 1, DeletedAt: "2026-10-15 12:00:00", Size: 10, Drive: "C",
				OrigPath: `C:\x\,y\a<b>.txt`, IPath: `C:\$Recycle.Bin\S\$Ia`, RPath: `C:\$Recycle.Bin\S\$Ra`, Exists: true},
			{Display: "笔记.md", MetaVersion: -1, Epoch: 2, DeletedAt: "2026-10-15 12:05:00", Size: 20, Drive: "D",
				OrigPath: parts.Illegal, IPath: `C:\$Recycle.Bin\S\$Ib`, RPath: `C:\$Recycle.Bin\S\$Rb`},
		},
		Parts: []*parts.Part{
			{PID: 1, IndexStart: 1, IndexEnd: 1, Count: 1, Size: 10, Positions: []int{0}, Label: `C:\x`, By: "dir",
				DirLevel: "2", TimeRange: "2026-10-15 12:00~12:00", Drives: "C"},
			{PID: 2, IndexStart: 2, IndexEnd: 2, Count: 1, Size: 20, Positions: []int{1}, Label: parts.Illegal, By: "dir",
				DirLevel: "2", TimeRange: "1970-01-01 08:00", Drives: "D", Deleted: 1},
		},
		Issues: []string{"一条异常"},
	}
}

func TestWriteReadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := sampleState()
	if err := Write(dir, want); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(Path(dir) + ".tmp"); !os.IsNotExist(err) {
		t.Error("原子写留下了 .tmp 文件")
	}
	raw, err := os.ReadFile(Path(dir))
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(raw) {
		t.Error("写出的不是合法 JSON")
	}
	// Go 的 encoding/json 默认转义 < >（\u003c），仍是合法 JSON，Python 读得懂——这里锁定这一事实。
	if !strings.Contains(string(raw), `\u003c`) {
		t.Error("state.json 里 < 应当被 Go 转义成 \\u003c")
	}
	got, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("往返后内容变了:\n得到 %+v\n期望 %+v", got, want)
	}
}

func TestReadMissingAndBroken(t *testing.T) {
	dir := t.TempDir()
	if _, err := Read(dir); err == nil || !strings.Contains(err.Error(), "没有找到快照") {
		t.Errorf("缺快照的报错不对: %v", err)
	}
	if err := os.WriteFile(Path(dir), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(dir); err == nil || !strings.Contains(err.Error(), "解析失败") {
		t.Errorf("坏快照的报错不对: %v", err)
	}
}

func TestGroupColAndAliveLeft(t *testing.T) {
	st := sampleState()
	if got := st.GroupCol(); got != "路径组" {
		t.Errorf("by=dir 的列名应为 路径组，实得 %q", got)
	}
	st.Params.By = "prefix"
	if got := st.GroupCol(); got != "前缀组" {
		t.Errorf("by=prefix 的列名应为 前缀组，实得 %q", got)
	}
	if got := st.AliveLeft(); got != 1 { // 第二条分区已删完
		t.Errorf("未删完条目数应为 1，实得 %d", got)
	}
}

func TestWriteListCSV(t *testing.T) {
	dir := t.TempDir()
	st := sampleState()
	path, err := WriteListCSV(dir, st)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(raw), "\xef\xbb\xbf") {
		t.Error("缺少 UTF-8 BOM，Excel 会乱码")
	}
	r := csv.NewReader(strings.NewReader(string(raw[3:])))
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	wantHeader := []string{"index", "编号", "时间范围", "路径组/前缀组", "删除时间", "字节", "盘", "文件名", "原路径", "$I路径", "$R数据存在"}
	if !reflect.DeepEqual(rows[0], wantHeader) {
		t.Errorf("表头不符:\n%v\n%v", rows[0], wantHeader)
	}
	if len(rows) != 1+len(st.Items) {
		t.Fatalf("行数 %d，期望 %d", len(rows), 1+len(st.Items))
	}
	// index 是 1 基全局序号，且必须落在所属分区声明的 index 区间内——这正是 Python 版修过的坑。
	for i, row := range rows[1:] {
		if row[0] != strconv.Itoa(i+1) {
			t.Errorf("第 %d 行 index=%s，期望 %d", i+1, row[0], i+1)
		}
		p := st.Parts[i]
		if row[1] != strconv.Itoa(p.PID) {
			t.Errorf("第 %d 行编号=%s，期望 %d", i+1, row[1], p.PID)
		}
		if row[2] != p.TimeRange || row[3] != p.Label {
			t.Errorf("第 %d 行的时间范围/分组不符: %v", i+1, row[:4])
		}
		if row[10] != []string{"1", "0"}[i] {
			t.Errorf("第 %d 行 $R 存在标志不符: %s", i+1, row[10])
		}
	}
	// 含逗号的原路径必须整体加引号，不能被拆成两列。
	if rows[1][8] != `C:\x\,y\a<b>.txt` {
		t.Errorf("带逗号的原路径读回来变了: %q", rows[1][8])
	}
}

func TestLedgerHeaderAndAppend(t *testing.T) {
	dir := t.TempDir()
	l, err := OpenLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Row(1, 2, sampleState().Items[0], "已删"); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	l2, err := OpenLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := l2.Row(2, 1, sampleState().Items[1], "失败: x"); err != nil {
		t.Fatal(err)
	}
	if err := l2.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, LedgerName))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(string(raw))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("账本行数 %d，期望 3（表头 + 两笔）", len(rows))
	}
	if rows[0][0] != "操作时间" || rows[0][8] != "结果" {
		t.Errorf("表头不符: %v", rows[0])
	}
	wantRow := []string{"1", "2", "2026-10-15 12:00:00", "10", "C", "a<b>.txt", `C:\x\,y\a<b>.txt`, "已删"}
	if !reflect.DeepEqual(rows[1][1:], wantRow) {
		t.Errorf("第一笔记账字段不符:\n%v\n%v", rows[1][1:], wantRow)
	}
	if rows[2][1] != "2" || rows[2][8] != "失败: x" {
		t.Errorf("第二笔记账字段不符: %v", rows[2])
	}
	if rows[1][0] == "" {
		t.Error("操作时间为空")
	}
}

func TestSplitDrives(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"c,G,", []string{"C", "G"}},
		{"z:", []string{"Z"}},
		{" C , D ", []string{"C", "D"}},
		{"", nil},
		{",,", nil},
	}
	for _, c := range cases {
		if got := SplitDrives(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("SplitDrives(%q) = %v，期望 %v", c.in, got, c.want)
		}
	}
}

func TestDefaultStateDirIsCWD(t *testing.T) {
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	if got := DefaultStateDir(); got != filepath.Join(dir, ".rbm") {
		t.Errorf("默认快照目录应为 <cwd>/.rbm，实得 %q", got)
	}
}
