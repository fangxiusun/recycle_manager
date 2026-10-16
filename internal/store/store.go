// Package store 负责快照与产物的落盘格式：state.json（内存快照的持久化形态）、
// scan_list.csv（本次快照全量清单）、ledger.csv（每笔删除的追加账本）。
// 字段名与 Python 版完全一致，两边的快照可以互相打开。
package store

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"rbm/internal/meta"
	"rbm/internal/parts"
)

const (
	StateName  = "state.json"
	LedgerName = "ledger.csv"
	ListName   = "scan_list.csv"

	timeLayout = "2006-01-02 15:04:05"
)

// Params 是一次 scan 的分区参数，写进快照便于复现。
type Params struct {
	Bucket     string `json:"bucket"`
	By         string `json:"by"`
	PrefixLen  int    `json:"prefix_len"`
	DirLevel   string `json:"dir_level"`
	MaxBuckets int    `json:"max_buckets"`
	MaxParts   int    `json:"max_parts"`
	MinCount   int    `json:"min_count"`
	Drives     string `json:"drives"`
}

// State 是一份快照：条目按编号顺序重排后的结果，加上分区、时间边界与异常记录。
type State struct {
	Created    string        `json:"created"`
	Params     Params        `json:"params"`
	Bounds     []float64     `json:"bounds"`
	Items      []*meta.Item  `json:"items"`
	Parts      []*parts.Part `json:"parts"`
	Issues     []string      `json:"issues"`
	LastDelete string        `json:"last_delete,omitempty"`
}

// Path 返回某个快照目录下的 state.json 路径。
func Path(stateDir string) string { return filepath.Join(stateDir, StateName) }

// Read 读快照；文件不存在时返回可直接判断的 error（调用方提示「请先运行 scan」）。
func Read(stateDir string) (*State, error) {
	raw, err := os.ReadFile(Path(stateDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("没有找到快照 %s，请先运行 scan", Path(stateDir))
		}
		return nil, err
	}
	var st State
	if err := json.Unmarshal(raw, &st); err != nil {
		return nil, fmt.Errorf("快照 %s 解析失败: %w", Path(stateDir), err)
	}
	return &st, nil
}

// Write 原子替换快照文件，避免半截 state.json。
func Write(stateDir string, st *State) error {
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	tmp := Path(stateDir) + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, Path(stateDir))
}

// GroupCol 是统计表里那一列的标题，随分组维度变。
func (st *State) GroupCol() string {
	if st.Params.By == "dir" {
		return "路径组"
	}
	return "前缀组"
}

// AliveLeft 统计还没删完的条目数，供 shell 提示符与 GUI 状态栏用。
func (st *State) AliveLeft() int {
	left := 0
	for _, p := range st.Parts {
		left += p.Count - p.Deleted
	}
	return left
}

// WriteListCSV 把本次快照的全量清单写成带 BOM 的 CSV，Excel 直接打开不乱码。
func WriteListCSV(stateDir string, st *State) (string, error) {
	belong := map[int]*parts.Part{}
	for _, p := range st.Parts {
		for _, pos := range p.Positions {
			belong[pos] = p
		}
	}
	path := filepath.Join(stateDir, ListName)
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return "", err
	}
	w := csv.NewWriter(f)
	// Python 的 csv.writer 默认行尾是 \r\n，这里对齐才能逐字节比对清单。
	w.UseCRLF = true
	if err := w.Write([]string{"index", "编号", "时间范围", "路径组/前缀组", "删除时间", "字节", "盘", "文件名", "原路径", "$I路径", "$R数据存在"}); err != nil {
		return "", err
	}
	for pos, it := range st.Items {
		p := belong[pos]
		pid, timerange, label := "?", "?", "?"
		if p != nil {
			pid, timerange, label = strconv.Itoa(p.PID), p.TimeRange, p.Label
		}
		exists := "0"
		if it.Exists {
			exists = "1"
		}
		row := []string{
			strconv.Itoa(pos + 1), pid, timerange, label, it.DeletedAt,
			strconv.FormatInt(it.Size, 10), it.Drive, it.Display, it.OrigPath, it.IPath, exists,
		}
		if err := w.Write(row); err != nil {
			return "", err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return "", err
	}
	return path, nil
}

// Ledger 是删除账本：追加写，每笔一行，删了什么、结果如何都留痕。
type Ledger struct {
	Path string
	f    *os.File
	w    *csv.Writer
}

// OpenLedger 打开账本；文件是新建的时补表头。
func OpenLedger(stateDir string) (*Ledger, error) {
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(stateDir, LedgerName)
	fresh := true
	if _, err := os.Stat(path); err == nil {
		fresh = false
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	l := &Ledger{Path: path, f: f, w: csv.NewWriter(f)}
	l.w.UseCRLF = true
	if fresh {
		if err := l.w.Write([]string{"操作时间", "编号", "index", "删除时间", "字节", "盘", "文件名", "原路径", "结果"}); err != nil {
			f.Close()
			return nil, err
		}
	}
	return l, nil
}

// Row 记一笔删除结果，index 用全局 1 基序号。
func (l *Ledger) Row(pid, index int, item *meta.Item, note string) error {
	return l.write([]string{
		time.Now().In(meta.LocalTZ).Format(timeLayout),
		strconv.Itoa(pid),
		strconv.Itoa(index),
		item.DeletedAt,
		strconv.FormatInt(item.Size, 10),
		item.Drive,
		item.Display,
		item.OrigPath,
		note,
	})
}

func (l *Ledger) write(row []string) error {
	l.w.Write(row)
	l.w.Flush()
	return l.w.Error()
}

// Close 收尾：把缓冲刷干净再关文件。
func (l *Ledger) Close() error {
	l.w.Flush()
	err := l.w.Error()
	if cerr := l.f.Close(); err == nil {
		err = cerr
	}
	return err
}

// DefaultStateDir 与 Python 版一致：当前工作目录下的 .rbm。
func DefaultStateDir() string {
	cwd, err := os.Getwd()
	if err != nil || cwd == "" {
		cwd = filepath.Dir(os.Args[0])
	}
	return filepath.Join(cwd, ".rbm")
}

// SplitDrives 把 "c,G," 这类写法清成 ["C","G"]；空串返回 nil。
func SplitDrives(spec string) []string {
	var out []string
	for _, d := range strings.Split(spec, ",") {
		d = strings.TrimSpace(strings.ToUpper(strings.TrimRight(d, ":")))
		if d != "" {
			out = append(out, d)
		}
	}
	return out
}
