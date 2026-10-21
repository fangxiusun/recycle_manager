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

// Stats 是一次扫描的分段计时与逐条计数：「慢在哪一段」由它回答，日志只读它。
// 每段只计自己那一段的墙钟，段外的开销不在里面，所以 Total 会小于调用方的总用时。
// Resolve/Build/Write/CSV 由调用链上的不同层填（engine 填分区构建，CLI/GUI 填落盘）。
type Stats struct {
	Resolve time.Duration // 取 SID 并确认各盘的回收站目录
	List    time.Duration // 各盘一次列目录（$I 名单、$R 名单都从这里来）
	ReadI   time.Duration // 逐个打开并读出 $I 的字节
	ParseI  time.Duration // 解码 $I 并换算展示字段（纯 CPU）
	StatR   time.Duration // 逐条判断对应的 $R 数据还在不在
	Cap     time.Duration // --max-scan 截断：取每个 $I 的写入时间再排序
	Build   time.Duration // 分区构建
	Write   time.Duration // state.json 落盘
	CSV     time.Duration // scan_list.csv 落盘

	Entries int // 列目录拿到的目录项总数
	Reads   int // 读过的 $I 个数
	Checks  int // $R 存在性判断次数
	Drives  []DriveStat
	Skipped []SkippedDrive // 自动枚举时被跳过的盘（不探测，日志里要说明）
}

// SkippedDrive 是一个没被探测的盘：类型给日志用来解释为什么跳过。
type SkippedDrive struct {
	Drive string
	Type  uint32 // windows.DRIVE_REMOTE / DRIVE_CDROM / DRIVE_NO_ROOT_DIR
}

// DriveStat 是单个盘的拆分：一个慢盘（机械盘、网络盘、空读卡器）能把总数拖平，
// 只有分盘才看得出来是整体慢还是某一盘慢。
type DriveStat struct {
	Drive   string
	Entries int
	Reads   int
	Resolve time.Duration
	List    time.Duration
	Read    time.Duration
	Stat    time.Duration
}

// Total 是已计分段之和。
func (s *Stats) Total() time.Duration {
	return s.Resolve + s.List + s.ReadI + s.ParseI + s.StatR + s.Cap +
		s.Build + s.Write + s.CSV
}

// drive 取（必要时新建）某个盘的计时行，盘符顺序按首次出现排。
func (s *Stats) drive(d string) *DriveStat {
	for i := range s.Drives {
		if s.Drives[i].Drive == d {
			return &s.Drives[i]
		}
	}
	s.Drives = append(s.Drives, DriveStat{Drive: d})
	return &s.Drives[len(s.Drives)-1]
}

// fmtDur 给计时日志用：亚毫秒段保留到 10µs，其余到毫秒，免得满屏 0s。
func fmtDur(d time.Duration) string {
	if d < time.Millisecond {
		return d.Round(10 * time.Microsecond).String()
	}
	return d.Round(time.Millisecond).String()
}

// TimingText 把分段计时排成日志文本：一行总计时、一行逐条计数，多于一个盘时逐盘再补一行。
func (s *Stats) TimingText() string {
	stages := []struct {
		key string
		dur time.Duration
	}{
		{"scan.stage.resolve", s.Resolve},
		{"scan.stage.list", s.List},
		{"scan.stage.read-i", s.ReadI},
		{"scan.stage.parse-i", s.ParseI},
		{"scan.stage.stat-r", s.StatR},
		{"scan.stage.cap", s.Cap},
		{"scan.stage.build", s.Build},
		{"scan.stage.write", s.Write},
		{"scan.stage.csv", s.CSV},
	}
	sep := i18n.T("scan.timing.sep")
	cells := make([]string, 0, len(stages)+1)
	for _, st := range stages {
		cells = append(cells, i18n.T(st.key)+" "+fmtDur(st.dur))
	}
	cells = append(cells, i18n.T("scan.stage.total")+" "+fmtDur(s.Total()))
	out := i18n.Tf("scan.timing.line", strings.Join(cells, sep)) + "\n" +
		i18n.Tf("scan.timing.counts", s.Reads, s.Checks, s.Entries)
	if len(s.Drives) > 1 {
		for _, d := range s.Drives {
			out += "\n" + i18n.Tf("scan.timing.drive", d.Drive, fmtDur(d.Resolve), fmtDur(d.List),
				fmtDur(d.Read), d.Reads, i18n.Noun(d.Reads, "个", "file", "files"), fmtDur(d.Stat))
		}
	}
	if len(s.Skipped) > 0 {
		cells := make([]string, 0, len(s.Skipped))
		for _, sk := range s.Skipped {
			cells = append(cells, sk.Drive+": "+i18n.T(volumeKey(sk.Type)))
		}
		out += "\n" + i18n.Tf("scan.timing.skip", strings.Join(cells, sep))
	}
	return out
}

// volumeKey 把 GetDriveType 的数字换成日志里的卷类型词。
func volumeKey(typ uint32) string {
	switch typ {
	case windows.DRIVE_REMOTE:
		return "scan.volume.remote"
	case windows.DRIVE_CDROM:
		return "scan.volume.cdrom"
	case windows.DRIVE_NO_ROOT_DIR:
		return "scan.volume.no-root"
	default:
		return "scan.volume.other"
	}
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
func BinDirs(drives []string) ([]BinDir, error) { return BinDirsTimed(drives, nil) }

// BinDirsTimed 与 BinDirs 同义，额外把每个盘的探测耗时（以及跳过的盘）记进 stats。
//
// drives 留空（自动枚举全部盘）时只探本地卷：网络盘、光驱、没有根目录的盘符一律跳过。
// 一条 SMB 映射指向失联的服务器时，GetFileAttributes 要等到重连超时才返回（本机实测单盘
// 21s），而 Windows 从网络位置删除的对象本来就不落到任何盘的 $Recycle.Bin，探了也不会有条目。
// 用户点名要扫的盘（--drives y）照旧探测，哪怕它是网络盘。
func BinDirsTimed(drives []string, stats *Stats) ([]BinDir, error) {
	if stats == nil {
		stats = &Stats{}
	}
	tSid := time.Now()
	sid, err := CurrentSID()
	stats.Resolve += time.Since(tSid)
	if err != nil {
		return nil, err
	}
	explicit := len(drives) > 0
	if !explicit {
		drives = LocalDrives()
	}
	var out []BinDir
	for _, d := range drives {
		drive := strings.TrimRight(d, "\\:")
		typ := winfs.DriveType(drive)
		if !explicit && skipVolume(typ) {
			stats.Skipped = append(stats.Skipped, SkippedDrive{Drive: drive, Type: typ})
			continue
		}
		path := filepath.Join(drive+":\\", "$Recycle.Bin", sid)
		t := time.Now()
		a, ok := winfs.Attrs(path)
		spent := time.Since(t)
		stats.Resolve += spent
		stats.drive(drive).Resolve += spent
		if ok && a&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
			out = append(out, BinDir{Drive: drive, Path: path})
		}
	}
	return out, nil
}

// skipVolume 报告自动枚举时该不该跳过这类卷。可移动盘（U 盘）不跳过：
// 它们和本地盘一样有按用户分的回收站。
func skipVolume(typ uint32) bool {
	switch typ {
	case windows.DRIVE_REMOTE, windows.DRIVE_CDROM, windows.DRIVE_NO_ROOT_DIR:
		return true
	}
	return false
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
	return parseIBytes(raw)
}

// parseIBytes 只做解码，不碰文件，好让 Scan 把「读」和「算」分开计时。
func parseIBytes(raw []byte) (*iHead, error) {
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
	return ScanTimed(dirs, maxScan, nil)
}

// ScanTimed 与 Scan 完全同义，只是额外把各段耗时和逐条计数写进 stats（传 nil 表示不计）。
// stats 的 Build/Write/CSV 三段不在这里填：分区构建属于 engine，落盘属于调用方。
func ScanTimed(dirs []BinDir, maxScan int, stats *Stats) (items []*Item, issues []string, overflow *Overflow) {
	if stats == nil {
		stats = &Stats{}
	}
	if len(dirs) == 0 {
		d, err := BinDirsTimed(nil, stats)
		if err != nil {
			return nil, []string{err.Error()}, nil
		}
		dirs = d
	}

	type dirEntry struct {
		drive   string
		root    string
		iNames  []string
		rNames  []string
		iStems  map[string]bool
		rStems  map[string]bool
		iStamps map[string]int64 // $I 文件名 -> 写入时间（微秒），来自列目录本身
	}
	var perDir []dirEntry
	total := 0
	for _, bd := range dirs {
		t := time.Now()
		names, err := os.ReadDir(bd.Path)
		spent := time.Since(t)
		ds := stats.drive(bd.Drive)
		ds.List += spent
		stats.List += spent
		if err != nil {
			issues = append(issues, fmt.Sprintf("%s: 无法列出目录 (%v)", bd.Path, err))
			continue
		}
		stats.Entries += len(names)
		ds.Entries = len(names)
		de := dirEntry{drive: bd.Drive, root: bd.Path, iStamps: map[string]int64{}}
		for _, e := range names {
			n := e.Name()
			switch {
			case strings.HasPrefix(n, "$I"):
				de.iNames = append(de.iNames, n)
				// DirEntry.Info() 用的是列目录时已经拿到的查找数据，不再发一次系统调用；
				// --max-scan 截断要按写入时间排序时就直接用它（取时间戳的代价从
				// 「每个 $I 一次 FindFirstFile+FindClose」变成 0）。
				if fi, ierr := e.Info(); ierr == nil {
					de.iStamps[n] = fi.ModTime().UnixMicro()
				}
			case strings.HasPrefix(n, "$R"):
				de.rNames = append(de.rNames, n[2:])
			}
		}
		sort.Strings(de.iNames)
		de.iStems = make(map[string]bool, len(de.iNames))
		for _, n := range de.iNames {
			de.iStems[n[2:]] = true
		}
		// 这次列目录已经把 $R 名单全拿到了，存成大写集合就能在内存里判「$R 数据还在不在」，
		// 不必逐条再发一次 GetFileAttributes。NTFS 本身不区分大小写，用大写键比原来的
		// lexists 口径更宽（同名不同写法的配对照样认），孤儿判断仍按原名逐字符比。
		de.rStems = make(map[string]bool, len(de.rNames))
		for _, stem := range de.rNames {
			de.rStems[strings.ToUpper(stem)] = true
		}
		perDir = append(perDir, de)
		total += len(de.iNames)
	}

	capped := maxScan > 0 && total > maxScan
	pickedByDrive := map[string][]string{}
	if capped {
		t := time.Now()
		type dated struct {
			stamp int64
			drive string
			name  string
		}
		list := make([]dated, 0, total)
		for _, de := range perDir {
			for _, name := range de.iNames {
				stamp, ok := de.iStamps[name]
				if !ok {
					// 列目录没带回这个文件的时间（极少见：拿条目信息时出错），退回逐个查。
					stamp, _ = winfs.ModTimeMicro(filepath.Join(de.root, name))
				}
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
		stats.Cap += time.Since(t)
	}

	byDrive := map[string][2]int{}
	for _, de := range perDir {
		ds := stats.drive(de.drive)
		picked := de.iNames
		if capped {
			picked = append([]string(nil), pickedByDrive[de.drive]...)
			sort.Strings(picked)
		}
		byDrive[de.drive] = [2]int{len(picked), len(de.iNames)}
		for _, name := range picked {
			iPath := filepath.Join(de.root, name)
			var head *iHead
			tRead := time.Now()
			raw, err := os.ReadFile(iPath)
			spent := time.Since(tRead)
			stats.ReadI += spent
			ds.Read += spent
			stats.Reads++
			ds.Reads++
			if err == nil {
				tParse := time.Now()
				head, err = parseIBytes(raw)
				stats.ParseI += time.Since(tParse)
			}
			var item *Item
			if err != nil {
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
			tStat := time.Now()
			item.Exists = de.rStems[strings.ToUpper(name[2:])]
			spent = time.Since(tStat)
			stats.StatR += spent
			ds.Stat += spent
			stats.Checks++
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
