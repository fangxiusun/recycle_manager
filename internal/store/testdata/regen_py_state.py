"""生成 Go 侧的黄金样例 testdata/py_state.json（Python 版真正写出的 state.json）。

    python G:\\workspace\\check_rycycle_bin\\rbm_go\\internal\\store\\testdata\\regen_py_state.py

条目是合成的（假 Z:\\ 路径），但 state.json 由 recycle_manager/rbmgr.py 的
build_state + save_state 原样产出，所以字段名、类型（含 dir_level 是字符串）、
数字写法都是 Python 侧的真实口径。Go 的 TestReadsPythonState 直接读它。
"""
from __future__ import annotations

import datetime as dt
import json
import os
import shutil
import sys
import tempfile
from types import SimpleNamespace

HERE = os.path.dirname(os.path.abspath(__file__))
RECYCLE = os.path.abspath(os.path.join(HERE, "..", "..", "..", "..", "recycle_manager"))
sys.path.insert(0, RECYCLE)

import rbmgr  # noqa: E402

TZ = dt.datetime.now().astimezone().tzinfo
BASE = dt.datetime(2026, 10, 15, 12, 0, tzinfo=TZ)
NAMES = ["chunk-46.js", "笔记-草稿.md", "test_output.txt", "vite.config.mjs", "write.lock"]
DIRS = [
    r"Z:\work\build1\out",
    r"Z:\work\build2\out",
    r"Z:\Users\x\AppData\Local\Temp\Vs9POM",
    r"Z:\Users\x\Documents\reports",
]


def synth(count: int) -> list[dict]:
    items = []
    for i in range(count):
        corrupt = i % 37 == 36  # 少量损坏条目，走「元数据损坏」分组
        epoch = 0.0 if corrupt else (BASE + dt.timedelta(minutes=3 * i)).timestamp()
        orig = "(元数据损坏)" if corrupt else DIRS[i % len(DIRS)] + "\\" + NAMES[i % len(NAMES)]
        stem = f"{i + 1:05d}{NAMES[i % len(NAMES)]}"
        items.append({
            "meta_version": -1 if corrupt else 2,
            "size": 1024 * (i % 17),
            "deleted_at": "1970-01-01 00:00:00" if corrupt else (BASE + dt.timedelta(minutes=3 * i)).strftime("%Y-%m-%d %H:%M:%S"),
            "epoch": epoch,
            "orig_path": orig,
            "display": "(元数据损坏)" if corrupt else NAMES[i % len(NAMES)],
            "unrecoverable": False,
            "drive": "Z",
            "i_path": f"Z:\\$Recycle.Bin\\S-1-5-21-FAKE\\$I{stem}",
            "r_path": f"Z:\\$Recycle.Bin\\S-1-5-21-FAKE\\$R{stem}",
            "exists": not corrupt,
        })
    return items


def main() -> int:
    args = SimpleNamespace(state_dir=tempfile.mkdtemp(prefix="rbm_py_state_"), bucket="gap:5m",
                           max_buckets=12, by="dir", prefix_len=3, max_parts=120, min_count=5,
                           dir_level="2", drives="Z")
    items = synth(150)
    state = rbmgr.build_state(items, ["Z:\\...\\$I00099ghost.txt: 对应 $R 数据不存在（残留元数据）"], args)
    rbmgr.save_state(args, state)
    src = os.path.join(args.state_dir, "state.json")
    dest = os.path.join(HERE, "py_state.json")
    shutil.copyfile(src, dest)
    shutil.rmtree(args.state_dir, ignore_errors=True)
    with open(dest, encoding="utf-8") as fh:
        got = json.load(fh)
    print(f"wrote {dest} ({os.path.getsize(dest)} bytes): items={len(got['items'])} "
          f"parts={len(got['parts'])} params={json.dumps(got['params'], ensure_ascii=False)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
