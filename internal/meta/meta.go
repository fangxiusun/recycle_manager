// Package meta 直接解析各盘 $Recycle.Bin\<当前用户 SID> 下的 $I 元数据文件，
// 不经 Shell COM。语义与 Python 版 rb_meta.py 一致：v2 头部路径长度是 u32@24、
// 路径从 28 起，解析失败或长度对不上都归为「元数据损坏」条目而不是丢掉。
package meta

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows"

	"rbm/internal/i18n"
	"rbm/internal/winfs"
)

// Item 是一条回收站条目：$I 元数据 + 落盘路径 + $R 是否还在。
type Item struct {
	MetaVersion   int64   `json:"meta_version"`
	Size          int64   `json:"size"`
	DeletedAt     string  `json:"deleted_at"`
	Epoch         float64 `json:"epoch"`
	OrigPath      string  `json:"orig_path"`
	Display       string  `json:"display"`
	Unrecoverable bool    `json:"unrecoverable"`
	Drive         string  `json:"drive"`
	IPath         string  `json:"i_path"`
	RPath         string  `json:"r_path"`
	Exists        bool    `json:"exists"`
}

// BinDir 是某个盘上属于当前用户的回收站目录。
type BinDir struct {
	Drive string
	Path  string
}

// Overflow 记录 --max-scan 截断的分布，调用方必须把它打印出来，不能静默丢弃。
type Overflow struct {
	Total   int
	Taken   int
	ByDrive map[string][2]int // 盘 -> {本次已扫, 该盘总数}
}

// LocalTZ 取当前系统时区的固定偏移，桶边界与时间字符串都按它算（同 Python 版 LOCAL_TZ）。
var LocalTZ = func() *time.Location {
	now := time.Now()
	name, offset := now.Zone()
	return time.FixedZone(name, offset)
}()

// CurrentSID 从进程令牌取当前用户 SID。
func CurrentSID() (string, error) {
	token := windows.GetCurrentProcessToken()
	tu, err := token.GetTokenUser()
	if err != nil {
		return "", errors.New(i18n.Tf("meta.sid.token", err))
	}
	sid := tu.User.Sid.String()
	if sid == "" {
		return "", errors.New(i18n.T("meta.sid.string"))
	}
	return sid, nil
}

// LocalDrives 返回本机存在的盘符（"C"、"D"…，不带冒号）。
func LocalDrives() []string {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil
	}
	var out []string
	for i := 0; i < 26; i++ {
		if mask>>i&1 != 0 {
			out = append(out, string(rune('A'+i)))
		}
	}
	return out
}

// BinDirs 返回各盘当前用户的回收站目录；目录不存在的盘自动跳过。
func BinDirs(drives []string) ([]BinDir, error) {
	sid, err := CurrentSID()
	if err != nil {
		return nil, err
	}
	if len(drives) == 0 {
		drives = LocalDrives()
	}
	var out []BinDir
	for _, d := range drives {
		drive := strings.TrimRight(d, "\\:")
		path := filepath.Join(drive+":\\", "$Recycle.Bin", sid)
		if a, ok := winfs.Attrs(path); ok && a&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
			out = append(out, BinDir{Drive: drive, Path: path})
		}
	}
	return out, nil
}

// FiletimeToTime 把 $I 里的 FILETIME（UTC 100ns 拍）换成本地时刻。
func FiletimeToTime(ft int64) time.Time {
	return time.UnixMicro(winfs.FiletimeUnixMicro(ft)).In(LocalTZ)
}

type iHead struct {
	metaVersion int64
	size        int64
	filetime    int64
	origPath    string
}

// parseIFile 解析单个 $I；长度自校验不通过或路径为空都返回 error，由 Scan 归为损坏条目。
//
// 这里的报错文字刻意不进 i18n 目录：它们只被 Scan 包进 issues 记录（落 state.json、与 Python
// 黄金样例逐条比对），从不单独面客。翻了语言就会改动落盘内容，那是数据口径而不是界面口径。
func parseIFile(iPath string) (*iHead, error) {
	raw, err := os.ReadFile(iPath)
	if err != nil {
		return nil, err
	}
	if len(raw) < 28 {
		return nil, fmt.Errorf("文件太短，只有 %d 字节", len(raw))
	}
	h := &iHead{
		metaVersion: int64(binary.LittleEndian.Uint64(raw[0:8])),
		size:        int64(binary.LittleEndian.Uint64(raw[8:16])),
		filetime:    int64(binary.LittleEndian.Uint64(raw[16:24])),
	}
	var charCount, payloadStart int
	if h.metaVersion >= 2 {
		charCount = int(binary.LittleEndian.Uint32(raw[24:28]))
		payloadStart = 28
	} else {
		if len(raw) < 32 {
			return nil, fmt.Errorf("v1 头部不完整，只有 %d 字节", len(raw))
		}
		charCount = int(int64(binary.LittleEndian.Uint64(raw[24:32])))
		payloadStart = 32
	}
	end := payloadStart + 2*charCount
	if end > len(raw) {
		end = len(raw)
	}
	h.origPath = decodeUTF16(raw[payloadStart:end])
	if h.origPath == "" {
		return nil, fmt.Errorf("路径为空")
	}
	if diff := len(raw) - (payloadStart + 2*charCount); diff > 64 || diff < -64 {
		return nil, fmt.Errorf("头部长度字段与文件实际大小不符（声明到 %d，实际 %d 字节）", payloadStart+2*charCount, len(raw))
	}
	// FILETIME 为 0 或明显离谱（全零头、被写坏的 64 位）时，Python 版走 Windows localtime
	// 会抛 OSError，条目落进「元数据损坏」；这里用同一条区间判掉，保证两边分区一致。
	// 区间就是本机实测 Python 能渲染的 epoch [86400, 32536713599]（约 1970-01-02 ～ 3001-01-18），
	// 正常删除时间不可能落在外面。
	if epoch := winfs.FiletimeUnixMicro(h.filetime); epoch < 86_400_000_000 || epoch > 32_536_713_599_000_000 {
		return nil, fmt.Errorf("删除时间不可表示（FILETIME=%d）", h.filetime)
	}
	return h, nil
}

func decodeUTF16(b []byte) string {
	if len(b)%2 != 0 {
		b = b[:len(b)-1]
	}
	units := make([]uint16, len(b)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(b[2*i:])
	}
	// 路径带 NUL 结尾，utf16.Decode 会把它变成 U+0000，一并裁掉。
	s := string(utf16.Decode(units))
	return strings.TrimRight(s, "\x00")
}

// buildItem 把头部换算成展示字段（与 Python parse_i_file 的返回一致）。
func (h *iHead) buildItem(iPath string) *Item {
	deleted := FiletimeToTime(h.filetime)
	unrecoverable := strings.HasPrefix(h.origPath, "::")
	display := h.origPath
	if unrecoverable {
		display = strings.ReplaceAll(display, "::", "")
	} else {
		display = baseName(strings.TrimRight(display, `\/`))
	}
	if display == "" {
		name := filepath.Base(iPath)
		if len(name) > 2 {
			display = name[2:]
		} else {
			display = "(根)"
		}
	}
	size := h.size
	if size < 0 {
		size = 0
	}
	return &Item{
		MetaVersion:   h.metaVersion,
		Size:          size,
		DeletedAt:     deleted.Format("2006-01-02 15:04:05"),
		Epoch:         float64(deleted.UnixMicro()) / 1e6,
		OrigPath:      h.origPath,
		Display:       display,
		Unrecoverable: unrecoverable,
	}
}

// baseName 等价于 os.path.basename（ntpath 口径：split(p)[1]）——取最后一个 \ 或 / 之后的部分。
// 光秃秃的盘符要单独处理：ntpath.split("D:") == ("D:", "")，Python 侧拿到空串后 display 会退回
// $I 文件名的尾巴，所以这里也返回空。
func baseName(path string) string {
	if i := strings.LastIndexAny(path, `\/`); i >= 0 {
		return path[i+1:]
	}
	if len(path) == 2 && path[1] == ':' {
		return ""
	}
	return path
}

// Scan 扫描可见条目，返回 (条目, 异常记录, 截断说明)。
// maxScan>0 且总数超过它时，只按 $I 写入时间取最老的 maxScan 个解析，其余留给下一轮。
func Scan(dirs []BinDir, maxScan int) (items []*Item, issues []string, overflow *Overflow) {
	if len(dirs) == 0 {
		d, err := BinDirs(nil)
		if err != nil {
			return nil, []string{err.Error()}, nil
		}
		dirs = d
	}

	type dirEntry struct {
		drive  string
		root   string
		iNames []string
		rNames []string
		iStems map[string]bool
	}
	var perDir []dirEntry
	total := 0
	for _, bd := range dirs {
		names, err := os.ReadDir(bd.Path)
		if err != nil {
			issues = append(issues, fmt.Sprintf("%s: 无法列出目录 (%v)", bd.Path, err))
			continue
		}
		de := dirEntry{drive: bd.Drive, root: bd.Path}
		for _, e := range names {
			n := e.Name()
			switch {
			case strings.HasPrefix(n, "$I"):
				de.iNames = append(de.iNames, n)
			case strings.HasPrefix(n, "$R"):
				de.rNames = append(de.rNames, n[2:])
			}
		}
		sort.Strings(de.iNames)
		de.iStems = make(map[string]bool, len(de.iNames))
		for _, n := range de.iNames {
			de.iStems[n[2:]] = true
		}
		perDir = append(perDir, de)
		total += len(de.iNames)
	}

	capped := maxScan > 0 && total > maxScan
	pickedByDrive := map[string][]string{}
	if capped {
		type dated struct {
			stamp int64
			drive string
			name  string
		}
		list := make([]dated, 0, total)
		for _, de := range perDir {
			for _, name := range de.iNames {
				stamp, _ := winfs.ModTimeMicro(filepath.Join(de.root, name))
				list = append(list, dated{stamp, de.drive, name})
			}
		}
		sort.SliceStable(list, func(i, j int) bool {
			if list[i].stamp != list[j].stamp {
				return list[i].stamp < list[j].stamp
			}
			if list[i].drive != list[j].drive {
				return list[i].drive < list[j].drive
			}
			return list[i].name < list[j].name
		})
		for _, d := range list[:maxScan] {
			pickedByDrive[d.drive] = append(pickedByDrive[d.drive], d.name)
		}
	}

	byDrive := map[string][2]int{}
	for _, de := range perDir {
		picked := de.iNames
		if capped {
			picked = append([]string(nil), pickedByDrive[de.drive]...)
			sort.Strings(picked)
		}
		byDrive[de.drive] = [2]int{len(picked), len(de.iNames)}
		for _, name := range picked {
			iPath := filepath.Join(de.root, name)
			var item *Item
			if head, err := parseIFile(iPath); err != nil {
				issues = append(issues, fmt.Sprintf("%s: 解析失败 (%v)", iPath, err))
				item = &Item{
					MetaVersion: -1,
					Size:        0,
					DeletedAt:   "1970-01-01 00:00:00",
					Epoch:       0,
					OrigPath:    "(元数据损坏)",
					Display:     name[2:],
				}
			} else {
				item = head.buildItem(iPath)
			}
			rPath := filepath.Join(de.root, "$R"+name[2:])
			item.Drive = de.drive
			item.IPath = iPath
			item.RPath = rPath
			item.Exists = winfs.Exists(rPath)
			if !item.Exists {
				issues = append(issues, fmt.Sprintf("%s: 对应 $R 数据不存在（残留元数据）", iPath))
			}
			items = append(items, item)
		}
		// 孤儿判断用完整 $I 名单，与本次是否截断无关
		var orphans []string
		for _, stem := range de.rNames {
			if !de.iStems[stem] {
				orphans = append(orphans, stem)
			}
		}
		sort.Strings(orphans)
		for _, stem := range orphans {
			issues = append(issues, fmt.Sprintf("$R%s: 缺少 $I 元数据，本次不处理", stem))
		}
	}

	if capped {
		overflow = &Overflow{Total: total, Taken: maxScan, ByDrive: byDrive}
	}
	return items, issues, overflow
}
