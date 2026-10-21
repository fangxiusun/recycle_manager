<div align="center">

# rbm

**Empty a stuffed Windows Recycle Bin in reviewable batches**

[简体中文](README.md) · **English**

[![Latest release](https://img.shields.io/github/v/release/fangxiusun/recycle_manager?label=latest&color=2f80ed)](https://github.com/fangxiusun/recycle_manager/releases)
[![Platform](https://img.shields.io/badge/platform-Windows%20x64-0078D4)](#-requirements)
[![License](https://img.shields.io/badge/license-MIT-2ea44f)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.24-00ADD8?logo=go&logoColor=white)](#-building-from-source)

[Why this exists](#-why-this-exists) · [⚠️ What it does to your data](#️-what-it-does-to-your-data) · [Features](#-features) · [Download](#-download) · [Quick start](#-quick-start) · [Documentation](#-documentation) · [Acknowledgements](#-acknowledgements)

</div>

<br>

rbm reads the Recycle Bin's own metadata (`$Recycle.Bin\<your SID>\$I*`), buckets what it finds by
deletion time, groups inside each bucket by original folder, and numbers the result into
**partitions** you delete one batch at a time — with per-batch counts and sizes, a progress line, a
full CSV manifest (`scan_list.csv`) and an append-only ledger of every deletion (`ledger.csv`).
One static executable per build: no Python, no CGO, no installer.

Double-click it and you get a native Win32 GUI (built on [`lxn/walk`](https://github.com/lxn/walk));
run it with a subcommand and you get a CLI; both share the same engine and the same snapshot file, so
"partition 3" means the same batch of things in either one.

## 🧭 Why this exists

The author does a lot of vibe coding with agents. Agents rewrite and regenerate files by the
thousand, and the files they delete don't disappear — they pile up in the Recycle Bin. On that
machine it reached **hundreds of thousands of objects**. At that size the system's *Empty Recycle
Bin* runs for a very long time with no feedback at all, and Explorer hangs and restarts itself while
it's at it. **That is the author's own machine, not a claim about every Windows install** — but it was
enough to make us want a way to empty the bin that you can actually watch, steer, and resume.

So instead of one blind "empty everything" call:

```
$Recycle.Bin\<your SID>\$I*   →   bucket by deletion time
                                →   group by original folder
                                →   numbered partitions
                                →   show 3                    see what's in the batch
                                →   delete 1-5,8-10,16        each batch confirmed on its own
```

A bin holding hundreds of thousands of objects becomes a series of small, boring, observable
operations instead of one unbreakable multi-hour block.

## ⚠️ What it does to your data

> [!WARNING]
> **Delete is permanent.** rbm removes the `$R` payload and the `$I` metadata directly. Nothing goes
> back into the Recycle Bin, nothing is recoverable.

- Scanning is **read-only**: rbm only parses `$Recycle.Bin\<SID>` metadata for the current user.
- Three gates stack up before anything is deleted: **preview / `--dry-run`** (default, touches no
  files) → **confirmation dialog** (states the batch's item count, size and partition ids) →
  **deletion fuse / `--max N`** (if the selection exceeds N items the whole batch is refused — and the
  ledger file isn't even opened).
- Everything is on disk afterwards: `scan_list.csv` is the full manifest of the snapshot, `ledger.csv`
  records one line per deletion attempt with its result.

## ✨ Features

| | Feature | Notes |
|:---:|---|---|
| 🗑️ | **Recycle Bin metadata** | Parses `$I` for the original path, deletion time, size and drive. Orphan `$R` files (no matching `$I`), special locations and damaged metadata are named in an anomaly list and left alone. |
| 📊 | **Two-level batching** | First bucket by deletion time (16 presets, the last one is free-form), then group inside each bucket by folder / character prefix / extension / first filename token, and number the result. |
| 🔍 | **Observable** | Per-batch item count and size, click a row to see its contents, per-item progress, and 未删完 / "unfinished" plus 剩余 / "left" columns when it's done. The scan also logs **per-stage timing** (nine stages + per-item counters + one line per drive + which drives auto-enumeration skipped), so the slow stage is visible at a glance. |
| 🛡️ | **Three gates** | Preview / `--dry-run` → confirmation dialog → `--max` fuse. Nothing moves by default. |
| 📄 | **Paper trail** | `scan_list.csv` (full manifest, UTF-8 BOM + CRLF so Excel opens it by double-click) and `ledger.csv` (one appended line per deletion). |
| 🖱️ | **GUI** | Double-click `rbm-gui.exe` and no console window appears at any point; scans on open; tick boxes, <kbd>Shift</kbd>-click spans, type `1-5,8-10,16`, or 全选 / "select all". |
| ⌨️ | **CLI** | `scan` / `parts` / `show` / `delete` / `shell`; the nine batching options map one-to-one onto the GUI parameter area. `shell` scans once and reuses the in-memory snapshot, so ids don't drift. |
| 🌍 | **Bilingual** | Chinese by default, English one click or one flag away — interface, menus, dialogs, help and errors. |
| 📦 | **Single file** | Static executable, `CGO_ENABLED=0`, six dependencies, unzip and run. |

## 📦 Download

Latest release: **[v1.0.2](https://github.com/fangxiusun/recycle_manager/releases/tag/v1.0.2)**;
all releases: <https://github.com/fangxiusun/recycle_manager/releases>. The tables below document
**v1.0.2**, read byte-for-byte off a fresh download right after it was published
(2026-10-17 13:46 local) — when a new version ships, re-measure per
[`docs/构建与验证.md`](docs/构建与验证.md) (Chinese) §4 instead of copying these numbers.

| File | Notes |
|---|---|
| `rbm-v1.0.2-windows-amd64.zip` | unzip and run: `rbm-gui.exe` (double-click for the GUI), `rbm.exe` (CLI and scripts), `README.md`, `README.en.md`, `LICENSE`, `THIRD-PARTY-NOTICES.txt`. |
| `rbm-v1.0.2-sha256.txt` | SHA-256 for the zip and for both `.exe` files. |

| Asset | Bytes | SHA-256 |
|---|---:|---|
| `rbm-v1.0.2-windows-amd64.zip` | 9 866 736 | `baf9132c46e6dc93682aaf20355db305fe55bd610e16191df18bfb00b78a4727` |
| `rbm-v1.0.2-sha256.txt` | 257 | `c0cc02e2576202bccd1007a0a81ba1ed421775866452445411fb7e5ba97436d4` |

The six members of the zip (same round; both executables are 9 777 152 bytes):

| Member | Bytes | PE | SHA-256 |
|---|---:|---|---|
| `rbm.exe` | 9 777 152 | PE32+, Subsystem **3** (console), 1 manifest match | `10ba8455c361abd9430222bff10557b6ebd430b9fb6d218e91edb22fbb66eaf7` |
| `rbm-gui.exe` | 9 777 152 | PE32+, Subsystem **2** (GUI), 1 manifest match | `e225b7102c57e6032c2f37a88bbcd976ad2fc492610c15395e46fe18e5a959bb` |
| `README.md` | 16 328 | — | `70346bdf5b6d4673cfd64cdec5b6f32788248be62d7cc08ee9543abc05ebfbca` |
| `README.en.md` | 18 473 | — | `db2fa7addae4fe8f26496bfeca40188a4c54515dc505981da9d92308d9bccb7b` |
| `LICENSE` | 1 073 | — | `85b95532ceba0391b252d1bf320581878cbb8576c1f1d180db0d4fde11e1a729` |
| `THIRD-PARTY-NOTICES.txt` | 9 749 | — | `50f9879cd16d81595da3323441b5f1d0f80b478eee570e407f4a7b73f4237a89` |

> [!NOTE]
> - The four text members are **byte-identical** to the files in the repository (compared with
>   `sha256sum` in the same round).
> - The two `.exe` lines in `rbm-v1.0.2-sha256.txt` are still labelled `dist/rbm.exe` /
>   `dist/rbm-gui.exe` while the zip stores bare names — **match the hashes, not the file names**.
>   Details in [`docs/构建与验证.md`](docs/构建与验证.md) (Chinese) §3.7 and §4.
> - The binaries are not code-signed, so Windows SmartScreen may warn about an unknown publisher on
>   first run.
> - The previous v1.0.1 zip held only three members; the licence texts ship starting with v1.0.2.

```powershell
Get-FileHash .\rbm-v1.0.2-windows-amd64.zip -Algorithm SHA256   # should match the first table row
```

## 💻 Requirements

- **Windows x86_64**, and only that: `internal/gui` depends on `lxn/walk` and
  `golang.org/x/sys/windows`, whose files carry `//go:build windows`, so the package does not even
  compile on Linux or macOS.
- No runtime dependencies. Everything is pure Go and built with `CGO_ENABLED=0`: four direct module
  requires (`lxn/walk`, `lxn/win`, `go-runewidth`, `golang.org/x/sys`) plus two indirect ones.
- It looks at the **current user's** `$Recycle.Bin\<SID>` only — other users' bins, and `$R` files
  with no matching `$I`, are reported in an anomaly list and left alone. Run it as the user whose
  Recycle Bin you want to manage.

## 🚀 Quick start

This section quotes the Chinese interface, because that is what a fresh install shows.
In English the buttons below read **【Preview delete (touches nothing)】** and
**【⚠ Delete forever: partitions N, items M】** — see [Language](#language).

### GUI

1. Unzip, **double-click `rbm-gui.exe`**. No console window appears at any point (it is built with
   `-H windowsgui`, so the loader never allocates one).
2. The window **scans on open** and shows what the Recycle Bin holds right now. Wait for the log line
   with the elapsed time and the partition count.
3. Pick batches in ③: tick boxes (<kbd>Shift</kbd>-click spans a range), or type ids like
   `1-5,8-10,16`, or 全选 to take every partition that still has undeleted items.
4. Click a row in ④ to see its contents in ⑤.
5. Press 预览删除（不动文件） first. Only then press 永久删除 N 个分区 / M 个条目 and confirm the dialog.
6. Check the 未删完 / 剩余 columns, then <kbd>F5</kbd> to rescan and take the next batch.

The panels are numbered ①–⑥ in the order you use them: ① scan options, ② scan + overview,
③ selection, ④ partition table, ⑤ item detail, ⑥ preview/delete + progress. Batch strategy is the
按时间分批 dropdown (16 presets, last one is free-form) plus 桶内分组 (folder / character prefix /
extension / first filename token).

Two things to know before you start:

- Opening the GUI **overwrites** `state.json` and `scan_list.csv` in the snapshot directory
  (default `<cwd>\.rbm`) with the current state of the bin. Keep a snapshot you still need by pointing
  ▼ 高级参数 → snapshot directory somewhere else first.
- Turning on 只看未删完 (show only unfinished) **clears your ticks** — the view changed, and stale
  ticks are how you delete the wrong batch. Sorting by column header does *not* clear ticks.

### CLI

Use `rbm.exe` for anything scripted (see "Which executable" below).

```bat
rbm.exe scan                        :: build a snapshot, write state.json + scan_list.csv
rbm.exe parts                       :: list the numbered partitions of that snapshot
rbm.exe parts --alive --sort size   :: only partitions with items left, biggest first
rbm.exe show 3                      :: list items inside partition 3
rbm.exe delete 1-5,8-10,16          :: dry-run: prints what would go, touches nothing
rbm.exe delete 1-5,8-10,16 --yes    :: permanently delete those batches
rbm.exe shell                       :: interactive: scan once, reuse that in-memory snapshot
rbm.exe --help                      :: subcommands;  rbm.exe <subcommand> -h  :: its options
```

Nine partition options (`--bucket --max-buckets --by --dir-level --prefix-len --max-parts --min-count
--drives --max-scan`) map one-to-one onto the GUI parameter area; every command takes `--state-dir`
(default `<cwd>\.rbm`) and `--lang zh|en`. **Exit codes**: `0` success / cancelled / dry-run / bin is
empty; `1` data or runtime error (no snapshot, id out of range, unknown bucket spec, no recycle bin
for this SID on the requested drive); `2` usage error, unconfirmed delete, or the `--max` fuse blew.

`shell` is the mode for a bin of that size: it scans once and keeps that snapshot in memory, so
partition ids don't drift while you work through them.

### Which executable to use

| | `rbm-gui.exe` | `rbm.exe` |
|---|---|---|
| Built with | `-ldflags "-H windowsgui"` | default |
| PE Subsystem | 2 (GUI) | 3 (console) |
| Use it for | double-clicking; never shows a console window | CLI, scripts, pipes |
| Console | the loader allocates none | one exists; the GUI path calls `FreeConsole` after the window opens |
| Errors | no terminal to print to | stderr goes to your terminal — use this one when debugging |

`rbm-gui.exe` does run the subcommands, but PowerShell doesn't wait for a GUI-subsystem process:
piping it or capturing its output can end up truncated. Stay on `rbm.exe` in scripts.

### Language

Chinese is the default; English is one menu click or one flag away. Highest priority first:

| How | Scope |
|---|---|
| `--lang en` — `rbm.exe parts --lang en`, or `rbm.exe --lang en` with no subcommand to open the GUI in English | this process only |
| `set RBM_LANG=en` | any process that inherits the environment variable |
| 视图 → 界面语言 → 英文 in the GUI (writes the file for you), or `%LOCALAPPDATA%\rbm\settings.json` field `"language"` | the standing default |
| none of the above | Chinese |

An unrecognised value falls through to the next level instead of erroring. Switching inside the GUI
re-texts the open window immediately — no restart — and logs the settings file it wrote; lines already
in the activity log keep the language they were written in.

**What stays Chinese in both languages:** everything written to disk. `state.json`'s field names and
values (including the recorded bucket spec `gap:自动断点(<= 12 桶)`), the Chinese headers of
`scan_list.csv` and `ledger.csv` and the ledger's result value 「已删」, and the group labels that feed
both files: 「其他」「全部」「元数据损坏」「特殊位置」「(根)」「(无扩展名)」. Those are data, not
interface — re-labelling them would invalidate snapshots already on disk and the byte-for-byte
comparison against the reference implementation's fixtures.

## 🔧 Building from source

```bat
build.bat
```

which is these three steps (any failure exits 1):

```bat
windres --input=rbm.rc --input-format=rc --output=rsrc.syso --output-format=coff
set CGO_ENABLED=0&& set GOTOOLCHAIN=local&& go build -o rbm.exe .
go build -ldflags "-H windowsgui" -o rbm-gui.exe .
```

Go 1.24.5 and `CGO_ENABLED=0` are what this is built with; `windres` (MinGW binutils) is the only
toolchain piece beyond Go. **Step 1 is not optional**: `go build` succeeds without `rsrc.syso` and then
the GUI dies immediately with `图形界面创建失败: TTM_ADDTOOL failed`, because Common Controls 6.0 can
only be requested through the embedded manifest.

Checks before pushing: `go vet ./...`, `gofmt -l internal/`, `go test -count=1 ./...`
(93 top-level cases + 75 sub-cases as of 2026-10-17, all green; they use synthetic `$I` fixtures and
temp directories, so `go test` never touches a real Recycle Bin and needs no Python. Ten of those cases
are the message catalog itself — every key must exist in both languages, and every `%d` in a string
must actually be consumed).

## 📚 Documentation

The three detailed guides under `docs/` are written in Chinese and quote the Chinese screen labels
(Chinese is the default language, see [Language](#language)); this file is the English entry point.

| Document | Covers |
|---|---|
| [docs/README.md](docs/README.md) (Chinese) | index of every document, plus the repository layout |
| [README.md](README.md) (Chinese) | the default-locale front page |
| [选项说明.md](docs/选项说明.md) (Chinese) | every parameter in ①: the 16 batch presets, the four grouping modes, the advanced options, and when each one takes effect |
| [图形界面使用与解析.md](docs/图形界面使用与解析.md) (Chinese) | the GUI in depth: widget tree, model/view, threading, where the three delete gates sit, why no console flashes, how the in-window language switch re-texts every control |
| [构建与验证.md](docs/构建与验证.md) (Chinese) | build steps and why `rsrc.syso` is not optional, the GitHub Actions release pipeline, released-asset verification, full CLI reference and measured exit codes, the language precedence chain and what deliberately stays Chinese, partition semantics and validation, test inventory, known limitations |
| [docs/roadmap.md](docs/roadmap.md) (Chinese) | done and not-yet-done |
| [docs/known-issues.md](docs/known-issues.md) (Chinese) | current limitations |
| [CHANGELOG.md](CHANGELOG.md) (Chinese) | user-visible changes per version |

## 📌 Known limitations (summary)

- Windows only; the current user's SID only; drive letters come from the `GetLogicalDrives` mask.
  **Auto-enumeration skips network drives, optical drives and letters with no root directory** — one
  stale mapped drive can hold a scan for twenty-something seconds. To look at a mapped drive's recycle
  bin, name it explicitly: `--drives y`.
- Partition ids are valid **within the current snapshot only** — a rescan renumbers.
- Time buckets render in the machine's local offset captured at startup, so items deleted across a DST
  boundary are interpreted with today's offset.
- A scan stops at `--max-scan` (default 50,000) and keeps the **oldest** items; it always prints how
  many existed, how many it took, and per-drive progress — nothing is silently dropped. Use
  `--max-scan 0` for no cap.
- Deletion is permanent (see the warning above).
- Two interface languages, no more: Chinese (default) and English. The three `docs/` guides are
  Chinese-only.

The full list lives in [docs/known-issues.md](docs/known-issues.md) (Chinese).

## 🗺️ Roadmap

- [x] **M0–M3**: metadata parsing and two-level batching, CLI, GUI, build and release pipeline
- [x] **M4–M5**: Chinese/English bilingual UI (including the in-window instant re-text) and the
  repaint-on-delete fix
- [x] **M6**: two READMEs, docs index / roadmap / known-issues / changelog, MIT licence and the full
  text of every dependency's licence
- [x] **M7**: per-stage scan timing in the log (so the bottleneck is visible), three speed-ups
  (a stale mapped drive no longer stalls the whole scan)
- [ ] **M8**: double-click verification of the downloaded package, A/B evidence on the real delete
  path, a repaint regression case, GUI smoke in CI, English versions of the detailed guides,
  an incremental `$I` cache

Details in [docs/roadmap.md](docs/roadmap.md) (Chinese).

## 🤝 Contributing

Issues and pull requests go through the [GitHub repository](https://github.com/fangxiusun/recycle_manager).
Before touching code, run `go vet ./...`, `gofmt -l internal/` and `go test -count=1 ./...` — the cases
use synthetic fixtures and temp directories only, so they never touch a real Recycle Bin. New strings
need entries in **both** language catalogs: `internal/i18n` has cases that assert every key exists in
each language and every `%d` in a string is actually consumed.

## 🙏 Acknowledgements

rbm is built on top of these open-source projects:

| Project | Used for |
|---|---|
| [`lxn/walk`](https://github.com/lxn/walk) | the Win32 GUI framework |
| [`lxn/win`](https://github.com/lxn/win) | Go bindings to the Windows API |
| [`mattn/go-runewidth`](https://github.com/mattn/go-runewidth) | character cell width for console output (`internal/tty` pins `EastAsianWidth` so the same snapshot doesn't realign with the machine locale) |
| [`golang.org/x/sys`](https://golang.org/x/sys) | system calls: directory enumeration, the drive mask, file attributes |
| [`clipperhouse/uax29/v2`](https://github.com/clipperhouse/uax29) | indirect: the Unicode grapheme width table `go-runewidth` uses |
| [`Knetic/govaluate`](https://github.com/Knetic/govaluate) | indirect: the expression evaluator pulled in by `lxn/walk` |

All third-party components and the full text of their licences are in
[THIRD-PARTY-NOTICES.txt](THIRD-PARTY-NOTICES.txt).

Thanks to the [LINUX DO](https://linux.do/) community for the support.

## 📄 License

rbm is released under the [MIT License](LICENSE).

<div align="center">
<br>
<sub>rbm · batched Windows Recycle Bin cleanup · © 2026 rbm contributors</sub>
</div>
