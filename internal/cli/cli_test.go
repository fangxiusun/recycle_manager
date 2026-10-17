package cli

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rbm/internal/i18n"
	"rbm/internal/meta"
	"rbm/internal/parts"
	"rbm/internal/store"
	"rbm/internal/tty"
)

// 这一层只测「解析与派发」：命令行解析、位置参数、shell 的逐行派发。
// 所有测试的 --state-dir 都指向 t.TempDir()，且一律 --no-scan，绝不碰真实回收站。

func newApp(stdin string) (*app, *bytes.Buffer, *bytes.Buffer) {
	var out, errb bytes.Buffer
	return &app{out: &out, err: &errb, in: bufio.NewReader(strings.NewReader(stdin))}, &out, &errb
}

func TestParseFlagsDefaults(t *testing.T) {
	opts, pos, help, err := parseFlags("scan", nil)
	if err != nil || help || len(pos) != 0 {
		t.Fatalf("空参数应当走默认值: %v %v %v", err, help, pos)
	}
	if opts.by != "dir" || opts.dirLevel != "2" || opts.sort != "time" {
		t.Errorf("默认分组维度/层级/排序不符: %+v", opts)
	}
	for name, got := range map[string]int{
		"prefix-len": opts.prefixLen, "max-buckets": opts.maxBuckets, "max-parts": opts.maxParts,
		"min-count": opts.minCount, "max-scan": opts.maxScan,
	} {
		want := map[string]int{
			"prefix-len": defaults.PrefixLen, "max-buckets": defaults.MaxBuckets,
			"max-parts": defaults.MaxParts, "min-count": defaults.MinCount, "max-scan": defaults.MaxScan,
		}[name]
		if got != want {
			t.Errorf("--%s 默认 %d，期望 %d", name, got, want)
		}
	}
	if opts.maxScan != 50000 {
		t.Errorf("--max-scan 默认应当是 50000（和文档一致），实得 %d", opts.maxScan)
	}
}

func TestParseFlagsForms(t *testing.T) {
	cases := []struct {
		name    string
		cmd     string
		argv    []string
		check   func(*testing.T, *options, []string)
		wantErr string
		help    bool
	}{
		{
			name: "等号写法", cmd: "scan", argv: []string{"--bucket=gap:5m"},
			check: func(t *testing.T, o *options, _ []string) {
				if o.bucket != "gap:5m" {
					t.Errorf("bucket = %q", o.bucket)
				}
			},
		},
		{
			name: "空格分隔与单横线", cmd: "scan", argv: []string{"-bucket", "daily", "--max-scan", "100"},
			check: func(t *testing.T, o *options, _ []string) {
				if o.bucket != "daily" || o.maxScan != 100 {
					t.Errorf("bucket=%q maxScan=%d", o.bucket, o.maxScan)
				}
			},
		},
		{
			name: "位置参数与选项混写", cmd: "delete", argv: []string{"1-5", "--yes", "8-9"},
			check: func(t *testing.T, _ *options, pos []string) {
				if strings.Join(pos, "|") != "1-5|8-9" {
					t.Errorf("位置参数 = %v", pos)
				}
			},
		},
		{
			name: "双横线之后全算位置参数", cmd: "show", argv: []string{"--", "12"},
			check: func(t *testing.T, _ *options, pos []string) {
				if strings.Join(pos, "|") != "12" {
					t.Errorf("pos = %v", pos)
				}
			},
		},
		{
			name: "布尔短别名 -v", cmd: "delete", argv: []string{"-v"},
			check: func(t *testing.T, o *options, _ []string) {
				if !o.verbose {
					t.Error("-v 应当打开 verbose")
				}
			},
		},
		{
			name: "取值型选项不接受 =值 之外的布尔写法", cmd: "parts", argv: []string{"--alive=yes"},
			wantErr: "不接受取值",
		},
		{
			name: "未知选项", cmd: "scan", argv: []string{"--nope"}, wantErr: "无法识别的选项: --nope",
		},
		{
			name: "缺取值", cmd: "scan", argv: []string{"--bucket"}, wantErr: "需要一个取值",
		},
		{
			name: "非法枚举", cmd: "scan", argv: []string{"--by", "hash"}, wantErr: "--by 的取值无法识别",
		},
		{
			name: "非法数字", cmd: "show", argv: []string{"--limit", "abc"}, wantErr: "--limit 的取值无法识别",
		},
		{
			name: "命令内 -h", cmd: "show", argv: []string{"-h"}, help: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			opts, pos, help, err := parseFlags(c.cmd, c.argv)
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("期望报错包含 %q，实得 %v", c.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("解析失败: %v", err)
			}
			if help != c.help {
				t.Errorf("help = %v，期望 %v", help, c.help)
			}
			if c.check != nil {
				c.check(t, opts, pos)
			}
		})
	}
}

func TestFillPositional(t *testing.T) {
	o := &options{}
	if err := fillPositional("show", nil, o); err == nil || !strings.Contains(err.Error(), "一个编号参数") {
		t.Errorf("show 缺编号应当报错: %v", err)
	}
	if err := fillPositional("show", []string{"1", "2"}, o); err == nil {
		t.Error("show 多给一个编号应当报错")
	}
	if err := fillPositional("show", []string{"12"}, o); err != nil || o.pid != "12" {
		t.Errorf("show 12 -> pid=%q err=%v", o.pid, err)
	}
	if err := fillPositional("delete", []string{"1-5,8"}, o); err != nil || o.pids != "1-5,8" {
		t.Errorf("delete 区间 -> pids=%q err=%v", o.pids, err)
	}
	if err := fillPositional("scan", []string{"多余"}, o); err == nil || !strings.Contains(err.Error(), "多余的参数") {
		t.Errorf("scan 带位置参数应当报错: %v", err)
	}
	if o.stateDir == "" {
		t.Error("stateDir 为空时应当补默认值")
	}
}

func TestFillPositionalDefaultsToCWD(t *testing.T) {
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(orig)
	o := &options{}
	if err := fillPositional("parts", nil, o); err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(tmp, ".rbm"); o.stateDir != want {
		t.Errorf("默认快照目录 = %q，期望 %q", o.stateDir, want)
	}
}

func TestSplitLine(t *testing.T) {
	cases := []struct {
		line string
		want []string
	}{
		{"show 12", []string{"show", "12"}},
		{"  delete\t1-5,8-10 , 16  ", []string{"delete", "1-5,8-10", ",", "16"}},
		{`parts --prefix "a b"`, []string{"parts", "--prefix", `"a b"`}},
		{"show 1 --prefix '笔记 '", []string{"show", "1", "--prefix", "'笔记 '"}},
		{"\r\n", nil},
		{`show "1`, []string{"show", `"1`}}, // 引号没闭合：剩下的都吞进这一词，和 Python 的 shlex 一样不报错
	}
	for _, c := range cases {
		got := splitLine(c.line)
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("splitLine(%q) = %v，期望 %v", c.line, got, c.want)
		}
	}
}

func TestMainUsagePaths(t *testing.T) {
	a, out, errb := newApp("")
	if code := a.main(nil); code != 2 || !strings.Contains(errb.String(), "用法") {
		t.Errorf("空参数应当打印用法并返回 2: code=%d err=%q", code, errb.String())
	}

	a, out, _ = newApp("")
	if code := a.main([]string{"--help"}); code != 0 || !strings.Contains(out.String(), "子命令") {
		t.Errorf("--help 应当返回 0 并打印总帮助: code=%d", code)
	}

	a, _, errb = newApp("")
	if code := a.main([]string{"nope"}); code != 2 || !strings.Contains(errb.String(), "无法识别的子命令") {
		t.Errorf("坏子命令应当返回 2: code=%d err=%q", code, errb.String())
	}

	a, out, _ = newApp("")
	if code := a.main([]string{"delete", "-h"}); code != 0 || !strings.Contains(out.String(), "dry-run") {
		t.Errorf("delete -h 应当打印该命令帮助: code=%d out=%q", code, out.String())
	}

	a, out, errb = newApp("")
	if code := a.main([]string{"show", "--state-dir", t.TempDir()}); code != 2 ||
		!strings.Contains(errb.String(), "一个编号参数") {
		t.Errorf("show 缺编号应当返回 2: code=%d err=%q", code, errb.String())
	}
	if out.Len() != 0 {
		t.Errorf("用法报错的命令不该往 stdout 写东西: %q", out.String())
	}
}

// 命令行下没有快照：报错走 stderr、退出码 1，不会去碰真实回收站。
func TestCommandsWithoutSnapshot(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct {
		cmd  string
		argv []string
	}{
		{"parts", []string{"--state-dir", dir}},
		{"show", []string{"1", "--state-dir", dir}},
		{"delete", []string{"1", "--state-dir", dir}},
	} {
		a, out, errb := newApp("")
		if code := a.main(append([]string{c.cmd}, c.argv...)); code != 1 {
			t.Errorf("%s 在无快照时的退出码 = %d，期望 1（stderr=%q）", c.cmd, code, errb.String())
		}
		if !strings.Contains(errb.String(), "没有找到快照") {
			t.Errorf("%s 的报错 = %q", c.cmd, errb.String())
		}
		if out.Len() != 0 {
			t.Errorf("%s 出错时不该往 stdout 写东西: %q", c.cmd, out.String())
		}
	}
}

func TestRunLineDispatch(t *testing.T) {
	a, out, _ := newApp("")
	a.sessStateDir = t.TempDir()
	a.inShell = true
	if code := a.runLine("   \n"); code != 0 {
		t.Errorf("空行不该有副作用: %d", code)
	}
	for _, q := range []string{"quit\n", "EXIT\n", "q\n", "退出\n"} {
		if code := a.runLine(q); code != quitSession {
			t.Errorf("%q 应当结束会话，实得 %d", q, code)
		}
	}
	if code := a.runLine("help\n"); code != 0 || !strings.Contains(out.String(), "quit") {
		t.Errorf("help 应当打印会话用法: code=%d", code)
	}
	out.Reset()
	if code := a.runLine("bogus\n"); code != 2 || !strings.Contains(out.String(), "无法识别的命令") {
		t.Errorf("坏命令应当报错但留住会话: code=%d out=%q", code, out.String())
	}
	out.Reset()
	if code := a.runLine("shell\n"); code != 0 || !strings.Contains(out.String(), "已经在 shell 里了") {
		t.Errorf("会话里再敲 shell 只提示: code=%d out=%q", code, out.String())
	}
	out.Reset()
	// 没有快照时 info 只说明现状，不报错也不清会话。
	if code := a.runLine("info\n"); code != 0 || !strings.Contains(out.String(), "没有快照") {
		t.Errorf("info 无快照时应当说明: code=%d out=%q", code, out.String())
	}
}

// 会话里的命令没写 --state-dir 时，必须沿用 shell 启动时那个目录，否则会写回默认 .rbm、和快照分家。
func TestRunLineInheritsStateDir(t *testing.T) {
	shellDir := t.TempDir()
	other := t.TempDir()
	a, out, _ := newApp("")
	a.inShell = true
	a.sessStateDir = shellDir
	if code := a.runLine("parts\n"); code != 1 || !strings.Contains(out.String(), shellDir) {
		t.Errorf("会话里的 parts 应当沿用 shell 的快照目录 %s: code=%d out=%q", shellDir, code, out.String())
	}
	out.Reset()
	if code := a.runLine("parts --state-dir " + other + "\n"); code != 1 ||
		!strings.Contains(out.String(), other) || strings.Contains(out.String(), shellDir) {
		t.Errorf("显式 --state-dir 应当盖掉会话默认: code=%d out=%q", code, out.String())
	}
}

func TestShellSessionQuitsAndNeverScans(t *testing.T) {
	dir := t.TempDir()
	a, out, _ := newApp("parts\ninfo\nquit\n")
	code := a.main([]string{"shell", "--no-scan", "--state-dir", dir})
	if code != 0 {
		t.Fatalf("shell 正常退出应当是 0，实得 %d", code)
	}
	text := out.String()
	if !strings.Contains(text, "回收站分批管理 shell") || !strings.Contains(text, "rbm> ") {
		t.Errorf("缺少欢迎语或提示符: %q", text)
	}
	if strings.Contains(text, "扫描") {
		t.Errorf("--no-scan 不该触发扫描: %q", text)
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Errorf("只读命令不该往快照目录写东西: %v err=%v", entries, err)
	}
}

// EOF（没有 quit）也要干净退出。
func TestShellExitsOnEOF(t *testing.T) {
	a, out, _ := newApp("\n\n")
	if code := a.main([]string{"shell", "--no-scan", "--state-dir", t.TempDir()}); code != 0 {
		t.Fatalf("EOF 退出应当是 0，实得 %d", code)
	}
	if !strings.Contains(out.String(), "\nrbm> ") && !strings.HasSuffix(out.String(), "rbm> \n") {
		t.Logf("EOF 前的提示符输出: %q", out.String())
	}
}

func TestPrompt(t *testing.T) {
	a, _, _ := newApp("")
	if got := a.prompt(); got != "rbm> " {
		t.Errorf("无快照时的提示符 = %q", got)
	}
	a.state = &store.State{
		Created: "2026-10-15 12:00:00",
		Items:   []*meta.Item{{Display: "a"}, {Display: "b"}, {Display: "c"}},
		Parts:   []*parts.Part{{PID: 1, Count: 3, Deleted: 1}},
	}
	got := a.prompt()
	if !strings.Contains(got, "快照 3条/1区 剩2]") {
		t.Errorf("提示符应当带规模: %q", got)
	}
	if !strings.Contains(got, "12:00:00") {
		t.Errorf("提示符里的快照时间应当是时分秒: %q", got)
	}
}

// ---------- 双语 ----------

// withLang 临时切语言并在用例结束后还原：语言是全局的，不还原会让后面的中文断言全线失败。
func withLang(t *testing.T, l i18n.Lang) {
	t.Helper()
	prev := i18n.Get()
	i18n.Set(l)
	t.Cleanup(func() { i18n.Set(prev) })
}

func TestSplitLangArgs(t *testing.T) {
	cases := []struct {
		name string
		argv []string
		lang string
		want []string
	}{
		{"没有 --lang", []string{"parts"}, "", []string{"parts"}},
		{"子命令前", []string{"--lang", "en", "parts"}, "en", []string{"parts"}},
		{"子命令后", []string{"parts", "--lang", "en"}, "en", []string{"parts"}},
		{"等号写法", []string{"--lang=en", "show", "1"}, "en", []string{"show", "1"}},
		{"多个取第一个", []string{"--lang", "en", "--lang", "zh", "parts"}, "en", []string{"parts"}},
		{"值缺失就丢掉", []string{"parts", "--lang"}, "", []string{"parts"}},
		{"长选项名不误伤", []string{"parts", "--language", "en"}, "", []string{"parts", "--language", "en"}},
		{"保留其它参数顺序", []string{"--lang", "zh", "show", "2", "--limit", "1"}, "zh",
			[]string{"show", "2", "--limit", "1"}},
	}
	for _, c := range cases {
		lang, rest := SplitLangArgs(c.argv)
		if lang != c.lang {
			t.Errorf("%s: --lang = %q，期望 %q", c.name, lang, c.lang)
		}
		if strings.Join(rest, "|") != strings.Join(c.want, "|") {
			t.Errorf("%s: 摘掉 --lang 后 = %v，期望 %v", c.name, rest, c.want)
		}
	}
}

// --lang 落在子命令前后都要生效，且错误流也一起换语言。
func TestLangFlagAffectsBothStreams(t *testing.T) {
	withLang(t, i18n.Zh)
	dir := t.TempDir()
	a, _, errb := newApp("")
	if code := a.main([]string{"parts", "--state-dir", dir}); code != 1 {
		t.Fatalf("无快照时退出码 = %d，期望 1", code)
	}
	if !strings.Contains(errb.String(), "没有找到快照") {
		t.Errorf("默认应当中文: %q", errb.String())
	}

	for _, argv := range [][]string{
		{"--lang", "en", "parts", "--state-dir", dir},
		{"--lang=en", "parts", "--state-dir", dir},
	} {
		i18n.Set(i18n.Zh) // 每轮都从中文起，验的才是 flag 本身
		a, _, errb = newApp("")
		if code := a.main(argv); code != 1 {
			t.Fatalf("%v 退出码 = %d，期望 1", argv, code)
		}
		if !strings.Contains(errb.String(), "No snapshot at") {
			t.Errorf("%v 应当出英文: %q", argv, errb.String())
		}
		// 一次调用里 flag 改的是全局语言，之后的输出保持英文（进程内不复原）
		if i18n.Get() != i18n.En {
			t.Errorf("%v 跑完之后语言应当停在英文，实得 %s", argv, i18n.Get())
		}
	}

	// 认不出的写法不报错，按优先级往下退，最后中文。
	i18n.Set(i18n.Zh)
	a, _, errb = newApp("")
	if code := a.main([]string{"--lang", "klingon", "parts", "--state-dir", dir}); code != 1 {
		t.Fatalf("坏语言值不该改变退出码: %d", code)
	}
	if !strings.Contains(errb.String(), "没有找到快照") {
		t.Errorf("--lang 认不出时应当退回中文: %q", errb.String())
	}
}

// 会话里 --lang 随写随切，并且对后续行继续生效。
func TestRunLineLangSwitchPersists(t *testing.T) {
	withLang(t, i18n.Zh)
	a, out, _ := newApp("")
	a.inShell = true
	a.sessStateDir = t.TempDir()
	if code := a.runLine("parts\n"); code != 1 || !strings.Contains(out.String(), "没有找到快照") {
		t.Fatalf("会话默认应当中文: code=%d out=%q", code, out.String())
	}
	out.Reset()
	if code := a.runLine("parts --lang en\n"); code != 1 || !strings.Contains(out.String(), "No snapshot at") {
		t.Errorf("会话里的 --lang 应当生效: code=%d out=%q", code, out.String())
	}
	if i18n.Get() != i18n.En {
		t.Errorf("切完应当停在英文")
	}
	out.Reset()
	if code := a.runLine("parts\n"); code != 1 || !strings.Contains(out.String(), "No snapshot at") {
		t.Errorf("语言应当跨行保持: code=%d out=%q", code, out.String())
	}
	out.Reset()
	// 认不出的语言值：不改语言，也不影响这一行的执行结果。
	if code := a.runLine("parts --lang klingon\n"); code != 1 ||
		!strings.Contains(out.String(), "No snapshot at") {
		t.Errorf("坏语言值不该改变行为: code=%d out=%q", code, out.String())
	}
	if i18n.Get() != i18n.En {
		t.Errorf("坏语言值不该改语言，实得 %s", i18n.Get())
	}
	out.Reset()
	if code := a.runLine("parts --lang 中文\n"); code != 1 || !strings.Contains(out.String(), "没有找到快照") {
		t.Errorf("--lang 应当认「中文」这种写法: code=%d out=%q", code, out.String())
	}
	out.Reset()
	if code := a.runLine("quit\n"); code != quitSession {
		t.Errorf("收尾：quit 仍应结束会话: %d", code)
	}
}

// sampleState 造 n 个「每区一条」的快照：条目合计正好 n，方便盯单复数。
func sampleState(n int) *store.State {
	st := &store.State{Created: "2026-10-16 12:00:00", Params: store.Params{By: "dir"}}
	for i := 0; i < n; i++ {
		st.Items = append(st.Items, &meta.Item{Display: "a.txt", Size: 1024, Drive: "G"})
		st.Parts = append(st.Parts, &parts.Part{
			PID: i + 1, Count: 1, Size: 1024, Label: "G:/x", Drives: "G",
			TimeRange: "2026-10-16 12:00", IndexStart: i + 1, IndexEnd: i + 1,
			TopLabels: "a.txt(1)",
		})
	}
	return st
}

// 英文有单复数、中文只有量词：两边都不能出现 "1 partitions" 这类写法。
func TestPartStatsPluralForms(t *testing.T) {
	withLang(t, i18n.Zh)
	var buf bytes.Buffer
	a := &app{out: &buf, err: &buf, in: bufio.NewReader(strings.NewReader(""))}
	a.printParts(sampleState(1), false, "time")
	zh := buf.String()
	for _, want := range []string{"条目合计 1 个 / ", "分区 1 个  编号 1..1"} {
		if !strings.Contains(zh, want) {
			t.Errorf("中文统计行缺 %q:\n%s", want, zh)
		}
	}

	buf.Reset()
	withLang(t, i18n.En)
	a.printParts(sampleState(1), false, "time")
	en := buf.String()
	for _, want := range []string{"Total 1 item /", "in 1 partition  ids 1..1"} {
		if !strings.Contains(en, want) {
			t.Errorf("英文统计行缺 %q:\n%s", want, en)
		}
	}
	for _, bad := range []string{"1 items", "1 partitions"} {
		if strings.Contains(en, bad) {
			t.Errorf("英文单数写成了 %q:\n%s", bad, en)
		}
	}

	buf.Reset()
	a.printParts(sampleState(3), false, "time")
	en = buf.String()
	for _, want := range []string{"Total 3 items /", "in 3 partitions"} {
		if !strings.Contains(en, want) {
			t.Errorf("英文复数缺 %q:\n%s", want, en)
		}
	}
}

// 表头宽度跟着语言换：英文表头更长，沿用中文宽度会被截成 "Dr…"。
func TestTableWidthsFollowLanguage(t *testing.T) {
	withLang(t, i18n.Zh)
	zhParts, zhItems := partsWidths(), itemWidths()
	withLang(t, i18n.En)
	enParts, enItems := partsWidths(), itemWidths()

	if len(zhParts) != len(enParts) || len(zhItems) != len(enItems) {
		t.Fatalf("宽度列数必须和表头一一对应: parts %d/%d items %d/%d",
			len(zhParts), len(enParts), len(zhItems), len(enItems))
	}
	st := sampleState(1)
	headers := partsHeaders(st)
	if len(headers) != len(enParts) {
		t.Fatalf("分区表头 %d 列，宽度 %d 列", len(headers), len(enParts))
	}
	// 盘 / Drive：中文一列宽 4，英文要 5 才不截断
	if zhParts[6] >= enParts[6] {
		t.Errorf("英文「盘」列不该比中文窄: zh=%d en=%d", zhParts[6], enParts[6])
	}
	if enItems[3] <= zhItems[3] {
		t.Errorf("英文「盘」列该更宽: zh=%d en=%d", zhItems[3], enItems[3])
	}
	for _, c := range []struct {
		lang i18n.Lang
		want string
	}{
		{i18n.Zh, "盘"}, {i18n.En, "Drive"},
	} {
		withLang(t, c.lang)
		line := strings.Split(tty.Table(partsHeaders(st), partsRows(st.Parts), partsWidths()), "\n")[0]
		if !strings.Contains(line, c.want) {
			t.Errorf("%s 表头应当完整出现 %q: %q", c.lang, c.want, line)
		}
	}
	// 反过来验一次：英文表头硬套中文宽度就会被截断——这正是宽度要跟着语言换的原因。
	withLang(t, i18n.En)
	truncated := strings.Split(tty.Table(partsHeaders(st), partsRows(st.Parts), zhParts), "\n")[0]
	if strings.Contains(truncated, "Drive") {
		t.Errorf("英文表头套中文宽度居然没截断，这条护栏没意义: %q", truncated)
	}
}
