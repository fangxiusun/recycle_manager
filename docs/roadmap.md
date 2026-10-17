# 路线图

已完成的项目都对应仓库里能查到的提交或发布资产；未完成的按当前真实缺口列，不排具体日期。

- [x] **M0 元数据与分批引擎**
  - [x] 解析 `$Recycle.Bin\<SID>` 的 `$I` 元数据（原始路径、删除时间、大小、盘符），
        与 Python 参考实现的样例逐字节比对（`internal/meta`、`internal/parts` 的黄金样例）
  - [x] 两级分批：按删除时间切桶（16 个预设 + 自定义断点），桶内按目录 / 字符前缀 / 扩展名 /
        文件名首段归堆，输出编号分区及条目数、大小
  - [x] 单盘与多盘扫描、逐盘进度、`--max-scan` 上限与超限提示
  - [x] 异常记录：孤儿 `$R`（缺 `$I`）、特殊位置、元数据损坏，点名但不动手
- [x] **M1 命令行**
  - [x] `scan` / `parts` / `show` / `delete` / `shell` 等子命令，九个分批参数与界面参数一一对应
  - [x] 三层删除闸门：`--dry-run` → 确认 → `--max` 保险丝
  - [x] `scan_list.csv`（完整明细，UTF-8 BOM + CRLF）与 `ledger.csv`（每笔删除追加一行）
  - [x] 退出码口径：0 成功 / 1 数据或运行错误 / 2 用法错误、未确认、保险丝断
- [x] **M2 图形界面**
  - [x] 打开即扫描、①–⑥ 六块面板按使用顺序排列、双击不出现控制台窗口
  - [x] 勾选 + Shift 连选 + `1-5,8-10,16` 直接输入 + 【全选】；点行看明细；「只看未删完」
  - [x] 删除进度逐条回报，跑完报「未删完」「剩余」
  - [x] 使用说明独立成可滚动的窗口；日志区跟着最新一行走；主按钮钉宽
  - [x] 删除确认框报出本次规模与分区编号，去掉手打 `DELETE`
- [x] **M3 构建与发布**
  - [x] `build.bat` 三步（windres → `rbm.exe` → `rbm-gui.exe`），`CGO_ENABLED=0`、无 CGO 依赖
  - [x] GitHub Actions 在 Windows runner 出双产物，校验 PE 头 / Subsystem / 内嵌 manifest，打包 zip + `sha256.txt`
  - [x] `.gitattributes` 固定检出换行符，`gofmt` 门不被 runner 的 `core.autocrlf` 顶红
  - [x] 与 Python 样例的比对钉住样例自己的时区，不跟 runner 时区走
  - [x] v1.0.1 发布资产三处对齐复核（release 说明、`sha256.txt`、REST API 的 `digest`）
- [x] **M4 中英双语**
  - [x] 消息目录 `internal/i18n`（中文为源语言，英文为译文），两个语言键齐全、`%d` 必被消费的检查用例
  - [x] 交互层全量双语：GUI 控件、菜单、确认框、日志；命令行九个子命令、帮助与报错
  - [x] 优先级 `--lang` > `RBM_LANG` > `settings.json` 的 `"language"` > 默认中文；未识别的值落到下一级
  - [x] GUI 切语言整窗即时换文字，不重启
  - [x] 落盘内容固定中文（`state.json`、CSV 表头、账本「已删」、兜底组名、时间格式）
- [x] **M5 重绘**
  - [x] 修复「删除跑完表格不立刻变灰，要鼠标逐行划过」：两个表换数据一律补一次重绘
- [x] **M6 文档与许可**
  - [x] `README.md` 简体中文默认门面 + `README.en.md` 英文版（特性、下载与哈希复核、图形界面 / 命令行
        两条快速上手、用哪个可执行文件、语言优先级、从源码构建）
  - [x] `docs/README.md` 索引与仓库结构树、`docs/roadmap.md`（本文件）、`docs/known-issues.md`、
        根目录 `CHANGELOG.md`
  - [x] `LICENSE`（MIT）与 `THIRD-PARTY-NOTICES.txt`（六个 Go 模块的许可证全文）
  - [x] 发布包带上两份 README 与许可文本（`release.yml` 的打包步骤，2026-10-17 改）
- [ ] **M7 还没做的**
  - [ ] 真机验证下载包：双击 `rbm-v1.0.1-windows-amd64.zip` 里的 `rbm-gui.exe` 能否开界面
        （CI 只做到字节层校验，Linux runner 跑不了 PE）
  - [ ] 删除路径的真机验证：目前只在非破坏探测里验证过重绘，真删这一条链没有 A/B 记录
  - [ ] 重绘回归用例：不建窗口的测试拿不到 Win32 绘制行为，需要一个建窗口后再断言的用例或人工步骤
  - [ ] 新的打包步骤（带 `README.en.md` / `LICENSE` / `THIRD-PARTY-NOTICES.txt`）还没真跑过一次发版，
        zip 成员清单要在下一次发版后按[构建与验证](构建与验证.md) §3.7 复核
  - [ ] `THIRD-PARTY-NOTICES.txt` 是手工维护：需要一个「go.mod 里有但声明里没有」的检查，或干脆生成脚本 + CI
  - [ ] 程序里的「关于」窗口不显示许可证（代码里没有许可相关文案），依赖清单也只在本文件与
        `THIRD-PARTY-NOTICES.txt` 里
  - [ ] `rbm-${VER}-sha256.txt` 里两行 exe 带 `dist/` 前缀、zip 成员名不带，发版前加一句 `cd dist`
  - [ ] `internal/meta` 的时区相关比对在非 UTC+8 的机器上是 `t.Skipf`（`meta_parity_test.go:57`），
        应改成按样例自带偏移换算而不是跳过
  - [ ] GUI 冒烟进 CI（构建产物起窗、截图、退出），避免字节层通过但界面起不来
  - [ ] `docs/` 三份中文详解的英文版
  - [ ] 代码签名：未签名产物首次运行会被 SmartScreen 拦一次
