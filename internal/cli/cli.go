// Package cli 是 rbm 的命令行外壳：scan/parts/show/delete/shell 五个子命令，
// 表头、退出码都对齐 Python 版，界面只做展示，判定都走 internal/engine。
// 所有面客文案取自 internal/i18n，默认中文，--lang en 或 RBM_LANG 切英文。
package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"rbm/internal/engine"
	"rbm/internal/i18n"
	"rbm/internal/meta"
	"rbm/internal/parts"
	"rbm/internal/store"
	"rbm/internal/tty"
)

// SplitLangArgs 把 --lang zh|en / --lang=zh 从参数里摘出来，剩下原样返回。
// 位置随意：rbm --lang en scan 与 rbm scan --lang en 都能认，多个 --lang 以第一个为准。
// 摘干净是必要的，否则子命令的选项表里没有 lang，会当成「无法识别的选项」。
func SplitLangArgs(argv []string) (string, []string) {
	lang := ""
	out := make([]string, 0, len(argv))
	for i := 0; i < len(argv); i++ {
		switch tok := argv[i]; {
		case tok == "--lang":
			if i+1 < len(argv) && lang == "" {
				lang = argv[i+1]
			}
			i++
		case strings.HasPrefix(tok, "--lang="):
			if lang == "" {
				lang = strings.TrimPrefix(tok, "--lang=")
			}
		default:
			out = append(out, tok)
		}
	}
	return lang, out
}

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
	// 启动时 main.go 已经摘过一次 --lang；这里再摘一次，Direct 调 Main 的调用方
	// （测试、被脚本直接 exec）也能用同一个写法切语言。
	if lang, rest := SplitLangArgs(argv); lang != "" {
		if l, ok := i18n.Parse(lang); ok {
			i18n.Set(l)
		}
		argv = rest
	}
	if len(argv) == 0 {
		fmt.Fprintln(a.err, i18n.T("cli.usage-hint"))
		return 2
	}
	head := argv[0]
	if head == "--help" || head == "-h" || head == "help" {
		fmt.Fprintln(a.out, i18n.T("cli.help.root"))
		return 0
	}
	if _, ok := flagSpecs[head]; !ok {
		fmt.Fprintln(a.err, i18n.Tf("cli.bad-subcommand", head))
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
	head := i18n.T("cli.usage-prefix")
	usage := func(key, args string) string {
		return head + i18n.T(key) + "\n\n" + i18n.T(args)
	}
	switch cmd {
	case "scan":
		return usage("cli.usage.scan", "cli.desc.scan") + "\n\n" +
			i18n.T("cli.help.options-heading") + "\n" +
			i18n.T("cli.help.partition-opts") + "\n" + i18n.T("cli.help.state-dir")
	case "parts":
		return usage("cli.usage.parts", "cli.desc.parts")
	case "show":
		return usage("cli.usage.show", "cli.desc.show")
	case "delete":
		return usage("cli.usage.delete", "cli.desc.delete")
	case "shell":
		return usage("cli.usage.shell", "cli.desc.shell")
	}
	return head
}

func fillPositional(cmd string, pos []string, opts *options) error {
	switch cmd {
	case "show":
		if len(pos) != 1 {
			return errors.New(i18n.T("cli.need-pid"))
		}
		opts.pid = pos[0]
	case "delete":
		if len(pos) != 1 {
			return errors.New(i18n.T("cli.need-pids"))
		}
		opts.pids = pos[0]
	default:
		if len(pos) > 0 {
			return errors.New(i18n.Tf("cli.extra-args", strings.Join(pos, " ")))
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
			return nil, nil, false, errors.New(i18n.Tf("cli.bad-flag", name))
		}
		if !needsValue {
			if hasValue {
				return nil, nil, false, errors.New(i18n.Tf("cli.flag-no-value", name))
			}
			setBool(opts, name, true)
			continue
		}
		if !hasValue {
			if i+1 >= len(argv) {
				return nil, nil, false, errors.New(i18n.Tf("cli.flag-need-value", name))
			}
			i++
			value = argv[i]
		}
		if err := setString(opts, name, value); err != nil {
			return nil, nil, false, errors.New(i18n.Tf("cli.flag-bad-value", name, value))
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
			return errors.New(i18n.T("cli.invalid-value"))
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
			return errors.New(i18n.T("cli.invalid-value"))
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
		return errors.New(i18n.T("cli.unknown-option"))
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

// 表头与列宽成对给出：英文列名普遍比中文长（「盘」占 3 列 vs "Drive" 占 5 列），
// 沿用中文那套宽度会把表头截成 "Dr…"，所以宽度跟着语言一起换。
func partsHeaders(st *store.State) []string {
	return []string{
		i18n.T("cli.th.pid"), i18n.T("cli.th.range"), st.GroupCol(), i18n.T("cli.th.count"),
		i18n.T("cli.th.size"), i18n.T("cli.th.index"), i18n.T("cli.th.drive"),
		i18n.T("cli.th.left"), i18n.T("cli.th.top"),
	}
}

func partsWidths() []int {
	if i18n.IsEn() {
		return []int{4, 24, 44, 7, 9, 12, 5, 4, 26}
	}
	return []int{4, 24, 44, 7, 9, 12, 4, 4, 26}
}

func itemWidths() []int {
	if i18n.IsEn() {
		return []int{7, 19, 9, 5, 28, 60}
	}
	return []int{7, 19, 9, 3, 28, 60}
}

func partsRows(list []*parts.Part) [][]string {
	rows := make([][]string, 0, len(list))
	for _, p := range list {
		rest := "-"
		if p.Deleted > 0 {
			rest = fmt.Sprintf("%d/%d", p.Count-p.Deleted, p.Count)
		}
		rows = append(rows, []string{
			strconv.Itoa(p.PID), p.TimeRange, p.Label, strconv.Itoa(p.Count),
			tty.Human(float64(p.Size)), fmt.Sprintf("%d-%d", p.IndexStart, p.IndexEnd),
			p.Drives, rest, p.TopLabels,
		})
	}
	return rows
}

func (a *app) printParts(st *store.State, alive bool, sortKey string) {
	chosen := engine.SortedParts(st, sortKey, alive)
	total := 0
	var size int64
	for _, p := range st.Parts {
		total += p.Count
		size += p.Size
	}
	a.printf("%s\n", i18n.Tf("cli.parts.line1", st.Created, paramsJSON(st.Params)))
	numbered := i18n.Tf("cli.parts.numbered", len(st.Parts))
	if len(st.Parts) == 0 {
		numbered = i18n.T("cli.parts.no-parts")
	}
	a.printf("%s\n", i18n.Tf("cli.parts.line2", total, i18n.Noun(total, "个", "item", "items"),
		tty.Human(float64(size)), len(st.Parts), i18n.Noun(len(st.Parts), "个", "partition", "partitions"),
		numbered))
	a.printf("%s\n", tty.Table(partsHeaders(st), partsRows(chosen), partsWidths()))
}

func (a *app) printItems(pairs []pair, header string) {
	a.printf("%s\n", header)
	headers := []string{"index", i18n.T("cli.th.deleted"), i18n.T("cli.th.size"),
		i18n.T("cli.th.drive"), i18n.T("cli.th.name"), i18n.T("cli.th.orig")}
	rows := make([][]string, 0, len(pairs))
	for _, pr := range pairs {
		rows = append(rows, []string{
			strconv.Itoa(pr.index), pr.item.DeletedAt, tty.Human(float64(pr.item.Size)),
			pr.item.Drive, pr.item.Display, pr.item.OrigPath,
		})
	}
	a.printf("%s\n", tty.Table(headers, rows, itemWidths()))
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
	start := time.Now()
	snap, err := engine.Scan(engineOptions(o))
	if err != nil {
		return a.fail(1, "%s", err.Error())
	}
	if snap.Overflow != nil {
		a.printOverflow(snap.Overflow)
	}
	st := snap.State
	tWrite := time.Now()
	err = store.Write(o.stateDir, st)
	snap.Stats.Write = time.Since(tWrite)
	if err != nil {
		return a.fail(1, "%s", err.Error())
	}
	tCSV := time.Now()
	listPath, err := store.WriteListCSV(o.stateDir, st)
	snap.Stats.CSV = time.Since(tCSV)
	wall := time.Since(start)
	if err != nil {
		return a.fail(1, "%s", err.Error())
	}
	if a.inShell {
		a.state = st
	}
	a.printParts(st, false, "time")
	if len(st.Issues) > 0 {
		a.printf("\n%s\n", i18n.Tf("cli.scan.issues", len(st.Issues)))
		for i, line := range st.Issues {
			if i == 5 {
				break
			}
			a.printf("  - %s\n", line)
		}
	}
	a.printf("\n%s\n", i18n.Tf("cli.scan.list-written", listPath))
	a.printf("%s\n", i18n.Tf("cli.scan.elapsed", wall.Round(10*time.Millisecond)))
	a.printf("%s\n", snap.Stats.TimingText())
	if snap.Overflow != nil {
		a.println(i18n.T("cli.scan.overflow-note"))
	}
	if len(st.Parts) == 0 {
		a.println(i18n.T("cli.scan.empty-1"))
		a.println(i18n.T("cli.scan.empty-2"))
		return 0
	}
	a.println(i18n.T("cli.scan.next-step"))
	return 0
}

func (a *app) println(s string) { a.printf("%s\n", s) }

func (a *app) printOverflow(ov *meta.Overflow) {
	a.printf("%s\n", i18n.Tf("cli.overflow.head", ov.Total,
		i18n.Noun(ov.Total, "个条目", "item", "items"), ov.Taken, ov.Taken, ov.Total-ov.Taken))
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
	a.printf("%s\n", i18n.Tf("cli.overflow.drives", strings.Join(cells, ", ")))
	a.println(i18n.T("cli.overflow.two-ways"))
	a.printf("%s\n", i18n.Tf("cli.overflow.option-a", (ov.Total/10000+1)*10000))
	a.println(i18n.T("cli.overflow.option-b"))
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
	header := i18n.Tf("cli.show.header",
		part.PID, part.TimeRange, part.Label, part.Count,
		i18n.Noun(part.Count, "个", "item", "items"), tty.Human(float64(part.Size)),
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
		hidden := len(pairs) - len(shown)
		// 中文这边只有量词短语、不分单复数；英文要 "1 global index" / "2 global indexes"
		unit := i18n.Noun(hidden, "个全局 index", "global index", "global indexes")
		if o.local {
			unit = i18n.Noun(hidden, "个分区内序号", "in-partition position", "in-partition positions")
		}
		a.printf("%s\n", i18n.Tf("cli.show.hidden", hidden, unit))
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
			prefix = i18n.T("cli.del.empty-prefix")
		}
		a.printf("%s\n", i18n.Tf("cli.del.nomatch", o.pids, prefix))
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
	a.printf("%s\n", i18n.Tf("cli.del.selected", len(chosen),
		i18n.Noun(len(chosen), "个", "partition", "partitions"), strings.Join(pids, ",")))
	slim := make([][]string, 0, len(chosen))
	for _, r := range partsRows(chosen) {
		slim = append(slim, r[:6])
	}
	a.printf("%s\n", tty.Table([]string{i18n.T("cli.th.pid"), i18n.T("cli.th.range"),
		i18n.T("cli.th.group2"), i18n.T("cli.th.count"), i18n.T("cli.th.size"), i18n.T("cli.th.index")},
		slim, []int{4, 24, 44, 7, 9, 12}))
	a.printf("%s\n", i18n.Tf("cli.del.will", count,
		i18n.Noun(count, "个条目", "item", "items"), tty.Human(float64(totalSize))))

	if o.dryRun {
		sample := count
		if o.sample < sample {
			sample = o.sample
		}
		a.printItems(dryRunPairs(st, chosen, sample), i18n.Tf("cli.del.dryrun-preview", sample))
		a.println(i18n.T("cli.del.dryrun-done"))
		return 0
	}
	if !o.yes {
		if !a.inShell {
			a.println(i18n.T("cli.del.need-yes"))
			return 2
		}
		if !a.ask(i18n.Tf("cli.del.ask", count,
			i18n.Noun(count, "个条目", "item", "items"), tty.Human(float64(totalSize)))) {
			a.println(i18n.T("cli.del.cancelled"))
			return 0
		}
	}

	report, err := engine.RunDelete(st, chosen, o.stateDir, o.max, o.every, o.verbose,
		func(n, total, done, failed int) {
			a.printf("%s\n", i18n.Tf("cli.del.progress", n, total, done, failed))
		})
	if err != nil && report == nil {
		// 保险丝或账本打不开：一条都没动
		a.println(err.Error())
		return 2
	}
	if err != nil {
		return a.fail(1, "%s", i18n.Tf("cli.del.interrupted", err))
	}
	a.printf("\n%s\n", i18n.Tf("cli.del.done", report.Done, report.Failed,
		filepath.Join(o.stateDir, store.LedgerName)))
	for i, line := range report.Errors {
		if i == 10 {
			break
		}
		a.printf("  ! %s\n", line)
	}
	a.printf("%s\n", i18n.Tf("cli.del.state-updated", store.Path(o.stateDir)))
	a.println(i18n.T("cli.del.rescan"))
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

func (a *app) cmdShell(o *options) int {
	a.inShell = true
	a.sessStateDir = o.stateDir
	a.println(i18n.T("cli.shell.intro"))
	a.printf("%s\n", i18n.Tf("cli.shell.state-dir", o.stateDir))
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
	return i18n.Tf("cli.prompt.state", created, len(a.state.Items),
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
	// 会话里也认 --lang，和命令行「位置随意」同一个口径
	lang, rest := SplitLangArgs(tokens)
	tokens = rest
	if lang != "" {
		if l, ok := i18n.Parse(lang); ok {
			i18n.Set(l)
		}
	}
	if len(tokens) == 0 {
		return 0
	}
	head := strings.ToLower(tokens[0])
	switch head {
	case "quit", "exit", "q", "退出":
		return quitSession
	case "help", "?", "h", "帮助":
		a.printf("%s", i18n.T("cli.help.shell"))
		return 0
	case "info":
		a.printInfo()
		return 0
	}
	if _, ok := flagSpecs[head]; !ok {
		a.printf("%s\n", i18n.Tf("cli.bad-shell-command", head, head))
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
		a.println(i18n.T("cli.shell.already-in"))
		return 0
	}
	return a.dispatch(head, opts)
}

func (a *app) printInfo() {
	stateDir := a.sessStateDir
	if a.state == nil {
		a.printf("%s\n", i18n.Tf("cli.info.no-snapshot", store.Path(stateDir)))
		return
	}
	st := a.state
	a.printf("%s\n", i18n.Tf("cli.info.line", st.Created, len(st.Items), len(st.Parts)))
	a.printf("%s\n", i18n.Tf("cli.info.params", paramsJSON(st.Params)))
	deleted := 0
	for _, p := range st.Parts {
		deleted += p.Deleted
	}
	if deleted > 0 {
		a.printf("%s\n", i18n.Tf("cli.info.deleted", deleted,
			i18n.Noun(deleted, "个", "item", "items")))
	}
	if st.LastDelete != "" {
		a.printf("%s\n", i18n.Tf("cli.info.last-delete", st.LastDelete))
	}
}
