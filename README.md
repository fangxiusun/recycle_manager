# rbm（Go 版）：Windows 回收站分批管理

`recycle_manager/` 那套 Python 工具的同功能 Go 实现：一个不依赖 Python、不依赖 CGO 的
`rbm.exe`，双击是原生 Win32 图形界面（lxn/walk），带子命令是原来的命令行，两边加交互 shell
共用同一个内核。

要解决的问题没变：回收站里堆了几十万个条目时，系统「清空回收站」会长时间无反馈。这里的做法是
直读 `$Recycle.Bin\<当前用户SID>\$I*` 元数据 → 按删除时间切时间桶 → 桶内按原路径目录（或字符
前缀/扩展名/文件名首段）分组 → 每个分区给一个编号并统计元素数与大小 → `show <编号>` 看清单 →
`delete 1-5,8-10,16` 分批永久删除，删一批确认一批。

**平台**：仅 Windows（本机 x86_64 实测）。**只读回收站的元数据，删除是永久删除，不进回收站、
不可恢复。**

---

## 1. 目录结构

```
main.go             17 行：有参数走 CLI，无参数开图形界面
rbm.rc               1 行：把 rbm.manifest 编进 exe
rbm.manifest         asInvoker + Common Controls 6.0 + PerMonitorV2/system DPI
internal/meta/     347 行  $I 头解析、SID/盘符发现、单次扫描上限与截断统计
internal/parts/    794 行  时间桶（含日历月与 3mo 这类 N 月步进）、四种分组维度、编号与重排
internal/store/    233 行  state.json / scan_list.csv / ledger.csv 的落盘格式
internal/engine/   273 行  CLI 与 GUI 共用的内核：建快照、选分区、三层闸门删除
internal/cli/      923 行  scan/parts/show/delete/shell 五个子命令与交互 shell
internal/gui/     1198 行  walk 面板：参数区 + 分区表 + 明细表 + 日志
internal/tty/      146 行  人话字节数、中英文混排宽度、表格与编号区间解析
internal/winfs/    164 行  存在性/属性判断、递归删除、FILETIME 换算
图形界面使用与解析.md  walk 那套窗体系统的构成与用法详解（控件树、模型视图、线程、删除闸门落点、免黑框）
```

判定逻辑（分区、编号、保险丝、账本、剩余计数）全部在 `internal/engine`，`cli` 和 `gui` 只做展示
与确认。所以 GUI 里看到的编号和命令行 `show`/`delete` 用的是同一份 `state.json`、同一套编号。

---

## 2. 构建

环境（本机实测）：Go 1.24.5、`CGO_ENABLED=0`、`GOTOOLCHAIN=local`，依赖只有 `go.mod` 里四个：
`github.com/lxn/walk`、`github.com/lxn/win`、`github.com/mattn/go-runewidth`、
`golang.org/x/sys`（都是纯 Go，无需 C 工具链，只有下面第 1 步的资源编译需要 `windres`）。

在 `rbm_go/` 目录里：

```bat
:: 1) 生成界面资源（必须在主包目录，产物 1228 字节）
windres --input=rbm.rc --input-format=rc --output=rsrc.syso --output-format=coff

:: 2) 编译：控制台子系统（PE subsystem=3）
set CGO_ENABLED=0&& set GOTOOLCHAIN=local&& go build -o rbm.exe .

:: 3) 可选：纯图形版，进程从头就不创建控制台（PE subsystem=2）
go build -ldflags "-H windowsgui" -o rbm-gui.exe .
```

- **第 1 步不能跳。** `go build` 在没有 `rsrc.syso` 时照样成功，但一开界面就失败：实测把
  `main.go`+`internal/` 拷到别处（没有 `rsrc.syso`）编译后运行，立刻退出码 1，stderr 是
  `图形界面创建失败: TTM_ADDTOOL failed`（walk 的控件要 Common Controls 6.0，而 v6 控件只能靠
  manifest 声明）。同一份代码带上 `rsrc.syso` 后运行 4 秒无任何报错。重跑上面 `windres` 命令
  得到的文件和仓库里那份 MD5 相同（`e997c9c9066df5e39c1994d71bbccf17`），所以这一步可复现。
- **两种产物都能当命令行用，区别只在控制台是否被创建过。** 实测用 `cmd /c` 分别调 console 版和
  `-H windowsgui` 版跑 `parts`：两者都被等待、都返回码 0、都输出同样的 124 行（各约 168 ms）。
  console 版双击也不会留黑框：GUI 分支建窗成功后 `internal/gui/gui.go:236` 调 `FreeConsole`
  甩掉系统为双击新建的控制台，实测 3 秒后枚举该进程的可见顶层窗口只有 1 个（1400×860 的主窗口），
  没有控制台属于它；但控制台可能闪一下（系统先建、程序再放），一点都不能闪就用第 3 步的 GUI 版。
  两条完整实测记录见 `图形界面使用与解析.md` §10。
- 自检三条：`go vet ./...`、`gofmt -l internal/`、`go test -count=1 ./...`（用例清单见 §7）。

---

## 3. 图形界面

启动：双击 `rbm.exe` / `rbm-gui.exe`，或在终端里直接运行不带参数的可执行文件。窗口标题
「回收站分批管理 rbm」，默认 1400×860，最小 1080×680，可拖中间的分隔条调整左右表宽度。

**一打开窗口就自动扫描一次**（不读上次那份快照）：界面显示的是回收站此刻的内容，编号也按此刻
内容重建。实测双击新构建的 exe，日志区依次出现「开始扫描…」→「扫描完成（用时 …）：条目 17,402 个
/ 1.8GB，分区 120 个」，两张表随之填满，全程没点过任何按钮（那次扫描没删任何东西，快照里
`deleted` 合计 0）。之后要刷新状态按 F5。自动扫描走的是与点按钮同一条路径，所以**打开窗口就会
把 `<快照目录>\state.json` 和 `scan_list.csv` 覆写成此刻的回收站**；想留住某份旧快照给命令行
`show`/`delete`，先改【快照目录】或备份那个 `.rbm` 文件夹。

界面自上而下（控件树、模型视图、线程与勾选的实现细节见 `图形界面使用与解析.md`）：

| 区域 | 内容 |
| --- | --- |
| 菜单 | 文件(F)：扫描/重扫(F5)、打开快照目录、退出；帮助(H)：使用说明、关于 rbm |
| 分区参数（11 项） | 时间桶、桶内分组、目录层级、前缀长度、桶数上限、分区上限、小组并入、限定盘符、单次扫描上限、删除保险丝、快照目录 |
| 动作行 | 【扫描 / 重扫 (F5)】【预览删除（不动文件）】【永久删除勾选的分区】 |
| 筛选行 | 按编号勾选 + 应用 / 清空勾选、文件名前缀过滤、只看未删完、排序 |
| 主区（左右分栏） | 左：分区统计 9 列（编号/时间范围/路径组/元素数/大小/index区间/盘/剩余/组内构成示例），最左列是勾选框；右：元素明细 6 列（index/删除时间/大小/盘/文件名/原路径）。两边各有一行概览标题（左：视图行数 / 未删完 / 勾选统计；右：当前分区或预览的说明），所以两张表的表头同高（实测 150% 缩放下两个列表窗口顶边同为 `y=727`，窗口高度改到 990/1290/1590 物理像素都还是 727） |
| 底部 | 进度条 + Consolas 9 的只读日志区 |

参数默认值和命令行/Python 版一致：分组 `dir`、目录层级 `2`、前缀长度 `3`、桶数上限 `12`、
分区上限 `120`、小组并入 `5`、单次扫描上限 `50000`、删除保险丝 `0`（不限）、排序「按时间」、
快照目录 `<当前目录>\.rbm`。「时间桶」留空 = 按删除时间断点自动切区（gap）；可填
`daily`/`hourly`/`monthly`/`3mo`/`30m`/`gap:5m`/`auto`。数字框留空按默认值，填非整数或负数会在
日志里回一句「「某某」要填整数，现在填的是 …」。

日常操作五步（和命令树一一对应，【帮助→使用说明】里也是这五步）：

1. 打开窗口即自动扫描一次；【扫描 / 重扫】≈ `rbm scan`：建新快照，写出 `state.json` +
   `scan_list.csv`，日志里给用时、条目数、分区数；超过「单次扫描上限」时打印截断说明和各盘
   「已扫/总数」，并给出两条继续办法（调大上限一次看全 / 删一批再重扫）。
2. 单击分区表某一行（或双击）≈ `rbm show <编号>`：右下方列出这个分区的元素，标题栏写清
   编号、时间范围、组名、元素数/大小、index 区间、盘符。
3. 在最左列点方框勾选要删的分区（选中后按空格也行）；**按住 Shift 再点另一个方框，两点之间
   （含两端）一起勾上**；也可以在「按编号勾选」里填 `1-5,8-10,16` 再点【应用】一次勾上一批。
   编号不在当前视图里（被「只看未删完」挡着）会明确告诉你勾了几个、哪些编号没找到。
4. 【预览删除】≈ `delete --dry-run`：只把选中统计和明细打进日志/右表，不碰文件。
5. 【永久删除勾选的分区】≈ `delete --yes`：先弹确认框（列出条目数、大小、编号、快照路径，
   默认按钮是「否」），确认后才真删，逐笔记入 `ledger.csv`，进度条每 100 条刷一次。

两张表都能**点列名排序**：点一下升序、再点同一列降序（表头有箭头）。分区表的勾选是按编号记的，
所以点表头重排也不会让勾选漂到别的分区上；明细表排序只影响显示顺序。

三层删除闸门是叠加的：**预览**（默认不动手）→ **确认框**（点否就一行「已取消，未做任何改动」）
→ **删除保险丝**（填 N，选中条目数超过 N 就整批不动，且此时账本文件都不会被打开）。

两个容易踩的行为，是设计如此：换「排序」下拉框或切换「只看未删完」会**清空勾选**（视图换了，
留着旧勾选可能删错批次）；点表头排序**不**清空，它只重排当前这批行。删完的分区整行变灰，
「剩余」列给的是 `未删/总数`（本机那个 70 条目的分区显示 `70/70`，删完是 `0/70`，不留空白）。

---

## 4. 命令行

```
回收站分批管理工具（分区编号 → 查看 → 批量删除）

用法: rbm <子命令> [选项]

子命令:
  scan    扫描回收站并按「时间桶 × 原路径目录」分区编号，写出统计
  parts   重列已扫描快照的分区统计
  show    查看某个编号分区内的元素列表（index 区间）
  delete  按编号（如 1-5,8-10,16）分批永久删除分区内容，默认 dry-run
  shell   进入交互 shell：启动即 scan，之后的 show/delete 复用同一份内存快照
```

`rbm <子命令> -h` 看单个命令的选项，`rbm --help` 看上面这段。九个分区参数
（`--bucket --max-buckets --by --dir-level --prefix-len --max-parts --min-count --drives
--max-scan`）与 Python 版同名同义，写法支持 `--name v`、`--name=v`、`-name`、布尔开关 `-v`，
选项和位置参数可以混写，`--` 之后全当位置参数。常用其余选项：

- `parts`：`--alive`、`--sort time|size|count`
- `show`：`--start N --end N`、`--prefix 串`、`--limit N`（默认 40，0 = 全部）、`--tail`（从尾部
  数着显示）、`--local`（序号换成分区内序号从 1 数，默认显示的是全局 index）
- `delete`：`--dry-run` / `--yes`、`--prefix 串`、`--max N`（保险丝）、`--sample N`（默认 15）、
  `--every N`（默认 200 刷一次进度）、`-v`
- 所有命令：`--state-dir 目录`，默认 `<当前目录>\.rbm`

**shell** 是给「几万条要删一上午」准备的：进去先 `scan` 一次，之后的 `show`/`delete` 都用内存里
这份快照，编号不会因为你删了一批就漂走。会话命令是 `scan / parts / show / delete / info /
help / quit|exit|q|退出`。提示符带着当前快照的规模，本机实测是
`rbm[18:24:54 快照 13534条/120区 剩13534]> `；用 `--no-scan` 进去时内存里还没有快照，头一个提示符
是光秃秃的 `rbm> `，等某条命令把快照读进来之后才开始带规模。会话里没写 `--state-dir` 的命令自动
沿用启动时那个目录。`--no-scan` 跳过启动扫描（只读旧快照）。

**退出码**：`0` 成功 / 取消 / dry-run；`1` 数据或运行错误（如没有快照、编号越界、条目数为 0）；
`2` 用法错误、未确认删除、或踩了保险丝。实测例子：`delete 12 --dry-run` → 0（两边一样），
`delete 999` → 1，`show 0` → 1，`delete 12 --sample 5`（确认框等不到输入）→ 2，
`delete 12 --max 1`（保险丝拦下）→ 2，`parts --sort 坏值` → 2。

**产物三件套**（都在快照目录里）：`state.json` 快照本体（可被 Python 版直接打开）；
`scan_list.csv` 本次快照的全量清单，带 UTF-8 BOM、CRLF 行尾，Excel 双击即开；`ledger.csv`
删除账本，每删一条追加一行（操作时间/编号/index/删除时间/字节/盘/文件名/原路径/结果），只在
真删时创建。

---

## 5. 与 Python 版对齐到什么程度（证据）

下面所有数字都是 2026-10-15 在本机同一块真实回收站上跑出来的比对，全部只读
（`scan`/`parts`/`show` 加 `delete` 的 dry-run/确认失败路径；所有比对之后 `state.json` 里
`deleted` 合计为 0、无 `last_delete`、没有生成 `ledger.csv`）。

**同一份内容**：13,534 个条目 / 1.6GB / 120 个分区 / 13 个时间边界，两边完全一致。

| 比对项 | 结果 |
| --- | --- |
| `scan` 的 stdout | 127 行 vs 127 行；指向同一个 `--state-dir` 时只有 2 行不同：`快照时间` 的秒数，以及 `完整清单已写出 …` 的路径写法（见 §6） |
| `state.json` | 解析后逐键相等：`params`/`bounds`/`items`(13534)/`parts`(120)/`issues` 全部相等，条目对象数组不同的条数为 0 |
| `scan_list.csv` | 逐字节相同：3,725,731 字节，BOM 与 12,990 个 CRLF 都对得上。另一轮 12,989 条目时也是 3,574,008 字节全等 |
| 闸门类命令 | `delete 12 --sample 5`、`delete 999`、`delete 12 --max 1`、`show 0` 四条的 stdout+stderr 与退出码逐字节相同 |
| 快照互开 | 拿 Go 刚写出的 `state.json` 让两边各自 `parts`：返回码都是 0，124 行输出 0 行不同（Python 完全读得懂 Go 写的紧凑 JSON） |
| `delete --dry-run` | 两边都是返回码 0，都只打清单不碰文件 |

**三份黄金样例**都由 Python 代码现场生成、Go 测试逐字段比对（合成数据，不含任何真实回收站内容）：

- `internal/parts/testdata/py_parts.json`（1,118,187 字节，5 组数据 / 49 份配置）：A 组跨 500 天、
  含 2% 损坏条目，覆盖隐式断点、日历月与季度、上限合并；B 组跨 2 天、无损坏条目，覆盖分钟级显式
  边界与四种分组维度。
- `internal/store/testdata/py_state.json`（63,253 字节，150 条目 / 18 分区 / 7 边界 / 1 条异常）：
  由 `rbmgr.py` 的 `build_state` + `save_state` 原样写出，所以字段名和类型是 Python 的真实口径。
- `internal/meta/testdata/py_meta.json`（3,140 字节，11 个正常用例 + 4 个报错用例）和
  `testdata/isample/` 里 15 个 `$I` 原件：Go 读同一批字节，比对 `meta_version`/`size`/
  `deleted_at`/`epoch`/`orig_path`/`display`/`unrecoverable`。

**耗时**（本机，同一块回收站 13,534 条目，各跑三轮，含 Python 解释器启动）：

| 命令 | Python | Go |
| --- | --- | --- |
| `scan`（全量元数据解析 + 分区 + 写两份产物） | 2210 / 2341 / 2401 ms | 1514 / 1550 / 1561 ms |
| `parts`（读快照重列） | 125 / 114 / 110 ms | 110 / 111 / 122 ms |

也就是说首次解析元数据那一段 Go 大约快三成到四成；而只读快照的命令两边都是 ~0.11 s，差别不明显
（瓶颈在快照文件本身）。

---

## 6. 有意的差异

1. **非法时间桶写法：Go 明确拒绝，Python 会挂死或静默改写。** 本机实测 Go：`0m`/`0h`/`0d`/`0mo`/`5x`
   → `无法识别的时间桶: …（可用 minutely/hourly/daily/weekly/monthly/Nm/Nh/Nd/Nmo/auto/gap[:30m]）`，
   退出码 1；`gap:0m` → `粒度必须大于 0: 0m`；`gap:abc` → `无法识别的粒度单位 "c"（只能用 s/m/h/d）`。
   Python 侧同样这些写法：`0m/0h/0d/0mo` 让 `time_bounds` 原地打转（6 秒超时仍未结束，exit 124），
   `gap:0m` 被当成普通 `gap`（边界数 4，无声改变结果），`5x`/`gap:abc` 抛 `ValueError`/`KeyError`。
   合法写法两边行为一致：`minutely hourly daily weekly monthly 3mo 30m gap:5m auto` 九个都正常出 127 行统计。
2. **用法错误的文案。** 退出码相同，文案不同：Go 打一行中文（`rbm parts: error: 选项 --sort 的取值无法识别: 坏值`），
   Python 打 argparse 的整段 usage + `invalid choice`。`show 12 --tail 3` 这类多余参数同理
   （Go：`需要且只需要一个编号参数`；Python：`unrecognized arguments: 3`）。
3. **`--state-dir` 的写法会被 Go 归一化。** Go 用 `filepath.Join`，`G:/tmp/x` 会变成
   `G:\tmp\x\scan_list.csv`；Python 的 `os.path.join` 原样拼接，输出 `G:/tmp/x\scan_list.csv`。
   这就是上面「同一目录只有 2 行不同」里的第二行，只影响这一行提示文案，不影响读写的目录。
4. **`state.json` 的文本形态不同，但内容等价。** Go 是紧凑 JSON 并把 `<` `>` `&` 转义成
   `\u003c` 等；Python 是 `ensure_ascii=False` 且逗号冒号后带空格。同一份 13,534 条快照，
   字节数 6,390,085（Go）比 6,705,382（Python）小，而解析后逐键相等，两边的快照可以互相打开。
5. **不可表示的删除时间。** FILETIME 为 0 或明显离谱时，Python 走 Windows 本地时间转换会抛
   `OSError`，条目落进「元数据损坏」哨兵值。Go 不依赖异常，用同一条区间判掉：
   epoch 落在 `[86400, 32536713599]` 秒（本机实测 Python 能渲染的窗口，约 1970-01-02 至
   3001-01-18）之外就按同一套哨兵值处理。`py_meta.json` 里 4 个报错用例专门钉住这条。
6. **原路径只有一个盘符时**（`D:`），`ntpath.basename` 给空串，Python 的 display 退回 `$I` 文件名的
   尾巴；Go 的 `baseName` 对光秃盘符返回空串，从而走同一个退回分支。
7. **时区实现方式不同。** Go 在启动时取当前 UTC 偏移构造 `FixedZone`，Python 用
   `datetime.astimezone()`。本机 `TimeZoneKeyName = China Standard Time`、
   `GetTimeZoneInformation` bias = −480 且无夏令时，两边完全一致。

---

## 7. 测试与再生成

`go test -count=1 ./...`：**65 个顶层用例 + 75 个子用例**，全绿；`go vet ./...` 与
`gofmt -l internal/` 干净。分布与覆盖点：

| 包 | 顶层用例 | 主要覆盖 |
| --- | --- | --- |
| `internal/cli` | 12 | 默认值（含 `--max-scan` 50000）、十一种选项写法解析、位置参数、shell 分行与继承快照目录、无快照时的退出码、EOF 干净退出、提示符内容 |
| `internal/engine` | 10 | 建快照的参数回写与空回收站、`SortedParts` 的 `--alive` 剔除已删完、`SelectedParts` 按 positions 重算大小与前缀收窄、`MatchPrefix` 忽略大小写、`DeleteOne` 单文件与目录树、保险丝拦下时一条都不动 / 正好等于上限时不拦、删完标记快照并逐笔记账本、`-v` 逐条回报 |
| `internal/gui` | 6 | 表头排序的 9/6 列比较与并列退到编号、同一列排两次结果一致、勾选按编号记所以重排后不漂、Shift 范围勾选（含锚点不在视图时不连选）、没按 Shift 不连选、换视图清空勾选并复位排序 |
| `internal/meta` | 12 | `$I` v1/v2 头解析、超长尾部的损坏判定、display 取名、`$I`/`$R` 配对与异常记录、`--max-scan` 截断与各盘计数、空目录、FILETIME 换算往返，外加与 `py_meta.json` 的逐字段比对（含 4 个不可表示时间的报错用例） |
| `internal/parts` | 11 | 时间桶写法接受/拒绝、分钟级边界对齐、断点边界覆盖全体、日历月桶跨月、分区内排序、标签与实际内容相符、损坏条目单独成组、目录分组同名合并，加上与 `py_parts.json` 的 49 份配置逐字段比对 |
| `internal/store` | 8 | 读写往返、缺文件/坏 JSON 的报错文案、能直接读 Python 生成的 `py_state.json`（字段名与类型口径）、`GroupCol`/`AliveLeft`、全量清单 CSV、账本表头与追加、`--drives` 解析、默认快照目录是当前目录的 `.rbm` |
| `internal/tty` | 6 | 字节数人话、东亚宽度算列、截断、表格排版这四项直接对着 Python 的输出比；另两项是补位与编号区间 `1-5,8-10,16` 的解析与越界 |

测试的安全边界：只用自建的假 `$I` 文件和临时目录。假回收站走 `meta.BinDir{Drive:"Z",Path:tmp}`
这类注入，不经命令行（`--drives` 只认真实盘符），所以测试永远不会删到真实条目，也不会覆盖谁的
`.rbm`。`internal/gui` 的用例更窄：只直接调模型的排序/勾选方法，既不建窗口也不碰文件。

想重跑交叉比对（需要本机有 Python，脚本自己找 `recycle_manager/` 的路径，在仓库任何位置都能跑）：

```bat
python internal\parts\testdata\regen_py_fixture.py
python internal\store\testdata\regen_py_state.py
python internal\meta\testdata\regen_py_meta.py
```

`regen_py_meta.py` 会把生成时的本机 UTC 偏移记进 `py_meta.json`；`meta` 的比对测试发现偏移变了就
自动跳过，避免在别的时区上误报。

---

## 8. 已知限制

- 只看当前用户 SID 名下的 `$Recycle.Bin\<SID>`，不处理别的 SID 目录，也不处理 `$R` 缺 `$I` 的孤儿
  （这类会在异常记录里点名，本次不动）。
- 盘符来自 `GetLogicalDrives` 的 A–Z 掩码；`--drives` 只接受真实存在的盘符字母。
- 时间分区按本机时区渲染。Go 侧是启动时的固定偏移，跨夏令时的历史条目会被按当前偏移解释
  （本机无夏令时，见 §6 第 7 条）。
- 编号只在当前快照内有效：重新扫描会按当时内容重新编号，已删完的条目不会再出现。
- 单次扫描默认最多 5 万条，超出时只取最老的那些建快照，并把「共多少、取了少、各盘已扫/总数、
  两条继续办法」写清楚，绝不静默丢弃；`--max-scan 0` 为不限。
- 删除是永久的：直接抹掉 `$R` 数据与 `$I` 元数据，不经过回收站，删了就没了。请配合
  【预览删除】/`--dry-run` 和「删除保险丝」/`--max` 使用，明细永远在 `scan_list.csv`，
  每一笔结果都在 `ledger.csv`。
