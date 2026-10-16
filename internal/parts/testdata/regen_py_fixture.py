"""生成 Go 侧的黄金样例 testdata/py_parts.json。

在仓库任意位置运行都可以（脚本自己算 recycle_manager 的路径）：
    python G:\\workspace\\check_rycycle_bin\\rbm_go\\internal\\parts\\testdata\\regen_py_fixture.py

条目由本文件自带的 synth 合成（假路径、固定种子），分区结果来自 recycle_manager/rb_parts.py 的
build_partitions，因此这份 fixture 只含合成数据，不含任何真实回收站内容。
Go 的 TestMatchesPythonFixture 逐字段比对它。

两组数据各有目的：
  A 组跨 500 天、含 2% 损坏条目（epoch=0）——覆盖隐式断点、日历月/季度、上限合并；
     注意显式细粒度（1m/30m）在这种跨度下会生成 56 年的边界表，所以 A 组只跑粗粒度。
  B 组跨 2 天、无损坏条目——专门覆盖分钟级显式边界与四种分组维度。
"""
from __future__ import annotations

import datetime as dt
import json
import os
import random
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
RECYCLE = os.path.abspath(os.path.join(HERE, "..", "..", "..", "..", "recycle_manager"))
sys.path.insert(0, RECYCLE)

import rb_parts  # noqa: E402

NAMES = ["chunk-46.js", "test_output.txt", "20261014.log", "ssr", "vite.config.mjs",
         "0b6f3a.json", "笔记-草稿.md", "webpack", "write.lock", ".env.local", ""]
PARENTS = [
    r"C:\Users\x\Temp\Vs9POM\ssr",
    r"C:\Users\x\Temp\lth-fixture\ssr",
    r"C:\Users\x\Temp\ocqyI\ssr",
    r"G:\workspace\local-tool-hub\dist-web\assets",
    r"C:\Users\x\Documents\reports",
    r"C:\Users\x",
    "C:" + chr(92),
]


def synth(n: int, seed: int, span_days: float, corrupt_p: float) -> list[dict]:
    """和 recycle_manager/test_partitions.py 的 synth 同构，只是把时间跨度和损坏比例摊开成参数。"""
    rng = random.Random(seed)
    base = dt.datetime(2026, 10, 15, tzinfo=dt.datetime.now().astimezone().tzinfo).timestamp()
    items = []
    for _ in range(n):
        epoch = base - rng.random() * 86400 * span_days
        name = rng.choice(NAMES) or f"rand{rng.randrange(999)}"
        parent = rng.choice(PARENTS)
        orig = parent if parent.endswith("\\") else parent + "\\" + name
        corrupt = rng.random() < corrupt_p
        if corrupt:
            epoch, orig = 0.0, "(元数据损坏)"
        items.append({
            "display": name,
            "meta_version": -1 if corrupt else 2,
            "epoch": epoch,
            "deleted_at": rb_parts.fmt(epoch, "%Y-%m-%d %H:%M:%S"),
            "size": rng.randrange(0, 5_000_000),
            "drive": rng.choice("CDG"),
            "orig_path": orig,
            "unrecoverable": False,
        })
    return items


def cfg(bucket, by, dir_level, prefix_len=3, max_buckets=12, max_parts=120, min_count=5) -> dict:
    return {"bucket": bucket, "by": by, "dir_level": dir_level, "prefix_len": prefix_len,
            "max_buckets": max_buckets, "max_parts": max_parts, "min_count": min_count}


SET_A_CONFIGS = [
    cfg(None, "dir", "2"),
    cfg("gap:5m", "dir", "2"),
    cfg("gap", "prefix", "2", prefix_len=4, max_buckets=8),
    cfg("auto", "dir", "3", max_buckets=6),
    cfg("daily", "dir", "1", max_parts=60),
    cfg("weekly", "ext", "2", max_buckets=24),
    cfg("monthly", "dir", "full"),
    cfg("3mo", "dir", "full", max_buckets=200),
    cfg("6mo", "token", "2", prefix_len=5, max_parts=25, min_count=2),
    cfg("monthly", "dir", "2", max_buckets=200, max_parts=15, min_count=0),
    cfg(None, "prefix", "2", prefix_len=1, max_buckets=2, max_parts=3, min_count=20),
]

SET_B_CONFIGS = [
    cfg("minutely", "dir", "2", max_buckets=200, max_parts=40),
    cfg("1m", "prefix", "2", prefix_len=3, max_buckets=200, max_parts=20),
    cfg("15m", "ext", "2", max_buckets=200),
    cfg("30m", "token", "2", prefix_len=5, max_buckets=100, max_parts=30, min_count=3),
    cfg("hourly", "dir", "full", max_buckets=100, max_parts=200, min_count=1),
    cfg("12h", "dir", "1", max_buckets=40),
    cfg("3d", "prefix", "2", prefix_len=8, max_buckets=40, min_count=1),
    cfg("daily", "dir", "2", max_buckets=40, max_parts=12, min_count=8),
]

MAX_BOUNDS_KEPT = 1000  # 超过这个长度的边界表不进 fixture（A 组的 daily 会有 2 万个）


def build_set(items: list[dict], configs: list[dict]) -> dict:
    entry = {"items": items, "configs": []}
    for c in configs:
        bounds, parts, ordered = rb_parts.build_partitions(
            items, c["bucket"], c["max_buckets"], c["by"], c["prefix_len"],
            c["max_parts"], c["min_count"], c["dir_level"])
        entry["configs"].append({
            "cfg": c,
            "bounds": bounds if len(bounds) <= MAX_BOUNDS_KEPT else None,
            "parts": parts,
            "ordered": len(ordered),
        })
    return entry


def main() -> int:
    out = {"note": "python rb_parts.build_partitions 的输出，条目由本脚本 synth 合成", "sets": []}
    for seed in (5, 11, 21):
        out["sets"].append(build_set(synth(300, seed, 500, 0.02), SET_A_CONFIGS))
    for seed in (31, 37):
        out["sets"].append(build_set(synth(250, seed, 2, 0.0), SET_B_CONFIGS))
    dest = os.path.join(HERE, "py_parts.json")
    with open(dest, "w", encoding="utf-8") as fh:
        json.dump(out, fh, ensure_ascii=False, separators=(",", ":"))
    n_cfg = sum(len(s["configs"]) for s in out["sets"])
    print(f"wrote {dest} ({os.path.getsize(dest)} bytes, {len(out['sets'])} sets, {n_cfg} configs)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
