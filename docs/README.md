# 文档

- [README（简体中文）](../README.md) — 项目主页，从这里开始
- [README (English)](../README.en.md) — 英文版
- [更新日志](../CHANGELOG.md) — 各版本的用户可见变更
- [路线图](roadmap.md) — 已完成与还没做的
- [已知问题](known-issues.md) — 用起来会撞到的限制及其原因

## 详解

- [选项说明](选项说明.md) — ① 参数区逐项：16 个分批预设、四种桶内分组、高级参数、各自什么时候生效
- [图形界面使用与解析](图形界面使用与解析.md) — 控件树、模型 / 视图、线程、三层删除闸门落在哪、
  为什么双击不出现控制台窗口、窗口内切语言怎么做到整窗即时换文字、排查表
- [构建与验证](构建与验证.md) — 三步构建与 `rsrc.syso` 为什么不能省、发布流水线、发布物复核、
  命令行全参考与退出码、语言优先级链与刻意保持中文的内容、分区口径、扫描分段计时与提速的 A/B、
  测试清单、已知限制

## 仓库结构

```
README.md · README.en.md · CHANGELOG.md · LICENSE · THIRD-PARTY-NOTICES.txt
build.bat                 三步构建：windres → rbm.exe → rbm-gui.exe
main.go                   入口：有子命令走命令行，没有则开图形界面
internal/
  meta/                   $I 元数据解析、盘符与回收站定位、分段计时（含与 Python 参考实现的样例比对）
  parts/                  两级分批：时间切桶 + 桶内归堆
  engine/                 扫描、删除、进度与保险丝
  store/                  快照 state.json、明细 scan_list.csv、账本 ledger.csv
  cli/                    九个子命令与参数
  gui/                    walk 窗体、模型 / 视图、整窗换文字
  i18n/                   中英消息目录与语言优先级
  tty/                    控制台相关
  winfs/                  Windows 文件系统细节（回收站目录名、盘类型、FILETIME）
docs/                     本目录
.github/workflows/        release.yml：Windows runner 出双产物并校验 PE 头
```

代码里注释使用中文；界面与命令行的文案不写在代码里，统一走 `internal/i18n` 的消息目录。
