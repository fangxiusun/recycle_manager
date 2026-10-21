<div align="center">

# rbm

**把塞满的 Windows 回收站拆成可审阅的批次，一批一批清空**

**简体中文** · [English](README.en.md)

[![最新版本](https://img.shields.io/github/v/release/fangxiusun/recycle_manager?label=%E6%9C%80%E6%96%B0%E7%89%88%E6%9C%AC&color=2f80ed)](https://github.com/fangxiusun/recycle_manager/releases)
[![平台](https://img.shields.io/badge/%E5%B9%B3%E5%8F%B0-Windows%20x64-0078D4)](#-系统要求)
[![许可证](https://img.shields.io/badge/%E8%AE%B8%E5%8F%AF%E8%AF%81-MIT-2ea44f)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.24-00ADD8?logo=go&logoColor=white)](#-从源码构建)

[为什么做这个](#-为什么做这个) · [⚠️ 它会怎么动你的数据](#️-它会怎么动你的数据) · [特性](#-特性) · [下载](#-下载) · [快速上手](#-快速上手) · [文档](#-文档) · [致谢](#-致谢)

</div>

<br>

rbm 读回收站自己的元数据（`$Recycle.Bin\<你的 SID>\$I*`），把读到的东西按删除时间切成时间桶、
桶内再按原始目录归堆，编成一批一批的**分区**，然后让你**一次删一批**。每一批都有条目数、总大小、
进度、完整明细（`scan_list.csv`）和逐笔记账的删除账本（`ledger.csv`）。
一个静态可执行文件搞定：不需要 Python，不用 CGO，也没有安装程序。

双击它 = 原生 Win32 图形界面（基于 [`lxn/walk`](https://github.com/lxn/walk)）；带子命令运行 = 命令行；
两边共用同一个引擎和同一份快照文件，所以「3 号分区」在两边指同一批东西。

## 🧭 为什么做这个

作者日常用 agent 做 vibe 编程。agent 成千地重写、重新生成文件，被它删掉的文件并不会消失——
全堆在回收站里。在这台机器上堆到了**几十万对象**。这个体量下，系统自带的「清空回收站」
会跑很久而且全程没有任何反馈，同时资源管理器卡死并自己重启一次。
**这是作者自己机器的情况，不是对每台 Windows 的断言**——但足够让人想要一个看得见、能指挥、
能中断续上的清空方式。

于是把一次盲目的「全清掉」换成：

```
$Recycle.Bin\<你的 SID>\$I*   →   按删除时间切桶
                                →   桶内按原始目录归堆
                                →   编号分区
                                →   show 3        看清这一批
                                →   delete 1-5,8-10,16   一次删几批，每批自己确认
```

几十万对象于是变成一连串小而无聊、看得见、能中途停下的操作，而不是一块撞不动的几小时。

## ⚠️ 它会怎么动你的数据

> [!WARNING]
> **删除是永久的。** rbm 直接抹掉 `$R` 数据与 `$I` 元数据，不回回收站，删了就没了。

- 扫描是**只读**的：只解析当前用户的 `$Recycle.Bin\<SID>` 元数据。
- 动手之前有三道叠加的闸门：**预览 / `--dry-run`**（默认，不碰文件）→ **确认框**（报出这一批的
  条目数、大小和分区编号）→ **删除保险丝 / `--max N`**（选中的条目数超过 N 就整批拒绝，
  连账本文件都不会打开）。
- 事后全在磁盘上：`scan_list.csv` 是本次快照的完整明细，`ledger.csv` 每一笔删除尝试追加一行结果。

## ✨ 特性

| | 功能 | 说明 |
|:---:|---|---|
| 🗑️ | **回收站元数据** | 解析 `$I` 拿到原始路径、删除时间、大小、所在盘；孤儿 `$R`（缺 `$I`）、特殊位置、元数据损坏都点名进异常记录，但本次不动。 |
| 📊 | **两级分批** | 先按删除时间切桶（16 个预设，最后一项自由填写），桶内再按目录 / 字符前缀 / 扩展名 / 文件名首段归堆，编号成批。 |
| 🔍 | **看得见** | 每批的条目数与大小、点一行看这批的内容、进度逐条回报、跑完报「未删完」「剩余」；扫描还会**分段计时**打进日志（九个阶段 + 逐条计数 + 每盘一行 + 自动枚举跳过了哪些盘），慢在哪一段一眼就看到。 |
| 🛡️ | **三道闸门** | 预览 / `--dry-run` → 确认框 → `--max` 保险丝，默认不动文件。 |
| 📄 | **留痕** | `scan_list.csv`（完整明细，UTF-8 BOM + CRLF，双击 Excel 打开）与 `ledger.csv`（逐笔结果）。 |
| 🖱️ | **图形界面** | 双击 `rbm-gui.exe` 全程不出现黑色控制台窗口；打开即扫描；勾选、<kbd>Shift</kbd> 连选、直接输入 `1-5,8-10,16`、【全选】。 |
| ⌨️ | **命令行** | `scan` / `parts` / `show` / `delete` / `shell`，九个分批参数与界面参数一一对应；`shell` 扫一次复用内存快照，编号不漂移。 |
| 🌍 | **中英双语** | 默认中文，英文一键或一参数可达；界面、菜单、确认框、帮助、报错全双语。 |
| 📦 | **单文件** | 静态可执行，`CGO_ENABLED=0`，六个依赖，解压即用。 |

## 📦 下载

最新版本的下载地址：**[v1.0.2](https://github.com/fangxiusun/recycle_manager/releases/tag/v1.0.2)**；
全部版本见 <https://github.com/fangxiusun/recycle_manager/releases>。下面这张表复核的是 **v1.0.2**
那一份资产（2026-10-17 13:46 发布后重新下载、逐字节读 zip 成员算出来的），
出新版本时请按[构建与验证](docs/构建与验证.md) §4 重新取一遍数字，别沿用。

| 文件 | 说明 |
|---|---|
| `rbm-v1.0.2-windows-amd64.zip` | 解压即用：`rbm-gui.exe`（双击开界面）、`rbm.exe`（命令行与脚本）、`README.md`、`README.en.md`、`LICENSE`、`THIRD-PARTY-NOTICES.txt`。 |
| `rbm-v1.0.2-sha256.txt` | 上述 zip 与两个 `.exe` 的 SHA-256。 |

| 资产 | 字节 | SHA-256 |
|---|---:|---|
| `rbm-v1.0.2-windows-amd64.zip` | 9 866 736 | `baf9132c46e6dc93682aaf20355db305fe55bd610e16191df18bfb00b78a4727` |
| `rbm-v1.0.2-sha256.txt` | 257 | `c0cc02e2576202bccd1007a0a81ba1ed421775866452445411fb7e5ba97436d4` |

zip 的六个成员（同一轮逐个读出，两个 exe 各自 9 777 152 字节）：

| 成员 | 字节 | PE | SHA-256 |
|---|---:|---|---|
| `rbm.exe` | 9 777 152 | PE32+，Subsystem **3**（控制台），manifest 命中 1 | `10ba8455c361abd9430222bff10557b6ebd430b9fb6d218e91edb22fbb66eaf7` |
| `rbm-gui.exe` | 9 777 152 | PE32+，Subsystem **2**（GUI），manifest 命中 1 | `e225b7102c57e6032c2f37a88bbcd976ad2fc492610c15395e46fe18e5a959bb` |
| `README.md` | 16 328 | — | `70346bdf5b6d4673cfd64cdec5b6f32788248be62d7cc08ee9543abc05ebfbca` |
| `README.en.md` | 18 473 | — | `db2fa7addae4fe8f26496bfeca40188a4c54515dc505981da9d92308d9bccb7b` |
| `LICENSE` | 1 073 | — | `85b95532ceba0391b252d1bf320581878cbb8576c1f1d180db0d4fde11e1a729` |
| `THIRD-PARTY-NOTICES.txt` | 9 749 | — | `50f9879cd16d81595da3323441b5f1d0f80b478eee570e407f4a7b73f4237a89` |

> [!NOTE]
> - 四个文本成员与仓库里对应文件**逐字节相同**（同一轮用 `sha256sum` 对过）。
> - `rbm-v1.0.2-sha256.txt` 里两行 exe 的名字仍写着 `dist/rbm.exe` / `dist/rbm-gui.exe`，而 zip 存的成员名
>   不带目录——**按哈希对齐，别按文件名**。细节见[构建与验证](docs/构建与验证.md) §3.7、§4。
> - 程序尚未做代码签名，首次运行 Windows SmartScreen 可能提示「未知发布者」。
> - 上一版 v1.0.1 的 zip 只有三个成员（`rbm.exe`、`rbm-gui.exe`、`README.md`），许可文本从 v1.0.2 起才带上。

```powershell
Get-FileHash .\rbm-v1.0.2-windows-amd64.zip -Algorithm SHA256   # 应与上表第一行一致
```

## 💻 系统要求

- **Windows x86_64**，且仅此一个平台：`internal/gui` 依赖 `lxn/walk` 与 `golang.org/x/sys/windows`，
  这些文件带 `//go:build windows`，整个包在 Linux、macOS 上编译不过。
- 无运行期依赖，全部纯 Go、`CGO_ENABLED=0`：四个直接依赖
  （`lxn/walk`、`lxn/win`、`go-runewidth`、`golang.org/x/sys`）+ 两个间接依赖。
- 只管**当前用户**的 `$Recycle.Bin\<SID>`。别人的回收站、以及缺 `$I` 的孤儿 `$R` 会进异常记录、
  不会被动到——想管谁的就用谁登录再跑。

## 🚀 快速上手

### 图形界面

1. 解压，**双击 `rbm-gui.exe`**。全程不会出现控制台窗口（用 `-H windowsgui` 构建，加载器不分配控制台）。
2. 窗口**打开即扫描**，等那行带耗时与分区数的日志。
3. 在 ③ 选批次：勾选（<kbd>Shift</kbd> 点击连选）、直接输入 `1-5,8-10,16`，或【全选】取还有未删条目的分区。
4. 点 ④ 的一行，在 ⑤ 看这批的内容。
5. 先按【预览删除（不动文件）】，再按【⚠ 永久删除 N 个分区 / M 个条目】并在确认框里确认。
6. 看「未删完」/「剩余」两列，<kbd>F5</kbd> 重扫，吃下一批。

六块面板按使用顺序编号：① 扫描选项、② 扫描 + 概览、③ 选择、④ 分区表、⑤ 条目明细、⑥ 预览 / 删除 + 进度。
分批策略在「按时间分批」下拉（16 个预设，最后一项自由填写），桶内归堆在「桶内分组」。

开始前要知道两件事：

- 打开 GUI 会**覆盖**快照目录（默认 `<cwd>\.rbm`）里的 `state.json` 和 `scan_list.csv`。
  要留住的快照先在【▼ 高级参数】里把快照目录指到别处。
- 打开「只看未删完」会**清空你的勾选**——视图变了，留着旧勾选就会删错批。点表头排序**不会**清勾选。

### 命令行

脚本里一律用 `rbm.exe`（见下面「用哪个可执行文件」）。

```bat
rbm.exe scan                        :: 建快照，写 state.json + scan_list.csv
rbm.exe parts                       :: 列出这份快照的编号分区
rbm.exe parts --alive --sort size   :: 只列还有剩余条目的，按大小排
rbm.exe show 3                      :: 列 3 号分区里的条目
rbm.exe delete 1-5,8-10,16          :: 预演：打印会删什么，不碰文件
rbm.exe delete 1-5,8-10,16 --yes    :: 永久删除这几批
rbm.exe shell                       :: 交互：扫一次后复用内存里的同一份快照
rbm.exe --help                      :: 子命令；rbm.exe <子命令> -h 看它的参数
```

九个分批参数（`--bucket --max-buckets --by --dir-level --prefix-len --max-parts --min-count
--drives --max-scan`）与图形界面参数区一一对应；每条命令都收 `--state-dir`（默认 `<cwd>\.rbm`）
和 `--lang zh|en`（见[语言](#语言)）。**退出码**：`0` 成功 / 取消 / 预演 / 回收站是空的；
`1` 数据或运行错误（没有快照、编号越界、断点写法不认识、指定盘上没有本 SID 的回收站）；
`2` 用法错误、删除未确认、或 `--max` 保险丝断了。

`shell` 是几十万对象该用的模式：扫一次把快照留在内存里，你逐批删除时分区编号不会漂。

### 用哪个可执行文件

| | `rbm-gui.exe` | `rbm.exe` |
|---|---|---|
| 构建方式 | `-ldflags "-H windowsgui"` | 默认 |
| PE Subsystem | 2（GUI） | 3（控制台） |
| 用于 | 双击；永远不出现控制台窗口 | 命令行、脚本、管道 |
| 控制台 | 加载器不分配 | 有；界面路径在窗口打开后调 `FreeConsole` |
| 报错 | 没有终端可打印 | stderr 进终端——排查用这个 |

`rbm-gui.exe` 也能跑子命令，但 PowerShell 不会等 GUI 子系统的进程：拿它接管道或捕获输出可能被截断。
脚本里留在 `rbm.exe` 上。

### 语言

中文是默认，英文一步可达。优先级从高到低：

| 方式 | 作用范围 |
|---|---|
| `--lang en`：`rbm.exe parts --lang en`，或 `rbm.exe --lang en` 不带子命令直接以英文开界面 | 当前进程 |
| `set RBM_LANG=en` | 继承该环境变量的进程 |
| 图形界面 视图 → 界面语言 → 英文（替你写文件），或 `%LOCALAPPDATA%\rbm\settings.json` 的 `"language"` | 常驻默认 |
| 以上都没有 | 中文 |

认不出的值落到下一级，不报错。界面里切换会立刻重贴整个窗口的文字，不需要重启，并把写入的设置文件记进日志；
日志里已经写下的那些行保持写下时的语言。

**两种语言下都保持中文的**：所有落盘的东西——`state.json` 的字段名与取值（含记录的断点规格
`gap:自动断点(<= 12 桶)`）、`scan_list.csv` 与 `ledger.csv` 的中文表头、账本结果值「已删」，
以及喂给这两个文件的分组标签「其他」「全部」「元数据损坏」「特殊位置」「(根)」「(无扩展名)」。
这些是数据不是界面：换标签会让磁盘上已有的快照失效，也会让与 Python 参考实现的逐字节比对失配。

## 🔧 从源码构建

```bat
build.bat
```

就是这三步（任一步失败退出 1）：

```bat
windres --input=rbm.rc --input-format=rc --output=rsrc.syso --output-format=coff
set CGO_ENABLED=0&& set GOTOOLCHAIN=local&& go build -o rbm.exe .
go build -ldflags "-H windowsgui" -o rbm-gui.exe .
```

Go 1.24.5、`CGO_ENABLED=0`；Go 之外唯一的工具链部件是 `windres`（MinGW binutils）。
**第 1 步不能跳**：没有 `rsrc.syso` 时 `go build` 照样成功，然后界面一开就死在
`图形界面创建失败: TTM_ADDTOOL failed`——Common Controls 6.0 只能通过内嵌 manifest 申请。

推送前的检查：`go vet ./...`、`gofmt -l internal/`、`go test -count=1 ./...`
（截至 2026-10-17 是 93 个顶层用例 + 75 个子用例，全绿；用例只用合成的 `$I` 样例和临时目录，
所以 `go test` 从不碰真实回收站，也不需要 Python。其中 10 个用例检查消息目录本身：
每个键两种语言都得有，串里的每个 `%d` 都得真被消费掉）。

## 📚 文档

`docs/` 下的三份详解是中文写的，引用的是中文界面标签（中文即默认语言，见上面的「语言」）。

| 文档 | 内容 |
|---|---|
| [文档索引](docs/README.md) | 全部文档的入口与仓库结构 |
| [选项说明](docs/选项说明.md) | ① 参数区逐项：16 个分批预设、四种分组方式、高级参数、各自什么时候生效 |
| [图形界面使用与解析](docs/图形界面使用与解析.md) | 控件树、模型 / 视图、线程、三道删除闸门落在哪、为什么双击不出现控制台、窗口内切语言怎么做到整窗即时换文字、排查表 |
| [构建与验证](docs/构建与验证.md) | 构建步骤与 `rsrc.syso` 为什么不能省、GitHub Actions 发布流水线、发布物复核、命令行全参考与实测退出码、语言优先级链与刻意保持中文的内容、分区口径与校验、测试清单、已知限制 |
| [路线图](docs/roadmap.md) | 已完成与还没做的 |
| [已知问题](docs/known-issues.md) | 当前版本的限制 |
| [更新日志](CHANGELOG.md) | 各版本的用户可见变更 |

## 📌 已知限制（摘要）

- 只有 Windows；只看当前用户的 SID；盘符来自 `GetLogicalDrives` 的掩码。
  **自动枚举会跳过网络盘、光盘和没有根目录的盘符**（一个失联的映射盘能把扫描拖住二十多秒）；
  要扫映射盘上的回收站就显式写 `--drives y`。
- 分区编号只在**当前快照**内有效——重新扫描会重新编号。
- 时间桶按启动时取到的本机偏移渲染，跨夏令时的历史条目按今天的偏移解释。
- 扫描到 `--max-scan`（默认 50 000）就停，并保留**最老**的那些；一定会打印共多少、取了多少、
  各盘进度——不静默丢弃。`--max-scan 0` 为不限。
- 删除是永久的（见上面的警告）。
- 界面只有中文和英文两种，`docs/` 下三份详解只有中文。

完整清单见[已知问题](docs/known-issues.md)。

## 🗺️ 路线图

- [x] **M0–M3**：元数据解析与两级分批、命令行、图形界面、构建与发布流水线
- [x] **M4–M5**：中英双语（含整窗即时切换）、删除后表格立即重绘
- [x] **M6**：中英文两份 README、文档索引 / 路线图 / 已知问题 / 更新日志、MIT 许可与第三方声明全文
- [x] **M7**：扫描分段计时进日志（卡点当场看得见）、三处提速（失联映射盘不再拖死扫描）
- [ ] **M8**：下载包的真机双击验证、删除链路真机 A/B、重绘回归用例、GUI 冒烟进 CI、文档英文版、
  `$I` 增量缓存

详情见[路线图](docs/roadmap.md)。

## 🤝 参与贡献

问题与拉取请求都走 [GitHub 仓库](https://github.com/fangxiusun/recycle_manager)。改代码前请先跑
`go vet ./...`、`gofmt -l internal/`、`go test -count=1 ./...`（用例只用合成样例和临时目录，
不会碰真实回收站）。新增文案请同时补两种语言的消息目录条目——
`internal/i18n` 里有用例检查「每个键两边都得有、每个 `%d` 都得被消费」。

## 🙏 致谢

rbm 构建在下列开源项目之上，在此致谢：

| 项目 | 用途 |
|---|---|
| [`lxn/walk`](https://github.com/lxn/walk) | Win32 图形界面框架 |
| [`lxn/win`](https://github.com/lxn/win) | Windows API 的 Go 绑定 |
| [`mattn/go-runewidth`](https://github.com/mattn/go-runewidth) | 命令行输出的字符宽度对齐（`internal/tty` 把 `EastAsianWidth` 锁死，避免同一份快照随系统 locale 错位） |
| [`golang.org/x/sys`](https://golang.org/x/sys) | 回收站目录枚举、盘符掩码等系统调用 |
| [`clipperhouse/uax29/v2`](https://github.com/clipperhouse/uax29) | 间接依赖：`go-runewidth` 使用的 Unicode 字素宽度表 |
| [`Knetic/govaluate`](https://github.com/Knetic/govaluate) | 间接依赖：`lxn/walk` 的数据绑定用表达式求值 |

全部第三方组件及其许可证全文见 [THIRD-PARTY-NOTICES.txt](THIRD-PARTY-NOTICES.txt)。

感谢 [LINUX DO](https://linux.do/) 社区的支持。

## 📄 许可证

rbm 依据 [MIT 许可证](LICENSE) 发布。

<div align="center">
<br>
<sub>rbm · Windows 回收站分批清理 · © 2026 rbm contributors</sub>
</div>
