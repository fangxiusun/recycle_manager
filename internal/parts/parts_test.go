package parts

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"rbm/internal/meta"
)

// ---------- 与 Python 版的逐字段交叉比对 ----------

type fixturePart struct {
	TimeBucket int     `json:"time_bucket"`
	RangeStart float64 `json:"range_start"`
	RangeEnd   float64 `json:"range_end"`
	Label      string  `json:"label"`
	Count      int     `json:"count"`
	Size       int64   `json:"size"`
	Positions  []int   `json:"positions"`
	By         string  `json:"by"`
	PrefixLen  int     `json:"prefix_len"`
	DirLevel   string  `json:"dir_level"`
	PID        int     `json:"pid"`
	IndexStart int     `json:"index_start"`
	IndexEnd   int     `json:"index_end"`
	TimeRange  string  `json:"time_range"`
	Drives     string  `json:"drives"`
	TopLabels  string  `json:"top_labels"`
	Deleted    int     `json:"deleted"`
}

type fixtureConfig struct {
	Cfg struct {
		Bucket     *string `json:"bucket"` // null = 隐式断点
		By         string  `json:"by"`
		DirLevel   string  `json:"dir_level"`
		PrefixLen  int     `json:"prefix_len"`
		MaxBuckets int     `json:"max_buckets"`
		MaxParts   int     `json:"max_parts"`
		MinCount   int     `json:"min_count"`
	} `json:"cfg"`
	Bounds  []float64     `json:"bounds"` // 超过 1000 个边界的配置不进 fixture
	Parts   []fixturePart `json:"parts"`
	Ordered int           `json:"ordered"`
}

type fixtureSet struct {
	Items   []*meta.Item    `json:"items"`
	Configs []fixtureConfig `json:"configs"`
}

type fixture struct {
	Note string       `json:"note"`
	Sets []fixtureSet `json:"sets"`
}

// TestMatchesPythonFixture 用 recycle_manager/rb_parts.py 的真实输出当黄金样例：
// 同一份条目、同一套参数，Go 的分区结果必须逐字段一致（含编号顺序与重排后的条目数）。
// 样例由 testdata/regen_py_fixture.py 生成，只含合成数据。
func TestMatchesPythonFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "py_parts.json"))
	if err != nil {
		t.Fatalf("读 fixture 失败: %v", err)
	}
	var fx fixture
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatalf("解析 fixture 失败: %v", err)
	}
	if len(fx.Sets) == 0 {
		t.Fatal("fixture 是空的")
	}
	total := 0
	for si, set := range fx.Sets {
		for ci, fc := range set.Configs {
			name := fmt.Sprintf("set%d_cfg%d_%s_%s", si, ci, fc.Cfg.By, bucketText(fc.Cfg.Bucket))
			t.Run(name, func(t *testing.T) {
				bounds, got, ordered, err := BuildPartitions(set.Items, derefBucket(fc.Cfg.Bucket),
					fc.Cfg.MaxBuckets, fc.Cfg.By, fc.Cfg.PrefixLen, fc.Cfg.MaxParts,
					fc.Cfg.MinCount, fc.Cfg.DirLevel)
				if err != nil {
					t.Fatalf("Go 分区报错: %v", err)
				}
				if len(ordered) != fc.Ordered {
					t.Errorf("重排后的条目数 %d != python %d", len(ordered), fc.Ordered)
				}
				if fc.Bounds != nil && !sameFloats(bounds, fc.Bounds) {
					t.Errorf("时间边界不一致: go %d 个, python %d 个", len(bounds), len(fc.Bounds))
				}
				if len(got) != len(fc.Parts) {
					t.Fatalf("分区数 %d != python %d", len(got), len(fc.Parts))
				}
				for i, want := range fc.Parts {
					if diff := partDiff(got[i], want); diff != "" {
						t.Errorf("分区 %d 不符: %s", i+1, diff)
					}
				}
			})
			total++
		}
	}
	t.Logf("%d 组配置与 python 结果一致", total)
}

func bucketText(b *string) string {
	if b == nil {
		return "auto-gap"
	}
	return *b
}

func derefBucket(b *string) string {
	if b == nil {
		return ""
	}
	return *b
}

func sameFloats(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// partDiff 返回第一个不一致的字段，形如 `label: go=x py=y`；全一致返回空串。
func partDiff(got *Part, want fixturePart) string {
	d := func(field string, a, b interface{}) string {
		if !reflect.DeepEqual(a, b) {
			return fmt.Sprintf("%s: go=%v py=%v", field, a, b)
		}
		return ""
	}
	for _, c := range []string{
		d("pid", got.PID, want.PID),
		d("time_bucket", got.TimeBucket, want.TimeBucket),
		d("range_start", got.RangeStart, want.RangeStart),
		d("range_end", got.RangeEnd, want.RangeEnd),
		d("label", got.Label, want.Label),
		d("count", got.Count, want.Count),
		d("size", got.Size, want.Size),
		d("by", got.By, want.By),
		d("prefix_len", got.PrefixLen, want.PrefixLen),
		d("dir_level", got.DirLevel, want.DirLevel),
		d("index_start", got.IndexStart, want.IndexStart),
		d("index_end", got.IndexEnd, want.IndexEnd),
		d("time_range", got.TimeRange, want.TimeRange),
		d("drives", got.Drives, want.Drives),
		d("top_labels", got.TopLabels, want.TopLabels),
		d("deleted", got.Deleted, want.Deleted),
		d("positions", got.Positions, want.Positions),
	} {
		if c != "" {
			return c
		}
	}
	return ""
}

// ---------- 不变量测试（移植自 recycle_manager/test_partitions.py） ----------

var (
	fixtureNames = []string{"chunk-46.js", "test_output.txt", "20261014.log", "ssr", "vite.config.mjs",
		"0b6f3a.json", "笔记-草稿.md", "webpack", "write.lock", ".env.local", ""}
	fixtureParents = []string{
		`C:\Users\x\Temp\Vs9POM\ssr`,
		`C:\Users\x\Temp\lth-fixture\ssr`,
		`C:\Users\x\Temp\ocqyI\ssr`,
		`G:\workspace\local-tool-hub\dist-web\assets`,
		`C:\Users\x\Documents\reports`,
		`C:\Users\x`,
		`C:\`,
	}
)

// synth 造 n 条合成条目，时间跨度 500 天，约 2% 是「元数据损坏」（epoch=0），与 Python 版同构。
func synth(n int, seed int64) []*meta.Item {
	rng := rand.New(rand.NewSource(seed))
	base := float64(time.Date(2026, 10, 15, 0, 0, 0, 0, meta.LocalTZ).Unix())
	items := make([]*meta.Item, 0, n)
	for i := 0; i < n; i++ {
		epoch := base - rng.Float64()*86400*500
		name := fixtureNames[rng.Intn(len(fixtureNames))]
		if name == "" {
			name = fmt.Sprintf("rand%d", rng.Intn(999))
		}
		parent := fixtureParents[rng.Intn(len(fixtureParents))]
		orig := parent
		if parent[len(parent)-1] != '\\' {
			orig = parent + "\\" + name
		}
		corrupt := rng.Float64() < 0.02
		if corrupt {
			epoch, orig = 0, Illegal
		}
		items = append(items, &meta.Item{
			Display:     name,
			MetaVersion: -1,
			Epoch:       epoch,
			DeletedAt:   Fmt(epoch, "2006-01-02 15:04:05"),
			Size:        int64(rng.Intn(5_000_000)),
			Drive:       []string{"C", "D", "G"}[rng.Intn(3)],
			OrigPath:    orig,
		})
		if !corrupt {
			items[len(items)-1].MetaVersion = 2
		}
	}
	return items
}

type cfgArgs struct {
	bucket     string
	by         string
	prefixLen  int
	maxBuckets int
	maxParts   int
	minCount   int
	dirLevel   string
}

// check 跑一遍分区并校验不变量，返回 (分区, 重排后的条目)。
func check(t *testing.T, items []*meta.Item, a cfgArgs) ([]*Part, []*meta.Item) {
	t.Helper()
	bounds, ps, ordered, err := BuildPartitions(items, a.bucket, a.maxBuckets, a.by,
		a.prefixLen, a.maxParts, a.minCount, a.dirLevel)
	if err != nil {
		t.Fatalf("分区失败 (%+v): %v", a, err)
	}
	var flat []int
	for _, p := range ps {
		flat = append(flat, p.Positions...)
	}
	if len(flat) != len(items) {
		t.Fatalf("条目丢失或重复: %d vs %d", len(flat), len(items))
	}
	seen := map[int]bool{}
	for _, p := range flat {
		if seen[p] {
			t.Fatalf("同一条目出现在多个分区")
		}
		seen[p] = true
	}
	for i := 0; i < len(items); i++ {
		if !seen[i] {
			t.Fatalf("index 未覆盖全部条目，缺 %d", i)
		}
	}
	if len(ordered) != len(items) {
		t.Fatalf("重排后的条目数变了: %d vs %d", len(ordered), len(items))
	}
	total := 0
	for _, p := range ps {
		if p.Count != len(p.Positions) {
			t.Errorf("分区 %d 的 count 与 positions 不符", p.PID)
		}
		var size int64
		for _, pos := range p.Positions {
			size += ordered[pos].Size
		}
		if p.Size != size {
			t.Errorf("分区 %d 的 size %d != 条目求和 %d", p.PID, p.Size, size)
		}
		if p.IndexEnd != p.IndexStart+p.Count-1 {
			t.Errorf("分区 %d 的 index 区间不闭合", p.PID)
		}
		if p.IndexStart != total+1 {
			t.Errorf("分区 %d 的 index 区间不连续: start=%d, 上一个 end=%d", p.PID, p.IndexStart, total)
		}
		total = p.IndexEnd
		want := make([]int, p.Count)
		for i := range want {
			want[i] = p.IndexStart - 1 + i
		}
		if !reflect.DeepEqual(p.Positions, want) {
			t.Errorf("分区 %d 声明的 index区间 %d-%d 与条目下标不符", p.PID, p.IndexStart, p.IndexEnd)
		}
		for _, pos := range p.Positions {
			e := ordered[pos].Epoch
			if e < p.RangeStart || e > p.RangeEnd {
				t.Errorf("条目时间 %v 不在分区 %d 的 %v~%v 内", e, p.PID, p.RangeStart, p.RangeEnd)
			}
		}
	}
	if total != len(items) {
		t.Fatalf("最后一个分区的 index_end %d != 条目数 %d", total, len(items))
	}
	if a.maxParts > 0 && len(ps) > a.maxParts {
		t.Fatalf("分区数 %d 超过上限 %d", len(ps), a.maxParts)
	}
	_ = bounds
	return ps, ordered
}

var (
	testBY      = []string{"dir", "prefix", "ext", "token"}
	testBuckets = []string{"", "minutely", "hourly", "daily", "weekly", "monthly", "15m", "30m",
		"3d", "12h", "2mo", "auto", "gap", "gap:5m"}
	testSizes     = []int{1, 5, 200, 1200}
	pickPrefixLen = []int{1, 2, 3, 8}
	pickMaxBucket = []int{2, 6, 12, 60}
	pickMaxPart   = []int{12, 60, 500}
	pickMinCount  = []int{1, 5, 20}
	pickDirLevel  = []string{"1", "2", "4"}
)

func TestAllConfigs(t *testing.T) {
	rng := rand.New(rand.NewSource(20261015))
	type c struct {
		n int
		a cfgArgs
	}
	var cases []c
	for _, n := range testSizes { // 每种规模跑满：规模是最容易出边界问题的维度
		for _, by := range testBY {
			for _, b := range testBuckets {
				cases = append(cases, c{n, cfgArgs{bucket: b, by: by,
					prefixLen:  pickPrefixLen[rng.Intn(4)],
					maxBuckets: pickMaxBucket[rng.Intn(4)],
					maxParts:   pickMaxPart[rng.Intn(3)],
					minCount:   pickMinCount[rng.Intn(3)],
					dirLevel:   pickDirLevel[rng.Intn(3)]}})
			}
		}
	}
	for i := 0; i < 320; i++ { // 随机补一批组合
		cases = append(cases, c{testSizes[rng.Intn(4)], cfgArgs{
			bucket:     testBuckets[rng.Intn(len(testBuckets))],
			by:         testBY[rng.Intn(len(testBY))],
			prefixLen:  pickPrefixLen[rng.Intn(4)],
			maxBuckets: pickMaxBucket[rng.Intn(4)],
			maxParts:   pickMaxPart[rng.Intn(3)],
			minCount:   pickMinCount[rng.Intn(3)],
			dirLevel:   pickDirLevel[rng.Intn(3)]}})
	}
	for _, tc := range cases {
		items := synth(tc.n, int64(tc.n*7+tc.a.prefixLen))
		got, _ := check(t, items, tc.a)
		if len(got) == 0 {
			t.Fatalf("没有产出任何分区: %+v", tc.a)
		}
	}
	t.Logf("ok，%d 组配置通过", len(cases))
}

func TestDirGroupsSameFolder(t *testing.T) {
	items := synth(300, 5)
	ps, ordered := check(t, items, cfgArgs{bucket: "gap:5m", by: "dir", prefixLen: 3,
		maxBuckets: 12, maxParts: 60, minCount: 5, dirLevel: "2"})
	for _, p := range ps {
		if p.Label == All || p.Label == Others || p.Label == Illegal {
			continue
		}
		dirs := map[string]bool{}
		for _, pos := range p.Positions {
			dirs[DirKey(ordered[pos], 2)] = true
		}
		if len(dirs) != 1 || !dirs[p.Label] {
			t.Fatalf("同一分区 %q 里混进了不同目录: %v", p.Label, dirs)
		}
	}
}

func TestGapBoundsCoverAll(t *testing.T) {
	items := synth(500, 3)
	epochs := make([]float64, len(items))
	for i, it := range items {
		epochs[i] = it.Epoch
	}
	bounds, err := TimeBounds(epochs, "gap", 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(bounds)-1 > 8 {
		t.Errorf("gap 桶数 %d 超过上限 8", len(bounds)-1)
	}
	ps, _ := check(t, items, cfgArgs{bucket: "gap", by: "dir", prefixLen: 3, maxBuckets: 8,
		maxParts: 320, minCount: 5, dirLevel: "2"})
	if len(ps) > 8*40 {
		t.Errorf("分区数 %d 超出预期", len(ps))
	}
}

func TestMinuteBoundsAligned(t *testing.T) {
	items := synth(200, 9)
	epochs := make([]float64, len(items))
	for i, it := range items {
		epochs[i] = it.Epoch
	}
	bounds, err := TimeBounds(epochs, "30m", 400)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range bounds {
		if math.Mod(b, 60) != 0 {
			t.Fatalf("边界 %v 不是整分钟", b)
		}
	}
}

// monthlyItems 跨 2026-05..10 定点放条目，8 月故意空着（月桶不该给它建区）。
func monthlyItems() []*meta.Item {
	marks := [][2]int{{5, 3}, {5, 28}, {6, 9}, {6, 30}, {7, 1}, {7, 15}, {9, 2}, {9, 22}, {10, 14}, {10, 15}}
	var items []*meta.Item
	for _, m := range marks {
		for i := 0; i < 3; i++ {
			epoch := float64(time.Date(2026, time.Month(m[0]), m[1], 12, 0, 0, 0, meta.LocalTZ).Unix())
			items = append(items, &meta.Item{Display: "note.md", MetaVersion: 2, Epoch: epoch,
				DeletedAt: Fmt(epoch, "2006-01-02 15:04:05"), Size: 1000, Drive: "C",
				OrigPath: `C:\Users\x\Documents\reports\note.md`})
		}
	}
	return items
}

func TestMonthlyBucketsSpanMonths(t *testing.T) {
	// 修复前 month 分支只会产出 1 个边界，全部条目挤进同一个分区——这条守住回归。
	items := monthlyItems()
	ps, ordered := check(t, items, cfgArgs{bucket: "monthly", by: "dir", prefixLen: 3,
		maxBuckets: 200, maxParts: 200, minCount: 1, dirLevel: "2"})
	buckets := map[int]bool{}
	for _, p := range ps {
		buckets[p.TimeBucket] = true
	}
	if len(buckets) < 2 {
		t.Fatalf("monthly 只切出一个时间桶，等于没分桶: %v", buckets)
	}
	if len(ps) != 5 {
		t.Fatalf("5 个有数据的月份各 1 个分区，实得 %d", len(ps))
	}
	for _, p := range ps {
		months := map[int]bool{}
		for _, pos := range p.Positions {
			months[int(local(ordered[pos].Epoch).Month())] = true
		}
		if len(months) != 1 {
			t.Fatalf("分区 %d 混了多个月: %v", p.PID, months)
		}
		month := onlyMonth(months)
		first := p.Positions[0]
		if p.RangeStart != floorMonth(ordered[first].Epoch, 1) {
			t.Errorf("%d 月分区的起点不是月首", month)
		}
	}
	for _, p := range ps {
		if local(p.RangeStart).Month() == 8 {
			t.Errorf("空月份（8 月）建了区")
		}
	}

	prev := -1
	for _, spec := range []string{"monthly", "2mo", "3mo", "6mo", "12mo"} {
		months := 1
		if spec != "monthly" {
			months = int(spec[0] - '0')
			if len(spec) == 4 {
				months = int(spec[0]-'0')*10 + int(spec[1]-'0')
			}
		}
		epochs := make([]float64, len(items))
		for i, it := range items {
			epochs[i] = it.Epoch
		}
		bounds, err := TimeBounds(epochs, spec, 200)
		if err != nil {
			t.Fatal(err)
		}
		ps2, _ := check(t, items, cfgArgs{bucket: spec, by: "dir", prefixLen: 3,
			maxBuckets: 200, maxParts: 200, minCount: 1, dirLevel: "2"})
		if len(ps2) == 0 {
			t.Fatalf("%s 没有分区", spec)
		}
		if !sort.Float64sAreSorted(bounds) {
			t.Fatalf("%s 边界没排序: %v", spec, bounds)
		}
		for i := 1; i < len(bounds); i++ {
			if bounds[i] == bounds[i-1] {
				t.Fatalf("%s 边界有重复: %v", spec, bounds)
			}
		}
		if len(bounds) < 2 {
			t.Fatalf("%s 边界退化: %v", spec, bounds)
		}
		if prev >= 0 && len(bounds) > prev {
			t.Errorf("%s 的边界数没随分组变粗而减少", spec)
		}
		prev = len(bounds)
		for _, b := range bounds {
			lt := local(b)
			if (int(lt.Month())-1)%months != 0 || lt.Day() != 1 {
				t.Errorf("%s 边界 %v 没锚在分组首月", spec, lt)
			}
		}
	}
}

func onlyMonth(m map[int]bool) int {
	for k := range m {
		return k
	}
	return 0
}

func TestSortedWithinPartition(t *testing.T) {
	items := synth(400, 11)
	ps, ordered := check(t, items, cfgArgs{bucket: "monthly", by: "prefix", prefixLen: 3,
		maxBuckets: 12, maxParts: 60, minCount: 5, dirLevel: "2"})
	for _, p := range ps {
		var last float64 = math.Inf(-1)
		for _, pos := range p.Positions {
			e := ordered[pos].Epoch
			if e < last {
				t.Fatalf("分区 %d 内未按删除时间排序", p.PID)
			}
			last = e
		}
	}
}

func TestLabelsStayHonest(t *testing.T) {
	// 「全部」只能表示整桶未拆分；同一时间桶最多一个「其他」。
	items := synth(600, 21)
	epochs := make([]float64, len(items))
	for i, it := range items {
		epochs[i] = it.Epoch
	}
	bounds, err := TimeBounds(epochs, "daily", 400)
	if err != nil {
		t.Fatal(err)
	}
	ps, _ := check(t, items, cfgArgs{bucket: "daily", by: "dir", prefixLen: 3,
		maxBuckets: 400, maxParts: 100000, minCount: 5, dirLevel: "2"})
	perBucket := map[int][]*Part{}
	for _, p := range ps {
		perBucket[p.TimeBucket] = append(perBucket[p.TimeBucket], p)
	}
	for b, group := range perBucket {
		total := 0
		for _, it := range items {
			if BucketOf(it.Epoch, bounds) == b {
				total++
			}
		}
		others := 0
		for _, p := range group {
			if p.Label == All && p.Count != total {
				t.Errorf("桶 %d 标了全部却只覆盖 %d/%d 条", b, p.Count, total)
			}
			if p.Label == Others {
				others++
			}
		}
		if others > 1 {
			t.Errorf("桶 %d 里有 %d 个其他分区", b, others)
		}
	}
}

func TestCorruptItemsGrouped(t *testing.T) {
	broken := &meta.Item{Display: "ABC123", MetaVersion: -1, OrigPath: Illegal, Size: 0, Epoch: 0, Drive: "C"}
	if got := GroupKey(broken, "dir", 3, 2); got != Illegal {
		t.Errorf("dir 分组没把损坏条目归到 %q: %q", Illegal, got)
	}
	if got := GroupKey(broken, "prefix", 3, 2); got != Illegal {
		t.Errorf("prefix 分组没把损坏条目归到 %q: %q", Illegal, got)
	}
	if got := DirKey(broken, 2); got != Illegal {
		t.Errorf("DirKey 没把损坏条目归到 %q: %q", Illegal, got)
	}
	ok := &meta.Item{MetaVersion: 2, OrigPath: `C:\a\b\c.txt`, Display: "c.txt"}
	if got := GroupKey(ok, "dir", 3, 2); got != `C:\a\b` { // 段数不超过 level 时不截断
		t.Errorf("浅目录不该截断: %q", got)
	}
	deep := &meta.Item{MetaVersion: 2, OrigPath: `C:\Windows\Temp\Vs9POM\ssr\x.js`, Display: "x.js"}
	if got := GroupKey(deep, "dir", 3, 2); got != `C:\…\Vs9POM\ssr` {
		t.Errorf("深目录截断结果不符: %q", got)
	}
}

func TestParseSpecAcceptAndReject(t *testing.T) {
	// Python 版对 "0m"/"0mo" 会死循环/崩溃，Go 版明确报错——这条锁住差异。
	for _, bad := range []string{"0m", "0mo", "0h", "0d", "gap:0m", "gap:0", "gap:abc", "nonsense", "5x", "1y", "-3m", "1mm"} {
		if _, err := ParseSpec(bad); err == nil {
			t.Errorf("%q 应当报错但通过了", bad)
		}
	}
	for _, good := range []string{"", "auto", "gap", "gap:5m", "minutely", "hourly", "daily", "weekly",
		"monthly", "1m", "15m", "30m", "3d", "12h", "2mo", "12mo"} {
		if _, err := ParseSpec(good); err != nil {
			t.Errorf("%q 应当合法: %v", good, err)
		}
	}
}

func TestFmtRangeAndLabelText(t *testing.T) {
	items := []*meta.Item{
		{Display: "a.txt", MetaVersion: 2, Drive: "C", OrigPath: `C:\Users\x\Temp\a.txt`},
		{Display: "b.txt", MetaVersion: 2, Drive: "D", OrigPath: `C:\Users\x\Temp\sub\b.txt`},
	}
	if got := FmtRange(0, 0); got == "" {
		t.Error("FmtRange 返回了空串")
	}
	if got := LabelText(`C:\Users\x\Temp`, items, []int{0, 1}, "dir", 2); got == "" {
		t.Error("LabelText 返回了空串")
	}
}
