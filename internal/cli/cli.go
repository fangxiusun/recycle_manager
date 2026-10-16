// Package cli 是 rbm 的命令行外壳：scan/parts/show/delete/shell 五个子命令，
// 文案、表头、退出码都对齐 Python 版，界面只做展示，判定都走 internal/engine。
package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"rbm/internal/engine"
	"rbm/internal/meta"
	"rbm/internal/parts"
	"rbm/internal/store"
	"rbm/internal/tty"
)

// 与 Python argparse 一致的单位置默认值
var defaults = struct {
	MaxBuckets int
	MaxParts   int
	MinCount   int
	PrefixLen  int
	MaxScan    int
	Sample     int
	Every      int
	Limit      int
}{12, 120, 5, 3, 50000, 15, 200, 40}

type options struct {
	stateDir   string
	bucket     string
	by         string
	dirLevel   string
	prefixLen  int
	maxBuckets int
	maxParts   int
	minCount   int
	drives     string
	maxScan    int

	alive   bool
	sort    string
	pid     string
	pids    string
	start   *int
	end     *int
	prefix  string
	limit   int
	tail    bool
	local   bool
	yes     bool
	dryRun  bool
	sample  int
	max     int
	every   int
	verbose bool
	noScan  bool
}

type app struct {
	out     io.Writer
	err     io.Writer
	in      *bufio.Reader
	inShell bool
	// shell 会话内存快照：编号必须和用户刚看到的统计表一致
	state        *store.State
	sessStateDir string
}

// Main 执行一次命令行调用，返回退出码。0 成功/取消/dry-run，1 用法或数据错误，2 未确认或踩保险丝。
func Main(argv []string) int {
	a := &app{out: os.Stdout, err: os.Stderr, in: bufio.NewReader(os.Stdin)}
	return a.main(argv)
}

var rootHelp = `回收站分批管理工具（分区编号 → 查看 → 批量删除）

用法: rbm <子命令> [选项]

子命令:
  scan    扫描回收站并按「时间桶 × 原路径目录」分区编号，写出统计
  parts   重列已扫描快照的分区统计
  show    查看某个编号分区内的元素列表（index 区间）
  delete  按编号（如 1-5,8-10,16）分批永久删除分区内容，默认 dry-run
  shell   进入交互 shell：启动即 scan，之后的 show/delete 复用同一份内存快照

不带任何参数直接运行 rbm.exe 会打开图形界面。用 rbm <子命令> -h 看该命令的选项。`

const partitionHelp = `  --bucket 文本           时间桶。不填=按删除时间断点自动切区（gap）。也可指定等宽：
                          minutely/hourly/daily/weekly/monthly/30m/15m/1h/3d/2mo，
                          或 gap:5m 表示只在间隔>=5分钟处断区，auto=自动选最细可用等宽
  --max-buckets N         时间桶数上限：gap 模式下取最大的若干个断点，auto 模式下选不超过该数的
                          最细等宽粒度（默认 12）
  --by dir|prefix|ext|token  桶内分组维度：dir 按原路径所在目录（默认）/prefix 文件名字符前缀/
                          ext 扩展名/token 文件名首段
  --dir-level N|full      dir 分组取原路径末尾几段：1 只看所在目录名（合并同类批），2 默认
                          （上级目录+目录，可区分每次构建），3 更具体，full 用完整路径
  --prefix-len N          prefix/token 模式的字符数（默认 3）
  --max-parts N           分区总数上限（默认 120；调小会把小组并成「其他」，调大保留更多具体路径）
  --min-count N           少于该元素数的分组并入「其他」（默认 5）
  --drives C,G            限定盘符，逗号分隔
  --max-scan N            一次最多扫描多少个条目（默认 50000，0 = 不限）。回收站更大时只取最老的
                          N 个建快照并明确提示，不会静默丢弃；删完再 scan 会继续处理剩下的`

// 每个子命令接受的 flag 名 -> 是否需要取值
var flagSpecs = map[string]map[string]bool{
	"scan": {
		"bucket": true, "max-buckets": true, "by": true, "dir-level": true, "prefix-len": true,
		"max-parts": true, "min-count": true, "drives": true, "max-scan": true, "state-dir": true,
	},
	"parts": {"alive": false, "sort": true, "state-dir": true},
	"show": {
		"start": true, "end": true, "prefix": true, "limit": true, "tail": false, "local": false,
		"state-dir": true,
	},
	"delete": {
		"yes": false, "dry-run": false, "sample": true, "max": true, "every": true, "verbose": false,
		"prefix": true, "state-dir": true,
	},
	"shell": {
		"bucket": true, "max-buckets": true, "by": true, "dir-level": true, "prefix-len": true,
		"max-parts": true, "min-count": true, "drives": true, "max-scan": true, "no-scan": false,
		"state-dir": true,
	},
}

var boolAliases = map[string]string{"v": "verbose"}

func (a *app) main(argv []string) int {
	if len(argv) == 0 {
		fmt.Fprintln(a.err, "用法: rbm <子命令> [选项]，或 rbm --help")
		return 2
	}
	head := argv[0]
	if head == "--help" || head == "-h" || head == "help" {
		fmt.Fprintln(a.out, rootHelp)
		return 0
	}
	if _, ok := flagSpecs[head]; !ok {
		fmt.Fprintf(a.err, "rbm: 无法识别的子命令 %s（可用: scan/parts/show/delete/shell）\n", head)
		return 2
	}
	opts, pos, help, err := parseFlags(head, argv[1:])
	if err != nil {
		fmt.Fprintf(a.err, "rbm %s: error: %v\n", head, err)
		return 2
	}
	if help {
		fmt.Fprintln(a.out, commandHelp(head))
		return 0
	}
	if err := fillPositional(head, pos, opts); err != nil {
		fmt.Fprintf(a.err, "rbm %s: error: %v\n", head, err)
		return 2
	}
	return a.dispatch(head, opts)
}

func (a *app) dispatch(cmd string, opts *options) int {
	switch cmd {
	case "scan":
		return a.cmdScan(opts)
	case "parts":
		return a.cmdParts(opts)
	case "show":
		return a.cmdShow(opts)
	case "delete":
		return a.cmdDelete(opts)
	case "shell":
		return a.cmdShell(opts)
	}
	return 2
}

func commandHelp(cmd string) string {
	head := "用法: rbm " + cmd
	switch cmd {
	case "scan":
		return head + " [选项]\n\n扫描回收站并按「时间桶 × 原路径目录」分区编号，写出统计。\n\n选项:\n" +
			partitionHelp + "\n  --state-dir 目录        快照目录（默认 当前目录\\.rbm）"
	case "parts":
		return head + " [--alive] [--sort time|size|count] [--state-dir 目录]\n\n重列已扫描快照的分区统计表。"
	case "show":
		return head + " <编号> [--start N] [--end N] [--prefix 串] [--limit N] [--tail] [--local]\n\n" +
			"查看某编号分区内的元素列表。--limit 0 全部显示。"
	case "delete":
		return head + " <编号区间> [--dry-run|--yes] [--prefix 串] [--max N] [--sample N] [--every N] [-v]\n\n" +
			"按编号分批永久删除（默认 dry-run，只预览不动手）。"
	case "shell":
		return head + " [scan 的全部选项] [--no-scan]\n\n进入交互 shell：启动即 scan，之后的 show/delete 复用同一份内存快照，编号不会漂。"
	}
	return head
}

func fillPositional(cmd string, pos []string, opts *options) error {
	switch cmd {
	case "show":
		if len(pos) != 1 {
			return fmt.Errorf("需要且只需要一个编号参数，例如: rbm show 12")
		}
		opts.pid = pos[0]
	case "delete":
		if len(pos) != 1 {
			return fmt.Errorf("需要编号区间参数，例如: rbm delete 1-5,8-10,16")
		}
		opts.pids = pos[0]
	default:
		if len(pos) > 0 {
			return fmt.Errorf("多余的参数: %s", strings.Join(pos, " "))
		}
	}
	if opts.stateDir == "" {
		opts.stateDir = store.DefaultStateDir()
	}
	return nil
}

// parseFlags 支持选项与位置参数混写（同 argparse），--name v / --name=v / -name / -v 都可以。
func parseFlags(cmd string, argv []string) (*options, []string, bool, error) {
	spec := flagSpecs[cmd]
	opts := &options{
		by:         "dir",
		dirLevel:   "2",
		prefixLen:  defaults.PrefixLen,
		maxBuckets: defaults.MaxBuckets,
		maxParts:   defaults.MaxParts,
		minCount:   defaults.MinCount,
		maxScan:    defaults.MaxScan,
		sort:       "time",
		limit:      defaults.Limit,
		sample:     defaults.Sample,
		every:      defaults.Every,
	}
	var pos []string
	help := false
	for i := 0; i < len(argv); i++ {
		tok := argv[i]
		if tok == "--" {
			pos = append(pos, argv[i+1:]...)
			break
		}
		if !strings.HasPrefix(tok, "-") || tok == "-" {
			pos = append(pos, tok)
			continue
		}
		name := strings.TrimLeft(tok, "-")
		value := ""
		hasValue := false
		if j := strings.IndexByte(name, '='); j >= 0 {
			value, hasValue = name[j+1:], true
			name = name[:j]
		}
		if name == "h" || name == "help" {
			help = true
			continue
		}
		if alias, ok := boolAliases[name]; ok && spec[alias] == false {
			name = alias
		}
		needsValue, ok := spec[name]
		if !ok {
			return nil, nil, false, fmt.Errorf("无法识别的选项: --%s", name)
		}
		if !needsValue {
			if hasValue {
				return nil, nil, false, fmt.Errorf("选项 --%s 不接受取值", name)
			}
			setBool(opts, name, true)
			continue
		}
		if !hasValue {
			if i+1 >= len(argv) {
				return nil, nil, false, fmt.Errorf("选项 --%s 需要一个取值", name)
			}
			i++
			value = argv[i]
		}
		if err := setString(opts, name, value); err != nil {
			return nil, nil, false, fmt.Errorf("选项 --%s 的取值无法识别: %s", name, value)
		}
	}
	return opts, pos, help, nil
}

func setBool(o *options, name string, v bool) {
	switch name {
	case "alive":
		o.alive = v
	case "tail":
		o.tail = v
	case "local":
		o.local = v
	case "yes":
		o.yes = v
	case "dry-run":
		o.dryRun = v
	case "verbose":
		o.verbose = v
	case "no-scan":
		o.noScan = v
	}
}

func setString(o *options, name, v string) error {
	ints := map[string]*int{
		"max-buckets": &o.maxBuckets, "prefix-len": &o.prefixLen, "max-parts": &o.maxParts,
		"min-count": &o.minCount, "max-scan": &o.maxScan, "sample": &o.sample, "max": &o.max,
		"every": &o.every, "limit": &o.limit,
	}
	if p, ok := ints[name]; ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return err
		}
		*p = n
		return nil
	}
	switch name {
	case "bucket":
		o.bucket = v
	case "by":
		if v != "dir" && v != "prefix" && v != "ext" && v != "token" {
			return fmt.Errorf("无效取值")
		}
		o.by = v
	case "dir-level":
		o.dirLevel = v
	case "drives":
		o.drives = v
	case "state-dir":
		o.stateDir = v
	case "sort":
		if v != "time" && v != "size" && v != "count" {
			return fmt.Errorf("无效取值")
		}
		o.sort = v
	case "prefix":
		o.prefix = v
	case "start":
		n, err := strconv.Atoi(v)
		if err != nil {
			return err
		}
		o.start = &n
	case "end":
		n, err := strconv.Atoi(v)
		if err != nil {
			return err
		}
		o.end = &n
	default:
		return fmt.Errorf("未知选项")
	}
	return nil
}

// ---------- 状态 ----------

// loadState 优先用内存里的那份快照：编号必须和用户刚看到的统计表一致。
func (a *app) loadState(o *options) (*store.State, error) {
	if a.inShell {
		if a.state == nil {
			st, err := store.Read(o.stateDir)
			if err != nil {
				return nil, err
			}
			a.state = st
		}
		return a.state, nil
	}
	return store.Read(o.stateDir)
}

// fail 打印一条终止性提示：命令行走 stderr，shell 里缩进一行打印后继续用同一份快照。
func (a *app) fail(code int, format string, args ...any) int {
	msg := fmt.Sprintf(format, args...)
	if a.inShell {
		fmt.Fprintf(a.out, "  %s\n", msg)
	} else {
		fmt.Fprintln(a.err, msg)
	}
	return code
}

func (a *app) printf(format string, args ...any) {
	fmt.Fprintf(a.out, format, args...)
}

// ---------- 输出 ----------

// paramsJSON 复刻 Python json.dumps(params, ensure_ascii=False) 的键序与空格。
func paramsJSON(p store.Params) string {
	fields := []string{
		jsonKV("bucket", p.Bucket),
		jsonKV("by", p.By),
		fmt.Sprintf("%q: %d", "prefix_len", p.PrefixLen),
		jsonKV("dir_level", p.DirLevel),
		fmt.Sprintf("%q: %d", "max_buckets", p.MaxBuckets),
		fmt.Sprintf("%q: %d", "max_parts", p.MaxParts),
		fmt.Sprintf("%q: %d", "min_count", p.MinCount),
		jsonKV("drives", p.Drives),
	}
	return "{" + strings.Join(fields, ", ") + "}"
}

func jsonKV(k, v string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	// Python 的 json.dumps 不转义 < > &，这里关掉 Go 默认打开的 HTML 转义
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return fmt.Sprintf("%q: %q", k, v)
	}
	return fmt.Sprintf("%q: %s", k, strings.TrimSpace(buf.String()))
}

func (a *app) printParts(st *store.State, alive bool, sortKey string) {
	chosen := engine.SortedParts(st, sortKey, alive)
	total := 0
	var size int64
	for _, p := range st.Parts {
		total += p.Count
		size += p.Size
	}
	a.printf("快照时间 %s  参数 %s\n", st.Created, paramsJSON(st.Params))
	numbered := fmt.Sprintf("编号 1..%d", len(st.Parts))
	if len(st.Parts) == 0 {
		numbered = "没有分区"
	}
	a.printf("条目合计 %d 个 / %s  分区 %d 个  %s\n", total, tty.Human(float64(size)), len(st.Parts), numbered)
	headers := []string{"编号", "时间范围", st.GroupCol(), "元素数", "大小", "index区间", "盘", "剩余", "组内构成/示例"}
	widths := []int{4, 24, 44, 7, 9, 12, 4, 4, 26}
	rows := make([][]string, 0, len(chosen))
	for _, p := range chosen {
		left := p.Count - p.Deleted
		rest := "-"
		if p.Deleted > 0 {
			rest = fmt.Sprintf("%d/%d", left, p.Count)
		}
		rows = append(rows, []string{
			strconv.Itoa(p.PID), p.TimeRange, p.Label, strconv.Itoa(p.Count),
			tty.Human(float64(p.Size)), fmt.Sprintf("%d-%d", p.IndexStart, p.IndexEnd),
			p.Drives, rest, p.TopLabels,
		})
	}
	a.printf("%s\n", tty.Table(headers, rows, widths))
}

func (a *app) printItems(pairs []pair, header string) {
	a.printf("%s\n", header)
	headers := []string{"index", "删除时间", "大小", "盘", "文件名", "原路径"}
	widths := []int{7, 19, 9, 3, 28, 60}
	rows := make([][]string, 0, len(pairs))
	for _, pr := range pairs {
		rows = append(rows, []string{
			strconv.Itoa(pr.index), pr.item.DeletedAt, tty.Human(float64(pr.item.Size)),
			pr.item.Drive, pr.item.Display, pr.item.OrigPath,
		})
	}
	a.printf("%s\n", tty.Table(headers, rows, widths))
}

// ---------- 子命令 ----------

func engineOptions(o *options) engine.Options {
	return engine.Options{
		Bucket:     o.bucket,
		By:         o.by,
		DirLevel:   o.dirLevel,
		PrefixLen:  o.prefixLen,
		MaxBuckets: o.maxBuckets,
		MaxParts:   o.maxParts,
		MinCount:   o.minCount,
		Drives:     store.SplitDrives(o.drives),
		DrivesSpec: o.drives,
		MaxScan:    o.maxScan,
		StateDir:   o.stateDir,
	}
}

func (a *app) cmdScan(o *options) int {
	snap, err := engine.Scan(engineOptions(o))
	if err != nil {
		return a.fail(1, "%s", err.Error())
	}
	if snap.Overflow != nil {
		a.printOverflow(snap.Overflow)
	}
	st := snap.State
	if err := store.Write(o.stateDir, st); err != nil {
		return a.fail(1, "%s", err.Error())
	}
	listPath, err := store.WriteListCSV(o.stateDir, st)
	if err != nil {
		return a.fail(1, "%s", err.Error())
	}
	if a.inShell {
		a.state = st
	}
	a.printParts(st, false, "time")
	if len(st.Issues) > 0 {
		a.printf("\n异常记录 %d 条（详见 scan_list.csv 同目录状态文件），示例:\n", len(st.Issues))
		for i, line := range st.Issues {
			if i == 5 {
				break
			}
			a.printf("  - %s\n", line)
		}
	}
	a.printf("\n完整清单已写出: %s\n", listPath)
	if snap.Overflow != nil {
		a.println("注意: 本次快照只覆盖最老的一部分条目，删完这批再 scan 会拿到剩下的内容。")
	}
	if len(st.Parts) == 0 {
		a.println("回收站里当前用户没有对象（0 个条目），这就是扫描完成的结果，不是错误。")
		a.println("没有可删的分区；清空回收站之后重新 scan 会拿到同样的 0。")
		return 0
	}
	a.println("下一步: show <编号> 查看内容；delete <编号区间> 分批删除（默认 dry-run，加 --yes 才真删）")
	return 0
}

func (a *app) println(s string) { a.printf("%s\n", s) }

func (a *app) printOverflow(ov *meta.Overflow) {
	a.printf("! 回收站共有 %d 个条目，超过本次 --max-scan %d，快照只建了其中最老的 %d 个"+
		"（按 $I 写入时间排序，约等于删除时间），另有 %d 个未纳入。\n", ov.Total, ov.Taken, ov.Taken, ov.Total-ov.Taken)
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
	a.printf("  各盘 已扫/总数: %s\n", strings.Join(cells, ", "))
	a.printf("  两种继续办法:\n")
	a.printf("    A) 一次看全 —— 把上限调到总数以上再重扫: scan --max-scan %d\n", (ov.Total/10000+1)*10000)
	a.printf("    B) 分批推进（推荐）—— 先删掉本快照里的分区，再重新 scan，剩下的条目会自动进新快照并重新编号\n")
	a.println("")
}

func (a *app) cmdParts(o *options) int {
	st, err := a.loadState(o)
	if err != nil {
		return a.fail(1, "%s", err.Error())
	}
	a.printParts(st, o.alive, o.sort)
	return 0
}

func (a *app) cmdShow(o *options) int {
	st, err := a.loadState(o)
	if err != nil {
		return a.fail(1, "%s", err.Error())
	}
	ids, err := tty.ParseIDs(o.pid, len(st.Parts))
	if err != nil {
		return a.fail(1, "%s", err.Error())
	}
	part := st.Parts[ids[0]-1]
	pairs := partPairs(st, part)
	if o.local {
		for i := range pairs {
			pairs[i].index = pairs[i].index - part.IndexStart + 1
		}
	}
	pairs = slicePairs(pairs, o)
	header := fmt.Sprintf("分区 %d｜%s｜%s｜%d 个 / %s｜index %d-%d｜盘 %s",
		part.PID, part.TimeRange, part.Label, part.Count, tty.Human(float64(part.Size)),
		part.IndexStart, part.IndexEnd, part.Drives)
	limit := len(pairs)
	if o.limit != 0 {
		limit = o.limit
	}
	shown := pairs
	if limit < len(shown) {
		if o.tail {
			shown = shown[len(shown)-limit:]
		} else {
			shown = shown[:limit]
		}
	}
	a.printItems(shown, header)
	if len(shown) < len(pairs) {
		unit := "个全局 index"
		if o.local {
			unit = "个分区内序号"
		}
		a.printf("（另有 %d %s 未显示；--limit 0 全部显示，--start/--end 指定区间）\n", len(pairs)-len(shown), unit)
	}
	return 0
}

// pair 是「序号 + 条目」，序号可以是全局 index 或分区内序号（--local）。
type pair struct {
	index int
	item  *meta.Item
}

func partPairs(st *store.State, part *parts.Part) []pair {
	out := make([]pair, 0, len(part.Positions))
	for _, pos := range part.Positions {
		out = append(out, pair{pos + 1, st.Items[pos]})
	}
	return out
}

// slicePairs 按 index 区间或前缀关键词过滤。
func slicePairs(pairs []pair, o *options) []pair {
	if o.prefix != "" {
		var kept []pair
		for _, pr := range pairs {
			if engine.MatchPrefix(pr.item, o.prefix) {
				kept = append(kept, pr)
			}
		}
		pairs = kept
	}
	if o.start != nil || o.end != nil {
		// 只给一端时按「那一个 index」处理，同 Python 的 lo/hi 回填写法
		lo, hi := 1, 1<<60
		if o.start != nil {
			lo = *o.start
			hi = lo
			if o.end != nil {
				hi = *o.end
			}
		} else {
			lo = *o.end
			hi = lo
		}
		var kept []pair
		for _, pr := range pairs {
			if pr.index >= lo && pr.index <= hi {
				kept = append(kept, pr)
			}
		}
		pairs = kept
	}
	return pairs
}

func (a *app) cmdDelete(o *options) int {
	st, err := a.loadState(o)
	if err != nil {
		return a.fail(1, "%s", err.Error())
	}
	ids, err := tty.ParseIDs(o.pids, len(st.Parts))
	if err != nil {
		return a.fail(1, "%s", err.Error())
	}
	chosen := engine.SelectedParts(st, ids, o.prefix)
	if len(chosen) == 0 {
		prefix := o.prefix
		if prefix == "" {
			prefix = "(空)"
		}
		a.printf("没有匹配项：编号 %s 加上前缀过滤 %s 后为空，未做任何改动。\n", o.pids, prefix)
		return 0
	}
	count := 0
	for _, p := range chosen {
		count += p.Count
	}
	totalSize := engine.TotalSize(chosen)
	pids := make([]string, len(chosen))
	for i, p := range chosen {
		pids[i] = strconv.Itoa(p.PID)
	}
	a.printf("选中分区 %d 个: %s\n", len(chosen), strings.Join(pids, ","))
	a.printf("%s\n", tty.Table([]string{"编号", "时间范围", "路径组/前缀组", "元素数", "大小", "index区间"},
		func() [][]string {
			rows := make([][]string, 0, len(chosen))
			for _, p := range chosen {
				rows = append(rows, []string{strconv.Itoa(p.PID), p.TimeRange, p.Label,
					strconv.Itoa(p.Count), tty.Human(float64(p.Size)),
					fmt.Sprintf("%d-%d", p.IndexStart, p.IndexEnd)})
			}
			return rows
		}(), []int{4, 24, 44, 7, 9, 12}))
	a.printf("本次将永久删除 %d 个条目，约 %s（不可恢复）\n", count, tty.Human(float64(totalSize)))

	if o.dryRun {
		sample := count
		if o.sample < sample {
			sample = o.sample
		}
		a.printItems(dryRunPairs(st, chosen, sample), fmt.Sprintf("dry-run 预览前 %d 条：", sample))
		a.println("未做任何改动。确认无误后加 --yes 执行真删除。")
		return 0
	}
	if !o.yes {
		if !a.inShell {
			a.println("这是不可逆操作。加 --yes 明确确认后再执行（或先用 --dry-run 看清单）。")
			return 2
		}
		if !a.ask(fmt.Sprintf("确认永久删除这 %d 个条目 / %s（不可恢复）？", count, tty.Human(float64(totalSize)))) {
			a.println("已取消，未做任何改动。")
			return 0
		}
	}

	report, err := engine.RunDelete(st, chosen, o.stateDir, o.max, o.every, o.verbose,
		func(n, total, done, failed int) {
			a.printf("  进度 %d/%d  已删 %d  失败 %d\n", n, total, done, failed)
		})
	if err != nil && report == nil {
		// 保险丝或账本打不开：一条都没动
		a.println(err.Error())
		return 2
	}
	if err != nil {
		return a.fail(1, "删除中断: %s", err.Error())
	}
	a.printf("\n完成：删除 %d 个，失败 %d 个，账本 %s\n", report.Done, report.Failed,
		filepath.Join(o.stateDir, store.LedgerName))
	for i, line := range report.Errors {
		if i == 10 {
			break
		}
		a.printf("  ! %s\n", line)
	}
	a.printf("快照已更新（%s）：标记了本次删掉的条目，编号不变。\n", store.Path(o.stateDir))
	a.println("要看回收站最新状态请重新 scan：编号按当前内容重建，shell 里这等于创建新上下文。")
	if report.Failed > 0 {
		return 1
	}
	return 0
}

func dryRunPairs(st *store.State, chosen []*parts.Part, sample int) []pair {
	out := []pair{}
	for _, p := range chosen {
		for _, pos := range p.Positions {
			out = append(out, pair{pos + 1, st.Items[pos]})
		}
	}
	if sample < len(out) {
		out = out[:sample]
	}
	return out
}

func (a *app) ask(question string) bool {
	a.printf("%s [y/N] ", question)
	line, err := a.in.ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(strings.TrimRight(line, "\r\n"))) {
	case "y", "yes":
		return true
	}
	return false
}

// ---------- 交互 shell ----------

const shellHelp = `可用命令（就是那几个子命令，参数写法一样）:
  scan [选项]                    重新扫描 + 分区，创建新快照；之后的 show/delete 都用这份新编号
                                 选项同 scan: --bucket --by --dir-level --max-parts --min-count
                                 --max-buckets --prefix-len --drives --max-scan
  parts [--alive] [--sort time|size|count]      重列当前快照的分区统计表
  show <编号> [--start N --end N] [--prefix 串] [--limit 0] [--tail] [--local]
  delete <编号> [--dry-run|--yes] [--prefix 串] [--max N] [--sample N] [-v]
                                 编号支持 1-5,8-10,16。不带 --yes 时会在这里问一次 y/N
  info                           当前快照来自哪一次 scan、参数是什么
  help | quit | exit | q
例子:
  parts --sort size
  show 12 --limit 20
  delete 12-15 --dry-run
  delete 12-15
  scan --by dir --dir-level 1 --bucket gap:10m
`

func (a *app) cmdShell(o *options) int {
	a.inShell = true
	a.sessStateDir = o.stateDir
	a.println("回收站分批管理 shell：scan 一次，show/delete 反复用同一份快照，编号不会漂。")
	a.printf("快照目录 %s    输入 help 看用法，quit 退出。\n", o.stateDir)
	if !o.noScan {
		a.cmdScan(o)
	}
	for {
		a.printf("%s", a.prompt())
		line, err := a.in.ReadString('\n')
		if err != nil && line == "" {
			a.println("")
			break
		}
		if a.runLine(line) == quitSession {
			break
		}
	}
	return 0
}

const quitSession = -1

func (a *app) prompt() string {
	if a.state == nil {
		return "rbm> "
	}
	created := a.state.Created
	if len(created) > 11 {
		created = created[11:]
	}
	return fmt.Sprintf("rbm[%s 快照 %d条/%d区 剩%d]> ", created, len(a.state.Items),
		len(a.state.Parts), a.state.AliveLeft())
}

// splitLine 按空白切词，支持单双引号包裹（引号本身留在词里，由调用方剥掉）。
func splitLine(line string) []string {
	var tokens []string
	cur := strings.Builder{}
	inQuote := rune(0)
	started := false
	flush := func() {
		if started {
			tokens = append(tokens, cur.String())
			cur.Reset()
			started = false
		}
	}
	for _, r := range line {
		switch {
		case inQuote != 0:
			if r == inQuote {
				inQuote = 0
			}
			cur.WriteRune(r)
		case r == '\'' || r == '"':
			inQuote = r
			cur.WriteRune(r)
			started = true
		case r == ' ' || r == '\t' || r == '\r' || r == '\n':
			flush()
		default:
			cur.WriteRune(r)
			started = true
		}
	}
	flush()
	return tokens
}

// runLine 执行会话里的一行；单条命令出错不该丢掉整个会话的上下文。
func (a *app) runLine(line string) int {
	tokens := splitLine(line)
	for i, t := range tokens {
		tokens[i] = strings.Trim(t, "'\"")
	}
	if len(tokens) == 0 {
		return 0
	}
	head := strings.ToLower(tokens[0])
	switch head {
	case "quit", "exit", "q", "退出":
		return quitSession
	case "help", "?", "h", "帮助":
		a.printf("%s", shellHelp)
		return 0
	case "info":
		a.printInfo()
		return 0
	}
	if _, ok := flagSpecs[head]; !ok {
		a.printf("  rbm %s: error: 无法识别的命令 %s（help 看用法）\n", head, head)
		return 2
	}
	opts, pos, help, err := parseFlags(head, tokens[1:])
	if err != nil {
		a.printf("  rbm %s: error: %v\n", head, err)
		return 2
	}
	if help {
		a.printf("%s\n", commandHelp(head))
		return 0
	}
	if err := fillPositional(head, pos, opts); err != nil {
		a.printf("  rbm %s: error: %v\n", head, err)
		return 2
	}
	seenStateDir := false
	for _, t := range tokens {
		if strings.HasPrefix(t, "--state-dir") {
			seenStateDir = true
			break
		}
	}
	if !seenStateDir {
		// 这一行没指定就用 shell 启动时的那个目录，否则会话里的命令会写回默认 .rbm，和快照分家
		opts.stateDir = a.sessStateDir
	}
	if head == "shell" {
		a.println("  已经在 shell 里了，直接输入 scan / show / delete 即可。")
		return 0
	}
	return a.dispatch(head, opts)
}

func (a *app) printInfo() {
	stateDir := a.sessStateDir
	if a.state == nil {
		a.printf("  当前内存里没有快照（磁盘上可能有，命令会退回读 %s）\n", store.Path(stateDir))
		return
	}
	st := a.state
	a.printf("  快照时间 %s  条目 %d  分区 %d\n", st.Created, len(st.Items), len(st.Parts))
	a.printf("  参数 %s\n", paramsJSON(st.Params))
	deleted := 0
	for _, p := range st.Parts {
		deleted += p.Deleted
	}
	if deleted > 0 {
		a.printf("  本快照内已标记删除 %d 个（编号不变；要看真实状态请 scan 重建）\n", deleted)
	}
	if st.LastDelete != "" {
		a.printf("  上次删除 %s\n", st.LastDelete)
	}
}
