// gui 界面文案：窗口标题、菜单、分组框、标签、按钮、气泡提示、表头、弹窗与日志。
// 中文照抄改造前的字面量，保证老用户看到的字一个都不变。
//
// 英文里的「数字 + 名词」一律写成「名词在前、数字在后」的标签式（partitions %d），
// 这样不需要单复数切换也不会出现 "1 partitions"；要写散文句式就得用 i18n.Noun 补 %s 槽，
// 而 %s 槽必须两边都有（动词个数要一致，i18n_test 会查），所以这里统一走标签式。
package i18n

var catalogGUI = map[string]entry{
	// ---------- 窗口 / 菜单 ----------
	"gui.title": {
		`回收站分批管理 rbm`,
		`rbm - Recycle Bin batch cleaner`,
	},
	"gui.menu.file": {`文件(&F)`, `&File`},
	"gui.menu.view": {`视图(&V)`, `&View`},
	"gui.menu.help": {`帮助(&H)`, `&Help`},
	"gui.menu.scan": {`扫描 / 重扫`, `Scan / rescan`},
	"gui.menu.open-dir": {
		`打开快照目录`,
		`Open snapshot folder`,
	},
	"gui.menu.quit": {`退出`, `Exit`},
	"gui.menu.adv":  {`展开 / 收起高级参数`, `Show / hide advanced options`},
	"gui.menu.select-all": {
		`全选当前视图的分区`,
		`Select all partitions in view`,
	},
	"gui.menu.clear-checks": {`清空勾选`, `Clear all ticks`},
	"gui.menu.usage":        {`使用说明`, `How to use`},
	"gui.menu.about":        {`关于 rbm`, `About rbm`},
	"gui.menu.language":     {`界面语言`, `Language`},
	"gui.menu.lang-zh":      {`中文`, `Chinese`},
	"gui.menu.lang-en":      {`英文`, `English`},
	"gui.run.create-fail": {
		`图形界面创建失败: %v`,
		`Could not open the window: %v`,
	},

	// ---------- 状态栏 / 进度 ----------
	"gui.sb.ready": {`● 就绪`, `● Ready`},
	"gui.sb.none":  {`尚未扫描`, `Not scanned yet`},
	"gui.sb.hint": {
		`F5 扫描；勾选分区后先预览再删除`,
		`F5 scans; tick partitions, preview before deleting`,
	},
	"gui.sb.scanning":   {`扫描中…`, `Scanning…`},
	"gui.sb.deleting":   {`删除中…`, `Deleting…`},
	"gui.progress-word": {`进度 `, `Progress `},
	"gui.progress.idle": {`空闲`, `idle`},
	"gui.progress.last-scan": {
		`进度 空闲｜上次扫描用时 %s`,
		`Progress idle | last scan took %s`,
	},
	"gui.progress.delete-step": {
		`进度 删除中 %d/%d（成功 %d，失败 %d）`,
		`Progress deleting %d/%d (done %d, failed %d)`,
	},
	"gui.progress.delete-end": {
		`进度 删除结束：成功 %d，失败 %d`,
		`Progress done: deleted %d, failed %d`,
	},
	"gui.sb.snapshot": {
		`快照 %s｜条目 %d 个 / %s｜分区 %d 个（编号 1..%d）`,
		`Snapshot %s | items %d / %s | partitions %d (ids 1..%d)`,
	},
	"gui.sb.snapshot-after": {
		`快照 %s｜未删完条目 %d｜最近删除 %s`,
		`Snapshot %s | items left %d | last delete %s`,
	},
	"gui.sb.delete-progress": {
		`删除中 %d/%d  已删 %d  失败 %d`,
		`Deleting %d/%d  done %d  failed %d`,
	},
	"gui.sb.no-tick": {
		`未勾选分区｜未删完条目 %d 个`,
		`Nothing ticked | items left %d`,
	},
	"gui.sb.ticked": {
		`已勾选 %d 个编号 / %d 个分区 / %d 个条目 %s`,
		`Ticked: ids %d / partitions %d / items %d %s`,
	},

	// ---------- 分组框标题 ----------
	"gui.box.config": {
		`① 扫描配置（改完点【扫描 / 重扫】生效，数字留空按默认值）`,
		`① Scan settings (press [Scan / rescan] to apply; blank numbers use defaults)`,
	},
	"gui.box.result": {`② 扫描结果`, `② Scan result`},
	"gui.box.select": {
		`③ 选择分区（勾选 = 要删的批次）`,
		`③ Pick partitions (a tick = a batch to delete)`,
	},
	"gui.box.parts": {
		`④ 分区列表（点方框勾选=要删的分区，按住 Shift 点另一个可以连选一段；单击行看明细，点列名排序）`,
		`④ Partitions (click a box to tick, Shift+click for a range, click a row for details, click a header to sort)`,
	},
	"gui.box.items": {`⑤ 元素明细`, `⑤ Items`},
	"gui.box.delete": {
		`⑥ 删除操作（三层闸门：预览 → 弹窗确认 → 保险丝）`,
		`⑥ Delete (three gates: preview -> confirm dialog -> fuse)`,
	},

	// ---------- 标签 ----------
	"gui.lab.bucket":      {`按时间分批`, `Batch by time`},
	"gui.lab.bucket-spec": {`写法`, `Value`},
	"gui.lab.by":          {`桶内分组`, `Group inside`},
	"gui.lab.dir-level":   {`目录层级`, `Folder depth`},
	"gui.lab.prefix-len":  {`前缀长度`, `Prefix length`},
	"gui.lab.max-buckets": {`桶数上限`, `Max batches`},
	"gui.lab.max-parts":   {`分区上限`, `Max partitions`},
	"gui.lab.min-count":   {`小组并入`, `Fold small groups`},
	"gui.lab.drives":      {`限定盘符`, `Drive letters`},
	"gui.lab.max-scan":    {`单次扫描上限`, `Scan cap`},
	"gui.lab.fuse":        {`删除保险丝`, `Delete fuse`},
	"gui.lab.state-dir":   {`快照目录`, `Snapshot folder`},
	"gui.lab.parts":       {`分区`, `Partitions`},
	"gui.lab.objects":     {`对象`, `Objects`},
	"gui.lab.alive":       {`未删完`, `Left`},
	"gui.lab.checked":     {`已勾选`, `Ticked`},
	"gui.lab.log":         {`操作日志`, `Activity log`},
	"gui.lab.by-ids":      {`按编号勾选`, `Tick by id`},
	"gui.lab.prefix-filter": {
		`文件名前缀过滤`,
		`Name prefix filter`,
	},
	"gui.cb.alive": {`只看未删完`, `Only not yet emptied`},
	// 英文写成「6 lines」而不是「lines: 6」：这一行是「操作日志」标签 + 计数两个控件拼出来的，
	// 后面那种会读成 "Activity log lines: 6"（真机截图里就是这样）。
	"gui.log.lines": {`%d 行`, `%d lines`},

	// ② 那四个总体数字：中文沿用「数字 + 个」的量词写法。
	// 英文里这一行是「标签控件 + 数值控件」并排，标签已经说了是什么，数值就别再把同一个名词抄一遍
	// （真机截图量出来是 "Partitions  partitions 120 (ids 1..120)"），未扫描时同理，
	// 标签 Ticked 后面只要「none」，不要 "Nothing ticked"。
	// 只有「未删完 / 已勾选」两项英文数值带名词：标签是 Left / Ticked，
	// 数值要同时给条目数和分区数，省掉名词就分不清哪个是哪个（中文靠「个条目 / 个分区」区分）。
	// 两边都是 %d 开头（gui.sum.none / gui.limit.no-limit 没有数字），调用方不用补 %s 槽。
	"gui.sum.none": {`未勾选`, `none`},
	"gui.sum.parts": {
		`%d 个（编号 1..%d）`,
		`%d (ids 1..%d)`,
	},
	"gui.sum.items": {`%d 个 / %s`, `%d / %s`},
	"gui.sum.alive": {`%d 个条目 / %d 个分区`, `items %d / partitions %d`},
	"gui.sum.checked": {
		`%d 个分区 / %d 个条目`,
		`partitions %d / items %d`,
	},
	"gui.limit.no-limit": {`不限`, `no cap`},

	// ---------- 按钮 ----------
	"gui.btn.adv-open": {`▼ 展开高级参数`, `▼ Advanced options`},
	"gui.btn.adv-close": {
		`▲ 收起高级参数`,
		`▲ Hide advanced options`,
	},
	"gui.btn.scan":         {`扫描 / 重扫 (F5)`, `Scan / rescan (F5)`},
	"gui.btn.open-dir":     {`打开目录`, `Open folder`},
	"gui.btn.select-all":   {`全选`, `Select all`},
	"gui.btn.clear-checks": {`清空勾选`, `Clear ticks`},
	"gui.btn.apply":        {`应用`, `Apply`},
	"gui.btn.clear-log":    {`清空`, `Clear`},
	"gui.btn.copy-log":     {`复制`, `Copy`},
	"gui.btn.preview": {
		`预览删除（不动文件）`,
		`Preview delete (touches nothing)`,
	},
	"gui.btn.delete-none": {
		`⚠ 永久删除（未勾选分区）`,
		`⚠ Delete forever (nothing ticked)`,
	},
	"gui.btn.delete-n": {
		`⚠ 永久删除 %d 个分区 / %d 个条目`,
		`⚠ Delete forever: partitions %d, items %d`,
	},
	"gui.btn.close":          {`关闭`, `Close`},
	"gui.btn.cancel":         {`取消`, `Cancel`},
	"gui.btn.confirm-delete": {`确认永久删除`, `Delete permanently`},

	// ---------- 输入框提示（CueBanner） ----------
	"gui.cue.drives": {`留空=全部`, `blank = all`},
	"gui.cue.fuse":   {`0=不限`, `0 = no cap`},
	"gui.cue.ids":    {`如 1-5,8-10,16`, `e.g. 1-5,8-10,16`},
	"gui.cue.prefix": {
		`空=不过滤（同 --prefix）`,
		`blank = no filter (= --prefix)`,
	},

	// ---------- 悬浮说明（ToolTipText） ----------
	"gui.tip.bucket": {
		`先按删除时间把条目切成几批：默认在删除记录的空档处切，也可以按固定粒度（每 15 分钟 / 每天 / 每月…），` +
			`或选「自定义」手填写法。这一项只管切批，每批里再按【桶内分组】分块。`,
		`Splits the bin into time batches: by default at gaps in the deletion records, or at a fixed ` +
			`width (every 15 min / day / month...); Custom lets you type the token. This only cuts the ` +
			`batches - [Group inside] splits each batch further.`,
	},
	"gui.tip.adv": {
		`桶数上限 / 分区上限 / 小组并入 / 限定盘符 / 单次扫描上限 / 删除保险丝 / 快照目录`,
		`Max batches / Max partitions / Fold small groups / Drive letters / Scan cap / Delete fuse / Snapshot folder`,
	},
	"gui.tip.max-buckets": {
		`时间切成几个桶；超过就把最接近的桶并起来`,
		`How many time batches at most; the closest ones get merged`,
	},
	"gui.tip.max-parts": {
		`分区总数上限；调小会把小组并成「其他」`,
		`Max partitions; a lower number folds small groups into 其他`,
	},
	"gui.tip.min-count": {
		`元素数不到这个数的组合进「其他」`,
		`Groups smaller than this go into 其他`,
	},
	"gui.tip.max-scan": {
		`超过就只取最老的这些建快照，日志会说明；0 = 不限`,
		`A bigger bin is capped to its oldest entries, stated in the log; 0 = no cap`,
	},
	"gui.tip.fuse": {
		`选中条目数超过这个数就整批不动（等价 --max）`,
		`If the ticked items exceed this, nothing is deleted (= --max)`,
	},
	"gui.tip.state-dir": {
		`state.json / scan_list.csv / ledger.csv 所在目录，命令行 --state-dir 指同一个目录`,
		`Folder holding state.json / scan_list.csv / ledger.csv - the same one --state-dir points at`,
	},
	"gui.tip.select-all": {
		`勾上当前视图里所有还有未删条目的分区（已删完的跳过）`,
		`Ticks every partition in view that still holds items (emptied ones are skipped)`,
	},
	"gui.tip.preview": {
		`只把选中统计和明细打进日志/右表，等价 rbm delete <编号> --dry-run`,
		`Sends the selection stats and details to the log / right table only (= rbm delete <ids> --dry-run)`,
	},
	"gui.tip.delete": {
		`真删：不进回收站、不可恢复。要在弹窗里点【确认永久删除】，并受「删除保险丝」约束。`,
		`Real delete: no Recycle Bin, no way back. You must click [Delete permanently] in the dialog, and the Delete fuse applies.`,
	},
	"gui.tip.copy-log": {
		`把整段日志放进剪贴板，方便贴给别人排查`,
		`Copies the whole log to the clipboard, handy when asking for help`,
	},

	// ---------- 下拉框显示文本 ----------
	"gui.by.dir":    {`目录 dir（默认）`, `dir - folder (default)`},
	"gui.by.prefix": {`字符前缀 prefix`, `prefix - leading chars`},
	"gui.by.ext":    {`扩展名 ext`, `ext - extension`},
	"gui.by.token":  {`文件名首段 token`, `token - first name segment`},
	"gui.level.1":   {`1 只看所在目录名`, `1 - folder name only`},
	"gui.level.2": {
		`2 上级目录+目录（默认）`,
		`2 - parent + folder (default)`,
	},
	"gui.level.3": {`3 更具体`, `3 - more specific`},
	"gui.level.full": {
		`full 完整路径`,
		`full - complete path`,
	},
	"gui.bucket.auto": {
		`自动找删除空档（默认）`,
		`Split at deletion gaps (default)`,
	},
	"gui.bucket.gap15m": {
		`只在 ≥15 分钟的空档切开（gap:15m）`,
		`Only at gaps >= 15 min (gap:15m)`,
	},
	"gui.bucket.gap1h": {
		`只在 ≥1 小时的空档切开（gap:1h）`,
		`Only at gaps >= 1 hour (gap:1h)`,
	},
	"gui.bucket.gap1d": {
		`只在 ≥1 天的空档切开（gap:1d）`,
		`Only at gaps >= 1 day (gap:1d)`,
	},
	"gui.bucket.w15m": {`每 15 分钟一批（15m）`, `Every 15 min (15m)`},
	"gui.bucket.w30m": {`每 30 分钟一批（30m）`, `Every 30 min (30m)`},
	"gui.bucket.hourly": {
		`每 1 小时一批（hourly）`,
		`Every hour (hourly)`,
	},
	"gui.bucket.daily": {`每 1 天一批（daily）`, `Every day (daily)`},
	"gui.bucket.w3d":   {`每 3 天一批（3d）`, `Every 3 days (3d)`},
	"gui.bucket.weekly": {
		`每 7 天一批（weekly）`,
		`Every week (weekly)`,
	},
	"gui.bucket.w14d": {`每 14 天一批（14d）`, `Every 14 days (14d)`},
	"gui.bucket.monthly": {
		`每 1 个自然月一批（monthly）`,
		`Every calendar month (monthly)`,
	},
	"gui.bucket.w3mo": {
		`每 3 个自然月一批（3mo）`,
		`Every 3 calendar months (3mo)`,
	},
	"gui.bucket.w6mo": {
		`每 6 个自然月一批（6mo）`,
		`Every 6 calendar months (6mo)`,
	},
	"gui.bucket.pick-auto": {
		`自动挑最合适的固定粒度（auto）`,
		`Pick the best fixed width (auto)`,
	},
	"gui.bucket.custom": {
		`自定义：手填写法`,
		`Custom: type the token`,
	},

	// ---------- 启动提示窗 ----------
	"gui.notice.title": {
		`正在扫描回收站`,
		`Scanning the Recycle Bin`,
	},
	"gui.notice.text": {
		`正在扫描，请等待完成…`,
		`Scanning, please wait...`,
	},
	"gui.notice.create-fail": {
		`启动提示窗创建失败: %v`,
		`Could not create the startup notice: %v`,
	},

	// ---------- 参数校验 ----------
	"gui.err.int": {
		`「%s」要填整数，现在填的是 %q`,
		`[%s] needs a whole number, you typed %q`,
	},
	"gui.err.negative": {
		`「%s」不能是负数，现在填的是 %d`,
		`[%s] cannot be negative, you typed %d`,
	},
	"gui.err.prefix-len": {
		`「前缀长度」在 %s 分组下至少 1 个字符`,
		`Prefix length must be at least 1 character for the %s grouping`,
	},
	"gui.err.column-title": {
		`改列名失败: %v`,
		`Could not rename the column: %v`,
	},

	// ---------- 扫描 ----------
	"gui.scan.all-drives": {`全部盘`, `all drives`},
	"gui.scan.auto-tier": {
		`自动找空档（不超过 %d 批）`,
		`automatic gaps (up to %d batches)`,
	},
	"gui.scan.custom-empty": {
		`「自定义」的写法留空了，这一轮按默认的自动找删除空档切批。`,
		`The Custom value was blank, so this round split at deletion gaps (the default).`,
	},
	"gui.scan.start": {
		`开始扫描（%s，按%s分批，单次上限 %s）…`,
		`Scanning (%s, batched by %s, cap %s)...`,
	},
	"gui.scan.fail": {`扫描失败: %v`, `Scan failed: %v`},
	"gui.scan.empty": {
		`扫描完成（用时 %s）：条目 0 个、分区 0 个——回收站里当前用户没有对象。` +
			`这是正常结果，不是错误；没有可删的分区。`,
		`Scan done in %s: 0 items, 0 partitions - the bin holds nothing for this user. That is a normal ` +
			`result, not an error; there is nothing to delete.`,
	},
	"gui.scan.done": {
		`扫描完成（用时 %s）：条目 %d 个 / %s，分区 %d 个，编号 1..%d。`,
		`Scan done in %s: items %d / %s in %d partitions, ids 1..%d.`,
	},
	"gui.scan.issues": {
		`异常记录 %d 条（全量在 scan_list.csv），示例:`,
		`Anomaly records: %d (all of them in scan_list.csv), examples:`,
	},
	"gui.scan.written": {
		"快照已写出: %s\n清单已写出: %s",
		"Snapshot written: %s\nManifest written: %s",
	},
	"gui.scan.nothing": {
		`没有可删的分区。回收站清空之后重新扫描会得到同样的 0。`,
		`No partition to delete. An empty bin rescans to the same 0.`,
	},
	"gui.scan.next": {
		`下一步：③ 里【全选】或点方框勾选 → ④ 单击看明细 → ⑥【预览删除】核对 → 再点【永久删除】并在弹窗里确认。`,
		`Next: [Select all] or tick boxes in ③, click a row in ④ for details, check with [Preview delete] in ⑥, then [Delete forever] and confirm in the dialog.`,
	},
	"gui.overflow.head": {
		"! 回收站共有 %d 个条目，超过本次「单次扫描上限」%d，快照只建了其中最老的 %d 个" +
			"（按 $I 写入时间排序，约等于删除时间），另有 %d 个未纳入。\n",
		"! The bin holds %d items, more than the Scan cap %d, so the snapshot covers only the oldest %d " +
			"of them (sorted by $I write time, roughly the deletion time); %d left out.\n",
	},
	"gui.overflow.drives": {
		"  各盘 已扫/总数: %s\n",
		"  Per drive scanned/total: %s\n",
	},
	"gui.overflow.two-ways": {
		"  两种继续办法:\n",
		"  Two ways to continue:\n",
	},
	"gui.overflow.option-a": {
		"    A) 一次看全 —— 把「单次扫描上限」调到 %d 以上再重扫\n",
		"    A) See everything at once -- raise the Scan cap above %d and rescan\n",
	},
	"gui.overflow.option-b": {
		`    B) 分批推进（推荐）—— 先删掉本快照里的分区，再重新扫描，剩下的条目会自动进新快照并重新编号`,
		`    B) Work in batches (recommended) -- delete the partitions in this snapshot, then rescan: the rest comes in and gets renumbered`,
	},

	// ---------- 明细表标题 ----------
	"gui.detail.none": {
		`（还没有快照：点【扫描 / 重扫】）`,
		`(no snapshot yet: press [Scan / rescan])`,
	},
	"gui.detail.pick-row": {
		`（还没点行：视图里有 %d 个分区，单击任一行看它的条目明细）`,
		`(no row clicked yet: the view holds %d partitions, click one to see its items)`,
	},
	"gui.detail.empty-view": {
		`（视图里 0 个分区：去掉「只看未删完」或重新扫描）`,
		`(0 partitions in view: untick Only not yet emptied, or rescan)`,
	},
	"gui.detail.no-match-prefix": {
		`分区 %d｜%s｜前缀过滤 %q 之后 0 个`,
		`Partition %d | %s | 0 items left after the prefix filter %q`,
	},
	"gui.detail.ask": {
		`请选择一个分区：在④里单击任意一行，这里列出它的文件清单`,
		`Pick a partition: click any row in ④ and its files show up here`,
	},
	"gui.parts-hint.none": {`（还没有快照）`, `(no snapshot yet)`},
	"gui.parts-hint.line": {
		`当前视图 %d 行｜快照共 %d 个分区（编号 1..%d）｜未删完条目 %d 个（在 %d 个分区里）｜勾选 %d 个分区 / %d 个条目 %s`,
		`View rows %d | snapshot partitions %d (ids 1..%d) | items left %d in %d partitions | ticked partitions %d / items %d %s`,
	},

	// ---------- 只看未删完 ----------
	"gui.alive.off": {
		`已关闭（显示全部编号）`,
		`off (every id is shown)`,
	},
	"gui.alive.on": {
		`已开启（只显示还有未删条目的分区）`,
		`on (only partitions that still hold items)`,
	},
	"gui.alive.nothing-hidden": {
		`只看未删完：%s。本快照里还没有「已删完」的分区（%d 个分区全部还有内容），` +
			`所以视图行数不变。删完一批之后再开，就能看到已清空的分区从列表里消失。`,
		`Only not yet emptied: %s. This snapshot has no emptied partition yet (all %d still hold items), so ` +
			`the row count does not change. Delete a batch and switch it on to watch the emptied rows disappear.`,
	},
	"gui.alive.line": {
		`只看未删完：%s。快照 %d 个分区，隐藏已删完 %d 个，视图 %d 行。`,
		`Only not yet emptied: %s. Snapshot partitions %d, emptied hidden %d, view rows %d.`,
	},
	"gui.alive.checks-lost": {
		`视图换了，原先勾选的 %d 个分区已经取消勾选（留着旧勾选可能删错批次），请重新勾选。`,
		`The view changed, so the partitions you had ticked (%d) are unticked - a stale tick can delete the wrong batch. Tick again.`,
	},

	// ---------- 勾选 ----------
	"gui.select.no-snapshot": {
		`还没有快照，先点【扫描 / 重扫】。`,
		`There is no snapshot yet - press [Scan / rescan] first.`,
	},
	"gui.select.busy": {
		`上一次任务还在跑，请等它结束。`,
		`The previous job is still running, please wait for it.`,
	},
	"gui.select.empty-view": {
		`当前视图是空的（多半是「只看未删完」把分区都筛掉了，或还没扫描），没有可全选的分区。`,
		`The view is empty (Only not yet emptied filtered everything out, or nothing was scanned) - nothing to select.`,
	},
	"gui.select.all-deleted": {
		`视图里 %d 行全都已经删完了，一个也没勾（要清掉这些空分区，重新扫描即可让它们消失）。`,
		`All %d rows are already emptied, so nothing was ticked (a rescan makes those spent partitions disappear).`,
	},
	"gui.select.skipped": {
		`已勾选当前视图的 %d 个分区；另外 %d 个已经没有未删条目，跳过了。`,
		`Ticked the %d live partitions in view; skipped %d that hold no items left.`,
	},
	"gui.select.ok": {
		`已勾选当前视图的 %d 个分区。`,
		`Ticked every live partition in view (%d).`,
	},
	"gui.clear.done": {
		`已清空勾选（原先勾着 %d 个分区）。`,
		`Ticks cleared (there were %d).`,
	},
	"gui.ids.help": {
		`「按编号勾选」里填编号写法，例如 1-5,8-10,16。`,
		`Type ids into Tick by id, for example 1-5,8-10,16.`,
	},
	"gui.ids.applied": {
		`已勾选 %d 个分区。`,
		`Partitions ticked: %d.`,
	},
	"gui.ids.missing": {
		`已勾选 %d 个分区；编号 %s 不在当前视图里（被「只看未删完」或分区上限挡着了），需要的话先取消筛选。`,
		`Partitions ticked: %d; ids %s are not in this view (filtered out by Only not yet emptied, or by the partition cap) - clear the filter if you need them.`,
	},

	// ---------- 预览 / 删除 ----------
	"gui.del.no-tick": {
		`没有勾选分区：在分区表最左列点方框（或选中后按空格）勾上要删的分区，也可以用【按编号勾选】。`,
		`Nothing is ticked: click a box in the left column of the partition table (or press Space on a selected row), or use [Tick by id].`,
	},
	"gui.del.no-match": {
		`没有匹配项：编号 %s 加上前缀过滤 %s 后为空，未做任何改动。`,
		`Nothing matched: ids %s plus the prefix filter %s came out empty; nothing was touched.`,
	},
	"gui.del.no-match-blank": {`(空)`, `(blank)`},
	"gui.del.preview-title": {
		`预览：选中 %d 个分区 / %d 个条目（明细列在下面，未做任何改动）`,
		`Preview: partitions %d / items %d selected (listed below; nothing was touched)`,
	},
	"gui.del.preview-after": {
		`预览未做任何改动。要真删请点【⚠ 永久删除】，再在弹窗里点【确认永久删除】。`,
		`The preview touched nothing. For a real delete press [⚠ Delete forever], then [Delete permanently] in the dialog.`,
	},
	"gui.del.brief-more": {
		`%s…（共 %d 个，完整清单在【预览删除】的日志里）`,
		`%s... (%d in total; the full list is in the preview log)`,
	},
	"gui.confirm.title": {
		`永久删除确认（不可恢复）`,
		`Confirm permanent delete (no way back)`,
	},
	"gui.confirm.head": {
		`⚠ 即将永久删除`,
		`⚠ About to delete for good`,
	},
	"gui.confirm.scale": {
		`%d 个分区 / %d 个条目，约 %s`,
		`Partitions %d / items %d, about %s`,
	},
	"gui.confirm.ids": {`选中编号：%s`, `Selected ids: %s`},
	"gui.confirm.prefix": {
		`（再按前缀 %q* 收窄）`,
		`(narrowed further by the prefix %q*)`,
	},
	"gui.confirm.warn": {
		`这些内容直接从 $Recycle.Bin 抹掉，不进回收站、无法恢复。`,
		`These are erased straight out of $Recycle.Bin - no Recycle Bin, no way back.`,
	},
	"gui.confirm.snapshot": {`快照：%s`, `Snapshot: %s`},
	"gui.confirm.ledger": {
		`逐笔记入同目录的 ledger.csv，删完能核对。`,
		`Every line is booked into ledger.csv in the same folder, so you can check afterwards.`,
	},
	"gui.confirm.hint": {
		`没核对清单的话先【取消】，回④/⑤看明细。`,
		`Have not checked the list? Press [Cancel] and look at ④/⑤ first.`,
	},
	"gui.confirm.create-fail": {
		`确认窗口创建失败: %v`,
		`Could not open the confirm dialog: %v`,
	},
	"gui.del.start": {
		`开始删除 %d 个条目…（每 %d 个刷一次进度）`,
		`Deleting %d items... (progress refreshes every %d)`,
	},
	"gui.del.cancelled": {
		`已取消，未做任何改动。`,
		`Cancelled; nothing was touched.`,
	},
	"gui.del.nothing-done": {
		`未做任何改动。`,
		`Nothing was touched.`,
	},
	"gui.del.interrupted": {
		`删除中断: %v`,
		`Delete interrupted: %v`,
	},
	"gui.del.ok": {
		`删除完成：成功 %d 个，失败 %d 个，账本 %s`,
		`Delete done: deleted %d, failed %d, ledger %s`,
	},
	"gui.del.partial": {
		`删除结束：成功 %d 个，失败 %d 个，账本 %s`,
		`Delete stopped: deleted %d, failed %d, ledger %s`,
	},
	"gui.del.more-errors": {
		`…另有 %d 条失败记录，见账本`,
		`... %d more failures, see the ledger`,
	},
	"gui.del.state-updated": {
		`快照已更新（%s）：标记了本次删掉的条目，编号不变。`,
		`Snapshot updated (%s): the items deleted this time are marked, ids unchanged.`,
	},
	"gui.del.rescan-hint": {
		`要看回收站最新状态请重新扫描：编号按当前内容重建。`,
		`Rescan to see the bin as it is now: ids are rebuilt from the current content.`,
	},

	// ---------- 日志区 ----------
	"gui.log.empty-copy": {
		`日志区是空的，没有可复制的内容。`,
		`The log is empty, there is nothing to copy.`,
	},
	"gui.log.copied": {
		`已复制 %d 行日志到剪贴板。`,
		`The whole log is on the clipboard now (%d lines).`,
	},
	"gui.log.copy-fail": {
		`复制到剪贴板失败: %v`,
		`Copy to clipboard failed: %v`,
	},
	"gui.log.adv-collapsed": {
		`高级参数已收起：桶数上限/分区上限/小组并入/限定盘符/单次扫描上限/删除保险丝/快照目录 仍按里面的值生效。`,
		`Advanced options are collapsed: Max batches / Max partitions / Fold small groups / Drive letters / Scan cap / Delete fuse / Snapshot folder still use the values inside.`,
	},

	// ---------- 打开目录 ----------
	"gui.open.mkdir-fail": {
		`建快照目录失败: %v`,
		`Could not create the snapshot folder: %v`,
	},
	"gui.open.fail": {
		`打开目录失败: %v`,
		`Could not open the folder: %v`,
	},
	"gui.opened": {
		`已在资源管理器里打开: %s`,
		`Opened in Explorer: %s`,
	},

	// ---------- 使用说明 / 关于 / 语言 ----------
	"gui.usage.title": {`rbm 使用说明`, `How to use rbm`},
	"gui.usage.hint": {
		`可以拖窗口边框放大，文字按住左键框选后 Ctrl+C 复制`,
		`Drag the border to enlarge; select the text and press Ctrl+C to copy`,
	},
	"gui.usage.create-fail": {
		`使用说明窗口创建失败: %v`,
		`Could not open the usage window: %v`,
	},
	"gui.about.title": {`关于 rbm`, `About rbm`},
	"gui.about.body": {
		"回收站分批管理工具 rbm（图形界面）\n\n" +
			"内核：$Recycle.Bin\\<当前用户SID> 的 $I 元数据直读 → 时间桶 × 原路径分组 → 编号 → 分批永久删除。\n" +
			"本机盘符 %d 个（%s）；命令行等价用法：rbm scan / parts / show / delete / shell。\n" +
			"删除不可恢复，请配合【预览删除】和「删除保险丝」使用。",
		"rbm - batch-wise Windows Recycle Bin cleaner (GUI)\n\n" +
			"Core: read the $I metadata under $Recycle.Bin\\<your SID> -> time batches x original-path groups " +
			"-> ids -> permanent delete in batches.\n" +
			"Local drives %d (%s); the CLI does the same: rbm scan / parts / show / delete / shell.\n" +
			"Deleting is final - use [Preview delete] together with the Delete fuse.",
	},
	"gui.lang.changed-zh": {
		`界面语言已切换为中文，设置写入 %s`,
		`Interface language is now Chinese; settings written to %s`,
	},
	"gui.lang.changed-en": {
		`界面语言已切换为英文，设置写入 %s`,
		`Interface language is now English; settings written to %s`,
	},
	"gui.lang.write-fail": {
		`语言设置写入失败: %v（本次窗口的文字已经换好，下次启动还会回到原语言）`,
		`Could not save the language choice: %v (this window did switch, the next start falls back)`,
	},
	"gui.usage.body": {
		`rbm 图形界面 = 命令行内核的一层薄壳，两边共用同一份快照文件。

界面按工作流分成六段，从上往下依次操作：

  ① 扫描配置 —— 按工作流只有三样常用项：
     「按时间分批」= 原来的时间桶，下拉里是「自动找删除空档（默认）/ 每 15 分钟一批 / 每天一批 /
     每 7 天一批 / 每 3 个自然月一批 …」，最后一项「自定义」才露出手填框（写 daily、30m、3mo、
     gap:5m、auto 这种原始写法）。
     「桶内分组」= 每批里按什么归堆，四项：目录 dir（默认，按原来所在目录）/ 字符前缀 prefix
     （文件名前几个字符）/ 扩展名 ext / 文件名首段 token（下划线、连字符、点之前那段）。
     只有对当前分组生效的参数才显示：「目录层级」在 目录 dir 下用，「前缀长度」在 字符前缀 prefix /
     文件名首段 token 下用，切分组时另一项会收起。
     其余六项（桶数上限、分区上限、小组并入、限定盘符、单次扫描上限、删除保险丝）
     和快照目录一起收在【▼ 高级参数】里，点开展开、点回收起；收起时值照旧生效。
  ② 扫描结果 —— 【扫描 / 重扫】≈ rbm scan：先按【按时间分批】切批、再按【桶内分组】归堆，
     然后给条目编号，写出 state.json 与 scan_list.csv；右边四个数字是此刻的总体状态
     （分区数 / 对象数 / 未删完 / 已勾选）。改了参数就重扫。
  ③ 选择分区 —— 【全选】勾上当前视图里所有还有内容的分区，【清空勾选】撤销；
     也可以在分区表最左列点方框（选中后按空格也行），按住 Shift 点另一个连选一段，
     或在【按编号勾选】填 1-5,8-10,16 一次勾上一批 ≈ delete 1-5,8-10,16。
     文件名前缀过滤 = 命令行的 --prefix；只看未删完 = --alive，只隐藏「已删完」的分区，
     刚扫完的快照没有已删完分区时它不会改变行数，日志会说明原因。
  ④ 分区列表 —— 单击某一行 ≈ rbm show <编号>，右下方列出该分区的元素。
  ⑤ 元素明细 —— 没选分区时是空的，选了就给 index / 删除时间 / 大小 / 盘 / 文件名 / 原路径。
  ⑥ 删除操作 —— 【预览删除】≈ rbm delete <编号> --dry-run，只列清单不动文件；
     【⚠ 永久删除 N 个分区 / M 个条目】≈ rbm delete <编号> --yes，
     要在这个弹窗里点【确认永久删除】才会真删，逐笔记入 ledger.csv。
     「删除保险丝」填 N：选中条目数超过 N 就整批不动（等价 --max N）。

两张表都能点列名排序：点一下升序，再点同一列降序（表头有箭头）。排序只动显示顺序，
编号和勾选都不变，所以原来的「排序」下拉框已经去掉。
换「只看未删完」会清空勾选（视图变了，留着旧勾选可能删错批次）；点表头排序不清空。

注意：删除是永久删除，不进回收站、不可恢复；编号只在当前这份快照里有效，
重新扫描会按当前内容重新编号。`,
		`The rbm GUI is a thin shell over the command-line core - both use the same snapshot file.

The window follows the workflow in six blocks, top to bottom:

  ① Scan settings - only three of them you touch often:
     [Batch by time] = the old time bucket. The drop-down offers Split at deletion gaps (default),
     every 15 min, every day, every week, every 3 calendar months...; only the last item, Custom,
     shows a text box for the raw token (daily, 30m, 3mo, gap:5m, auto).
     [Group inside] = how each batch is sorted into piles, four choices: dir (default, by the folder
     the file lived in) / prefix (leading characters of the name) / ext (extension) /
     token (the first segment of the name, up to an underscore, hyphen or dot).
     Only the options that actually apply are shown: [Folder depth] is used by dir,
     [Prefix length] by prefix / token, and the other one collapses when you switch.
     The remaining six (Max batches, Max partitions, Fold small groups, Drive letters, Scan cap,
     Delete fuse) plus Snapshot folder live behind [▼ Advanced options]; collapsing them keeps
     the values in effect.
  ② Scan result - [Scan / rescan] is roughly rbm scan: cut the batches, sort each batch into piles,
     number the items, write state.json and scan_list.csv. The four figures on the right are the
     current totals (partitions / objects / left / ticked). Change a parameter, then rescan.
  ③ Pick partitions - [Select all] ticks every partition in view that still holds items,
     [Clear ticks] undoes it; you can also click a box in the left column (or press Space on a
     selected row), Shift+click another row for a range, or type 1-5,8-10,16 into [Tick by id]
     which is the same as delete 1-5,8-10,16.
     [Name prefix filter] = --prefix; [Only not yet emptied] = --alive and hides only the partitions
     that are already deleted - a fresh snapshot has none, so the row count may not change and the
     log says why.
  ④ Partitions - click a row = rbm show <id>, its items appear at the bottom right.
  ⑤ Items - empty until you pick a partition, then index / deleted time / size / drive / name /
     original path.
  ⑥ Delete - [Preview delete] = rbm delete <ids> --dry-run, a list and nothing more;
     [⚠ Delete forever: partitions N, items M] = rbm delete <ids> --yes, and only after you click
     [Delete permanently] in that dialog does anything go; every line is booked into ledger.csv.
     Delete fuse N: if the ticked items exceed N, the whole batch is left alone (= --max N).

Both tables sort by clicking a header: once for ascending, again for the same column for
descending (the arrow shows it). Sorting only changes display order - ids and ticks stay as they
were, which is why the separate sort drop-down is gone.
Switching [Only not yet emptied] clears the ticks (the view changed, and a stale tick could delete
the wrong batch); clicking a header does not.

Careful: deleting is permanent - no Recycle Bin, no way back; ids are only valid inside the current
snapshot, and a rescan renumbers whatever is still there.`,
	},
}

func init() {
	for k, v := range catalogGUI {
		catalog[k] = v
	}
}
