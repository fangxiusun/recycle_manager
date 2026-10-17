# rbm — empty a stuffed Windows Recycle Bin in reviewable batches

**rbm** reads the Recycle Bin's own metadata, splits what it finds into numbered batches, and lets you
permanently delete a batch at a time — with per-batch counts and sizes, a progress line, a full CSV
manifest, and an append-only ledger of every deletion. One static executable per build: no Python, no
CGO, no installer.

Double-click it and you get a native Win32 GUI (built on `lxn/walk`); run it with a subcommand and you
get a CLI; both share the same engine and the same snapshot file, so a partition number means the same
thing in both.

> **Bilingual: Chinese by default, English one click or one flag away** — see [Language](#language).
> The rest of this file quotes the Chinese labels, because that is what a fresh install shows.

---

## Why this exists

I do a lot of vibe coding with agents. Agents rewrite and regenerate files by the thousand, and the
files they delete don't disappear — they pile up in the Recycle Bin. On my machine that reached
**hundreds of thousands of objects**. At that size the system's *Empty Recycle Bin* runs for a very
long time with no feedback at all, and Explorer hangs and restarts itself while it's at it. That is my
own machine, not a claim about every Windows install — but it was enough to make me want a way to empty
the bin that I could actually watch, steer, and resume.

So instead of one blind "empty everything" call:

```
$Recycle.Bin\<your SID>\$I*   →   bucket by deletion time   →   group by original folder
                                →   numbered partitions      →   show a partition
                                →   delete partition 1-5, 8-10, 16
```

Each batch is confirmed on its own, so a bin holding hundreds of thousands of objects becomes a series
of small, boring, observable operations instead of one unbreakable multi-hour block.

---

## ⚠️ What rbm does to your data

- **Delete is permanent.** It removes the `$R` payload and the `$I` metadata directly. Nothing goes
  back into the Recycle Bin, nothing is recoverable.
- Scanning is **read-only** — rbm only parses `$Recycle.Bin\<SID>` metadata for the current user.
- Three gates stack up before anything is deleted: **preview / `--dry-run`** (default, touches no
  files) → **confirmation dialog** (states the size of the batch and the partition ids) →
  **deletion fuse / `--max N`** (if the selection exceeds N items the whole batch is refused — and the
  ledger file isn't even opened).
- Everything is on disk afterwards: `scan_list.csv` is the full manifest of the snapshot, `ledger.csv`
  records one line per deletion attempt with its result.

---

## Language

Chinese is the default; English is one menu click or one flag away. Highest priority first:

| How | Scope |
| --- | --- |
| `--lang en` — `rbm.exe parts --lang en`, or `rbm.exe --lang en` with no subcommand to open the GUI in English | this process only |
| `set RBM_LANG=en` | any process that inherits the environment variable |
| 视图 → 界面语言 → 英文 in the GUI (writes the file for you), or `%LOCALAPPDATA%\rbm\settings.json` field `"language"` | the standing default |
| none of the above | Chinese |

An unrecognised value falls through to the next level instead of erroring. Switching inside the GUI
re-texts the open window immediately — no restart — and logs the settings file it wrote; lines already
in the activity log keep the language they were written in. If you run in English, the two buttons
Quick start describes below read **【Preview delete (touches nothing)】** and
**【⚠ Delete forever: partitions N, items M】**.

**What stays Chinese in both languages:** everything written to disk. `state.json`'s field names and
values (including the recorded bucket spec `gap:自动断点(<= 12 桶)`), the Chinese headers of
`scan_list.csv` and `ledger.csv` and the ledger's result value 「已删」, and the group labels that feed
both files: 「其他」「全部」「元数据损坏」「特殊位置」「(根)」「(无扩展名)」. Those are data, not
interface — re-labelling them would invalidate snapshots already on disk and the byte-for-byte
comparison against the reference implementation's fixtures.

---

## Requirements

- **Windows x86_64**, and only that: `internal/gui` depends on `lxn/walk` and `golang.org/x/sys/windows`,
  whose files carry `//go:build windows`, so the package does not even compile on Linux or macOS.
- No runtime dependencies. Everything is pure Go and built with `CGO_ENABLED=0`: four direct module
  requires (`lxn/walk`, `lxn/win`, `go-runewidth`, `golang.org/x/sys`) plus two indirect ones.
- It looks at the **current user's** `$Recycle.Bin\<SID>` only — other users' bins, and `$R` files with
  no matching `$I`, are reported in an anomaly list and left alone. Run it as the user whose Recycle Bin
  you want to manage.

## Download

Latest release: **[v1.0.1](https://github.com/fangxiusun/recycle_manager/releases/tag/v1.0.1)**
(all releases: <https://github.com/fangxiusun/recycle_manager/releases>)

| Asset | Bytes | SHA-256 |
| --- | ---: | --- |
| `rbm-v1.0.1-windows-amd64.zip` | 9,723,847 | `6db729491ce1efd45c65edda83685a41eaa0fb4916ca2d8d0a1f1d0084282452` |
| `rbm-v1.0.1-sha256.txt` | 257 | `dbcca8965558304a11f5f7c81f9c16d5fb039cbfdebcdb93e7ff8e7a0612e74c` |

The zip contains three files: `rbm.exe` and `rbm-gui.exe` (9,623,040 bytes each) and `README.md`.
Checked on 2026-10-17 by re-downloading the assets and reading the PE headers of the members:

| Inside the zip | PE Subsystem | SHA-256 |
| --- | ---: | --- |
| `rbm.exe` | 3 (console) | `9665a2245ed0b1193ad1a5e0abb405917d57a571fe8e01c5dfab12eae312cbdc` |
| `rbm-gui.exe` | 2 (GUI) | `82fe1bb7291e0ac86a9a446db5514890ee48de33c3a64de7e38318b5c7bdcf5b` |

```powershell
Get-FileHash .\rbm-v1.0.1-windows-amd64.zip -Algorithm SHA256   # should match the first table row
```

Note: the two `.exe` lines in `rbm-v1.0.1-sha256.txt` are labelled `dist/rbm.exe` / `dist/rbm-gui.exe`
while the zip stores bare names — match the hashes, not the file names. Details in
[`docs/构建与验证.md`](docs/构建与验证.md) §4.

## Quick start — GUI

1. Unzip, **double-click `rbm-gui.exe`**. No console window appears at any point (it is built with
   `-H windowsgui`, so the loader never allocates one).
2. The window **scans on open** and shows what the Recycle Bin holds right now. Wait for the log line
   with the elapsed time and the partition count.
3. Pick batches in ③: tick boxes (Shift-click spans a range), or type ids like `1-5,8-10,16`,
   or 【全选】 to take every partition that still has undeleted items.
4. Click a row in ④ to see its contents in ⑤.
5. Press **【预览删除（不动文件）】** first. Only then press
   **【⚠ 永久删除 N 个分区 / M 个条目】** and confirm the dialog.
6. Check the 「未删完」/「剩余」 columns, then F5 to rescan and take the next batch.

The panels are numbered ①–⑥ in the order you use them: ① scan options, ② scan + overview,
③ selection, ④ partition table, ⑤ item detail, ⑥ preview/delete + progress. Batch strategy is the
「按时间分批」 dropdown (16 presets, last one is free-form) plus 「桶内分组」 (folder / character
prefix / extension / first filename token).

Two things to know before you start:

- Opening the GUI **overwrites** `state.json` and `scan_list.csv` in the snapshot directory
  (default `<cwd>\.rbm`) with the current state of the bin. Keep a snapshot you still need by pointing
  【▼ 高级参数】→ snapshot directory somewhere else first.
- Turning on 「只看未删完」 (show only unfinished) **clears your ticks** — the view changed, and stale
  ticks are how you delete the wrong batch. Sorting by column header does *not* clear ticks.

## Quick start — CLI

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
(default `<cwd>\.rbm`) and `--lang zh|en` (see [Language](#language)). **Exit codes**: `0` success /
cancelled / dry-run / bin is empty;
`1` data or runtime error (no snapshot, id out of range, unknown bucket spec, no recycle bin for this
SID on the requested drive); `2` usage error, unconfirmed delete, or the `--max` fuse blew.

`shell` is the mode for a bin of that size: it scans once and keeps that snapshot in memory, so
partition ids don't drift while you work through them.

Artifacts, all in the snapshot directory: `state.json` (the snapshot), `scan_list.csv` (full manifest,
UTF-8 BOM + CRLF so Excel opens it by double-click), `ledger.csv` (one appended line per deletion,
created only on a real delete).

## Which executable to use

| | `rbm-gui.exe` | `rbm.exe` |
| --- | --- | --- |
| Built with | `-ldflags "-H windowsgui"` | default |
| PE Subsystem | 2 (GUI) | 3 (console) |
| Use it for | double-clicking; never shows a console window | CLI, scripts, pipes |
| Console | the loader allocates none | one exists; the GUI path calls `FreeConsole` after the window opens |
| Errors | no terminal to print to | stderr goes to your terminal — use this one when debugging |

`rbm-gui.exe` does run the subcommands, but PowerShell doesn't wait for a GUI-subsystem process: piping
it or capturing its output can end up truncated. Stay on `rbm.exe` in scripts.

## Documentation

The three docs under `docs/` are written in Chinese and quote the Chinese screen labels (the default
language, see [Language](#language)); this README is the English entry point.

| File | What it covers |
| --- | --- |
| `README.md` | this file — what/why/which/how to get started |
| [`docs/选项说明.md`](docs/选项说明.md) (Chinese) | every parameter in ① : the 16 batch presets, the four grouping modes, the advanced options, and when each one takes effect |
| [`docs/图形界面使用与解析.md`](docs/图形界面使用与解析.md) (Chinese) | the GUI in depth: widget tree, model/view, threading, where the three delete gates sit, why no console flashes, how the in-window language switch re-texts every control |
| [`docs/构建与验证.md`](docs/构建与验证.md) (Chinese) | build steps and why `rsrc.syso` is not optional, the GitHub Actions release pipeline, released-asset verification, full CLI reference and measured exit codes, the language precedence chain and what deliberately stays Chinese, partition semantics and validation, test inventory, known limitations |

## Build from source

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
(93 top-level cases + 75 sub-cases as of 2026-10-17, all green; they use synthetic `$I` fixtures and temp
directories, so `go test` never touches a real Recycle Bin and needs no Python. Ten of those cases are the
message catalog itself — every key must exist in both languages, and every `%d` in a string must actually
be consumed).

## Known limitations

- Windows only; current user's SID only; drive letters come from the `GetLogicalDrives` mask.
- Partition ids are valid **within the current snapshot only** — a rescan renumbers.
- Time buckets render in the machine's local offset captured at startup, so items deleted across a DST
  boundary are interpreted with today's offset.
- A scan stops at `--max-scan` (default 50,000) and keeps the **oldest** items; it always prints how
  many existed, how many it took, and per-drive progress — nothing is silently dropped. Use
  `--max-scan 0` for no cap.
- Deletion is permanent (see the warning above).
- Two interface languages, no more: Chinese (default) and English. The three `docs/` guides are
  Chinese-only, and lines already written to the GUI's activity log keep the language they were written
  in (see [Language](#language)).
- No `LICENSE` file is committed in this repository yet.
