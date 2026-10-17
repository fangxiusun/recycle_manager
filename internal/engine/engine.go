// Package engine 是 CLI 与 GUI 共用的内核：扫描建快照、按编号选分区、
// 三层闸门之下的真删除与账本。界面只做展示，判定逻辑都收在这里。
package engine

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"rbm/internal/i18n"
	"rbm/internal/meta"
	"rbm/internal/parts"
	"rbm/internal/store"
	"rbm/internal/winfs"
)

// Options 是一次 scan 的全部分区参数（默认值由调用方给，同 Python 的 argparse 默认）。
type Options struct {
	Bucket     string
	By         string
	DirLevel   string
	PrefixLen  int
	MaxBuckets int
	MaxParts   int
	MinCount   int
	Drives     []string
	DrivesSpec string
	MaxScan    int
	StateDir   string
}

// Snapshot 是一份可用的快照 + 截断说明。
type Snapshot struct {
	State    *store.State
	Overflow *meta.Overflow
}

// Scan 扫描回收站并分区。条目为 0 不是错误：回收站本来就是空的，照样落一份空快照，
// 由调用方按「扫描完成，0 个」报出去。只有指定的盘根本没有当前用户的回收站目录才算用法错。
func Scan(o Options) (*Snapshot, error) {
	dirs, err := meta.BinDirs(o.Drives)
	if err != nil {
		return nil, err
	}
	if len(o.Drives) > 0 && len(dirs) == 0 {
		return nil, errors.New(i18n.Tf("engine.no-bin", o.DrivesSpec))
	}
	items, issues, overflow := meta.Scan(dirs, o.MaxScan)
	st, err := BuildState(items, issues, o)
	if err != nil {
		return nil, err
	}
	return &Snapshot{State: st, Overflow: overflow}, nil
}

// BuildState 把条目分区成快照。items 为空是合法输入：得到一份 Items/Parts 都是空表、
// 参数照常记录的快照，调用方据此报「扫描完成，0 个」。
func BuildState(items []*meta.Item, issues []string, o Options) (*store.State, error) {
	bucket := o.Bucket
	if bucket == "" {
		bucket = fmt.Sprintf("gap:自动断点(<= %d 桶)", o.MaxBuckets)
	}
	drives := o.DrivesSpec
	if drives == "" {
		drives = "all"
	}
	bounds, ps, ordered, err := parts.BuildPartitions(items, o.Bucket, o.MaxBuckets, o.By,
		o.PrefixLen, o.MaxParts, o.MinCount, o.DirLevel)
	if err != nil {
		return nil, err
	}
	if issues == nil {
		// Python 侧写的是 json.dumps([])，空列表也得留成 "issues":[]，不能是 null
		issues = []string{}
	}
	return &store.State{
		Created: time.Now().In(meta.LocalTZ).Format("2006-01-02 15:04:05"),
		Params: store.Params{
			Bucket:     bucket,
			By:         o.By,
			PrefixLen:  o.PrefixLen,
			DirLevel:   o.DirLevel,
			MaxBuckets: o.MaxBuckets,
			MaxParts:   o.MaxParts,
			MinCount:   o.MinCount,
			Drives:     drives,
		},
		Bounds: bounds,
		Items:  ordered,
		Parts:  ps,
		Issues: issues,
	}, nil
}

// SortedParts 按 sort/alive 组织分区行，口径同 Python 的 print_parts。
func SortedParts(st *store.State, sortKey string, alive bool) []*parts.Part {
	chosen := make([]*parts.Part, 0, len(st.Parts))
	for _, p := range st.Parts {
		if alive && p.Deleted >= p.Count {
			continue
		}
		chosen = append(chosen, p)
	}
	switch sortKey {
	case "size":
		sort.SliceStable(chosen, func(i, j int) bool {
			if chosen[i].Size != chosen[j].Size {
				return chosen[i].Size > chosen[j].Size
			}
			return chosen[i].PID < chosen[j].PID
		})
	case "count":
		sort.SliceStable(chosen, func(i, j int) bool {
			if chosen[i].Count != chosen[j].Count {
				return chosen[i].Count > chosen[j].Count
			}
			return chosen[i].PID < chosen[j].PID
		})
	}
	return chosen
}

// MatchPrefix 判断文件名是否以某前缀开头（忽略大小写）。
func MatchPrefix(item *meta.Item, needle string) bool {
	return strings.HasPrefix(strings.ToLower(item.Display), strings.ToLower(needle))
}

// SelectedParts 按编号取分区，可选用 prefix 进一步收窄到匹配的文件名前缀。
func SelectedParts(st *store.State, ids []int, prefix string) []*parts.Part {
	chosen := []*parts.Part{}
	for _, pid := range ids {
		src := st.Parts[pid-1]
		positions := append([]int(nil), src.Positions...)
		label := src.Label
		if prefix != "" {
			filtered := make([]int, 0, len(positions))
			for _, p := range positions {
				if MatchPrefix(st.Items[p], prefix) {
					filtered = append(filtered, p)
				}
			}
			positions = filtered
			label = fmt.Sprintf("%s/%s*", src.Label, prefix)
		}
		if len(positions) == 0 {
			continue
		}
		var size int64
		for _, p := range positions {
			size += st.Items[p].Size
		}
		cp := *src
		cp.Label = label
		cp.Count = len(positions)
		cp.Size = size
		cp.Positions = positions
		chosen = append(chosen, &cp)
	}
	return chosen
}

// DeleteOne 永久删除一个回收站条目（$R 数据 + $I 元数据），返回 (是否干净删完, 说明)。
//
// 返回的说明会原样写进 ledger.csv 的「结果」列（「已删」或错误摘要），属于落盘口径，
// 所以这里的中文串不进 i18n 目录；界面要表达「删除失败」有自己的日志文案。
func DeleteOne(item *meta.Item) (bool, string) {
	var errors []string
	if winfs.Exists(item.RPath) {
		if winfs.IsDir(item.RPath) {
			skipped, err := winfs.RemoveTree(item.RPath)
			if skipped != "" {
				errors = append(errors, "子项删不掉: "+skipped)
			}
			if err != nil {
				errors = append(errors, "rmtree "+err.Error())
			}
		} else if err := winfs.RemoveFile(item.RPath); err != nil {
			errors = append(errors, "$R "+err.Error())
		}
	}
	if err := winfs.RemoveFile(item.IPath); err != nil {
		errors = append(errors, "$I "+err.Error())
	}
	if len(errors) > 0 {
		return false, strings.Join(errors, "; ")
	}
	return true, "已删"
}

// Progress 回调：n 是本次已处理条数。
type Progress func(n, total, done, failed int)

// Report 是一次批量删除的结果。
type Report struct {
	Done   int
	Failed int
	Errors []string
}

// RunDelete 执行真删除：逐条删、逐条记账、删完更新快照的 deleted 标记（编号不变）。
// max>0 是保险丝：选中条目数超过它就整批不动。
func RunDelete(st *store.State, chosen []*parts.Part, stateDir string, max, every int,
	verbose bool, progress Progress) (*Report, error) {
	type row struct {
		pid  int
		pos  int
		item *meta.Item
	}
	var collected []row
	for _, p := range chosen {
		for _, pos := range p.Positions {
			collected = append(collected, row{p.PID, pos, st.Items[pos]})
		}
	}
	if max > 0 && len(collected) > max {
		return nil, errors.New(i18n.Tf("engine.fuse", len(collected), max))
	}
	if every < 1 {
		every = 1
	}
	ledger, err := store.OpenLedger(stateDir)
	if err != nil {
		return nil, err
	}
	rep := &Report{}
	okPositions := map[int]bool{}
	for n, r := range collected {
		ok, note := DeleteOne(r.item)
		if ok {
			rep.Done++
			okPositions[r.pos] = true
		} else {
			rep.Failed++
			rep.Errors = append(rep.Errors, fmt.Sprintf("[%d] %s: %s", r.pid, r.item.Display, note))
		}
		if err := ledger.Row(r.pid, r.pos+1, r.item, note); err != nil {
			ledger.Close()
			return rep, err
		}
		if progress != nil && (verbose || (n+1)%every == 0 || n+1 == len(collected)) {
			progress(n+1, len(collected), rep.Done, rep.Failed)
		}
	}
	ledger.Close()

	touched := map[int]bool{}
	for _, p := range chosen {
		touched[p.PID] = true
	}
	for _, p := range st.Parts {
		if touched[p.PID] {
			hit := 0
			for _, pos := range p.Positions {
				if okPositions[pos] {
					hit++
				}
			}
			p.Deleted += hit
		}
	}
	st.LastDelete = time.Now().In(meta.LocalTZ).Format("2006-01-02 15:04:05")
	if err := store.Write(stateDir, st); err != nil {
		return rep, err
	}
	return rep, nil
}

// TotalSize 汇总若干分区的字节数，删除前的提示用。
func TotalSize(ps []*parts.Part) int64 {
	var n int64
	for _, p := range ps {
		n += p.Size
	}
	return n
}
