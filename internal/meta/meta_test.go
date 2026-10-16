package meta

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"rbm/internal/winfs"
)

// ---- 造 $I 文件：全部落在 t.TempDir()，不碰真实回收站 ----

func encodePath(p string) []byte {
	units := utf16.Encode([]rune(p + "\x00"))
	b := make([]byte, 2*len(units))
	for i, u := range units {
		binary.LittleEndian.PutUint16(b[2*i:], u)
	}
	return b
}

// iBytes 按 v2 头部拼一个 $I：q@0 版本、q@8 大小、q@16 FILETIME、u32@24 路径字符数、utf-16-le 路径。
// v1 的差别只有：字符数是 q@24，路径从 32 起。
func iBytes(origPath string, size int64, when time.Time, version int) []byte {
	ft := when.UTC().UnixNano()/100 + winfs.FiletimeEpochTicks
	payload := encodePath(origPath)
	head := make([]byte, 28)
	if version < 2 {
		head = make([]byte, 32)
	}
	binary.LittleEndian.PutUint64(head[0:], uint64(version))
	binary.LittleEndian.PutUint64(head[8:], uint64(size))
	binary.LittleEndian.PutUint64(head[16:], uint64(ft))
	if version >= 2 {
		binary.LittleEndian.PutUint32(head[24:], uint32(len(payload)/2))
	} else {
		binary.LittleEndian.PutUint64(head[24:], uint64(len(payload)/2))
	}
	return append(head, payload...)
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readBack(t *testing.T, path string) *iHead {
	t.Helper()
	h, err := parseIFile(path)
	if err != nil {
		t.Fatalf("解析 %s 失败: %v", filepath.Base(path), err)
	}
	return h
}

// ---- parseIFile ----

func TestParseIFileV2(t *testing.T) {
	dir := t.TempDir()
	when := time.Date(2026, 10, 15, 12, 34, 56, 0, time.UTC)
	p := filepath.Join(dir, "$I00001note.txt")
	writeFile(t, p, iBytes(`C:\Users\x\Documents\note.txt`, 4096, when, 2))
	h := readBack(t, p)
	if h.metaVersion != 2 || h.size != 4096 {
		t.Errorf("头部字段不符: %+v", h)
	}
	if h.origPath != `C:\Users\x\Documents\note.txt` {
		t.Errorf("原路径解码错: %q", h.origPath)
	}
	if got := FiletimeToTime(h.filetime); got.UTC().Format("2006-01-02 15:04:05") != "2026-10-15 12:34:56" {
		t.Errorf("FILETIME 换算错: %v", got)
	}
}

func TestParseIFileV1(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "$I00002old.bin")
	writeFile(t, p, iBytes(`D:\data\old.bin`, 123, time.Unix(1700000000, 0), 1))
	h := readBack(t, p)
	if h.metaVersion != 1 || h.size != 123 {
		t.Errorf("v1 头部字段不符: %+v", h)
	}
	if h.origPath != `D:\data\old.bin` {
		t.Errorf("v1 原路径解码错: %q", h.origPath)
	}
}

func TestParseIFileDamage(t *testing.T) {
	dir := t.TempDir()
	long := `C:\Users\x\Temp\` + strings.Repeat("d", 300) + ".txt"
	full := iBytes(long, 1, time.Unix(1700000000, 0), 2)

	// 声明长度比实际大出一截：判损坏（Python 版同样是 |差值|>64 才算坏）。
	truncated := filepath.Join(dir, "$Ishort")
	writeFile(t, truncated, full[:len(full)-200])
	if _, err := parseIFile(truncated); err == nil {
		t.Error("被截断 200 字节的 $I 应当判损坏")
	}
	// 容差内（尾部多出 64 字节以内）仍然可解析，和 Python 一致。
	padded := filepath.Join(dir, "$Ipadded")
	writeFile(t, padded, append(append([]byte{}, full...), make([]byte, 40)...))
	if h, err := parseIFile(padded); err != nil || h.origPath != long {
		t.Errorf("尾部 40 字节容差内应当可解析: %v %v", h, err)
	}
	// 头部不足 28 字节。
	tooShort := filepath.Join(dir, "$Itiny")
	writeFile(t, tooShort, full[:20])
	if _, err := parseIFile(tooShort); err == nil {
		t.Error("20 字节的 $I 应当报错")
	}
	// 路径字符数为 0 → 空路径。
	empty := filepath.Join(dir, "$Iempty")
	b := append([]byte{}, full...)
	binary.LittleEndian.PutUint32(b[24:], 0)
	writeFile(t, empty, b)
	if _, err := parseIFile(empty); err == nil {
		t.Error("路径为空应当报错")
	}
	// 读不到的文件也是错误而不是 panic。
	if _, err := parseIFile(filepath.Join(dir, "$Inope")); err == nil {
		t.Error("文件不存在应当报错")
	}
}

// ---- buildItem 的展示字段 ----

func TestBuildItemDisplay(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		iName, orig, want string
		unsafe            bool
		size              int64
	}{
		{"$I0000A", `C:\Users\x\Temp\a.txt`, "a.txt", false, 10},
		{"$I0000B", `C:\Users\x\Temp\sub\`, "sub", false, 10},
		{"$I0000C", `::{6D809000}`, "{6D809000}", true, 10},
		{"$I0000D", `C:\`, "0000D", false, 10},        // 光盘符：ntpath 给空串，退回 $I 文件名尾巴（同 Python）
		{"$I0000E", `C:\x\y.bin`, "y.bin", false, -5}, // 负数大小夹到 0
	}
	for _, c := range cases {
		p := filepath.Join(dir, c.iName)
		h := &iHead{metaVersion: 2, size: c.size, filetime: 116444736000000000, origPath: c.orig}
		it := h.buildItem(p)
		if it.Display != c.want {
			t.Errorf("origPath=%q 的 display 期望 %q，实得 %q", c.orig, c.want, it.Display)
		}
		if it.Unrecoverable != c.unsafe {
			t.Errorf("origPath=%q 的 unrecoverable 期望 %v，实得 %v", c.orig, c.unsafe, it.Unrecoverable)
		}
		if c.size < 0 && it.Size != 0 {
			t.Errorf("负数 size 应当夹到 0，实得 %d", it.Size)
		}
	}
	// 原路径以分隔符结尾且去掉后没有名字（如 "C:\" 之外的 "\"）时，退回用 $I 文件名；
	// 文件名也短到没有剩余字符时才是「(根)」。
	root := &iHead{metaVersion: 2, size: 1, filetime: 1, origPath: `\`}
	if got := root.buildItem(filepath.Join(dir, "$I0000F")).Display; got != "0000F" {
		t.Errorf("display 为空时应当退回 $I 文件名后缀，实得 %q", got)
	}
	if got := root.buildItem(filepath.Join(dir, "$I")).Display; got != "(根)" {
		t.Errorf("$I 文件名也没有后缀时应当是 (根)，实得 %q", got)
	}
}

// ---- Scan：配对、异常记录、截断 ----

func makePair(t *testing.T, dir, stem, origPath string, size int64, when time.Time, version int) {
	t.Helper()
	writeFile(t, filepath.Join(dir, "$I"+stem), iBytes(origPath, size, when, version))
	writeFile(t, filepath.Join(dir, "$R"+stem), []byte(strings.Repeat("x", 16)))
}

func TestScanPairsAndIssues(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)
	makePair(t, dir, "001a.txt", `Z:\work\build1\out\a.txt`, 100, base, 2)
	makePair(t, dir, "002b.txt", `Z:\work\build2\out\b.txt`, 200, base.Add(time.Minute), 2)
	// 只有 $I，没有 $R：残留元数据，要进 issues。
	writeFile(t, filepath.Join(dir, "$I003c.txt"), iBytes(`Z:\work\build1\out\c.txt`, 300, base, 2))
	// 只有 $R，没有 $I：孤儿，要进 issues 且不处理。
	writeFile(t, filepath.Join(dir, "$R004d.txt"), []byte("y"))
	// 坏掉的 $I：归为「元数据损坏」条目而不是丢掉。
	writeFile(t, filepath.Join(dir, "$I005e.txt"), []byte("garbage"))

	items, issues, overflow := Scan([]BinDir{{Drive: "Z", Path: dir}}, 0)
	if overflow != nil {
		t.Errorf("没超上限却报了 overflow: %+v", overflow)
	}
	if len(items) != 4 {
		t.Fatalf("条目数 %d，期望 4（含 1 个损坏条目）", len(items))
	}
	var corrupt int
	for _, it := range items {
		if it.Drive != "Z" {
			t.Errorf("盘符应当是 Z: %+v", it)
		}
		if !strings.HasPrefix(it.IPath, dir) || !strings.HasPrefix(it.RPath, dir) {
			t.Errorf("路径没落在临时目录: %s / %s", it.IPath, it.RPath)
		}
		if it.MetaVersion < 0 {
			corrupt++
			if it.OrigPath != "(元数据损坏)" {
				t.Errorf("损坏条目的占位原路径不符: %q", it.OrigPath)
			}
			if it.DeletedAt != "1970-01-01 00:00:00" {
				t.Errorf("损坏条目的时间戳不符: %q", it.DeletedAt)
			}
		}
	}
	if corrupt != 1 {
		t.Errorf("损坏条目数 %d，期望 1", corrupt)
	}
	joined := strings.Join(issues, "\n")
	if !strings.Contains(joined, "$R004d.txt") || !strings.Contains(joined, "缺少 $I") {
		t.Errorf("没有报孤儿 $R: %v", issues)
	}
	if !strings.Contains(joined, "$I003c.txt") || !strings.Contains(joined, "$R 数据不存在") {
		t.Errorf("没有报残留 $I: %v", issues)
	}
	if !strings.Contains(joined, "$I005e.txt") || !strings.Contains(joined, "解析失败") {
		t.Errorf("没有报解析失败: %v", issues)
	}
	if items[0].Display != "a.txt" {
		t.Errorf("条目应按 $I 名字排序，第一条实为 %q", items[0].Display)
	}
	if !items[0].Exists || items[2].Exists {
		t.Errorf("Exists 标志不符: %v %v", items[0].Exists, items[2].Exists)
	}
}

func TestScanMaxScanTruncation(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	const total = 40
	for i := 1; i <= total; i++ {
		stem := fmt.Sprintf("%03ditem%d.bin", i, i)
		when := base.Add(time.Duration(i) * 7 * time.Minute)
		makePair(t, dir, stem, fmt.Sprintf(`Z:\work\build%d\out\art%d.bin`, i%4, i), 1024, when, 2)
		// 截断时按 $I 的写入时间取最老的：这里让 mtime 与删除时间一致。
		if err := os.Chtimes(filepath.Join(dir, "$I"+stem), when, when); err != nil {
			t.Fatal(err)
		}
	}
	items, _, ov := Scan([]BinDir{{Drive: "Z", Path: dir}}, 25)
	if ov == nil {
		t.Fatal("超过 --max-scan 却没有截断说明")
	}
	if ov.Total != total || ov.Taken != 25 {
		t.Errorf("截断说明数字不符: %+v", ov)
	}
	if got := ov.ByDrive["Z"]; got != [2]int{25, total} {
		t.Errorf("各盘 已扫/总数 不符: %v", got)
	}
	if len(items) != 25 {
		t.Fatalf("条目数 %d，期望 25", len(items))
	}
	var newest time.Time
	for _, it := range items {
		ts, err := time.ParseInLocation("2006-01-02 15:04:05", it.DeletedAt, LocalTZ)
		if err != nil {
			t.Fatal(err)
		}
		if ts.After(newest) {
			newest = ts
		}
	}
	if want := base.Add(25 * 7 * time.Minute); !newest.Equal(want) {
		t.Errorf("最老 25 个的截止时间 %v，期望 %v", newest, want)
	}
	// 不截断时应当看到全部 40 个。
	all, _, ov2 := Scan([]BinDir{{Drive: "Z", Path: dir}}, 0)
	if len(all) != total || ov2 != nil {
		t.Errorf("maxScan=0 应当全扫: %d %+v", len(all), ov2)
	}
}

func TestScanEmptyDir(t *testing.T) {
	items, issues, ov := Scan([]BinDir{{Drive: "Z", Path: t.TempDir()}}, 0)
	if len(items) != 0 || ov != nil {
		t.Errorf("空目录应当没有条目: %d %+v", len(items), ov)
	}
	if len(issues) != 0 {
		t.Errorf("空目录不该有异常: %v", issues)
	}
	items, issues, _ = Scan([]BinDir{{Drive: "Z", Path: filepath.Join(t.TempDir(), "nope")}}, 0)
	if len(items) != 0 {
		t.Error("不存在的目录不该产出条目")
	}
	if len(issues) != 1 || !strings.Contains(issues[0], "无法列出目录") {
		t.Errorf("不存在的目录要报无法列出: %v", issues)
	}
}

func TestFiletimeRoundTrip(t *testing.T) {
	now := time.Now()
	ft := now.UTC().UnixNano()/100 + winfs.FiletimeEpochTicks
	got := FiletimeToTime(ft)
	if diff := got.Sub(now); diff < -time.Second || diff > time.Second {
		t.Errorf("FILETIME 往返偏差过大: %v vs %v", got, now)
	}
	if got.Format("2006-01-02 15:04:05") != now.In(LocalTZ).Format("2006-01-02 15:04:05") {
		t.Errorf("本地时刻字符串不符: %v / %v", got, now.In(LocalTZ))
	}
}

func TestBaseNameAndDecode(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{`C:\a\b.txt`, "b.txt"},
		{`C:\a\b\`, ""}, // baseName 不裁尾分隔符，裁剪在 buildItem 里做
		{`forward/slash.txt`, "slash.txt"},
		{`plain.txt`, "plain.txt"},
	} {
		if got := baseName(c.in); got != c.want {
			t.Errorf("baseName(%q) = %q，期望 %q", c.in, got, c.want)
		}
	}
	if got := decodeUTF16(encodePath(`C:\中文\笔记.md`)); got != `C:\中文\笔记.md` {
		t.Errorf("utf-16 往返失败: %q", got)
	}
	if got := decodeUTF16([]byte{0x41, 0x00, 0x42}); got != "A" {
		t.Errorf("奇数长度应当丢掉最后半个字: %q", got)
	}
}
