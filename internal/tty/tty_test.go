package tty

import (
	"strconv"
	"strings"
	"testing"
)

// 期望值全部由 Python 版 rbmgr.py 的 wlen/truncate/human/table/parse_ids 现算冻结
// （脚本见 testdata 说明与 README「与 Python 版的差异」一节）。

func TestWLenMatchesPython(t *testing.T) {
	cases := []struct {
		text string
		want int
	}{
		{"abc", 3},
		{"中文", 4},
		{"中 a 文", 7},
		{"ａｂｃ１２３", 12}, // 全角：W
		{"ﾊﾝｶｸ", 4},    // 半角假名：H
		{"笔记-草稿.md", 12},
		{`C:\Users\x\Temp\Vs9POM\ssr`, 26},
		{"…", 1}, // A（ambiguous）按 1 列，和 Python 一致
		{"a…b", 3},
		{"\U0001f389", 2}, // emoji
		{"\U0001f389x", 3},
		{"é", 1},
		{"éx", 2}, // 组合符号本身 0 列，这里抬到 1 列以对齐 Python
		{"write.lock", 10},
		{".env.local", 10},
		{"元数据损坏", 10},
		{"其他", 4},
		{"全部", 4},
		{"特殊位置", 8},
		{"2026-10-15 12:11~14:17", 22},
		{"esbuild-a42feca2891ced1e3.js", 28},
		{"①②", 2},
		{"±×÷", 3},
		{"ΑΒΓΩ", 4},
		{"한글テスト", 10},
		{"→", 1},
		{"❤", 1},
		{"", 0},
	}
	for _, c := range cases {
		if got := WLen(c.text); got != c.want {
			t.Errorf("WLen(%q) = %d，期望 %d", c.text, got, c.want)
		}
	}
}

func TestTruncateMatchesPython(t *testing.T) {
	cases := []struct {
		text   string
		width  int
		want10 string
		want5  string
	}{
		{"中 a 文", 10, "中 a 文", "中 a…"},
		{"ａｂｃ１２３", 10, "ａｂｃ１…", "ａｂ…"},
		{"笔记-草稿.md", 10, "笔记-草稿…", "笔记…"},
		{`C:\Users\x\Temp\Vs9POM\ssr`, 10, `C:\Users\…`, `C:\U…`},
		{"元数据损坏", 10, "元数据损坏", "元数…"},
		{"特殊位置", 10, "特殊位置", "特殊…"},
		{"2026-10-15 12:11~14:17", 10, "2026-10-1…", "2026…"},
		{"esbuild-a42feca2891ced1e3.js", 10, "esbuild-a…", "esbu…"},
		{"write.lock", 10, "write.lock", "writ…"},
		{"한글テスト", 10, "한글テスト", "한글…"}, // 宽度正好等于上限就不截断
		{"abc", 10, "abc", "abc"},
	}
	for _, c := range cases {
		if got := Truncate(c.text, 10); got != c.want10 {
			t.Errorf("Truncate(%q, 10) = %q，期望 %q", c.text, got, c.want10)
		}
		if got := Truncate(c.text, 5); got != c.want5 {
			t.Errorf("Truncate(%q, 5) = %q，期望 %q", c.text, got, c.want5)
		}
		if WLen(Truncate(c.text, c.width)) > c.width {
			t.Errorf("Truncate(%q, %d) 结果仍然超宽", c.text, c.width)
		}
	}
}

func TestPad(t *testing.T) {
	for _, c := range []struct {
		text          string
		width, spaces int
	}{
		{"abc", 6, 3},
		{"中文", 6, 2},
		{"很长的一个中文串", 4, 0}, // 已经超宽就不补
	} {
		got := Pad(c.text, c.width)
		if WLen(got) < c.width {
			t.Errorf("Pad(%q, %d) 没填满: %q", c.text, c.width, got)
		}
		if n := len(got) - len(c.text); n != c.spaces {
			t.Errorf("Pad(%q, %d) 补了 %d 个空格，期望 %d", c.text, c.width, n, c.spaces)
		}
	}
}

func TestHumanMatchesPython(t *testing.T) {
	cases := []struct {
		n    float64
		want string
	}{
		{0, "0B"},
		{1, "1B"},
		{512, "512B"},
		{1023, "1023B"},
		{1024, "1.0KB"},
		{1536, "1.5KB"},
		{1048575, "1024.0KB"}, // 1023.999… 四舍五入到 1024.0，Python 同样如此
		{1048576, "1.0MB"},
		{1073741824, "1.0GB"},
		{1099511627776, "1.0TB"},
		{5497558138880, "5.0TB"},
		{1024 * 1024 * 1024 * 1024 * 1024, "1024.0TB"},
	}
	for _, c := range cases {
		if got := Human(c.n); got != c.want {
			t.Errorf("Human(%v) = %q，期望 %q", c.n, got, c.want)
		}
	}
}

// TestTableMatchesPython 冻结的是 Python table() 对同一组数据的完整渲染结果。
func TestTableMatchesPython(t *testing.T) {
	headers := []string{"编号", "时间范围", "路径组", "元素数"}
	rows := [][]string{
		{"1", "2026-10-15 12:11~14:17", `C:\Users\x\Temp\Vs9POM\ssr`, "294"},
		{"2", "2026-10-15 12:11~14:17", "元数据损坏", "11"},
		{"3", "2026-10-15 12:11~14:17", "笔记-草稿.md 与一段很长的中文路径组名称", "8"},
	}
	widths := []int{4, 22, 24, 7}
	want := strings.Join([]string{
		"编号  时间范围                路径组                    元素数 ",
		"----  ----------------------  ------------------------  -------",
		"1     2026-10-15 12:11~14:17  C:\\Users\\x\\Temp\\Vs9POM\\…  294    ",
		"2     2026-10-15 12:11~14:17  元数据损坏                11     ",
		"3     2026-10-15 12:11~14:17  笔记-草稿.md 与一段很长…  8      ",
	}, "\n")
	if got := Table(headers, rows, widths); got != want {
		t.Errorf("Table 渲染与 python 不一致:\n实得:\n%s\n期望:\n%s\n", got, want)
	}
}

func TestParseIDs(t *testing.T) {
	ok := []struct {
		spec string
		want []int
	}{
		{"1-5,8-10,16", []int{1, 2, 3, 4, 5, 8, 9, 10, 16}},
		{"3", []int{3}},
		{"5-1", []int{1, 2, 3, 4, 5}},    // 反着写按区间处理
		{"1,1,2", []int{1, 2}},           // 去重且保持首次出现顺序
		{" 2:4 , 7 ", []int{2, 3, 4, 7}}, // 冒号与空格都当分隔
		{"", nil},
	}
	for _, c := range ok {
		got, err := ParseIDs(c.spec, 20)
		if err != nil {
			t.Errorf("ParseIDs(%q) 报错: %v", c.spec, err)
			continue
		}
		if len(got) == 0 && len(c.want) == 0 {
			continue
		}
		if strings.Join(ints(got), ",") != strings.Join(ints(c.want), ",") {
			t.Errorf("ParseIDs(%q) = %v，期望 %v", c.spec, got, c.want)
		}
	}
	bad := []string{"0", "21", "1-21", "abc", "1-", "-", "1..3", "1,abc"}
	for _, spec := range bad {
		if _, err := ParseIDs(spec, 20); err == nil {
			t.Errorf("ParseIDs(%q) 应当报错", spec)
		}
	}
}

func ints(xs []int) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = strconv.Itoa(x)
	}
	return out
}
