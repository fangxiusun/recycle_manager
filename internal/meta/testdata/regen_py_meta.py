"""生成 Go 侧的黄金样例 testdata/py_meta.json + testdata/isample/$I*。

在仓库任意位置运行都可以（脚本自己算 recycle_manager 的路径）：
    python G:\\workspace\\check_rycycle_bin\\rbm_go\\internal\\meta\\testdata\\regen_py_meta.py

每个样例 $I 文件都由本脚本合成（假 Z:\\ / G:\\ 路径，不碰任何真实回收站内容），
解析结果来自 recycle_manager/rb_meta.py 的 parse_i_file 原样产出，
所以 deleted_at 的时区口径、epoch 的浮点写法、display 的取名规则都是 Python 侧真实行为。
Go 的 TestMatchesPythonMeta 读同一批 $I 文件逐字段比对。

$I 文件本身留在 testdata/isample 下：Go 测试读这些字节，不重新合成，
保证两边解析的输入完全一致。
"""
from __future__ import annotations

import datetime as dt
import json
import os
import shutil
import struct
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
RECYCLE = os.path.abspath(os.path.join(HERE, "..", "..", "..", "..", "recycle_manager"))
SAMPLES = os.path.join(HERE, "isample")
EPOCH_FT = 116444736000000000  # 1601-01-01 → 1970-01-01 的 100ns 拍

sys.path.insert(0, RECYCLE)
import rb_meta  # noqa: E402

TZ = dt.datetime.now().astimezone().tzinfo


def local_filetime(text: str) -> int:
    """按本地墙钟写 FILETIME（$I 里的删除时间就是本地时间口径）。"""
    naive = dt.datetime.strptime(text, "%Y-%m-%d %H:%M:%S").replace(tzinfo=TZ)
    return int(naive.timestamp() * 10_000_000) + EPOCH_FT


def i_bytes(path: str, size: int, filetime: int, version: int) -> bytes:
    units = [ord(c) for c in path] + [0]
    payload = "".join(chr(u) for u in units).encode("utf-16-le")
    head = struct.pack("<qqq", version, size, filetime)
    if version >= 2:
        head += struct.pack("<I", len(units))
    else:
        head += struct.pack("<q", len(units))
    return head + payload


CASES = [
    # (stem, 原路径, size, 本地删除时间, 版本)
    ("100001winter", r"Z:\work\build1\out\chunk-46.js", 1024, "2026-01-05 08:30:00", 2),
    ("100002summer", r"Z:\work\build1\out\vite.config.mjs", 98765, "2026-07-01 23:59:59", 2),
    ("100003cjk", r"G:\笔记\草稿 与 一段很长的中文目录名\笔记-草稿.md", 4096, "2026-10-15 12:11:14", 2),
    ("100004lower", r"Z:\work\zero.bin", 7, "1970-01-02 08:00:00", 2),  # epoch 86400：Python 渲染得动的最老一刻
    ("100005y2038", r"Z:\work\far_future.bin", 1 << 30, "2038-01-19 11:14:08", 2),
    ("100007fraction", r"Z:\work\half.bin", 1, "2026-03-01 00:00:01", 2),
    ("100008trailing", "C:\\Users\\x\\Temp\\Vs9POM\\sub\\", 0, "2026-05-20 10:00:00", 2),
    ("100009guid", r"::{6D809000-9578-4E4D-A416-0803501EA482}", 12, "2026-02-02 02:02:02", 2),
    ("100010negsize", r"Z:\work\negative.bin", -5, "2026-04-04 04:04:04", 2),
    ("100011v1", r"Z:\work\legacy.bin", 55, "2025-11-11 11:11:11", 1),
    ("100012drive", "D:\\", 3, "2026-09-09 09:09:09", 2),
]

# 这批 FILETIME 让 Python 侧走 Windows localtime 时直接抛 OSError，Scan 把它归为「元数据损坏」。
# 实测 Python 能渲染的区间是 epoch [86400, 32536713599]（本机 UTC+8：本地 1970-01-02 08:00:00
# ～ 3001-01-18 15:59:59）。Go 没有这个运行时限制，靠 parseIFile 里的「本地年份 1970～2999」
# 区间给出同判——两个边界各差一天量级，回收站里不可能出现这种删除时间。
BAD_FILETIME = [
    ("200001ft0", r"Z:\work\allzero.bin", 9, 0, 2),  # 全零头
    ("200002pre1970", r"Z:\work\old.bin", 8, EPOCH_FT - 1, 2),  # 1600 年
    ("200003y1970", r"Z:\work\edge.bin", 8, EPOCH_FT + 86399 * 10_000_000, 2),  # epoch 86399
    ("200004y3001", r"Z:\work\farfar.bin", 8, EPOCH_FT + 32536713600 * 10_000_000, 2),  # 3001 年
]


def main() -> None:
    if os.path.isdir(SAMPLES):
        shutil.rmtree(SAMPLES)
    os.makedirs(SAMPLES)
    out = {
        "local_utc_offset_now": int(dt.datetime.now().astimezone().utcoffset().total_seconds()),
        "cases": [],
        "error_cases": [],
    }

    def write(stem: str, path: str, size: int, ft: int, version: int) -> str:
        raw = i_bytes(path, size, ft, version)
        i_path = os.path.join(SAMPLES, "$I" + stem)
        with open(i_path, "wb") as handle:
            handle.write(raw)
        return i_path

    for stem, path, size, when, version in CASES:
        ft = local_filetime(when)
        if stem.endswith("fraction"):
            ft += 5  # 半拍（500ns）：两边都只渲染到整秒，用来验证取整口径一致
        parsed = rb_meta.parse_i_file(write(stem, path, size, ft, version))
        out["cases"].append(
            {
                "stem": stem,
                "meta_version": parsed["meta_version"],
                "size": parsed["size"],
                "deleted_at": parsed["deleted_at"],
                "epoch": parsed["epoch"],
                "orig_path": parsed["orig_path"],
                "display": parsed["display"],
                "unrecoverable": parsed["unrecoverable"],
            }
        )

    for stem, path, size, ft, version in BAD_FILETIME:
        try:
            rb_meta.parse_i_file(write(stem, path, size, ft, version))
        except Exception as exc:  # noqa: BLE001
            out["error_cases"].append({"stem": stem, "py_error": type(exc).__name__})
            continue
        raise SystemExit(f"{stem} 期望 Python 抛错，实际解析成功")

    with open(os.path.join(HERE, "py_meta.json"), "w", encoding="utf-8") as handle:
        json.dump(out, handle, ensure_ascii=False, indent=1)
    print(f"写出 {len(out['cases'])} 例 + {len(out['error_cases'])} 例必报错 → py_meta.json（$I 样例在 isample/）")


if __name__ == "__main__":
    main()
