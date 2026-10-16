// Package parts 移植 rb_parts.py 的两级分区：先按删除时间切桶，桶内再按
// 「原路径所在目录」（或前缀/扩展名/首段）分组。
//
// 移植时保持了三处会影响结果的 Python 语义：
//   - dict 的插入顺序：分组顺序参与 _cap 的并列取值，所以这里用 keys 切片 + map 复刻；
//   - sort 稳定性：并列项按原顺序保留，排序键统一用 (epoch, display)；
//   - min/max 取第一个极值，而不是任意一个。
package parts

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"rbm/internal/meta"
)

// 分组占位标签，和 Python 版字面一致（state.json 也直接写这些串）。
const (
	Others  = "其他"
	All     = "全部"
	Illegal = "元数据损坏"
	Special = "特殊位置"
)

const (
	second = 1
	minute = 60
	hour   = 3600
	day    = 86400

	// fullLevel 表示「不截断目录」，等价于 Python 的 10**6。
	fullLevel = 1000000
)

// 时间桶规格
type Spec struct {
	Name string // 用户写法，如 daily / 30m / 3mo / gap
	Kind string // gap | step | month | auto
	Arg  int
}

// 等宽时间桶：由细到粗，auto 依次尝试直到桶数不超上限
var candidates = []Spec{
	{Name: "1m", Kind: "step", Arg: minute},
	{Name: "1h", Kind: "step", Arg: hour},
	{Name: "1d", Kind: "step", Arg: day},
	{Name: "3d", Kind: "step", Arg: 3 * day},
	{Name: "7d", Kind: "step", Arg: 7 * day},
	{Name: "14d", Kind: "step", Arg: 14 * day},
	{Name: "30d", Kind: "step", Arg: 30 * day},
	{Name: "90d", Kind: "step", Arg: 90 * day},
	{Name: "180d", Kind: "step", Arg: 180 * day},
}

var aliases = map[string]Spec{
	"minutely": {Name: "1m", Kind: "step", Arg: minute},
	"hourly":   {Name: "1h", Kind: "step", Arg: hour},
	"daily":    {Name: "1d", Kind: "step", Arg: day},
	"weekly":   {Name: "7d", Kind: "step", Arg: 7 * day},
	"monthly":  {Name: "1mo", Kind: "month", Arg: 1},
	"auto":     {Name: "auto", Kind: "auto", Arg: 0},
	"gap":      {Name: "gap", Kind: "gap", Arg: 0},
}

// ParseSpec 归一化用户写法。bucket 为空时按隐式 gap 处理。
func ParseSpec(bucket string) (Spec, error) {
	if bucket == "" {
		return Spec{Name: "gap", Kind: "gap"}, nil
	}
	key := strings.ToLower(strings.TrimSpace(bucket))
	if s, ok := aliases[key]; ok {
		return s, nil
	}
	if strings.HasPrefix(key, "gap") {
		_, tail, _ := strings.Cut(key, ":")
		arg := 0
		if tail != "" {
			n, err := parseStep(tail)
			if err != nil {
				return Spec{}, err
			}
			arg = n
		}
		return Spec{Name: key, Kind: "gap", Arg: arg}, nil
	}
	if strings.HasSuffix(key, "mo") {
		n, err := strconv.Atoi(strings.TrimSuffix(key, "mo"))
		if err != nil || n <= 0 {
			return Spec{}, fmt.Errorf("无法识别的时间桶: %s（可用 minutely/hourly/daily/weekly/monthly/Nm/Nh/Nd/Nmo/auto/gap[:30m]）", bucket)
		}
		return Spec{Name: key, Kind: "month", Arg: n}, nil
	}
	if s, err := parseStep(key); err == nil {
		return Spec{Name: key, Kind: "step", Arg: s}, nil
	}
	return Spec{}, fmt.Errorf("无法识别的时间桶: %s（可用 minutely/hourly/daily/weekly/monthly/Nm/Nh/Nd/Nmo/auto/gap[:30m]）", bucket)
}

// parseStep 把 30m/12h/3d/90s 换成秒；粒度必须为正，否则边界会原地打转。
func parseStep(text string) (int, error) {
	if text == "" {
		return 0, fmt.Errorf("粒度不能为空，例如 gap:5m")
	}
	var unit int
	switch text[len(text)-1] {
	case 's':
		unit = second
	case 'm':
		unit = minute
	case 'h':
		unit = hour
	case 'd':
		unit = day
	default:
		return 0, fmt.Errorf("无法识别的粒度单位 %q（只能用 s/m/h/d）", text[len(text)-1:])
	}
	n, err := strconv.Atoi(text[:len(text)-1])
	if err != nil {
		return 0, fmt.Errorf("粒度的数字无法识别: %s", text)
	}
	if n <= 0 {
		return 0, fmt.Errorf("粒度必须大于 0: %s", text)
	}
	return n * unit, nil
}

func local(epoch float64) time.Time {
	return time.UnixMicro(int64(math.Round(epoch * 1e6))).In(meta.LocalTZ)
}

func floorDay(epoch float64) float64 {
	t := local(epoch)
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, meta.LocalTZ)
	return float64(d.Unix())
}

// floorMonth 锚到「组内第一个月」的月首：3mo 恒落在 1/4/7/10 月。
func floorMonth(epoch float64, months int) float64 {
	t := local(epoch)
	index := t.Year()*12 + (int(t.Month())-1)/months*months
	year, month := divMod(index, 12)
	d := time.Date(year, time.Month(month+1), 1, 0, 0, 0, 0, meta.LocalTZ)
	return float64(d.Unix())
}

// nextMonth 从月首整月推进，用于把日历月边界一路铺满。
func nextMonth(epoch float64, months int) float64 {
	t := local(epoch)
	year, month := divMod(t.Year()*12+int(t.Month())-1+months, 12)
	d := time.Date(year, time.Month(month+1), 1, 0, 0, 0, 0, meta.LocalTZ)
	return float64(d.Unix())
}

func divMod(a, b int) (int, int) { return a / b, a % b }

// GapBounds 按「相邻删除时间的断点」切桶：断点越大越优先，落点自然是爆发段边界。
func GapBounds(epochs []float64, minGap, maxBuckets int) []float64 {
	if len(epochs) == 0 {
		return nil
	}
	ordered := append([]float64(nil), epochs...)
	sort.Float64s(ordered)
	type gapPair struct {
		delta float64
		epoch float64
	}
	var gaps []gapPair
	threshold := minGap
	if threshold == 0 {
		threshold = minute
	}
	for i := 1; i < len(ordered); i++ {
		delta := ordered[i] - ordered[i-1]
		if delta >= float64(threshold) {
			gaps = append(gaps, gapPair{delta, ordered[i]})
		}
	}
	if minGap == 0 {
		// (delta, epoch) 整体降序，取前 max_buckets-1 个
		sort.SliceStable(gaps, func(i, j int) bool {
			if gaps[i].delta != gaps[j].delta {
				return gaps[i].delta > gaps[j].delta
			}
			return gaps[i].epoch > gaps[j].epoch
		})
		keep := max(0, maxBuckets-1)
		if len(gaps) > keep {
			gaps = gaps[:keep]
		}
	}
	seen := map[float64]bool{}
	var cuts []float64
	for _, g := range gaps {
		c := math.Floor(g.epoch/minute) * minute
		if !seen[c] {
			seen[c] = true
			cuts = append(cuts, c)
		}
	}
	sort.Float64s(cuts)
	first := math.Floor(ordered[0]/minute) * minute
	bounds := []float64{first}
	for _, c := range cuts {
		if c > first {
			bounds = append(bounds, c)
		}
	}
	return append(bounds, ordered[len(ordered)-1]+1)
}

// AutoStepBounds 挑「桶数不超过 maxBuckets 的最细等宽粒度」。
func AutoStepBounds(epochs []float64, maxBuckets int) []float64 {
	low, high := minEpoch(epochs), maxEpoch(epochs)
	for _, c := range candidates {
		start := floorDay(low)
		for start+float64(c.Arg) <= low {
			start += float64(c.Arg)
		}
		if math.Floor((high-start)/float64(c.Arg))+1 <= float64(maxBuckets) {
			return stepBounds(start, high, float64(c.Arg))
		}
	}
	step := math.Floor((high-low)/math.Max(1, float64(maxBuckets-1))) + 1
	bounds := []float64{low - 1}
	for bounds[len(bounds)-1] < high {
		bounds = append(bounds, bounds[len(bounds)-1]+step)
	}
	return bounds
}

func stepBounds(start, high, step float64) []float64 {
	bounds := []float64{start}
	for bounds[len(bounds)-1] <= high {
		bounds = append(bounds, bounds[len(bounds)-1]+step)
	}
	return bounds
}

// TimeBounds 返回时间桶边界（升序，覆盖全部条目）。
func TimeBounds(epochs []float64, bucket string, maxBuckets int) ([]float64, error) {
	if len(epochs) == 0 {
		return nil, nil
	}
	spec, err := ParseSpec(bucket)
	if err != nil {
		return nil, err
	}
	low, high := minEpoch(epochs), maxEpoch(epochs)
	switch {
	case spec.Kind == "gap":
		return GapBounds(epochs, spec.Arg, maxBuckets), nil
	case spec.Name == "auto":
		return AutoStepBounds(epochs, maxBuckets), nil
	case spec.Kind == "month":
		bounds := []float64{floorMonth(low, spec.Arg)}
		for bounds[len(bounds)-1] <= high {
			bounds = append(bounds, nextMonth(bounds[len(bounds)-1], spec.Arg))
		}
		return bounds, nil
	}
	step := float64(spec.Arg)
	start := floorDay(low)
	for start+step <= low {
		start += step
	}
	return stepBounds(start, high, step), nil
}

// BucketOf 二分定位 epoch 落在哪个桶。
func BucketOf(epoch float64, bounds []float64) int {
	lo, hi := 0, len(bounds)-1
	for lo < hi {
		mid := (lo + hi) / 2
		if epoch < bounds[mid+1] {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return lo
}

// IsCorrupt 判断 $I 解析失败的条目：meta 用 -1 标记，display 仍是 $I 后缀名。
func IsCorrupt(item *meta.Item) bool { return item.MetaVersion < 0 }

// ParentDir 返回条目原来所在的目录（带盘符）。
func ParentDir(item *meta.Item) string {
	path := strings.TrimRight(item.OrigPath, `\/`)
	if !strings.Contains(path, "\\") {
		return ""
	}
	i := strings.LastIndex(path, "\\")
	return path[:i]
}

// NormalizeLevel 把 --dir-level 写法换成段数；full/all/0/不认识的写法都等于不截断。
func NormalizeLevel(level string) int {
	s := strings.TrimSpace(level)
	if strings.ToLower(s) == "full" || strings.ToLower(s) == "all" || s == "0" {
		return fullLevel
	}
	if n, err := strconv.Atoi(s); err == nil {
		if n < 1 {
			return 1
		}
		return n
	}
	return fullLevel
}

// DirKey 按目录末尾 N 段分组，保留盘符；截断处用 … 标记。
func DirKey(item *meta.Item, level int) string {
	if IsCorrupt(item) {
		return Illegal
	}
	if item.Unrecoverable {
		return Special
	}
	parent := ParentDir(item)
	if parent == "" {
		return "(根)"
	}
	drive, rest, _ := strings.Cut(parent, "\\")
	var segs []string
	for _, s := range strings.Split(rest, "\\") {
		if s != "" {
			segs = append(segs, s)
		}
	}
	if len(segs) == 0 {
		return drive + "\\"
	}
	truncated := level < len(segs)
	shown := segs
	if truncated {
		shown = segs[len(segs)-level:]
	}
	joined := strings.Join(shown, "\\")
	if truncated {
		return drive + "\\…\\" + joined
	}
	return drive + "\\" + joined
}

// GroupKey 是桶内分组的键。
func GroupKey(item *meta.Item, by string, prefixLen, dirLevel int) string {
	if IsCorrupt(item) {
		return Illegal
	}
	if by == "dir" {
		return DirKey(item, dirLevel)
	}
	display := strings.ToLower(item.Display)
	if item.Unrecoverable {
		return Special
	}
	if display == "" {
		return Others
	}
	switch by {
	case "ext":
		i := strings.LastIndex(display, ".")
		if i <= 0 {
			return "(无扩展名)"
		}
		return display[i:]
	case "token":
		name := display
		for _, sep := range []string{"-", "_", ".", " ", "\t", "@", "#"} {
			name = strings.ReplaceAll(name, sep, "\x00")
		}
		head := strings.Split(name, "\x00")[0]
		if head == "" {
			return Others
		}
		return clampRunes(head, max(prefixLen, 12))
	}
	return clampRunes(display, max(prefixLen, 1))
}

// clampRunes 按字符数截断，等价于 Python 的 s[:n]。
func clampRunes(s string, n int) string {
	count := 0
	for i := range s {
		if count == n {
			return s[:i]
		}
		count++
	}
	return s
}

// Fmt 按 Python 的 strftime 样式格式化 epoch。
func Fmt(epoch float64, style string) string {
	t := local(epoch)
	switch style {
	case "%Y-%m-%d %H:%M:%S":
		return t.Format("2006-01-02 15:04:05")
	case "%Y-%m-%d %H:%M":
		return t.Format("2006-01-02 15:04")
	case "%H:%M":
		return t.Format("15:04")
	case "%m-%d %H:%M":
		return t.Format("01-02 15:04")
	default:
		return t.Format("2006-01-02")
	}
}

// FmtRange 渲染分区时间范围，分钟级也看得清。
func FmtRange(start, end float64) string {
	if end-start < 90 {
		return Fmt(start, "%Y-%m-%d %H:%M:%S")
	}
	left, right := local(start), local(end)
	sameDay := left.Year() == right.Year() && left.YearDay() == right.YearDay()
	if sameDay {
		return Fmt(start, "%Y-%m-%d %H:%M") + "~" + Fmt(end, "%H:%M")
	}
	if end-start < 7*day {
		return Fmt(start, "%m-%d %H:%M") + "~" + Fmt(end, "%m-%d %H:%M")
	}
	return Fmt(start, "%Y-%m-%d") + "~" + Fmt(end, "%Y-%m-%d")
}

// LabelText 生成分组列的展示文本；「其他/全部/元数据损坏/特殊位置」列出内部构成 top4。
func LabelText(label string, items []*meta.Item, positions []int, by string, dirLevel int) string {
	if label != All && label != Others && label != Illegal && label != Special {
		return label
	}
	order := []string{}
	counts := map[string]int{}
	for _, p := range positions {
		var head string
		if by == "dir" {
			key := DirKey(items[p], dirLevel)
			i := strings.LastIndex(key, "\\")
			if i >= 0 {
				head = key[i+1:]
			} else {
				head = key
			}
		} else {
			head = strings.ToLower(clampRunes(items[p].Display, 6))
		}
		if _, ok := counts[head]; !ok {
			order = append(order, head)
		}
		counts[head]++
	}
	top := append([]string(nil), order...)
	sort.SliceStable(top, func(i, j int) bool {
		if counts[top[i]] != counts[top[j]] {
			return counts[top[i]] > counts[top[j]]
		}
		return top[i] < top[j]
	})
	if len(top) > 4 {
		top = top[:4]
	}
	partsOut := make([]string, len(top))
	for i, k := range top {
		partsOut[i] = fmt.Sprintf("%s(%d)", k, counts[k])
	}
	return strings.Join(partsOut, ", ")
}

// Part 是一个分区：一段时间 + 一组路径/文件名相近的条目。
type Part struct {
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

func makePart(b int, key string, positions []int, items []*meta.Item, bounds []float64,
	by string, prefixLen int, dirLevel string, singleGroup bool) *Part {
	pos := sortPositions(positions, items)
	start := bounds[b]
	end := bounds[len(bounds)-1] + minute
	if b+1 < len(bounds) {
		end = bounds[b+1]
	}
	label, groupBy := key, by
	if singleGroup {
		label, groupBy = All, "all"
	}
	var size int64
	for _, p := range pos {
		size += items[p].Size
	}
	return &Part{
		TimeBucket: b,
		RangeStart: start,
		RangeEnd:   end - 1,
		Label:      label,
		Count:      len(pos),
		Size:       size,
		Positions:  pos,
		By:         groupBy,
		PrefixLen:  prefixLen,
		DirLevel:   dirLevel,
	}
}

func sortPositions(positions []int, items []*meta.Item) []int {
	out := append([]int(nil), positions...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := items[out[i]], items[out[j]]
		if a.Epoch != b.Epoch {
			return a.Epoch < b.Epoch
		}
		return a.Display < b.Display
	})
	return out
}

// groups 是「保插入顺序的 map」，Python 的 dict 就是这个语义。
type groups struct {
	keys []string
	m    map[string][]int
}

func newGroups() *groups { return &groups{m: map[string][]int{}} }

func (g *groups) add(key string, pos int) {
	if _, ok := g.m[key]; !ok {
		g.keys = append(g.keys, key)
	}
	g.m[key] = append(g.m[key], pos)
}

// BuildPartitions 切分并编号，返回 (时间桶边界, 分区列表, 按编号顺序重排后的条目)。
// 重排让每条的全局 index（位置 + 1）正好落在所属分区声明的 index区间 内。
func BuildPartitions(items []*meta.Item, bucket string, maxBuckets int, by string,
	prefixLen, maxParts, minCount int, dirLevel string) ([]float64, []*Part, []*meta.Item, error) {
	epochs := make([]float64, len(items))
	for i, it := range items {
		epochs[i] = it.Epoch
	}
	bounds, err := TimeBounds(epochs, bucket, maxBuckets)
	if err != nil {
		return nil, nil, nil, err
	}
	level := NormalizeLevel(dirLevel)

	bucketOrder := []int{}
	buckets := map[int]*groups{}
	for pos, it := range items {
		b := BucketOf(it.Epoch, bounds)
		g, ok := buckets[b]
		if !ok {
			g = newGroups()
			buckets[b] = g
			bucketOrder = append(bucketOrder, b)
		}
		g.add(GroupKey(it, by, prefixLen, level), pos)
	}
	sort.Ints(bucketOrder)

	out := []*Part{}
	for _, b := range bucketOrder {
		g := buckets[b]
		keys := append([]string(nil), g.keys...)
		byKey := map[string][]int{}
		for _, k := range keys {
			byKey[k] = append([]int(nil), g.m[k]...)
		}
		mergedAny := false
		if len(keys) > 1 {
			var small []string
			for _, k := range keys {
				if len(byKey[k]) < minCount && k != Others {
					small = append(small, k)
				}
			}
			if len(small) > 0 {
				merged := append([]int(nil), byKey[Others]...)
				sort.Strings(small)
				for _, k := range small {
					merged = append(merged, byKey[k]...)
					delete(byKey, k)
				}
				kept := make([]string, 0, len(keys))
				for _, k := range keys {
					// 被并入「其他」的小组已从 byKey 删掉，这里同步从顺序里剔除
					if _, ok := byKey[k]; ok && k != Others {
						kept = append(kept, k)
					}
				}
				keys = append(kept, Others)
				byKey[Others] = merged
				mergedAny = true
			}
		}
		single := len(keys) == 1 && !mergedAny
		for _, key := range keys {
			// 「元数据损坏」是重要提示，不能因为整桶只有一组就被含糊成「全部」
			out = append(out, makePart(b, key, byKey[key], items, bounds, by, prefixLen, dirLevel,
				single && key != Illegal))
		}
	}

	out = capParts(out, items, maxParts)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.RangeStart != b.RangeStart {
			return a.RangeStart < b.RangeStart
		}
		if a.RangeEnd != b.RangeEnd {
			return a.RangeEnd < b.RangeEnd
		}
		if (a.Label != All) != (b.Label != All) {
			return a.Label == All
		}
		return a.Label < b.Label
	})

	for i, part := range out {
		positions := sortPositions(part.Positions, items)
		part.Positions = positions
		part.PID = i + 1
		part.IndexStart = 1
		if i > 0 {
			part.IndexStart = out[i-1].IndexEnd + 1
		}
		part.IndexEnd = part.IndexStart + len(positions) - 1
		part.Count = len(positions)
		var size int64
		minE, maxE := items[positions[0]].Epoch, items[positions[0]].Epoch
		for _, p := range positions {
			size += items[p].Size
			if items[p].Epoch < minE {
				minE = items[p].Epoch
			}
			if items[p].Epoch > maxE {
				maxE = items[p].Epoch
			}
		}
		part.Size = size
		if minE < part.RangeStart {
			part.RangeStart = minE
		}
		if maxE > part.RangeEnd {
			part.RangeEnd = maxE
		}
		part.TimeRange = FmtRange(part.RangeStart, part.RangeEnd)
		driveSet := map[string]bool{}
		for _, p := range positions {
			driveSet[items[p].Drive] = true
		}
		drives := make([]string, 0, len(driveSet))
		for d := range driveSet {
			drives = append(drives, d)
		}
		sort.Strings(drives)
		part.Drives = strings.Join(drives, "")
		part.TopLabels = LabelText(part.Label, items, positions, part.By, level)
	}

	ordered := make([]*meta.Item, 0, len(items))
	for _, part := range out {
		old := part.Positions
		start := len(ordered)
		fresh := make([]int, len(old))
		for i, p := range old {
			fresh[i] = start + i
			ordered = append(ordered, items[p])
		}
		part.Positions = fresh
	}
	return bounds, out, ordered, nil
}

// capParts 分区超上限时反复把最小分区并入邻近分区。
func capParts(parts []*Part, items []*meta.Item, maxParts int) []*Part {
	for len(parts) > maxParts {
		smallest := parts[0]
		for _, p := range parts {
			if lessPart(p, smallest) {
				smallest = p
			}
		}
		var sameBucket []*Part
		for _, p := range parts {
			if p.TimeBucket == smallest.TimeBucket && p != smallest {
				sameBucket = append(sameBucket, p)
			}
		}
		var host *Part
		if len(sameBucket) > 0 {
			for _, p := range sameBucket {
				if p.Label == Others {
					host = p
					break
				}
			}
			if host == nil {
				for _, p := range sameBucket {
					if host == nil || p.Count > host.Count {
						host = p
					}
				}
			}
		} else {
			for _, p := range parts {
				if p == smallest {
					continue
				}
				if host == nil || math.Abs(p.RangeStart-smallest.RangeStart) < math.Abs(host.RangeStart-smallest.RangeStart) {
					host = p
				}
			}
		}
		if host == nil {
			break
		}
		host.Positions = append(host.Positions, smallest.Positions...)
		if smallest.RangeStart < host.RangeStart {
			host.RangeStart = smallest.RangeStart
		}
		if smallest.RangeEnd > host.RangeEnd {
			host.RangeEnd = smallest.RangeEnd
		}
		if smallest.TimeBucket < host.TimeBucket {
			host.TimeBucket = smallest.TimeBucket
		}
		if host.Label != smallest.Label {
			host.Label = Others
		}
		host.Count = len(host.Positions)
		var size int64
		for _, p := range host.Positions {
			size += items[p].Size
		}
		host.Size = size
		kept := make([]*Part, 0, len(parts)-1)
		for _, p := range parts {
			if p != smallest {
				kept = append(kept, p)
			}
		}
		parts = kept
	}
	return parts
}

// lessPart 对应 min(parts, key=(count, range_start, label))：并列时取先出现的。
func lessPart(a, b *Part) bool {
	if a.Count != b.Count {
		return a.Count < b.Count
	}
	if a.RangeStart != b.RangeStart {
		return a.RangeStart < b.RangeStart
	}
	return a.Label < b.Label
}

func minEpoch(epochs []float64) float64 {
	m := epochs[0]
	for _, e := range epochs {
		if e < m {
			m = e
		}
	}
	return m
}

func maxEpoch(epochs []float64) float64 {
	m := epochs[0]
	for _, e := range epochs {
		if e > m {
			m = e
		}
	}
	return m
}
