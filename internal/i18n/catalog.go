package i18n

// entry 是一条文案的两种写法。中文是基准（默认语言），英文为对照。
type entry struct {
	zh string
	en string
}

// catalog 是全部可切换文案。键按「包.用途」命名，同一段话在多处出现时共用一键。
//
// 有意留在目录之外、永远显示中文的东西（落盘口径，见包注释）：
// parts.Others/All/Illegal/Special 与「(根)」「(无扩展名)」「(元数据损坏)」这些会进
// state.json 与 CSV 的分组标签；ledger.csv 的状态「已删」；meta 里进 issues 的异常记录；
// scan_list.csv / ledger.csv 的表头；state.json 的字段名与 gap:自动断点(<= N 桶) 的说明。
var catalog = map[string]entry{
	// ── tty：编号写法解析 ──
	"tty.ids.bad": {
		"编号写法无法识别: %s（示例: 1-5,8-10,16）",
		"Cannot read the partition ids: %s (example: 1-5,8-10,16)",
	},
	"tty.ids.range": {
		"编号 %s 超出范围 1-%d",
		"Partition id %s is out of range 1-%d",
	},

	// ── parts：时间桶与粒度参数 ──
	"parts.bucket.unknown": {
		"无法识别的时间桶: %s（可用 minutely/hourly/daily/weekly/monthly/Nm/Nh/Nd/Nmo/auto/gap[:30m]）",
		"Unrecognized time bucket: %s (use minutely/hourly/daily/weekly/monthly/Nm/Nh/Nd/Nmo/auto/gap[:30m])",
	},
	"parts.grain.empty": {
		"粒度不能为空，例如 gap:5m",
		"The granularity is empty, e.g. gap:5m",
	},
	"parts.grain.unit": {
		"无法识别的粒度单位 %q（只能用 s/m/h/d）",
		"Unrecognized granularity unit %q (only s/m/h/d are allowed)",
	},
	"parts.grain.number": {
		"粒度的数字无法识别: %s",
		"Cannot read the granularity number: %s",
	},
	"parts.grain.positive": {
		"粒度必须大于 0: %s",
		"The granularity must be greater than 0: %s",
	},

	// ── meta：取当前用户 SID ──
	// 两侧都用 %v 而不是 %w：全仓库没有任何 errors.Is/As/Unwrap 调用，
	// 换语言时保住动词一致比保住包装链更有用。
	"meta.sid.token": {
		"GetTokenInformation 失败: %v",
		"GetTokenInformation failed: %v",
	},
	"meta.sid.string": {
		"ConvertSidToStringSidW 失败",
		"ConvertSidToStringSidW failed",
	},

	// ── store：快照文件与展示用列标题 ──
	"store.no-snapshot": {
		"没有找到快照 %s，请先运行 scan",
		"No snapshot at %s — run scan first",
	},
	"store.bad-snapshot": {
		"快照 %s 解析失败: %v",
		"Failed to parse the snapshot %s: %v",
	},
	"store.group-col.dir": {
		"路径组",
		"Path group",
	},
	"store.group-col.prefix": {
		"前缀组",
		"Prefix group",
	},

	// ── engine：扫描前置条件与删除保险丝 ──
	"engine.no-bin": {
		"指定盘 %s 下没有当前用户的回收站目录",
		"No Recycle Bin folder for the current user on drive(s) %s",
	},
	"engine.fuse": {
		"条目数 %d 超过 --max %d，已中止。可分区分批删。",
		"%d items exceed --max %d, so nothing was touched. Delete one or two partitions at a time.",
	},

	// ── 扫描分段计时（CLI 与 GUI 的日志共用同一套段名）──
	"scan.timing.line":   {"分段计时：%s", "Stage timing: %s"},
	"scan.timing.sep":    {"｜", " | "},
	"scan.timing.counts": {`逐条计数：$I 读取 %d 次，$R 检查 %d 次，列目录拿到 %d 个目录项`, `Per-item counts: %d $I reads, %d $R checks, %d dir entries listed`},
	"scan.timing.drive": {
		`%s 盘：探测 %s｜列目录 %s｜$I 读取 %s（%d %s）｜$R 检查 %s`,
		`Drive %s: probe %s | listing %s | $I read %s (%d %s) | $R check %s`,
	},
	"scan.timing.skip": {
		`自动枚举跳过的盘（没有探测）：%s`,
		`Drives skipped by auto-enumeration (not probed): %s`,
	},
	"scan.volume.remote":  {"网络盘", "network drive"},
	"scan.volume.cdrom":   {"光驱", "optical drive"},
	"scan.volume.no-root": {"无根目录（读卡器空着或设备已断开）", "no root (empty reader, unplugged device)"},
	"scan.volume.other":   {"非本地卷", "non-local volume"},
	"scan.stage.resolve":  {"目录解析", "bin dirs"},
	"scan.stage.list":     {"列目录", "listing"},
	"scan.stage.read-i":   {"$I 读取", "$I read"},
	"scan.stage.parse-i":  {"$I 解析", "$I decode"},
	"scan.stage.stat-r":   {"$R 检查", "$R check"},
	"scan.stage.cap":      {"截断排序", "cap sort"},
	"scan.stage.build":    {"分区构建", "partitioning"},
	"scan.stage.write":    {"快照写入", "snapshot write"},
	"scan.stage.csv":      {"清单写入", "manifest write"},
	"scan.stage.total":    {"分段合计", "stages total"},
}
