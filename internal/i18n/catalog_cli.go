package i18n

// CLI 文案。中文侧与改造前的字面量逐字一致（测试与文档都按中文串断言），
// 英文侧是同一条意思，格式动词的个数与顺序都对齐。
//
// 两侧本来就写成一样的东西不进目录，留在调用处当字面量：`rbm <cmd>: error:` 前缀、
// `[y/N]`、`rbm> ` 提示符、表头 "index"、`  - %s` 这类缩进模板，以及 paramsJSON 那行
// 参数回显（复刻 Python 的 json.dumps，属于落盘口径而不是界面文案）。
var catalogCLI = map[string]entry{
	// ── 用法与参数错误 ──
	"cli.usage-hint": {
		`用法: rbm <子命令> [选项]，或 rbm --help`,
		`Usage: rbm <subcommand> [options], or rbm --help`,
	},
	"cli.bad-subcommand": {
		`rbm: 无法识别的子命令 %s（可用: scan/parts/show/delete/shell）`,
		`rbm: unrecognized subcommand %s (available: scan/parts/show/delete/shell)`,
	},
	"cli.bad-flag": {
		`无法识别的选项: --%s`,
		`Unrecognized option: --%s`,
	},
	"cli.flag-no-value": {
		`选项 --%s 不接受取值`,
		`Option --%s takes no value`,
	},
	"cli.flag-need-value": {
		`选项 --%s 需要一个取值`,
		`Option --%s needs a value`,
	},
	"cli.flag-bad-value": {
		`选项 --%s 的取值无法识别: %s`,
		`Cannot read the value of option --%s: %s`,
	},
	"cli.need-pid": {
		`需要且只需要一个编号参数，例如: rbm show 12`,
		`Exactly one partition id is required, e.g. rbm show 12`,
	},
	"cli.need-pids": {
		`需要编号区间参数，例如: rbm delete 1-5,8-10,16`,
		`A partition id range is required, e.g. rbm delete 1-5,8-10,16`,
	},
	"cli.extra-args": {
		`多余的参数: %s`,
		`Unexpected extra arguments: %s`,
	},
	"cli.invalid-value": {
		`无效取值`,
		`invalid value`,
	},
	"cli.unknown-option": {
		`未知选项`,
		`unknown option`,
	},
	"cli.bad-shell-command": {
		`  rbm %s: error: 无法识别的命令 %s（help 看用法）`,
		`  rbm %s: error: unrecognized command %s (type help for usage)`,
	},

	// ── 根帮助 ──
	"cli.help.root": {
		`回收站分批管理工具（分区编号 → 查看 → 批量删除）

用法: rbm <子命令> [选项]

子命令:
  scan    扫描回收站并按「时间桶 × 原路径目录」分区编号，写出统计
  parts   重列已扫描快照的分区统计
  show    查看某个编号分区内的元素列表（index 区间）
  delete  按编号（如 1-5,8-10,16）分批永久删除分区内容，默认 dry-run
  shell   进入交互 shell：启动即 scan，之后的 show/delete 复用同一份内存快照

通用选项:
  --lang zh|en            界面语言。也可用环境变量 RBM_LANG，
                          或 LOCALAPPDATA 下 rbm\settings.json 的 "language"

不带任何参数直接运行 rbm.exe 会打开图形界面。用 rbm <子命令> -h 看该命令的选项。`,
		`Batch-wise Windows Recycle Bin cleaner (numbered partitions -> inspect -> delete in batches)

Usage: rbm <subcommand> [options]

Subcommands:
  scan    Scan the Recycle Bin, split it into numbered partitions and print the stats
  parts   Reprint the partition table of the snapshot already scanned
  show    List the items inside one numbered partition (index range)
  delete  Permanently delete whole partitions by id (e.g. 1-5,8-10,16); dry-run by default
  shell   Interactive shell: scans on start, then show/delete reuse that one in-memory snapshot

Shared option:
  --lang zh|en            Interface language. Also via RBM_LANG,
                          or the "language" field in rbm\settings.json under LOCALAPPDATA

Running rbm.exe with no arguments opens the GUI. rbm <subcommand> -h shows that command's options.`,
	},

	// ── 分区选项块（scan 与 shell 共用）──
	"cli.help.partition-opts": {
		`  --bucket 文本           时间桶。不填=按删除时间断点自动切区（gap）。也可指定等宽：
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
                          N 个建快照并明确提示，不会静默丢弃；删完再 scan 会继续处理剩下的`,
		`  --bucket TEXT           Time bucket. Empty = split at gaps in the deletion time (gap). Fixed
                          widths also work: minutely/hourly/daily/weekly/monthly/30m/15m/1h/3d/2mo,
                          or gap:5m to break only where the gap is >= 5 minutes;
                          auto = pick the finest usable fixed width
  --max-buckets N         Max time buckets: gap mode keeps the largest gaps, auto mode picks a
                          fixed width no coarser than this (default 12)
  --by dir|prefix|ext|token  Grouping inside a bucket: dir = folder of the original path (default) /
                          prefix = leading characters of the file name / ext = extension /
                          token = first segment of the file name
  --dir-level N|full      Trailing path segments dir grouping uses: 1 = folder name only (merges
                          like batches), 2 = default (parent + folder, tells builds apart),
                          3 = more specific, full = complete path
  --prefix-len N          Character count for prefix/token (default 3)
  --max-parts N           Max partitions (default 120; lower folds small groups into "其他",
                          higher keeps more specific paths)
  --min-count N           Groups smaller than this fold into "其他" (default 5)
  --drives C,G            Restrict to these drive letters, comma separated
  --max-scan N            Max items per scan (default 50000, 0 = no cap). A bigger bin is capped to
                          the OLDEST N items, stated out loud and never dropped silently; delete
                          that batch, scan again and the rest comes in`,
	},
	"cli.help.state-dir": {
		`  --state-dir 目录        快照目录（默认 当前目录\.rbm）`,
		`  --state-dir DIR         Snapshot folder (default .rbm under the current directory)`,
	},
	"cli.help.options-heading": {
		`选项:`,
		`Options:`,
	},

	// ── 各子命令帮助 ──
	"cli.usage-prefix": {
		`用法: rbm `,
		`Usage: rbm `,
	},
	"cli.usage.scan": {
		`scan [选项]`,
		`scan [options]`,
	},
	"cli.desc.scan": {
		`扫描回收站并按「时间桶 × 原路径目录」分区编号，写出统计。`,
		`Scan the Recycle Bin, split it into numbered partitions (time bucket x original folder) and print the stats.`,
	},
	"cli.usage.parts": {
		`parts [--alive] [--sort time|size|count] [--state-dir 目录]`,
		`parts [--alive] [--sort time|size|count] [--state-dir DIR]`,
	},
	"cli.desc.parts": {
		`重列已扫描快照的分区统计表。`,
		`Reprint the partition table of the snapshot already scanned.`,
	},
	"cli.usage.show": {
		`show <编号> [--start N] [--end N] [--prefix 串] [--limit N] [--tail] [--local]`,
		`show <id> [--start N] [--end N] [--prefix STR] [--limit N] [--tail] [--local]`,
	},
	"cli.desc.show": {
		`查看某编号分区内的元素列表。--limit 0 全部显示。`,
		`List the items inside one numbered partition. --limit 0 shows all of them.`,
	},
	"cli.usage.delete": {
		`delete <编号区间> [--dry-run|--yes] [--prefix 串] [--max N] [--sample N] [--every N] [-v]`,
		`delete <ids> [--dry-run|--yes] [--prefix STR] [--max N] [--sample N] [--every N] [-v]`,
	},
	"cli.desc.delete": {
		`按编号分批永久删除（默认 dry-run，只预览不动手）。`,
		`Permanently delete whole partitions by id (dry-run by default: it previews and touches nothing).`,
	},
	"cli.usage.shell": {
		`shell [scan 的全部选项] [--no-scan]`,
		`shell [every scan option] [--no-scan]`,
	},
	"cli.desc.shell": {
		`进入交互 shell：启动即 scan，之后的 show/delete 复用同一份内存快照，编号不会漂。`,
		`Enter the interactive shell: it scans on start, then show/delete reuse that one in-memory snapshot, so partition ids don't drift while you work.`,
	},

	// ── shell 帮助 ──
	"cli.help.shell": {
		`可用命令（就是那几个子命令，参数写法一样）:
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
`,
		`Commands (the same subcommands, the same option syntax):
  scan [options]                 Rescan and repartition into a new snapshot; later show/delete use these ids
                                 Options as scan: --bucket --by --dir-level --max-parts --min-count
                                 --max-buckets --prefix-len --drives --max-scan
  parts [--alive] [--sort time|size|count]      Reprint the current snapshot's partition table
  show <id> [--start N --end N] [--prefix STR] [--limit 0] [--tail] [--local]
  delete <ids> [--dry-run|--yes] [--prefix STR] [--max N] [--sample N] [-v]
                                 Ids like 1-5,8-10,16. Without --yes it asks y/N right here
  info                           Which scan the current snapshot came from, and with what options
  help | quit | exit | q
Examples:
  parts --sort size
  show 12 --limit 20
  delete 12-15 --dry-run
  delete 12-15
  scan --by dir --dir-level 1 --bucket gap:10m
`,
	},

	// ── 表头 ──
	"cli.th.pid":     {`编号`, `ID`},
	"cli.th.range":   {`时间范围`, `Time range`},
	"cli.th.count":   {`元素数`, `Items`},
	"cli.th.size":    {`大小`, `Size`},
	"cli.th.index":   {`index区间`, `Index range`},
	"cli.th.drive":   {`盘`, `Drive`},
	"cli.th.left":    {`剩余`, `Left`},
	"cli.th.top":     {`组内构成/示例`, `Contents`},
	"cli.th.group2":  {`路径组/前缀组`, `Path/prefix group`},
	"cli.th.deleted": {`删除时间`, `Deleted`},
	"cli.th.name":    {`文件名`, `Name`},
	"cli.th.orig":    {`原路径`, `Original path`},

	// ── 分区统计表 ──
	"cli.parts.line1": {
		`快照时间 %s  参数 %s`,
		`Snapshot %s  options %s`,
	},
	"cli.parts.numbered": {
		`编号 1..%d`,
		`ids 1..%d`,
	},
	"cli.parts.no-parts": {
		`没有分区`,
		`no partitions`,
	},
	"cli.parts.line2": {
		`条目合计 %d %s / %s  分区 %d %s  %s`,
		`Total %d %s / %s  in %d %s  %s`,
	},

	// ── scan ──
	"cli.scan.issues": {
		`异常记录 %d 条（详见 scan_list.csv 同目录状态文件），示例:`,
		`Anomaly records: %d (see the state file next to scan_list.csv); examples:`,
	},
	"cli.scan.list-written": {
		`完整清单已写出: %s`,
		`Full manifest written: %s`,
	},
	"cli.scan.overflow-note": {
		`注意: 本次快照只覆盖最老的一部分条目，删完这批再 scan 会拿到剩下的内容。`,
		`Note: this snapshot covers only the oldest slice of the bin. Delete this batch, then scan again to pick up the rest.`,
	},
	"cli.scan.empty-1": {
		`回收站里当前用户没有对象（0 个条目），这就是扫描完成的结果，不是错误。`,
		`The Recycle Bin holds no objects for this user (0 items). That is a completed scan, not an error.`,
	},
	"cli.scan.empty-2": {
		`没有可删的分区；清空回收站之后重新 scan 会拿到同样的 0。`,
		`Nothing to delete; empty the bin and scan again and you get the same 0.`,
	},
	"cli.scan.next-step": {
		`下一步: show <编号> 查看内容；delete <编号区间> 分批删除（默认 dry-run，加 --yes 才真删）`,
		`Next: show <id> to inspect; delete <ids> to delete in batches (dry-run by default, --yes to really delete)`,
	},

	// ── 扫描截断说明 ──
	"cli.overflow.head": {
		`! 回收站共有 %d %s，超过本次 --max-scan %d，快照只建了其中最老的 %d 个（按 $I 写入时间排序，约等于删除时间），另有 %d 个未纳入。`,
		`! The bin holds %d %s, more than this --max-scan %d, so the snapshot covers only the oldest %d of them (sorted by $I write time, roughly the deletion time); %d left out.`,
	},
	"cli.overflow.drives": {
		`  各盘 已扫/总数: %s`,
		`  Per drive scanned/total: %s`,
	},
	"cli.overflow.two-ways": {
		`  两种继续办法:`,
		`  Two ways to continue:`,
	},
	"cli.overflow.option-a": {
		`    A) 一次看全 —— 把上限调到总数以上再重扫: scan --max-scan %d`,
		`    A) See everything at once -- raise the cap above the total and rescan: scan --max-scan %d`,
	},
	"cli.overflow.option-b": {
		`    B) 分批推进（推荐）—— 先删掉本快照里的分区，再重新 scan，剩下的条目会自动进新快照并重新编号`,
		`    B) Work in batches (recommended) -- delete the partitions in this snapshot, then scan again: the rest comes in and gets renumbered`,
	},

	// ── show ──
	"cli.show.header": {
		`分区 %d｜%s｜%s｜%d %s / %s｜index %d-%d｜盘 %s`,
		`Partition %d | %s | %s | %d %s / %s | index %d-%d | drive %s`,
	},
	"cli.show.hidden": {
		`（另有 %d %s 未显示；--limit 0 全部显示，--start/--end 指定区间）`,
		`(not shown: %d %s; --limit 0 shows all, --start/--end picks a range)`,
	},

	// ── delete ──
	"cli.del.nomatch": {
		`没有匹配项：编号 %s 加上前缀过滤 %s 后为空，未做任何改动。`,
		`Nothing matched: ids %s with prefix filter %s came out empty, so nothing was touched.`,
	},
	"cli.del.empty-prefix": {
		`(空)`,
		`(empty)`,
	},
	"cli.del.selected": {
		`选中分区 %d %s: %s`,
		`Selected %d %s: %s`,
	},
	"cli.del.will": {
		`本次将永久删除 %d %s，约 %s（不可恢复）`,
		`This will permanently delete %d %s, about %s (not recoverable)`,
	},
	"cli.del.dryrun-preview": {
		`dry-run 预览前 %d 条：`,
		`dry-run preview of the first %d:`,
	},
	"cli.del.dryrun-done": {
		`未做任何改动。确认无误后加 --yes 执行真删除。`,
		`Nothing was touched. When the list looks right, add --yes to really delete.`,
	},
	"cli.del.need-yes": {
		`这是不可逆操作。加 --yes 明确确认后再执行（或先用 --dry-run 看清单）。`,
		`This is irreversible. Add --yes to confirm explicitly (or run --dry-run first to see the list).`,
	},
	"cli.del.ask": {
		`确认永久删除这 %d %s / %s（不可恢复）？`,
		`Really delete %d %s / %s permanently, with no way back?`,
	},
	"cli.del.cancelled": {
		`已取消，未做任何改动。`,
		`Cancelled; nothing was touched.`,
	},
	"cli.del.progress": {
		`  进度 %d/%d  已删 %d  失败 %d`,
		`  progress %d/%d  deleted %d  failed %d`,
	},
	"cli.del.interrupted": {
		`删除中断: %s`,
		`Delete interrupted: %s`,
	},
	"cli.del.done": {
		`完成：删除 %d 个，失败 %d 个，账本 %s`,
		`Done: %d deleted, %d failed, ledger %s`,
	},
	"cli.del.state-updated": {
		`快照已更新（%s）：标记了本次删掉的条目，编号不变。`,
		`Snapshot updated (%s): the items just deleted are marked, ids unchanged.`,
	},
	"cli.del.rescan": {
		`要看回收站最新状态请重新 scan：编号按当前内容重建，shell 里这等于创建新上下文。`,
		`To see the bin as it is now, scan again: ids are rebuilt from what is actually there, and inside shell that starts a fresh context.`,
	},

	// ── shell ──
	"cli.shell.intro": {
		`回收站分批管理 shell：scan 一次，show/delete 反复用同一份快照，编号不会漂。`,
		`Batch-wise Recycle Bin shell: scan once, then show/delete reuse that one snapshot so ids don't drift.`,
	},
	"cli.shell.state-dir": {
		`快照目录 %s    输入 help 看用法，quit 退出。`,
		`Snapshot folder %s    Type help for usage, quit to leave.`,
	},
	"cli.shell.already-in": {
		`  已经在 shell 里了，直接输入 scan / show / delete 即可。`,
		`  You are already in the shell -- just type scan / show / delete.`,
	},
	"cli.prompt.state": {
		`rbm[%s 快照 %d条/%d区 剩%d]> `,
		`rbm[%s items=%d parts=%d left=%d]> `,
	},

	// ── info ──
	"cli.info.no-snapshot": {
		`  当前内存里没有快照（磁盘上可能有，命令会退回读 %s）`,
		`  No snapshot in memory (there may be one on disk; commands fall back to reading %s)`,
	},
	"cli.info.line": {
		`  快照时间 %s  条目 %d  分区 %d`,
		`  Snapshot %s  items %d  partitions %d`,
	},
	"cli.info.params": {
		`  参数 %s`,
		`  Options %s`,
	},
	"cli.info.deleted": {
		`  本快照内已标记删除 %d %s（编号不变；要看真实状态请 scan 重建）`,
		`  Marked deleted inside this snapshot: %d %s (ids unchanged; scan again for the real state)`,
	},
	"cli.info.last-delete": {
		`  上次删除 %s`,
		`  Last delete %s`,
	},
}

func init() {
	for k, v := range catalogCLI {
		catalog[k] = v
	}
}
