#!/usr/bin/env python3
"""Lower the declared WebAssembly memory maximum (pages) without changing section size.

The stock llama.cpp wasm from yzma declares 65536 pages (4 GiB). We rewrite that
LEB128 in place to 16384 pages (1 GiB). Both encodings are 3 bytes, so offsets stay put.
"""
from __future__ import annotations
import sys
from pathlib import Path

def read_leb(data: bytes, i: int):
    result = 0
    shift = 0
    while True:
        b = data[i]
        i += 1
        result |= (b & 0x7F) << shift
        if not (b & 0x80):
            return result, i
        shift += 7

def patch(path: Path, max_pages: int = 16384) -> None:
    data = bytearray(path.read_bytes())
    if data[:4] != b"\x00asm":
        raise SystemExit(f"{path} is not a wasm module")
    i = 8
    found = False
    while i < len(data):
        sec_id = data[i]
        i += 1
        size, i = read_leb(data, i)
        end = i + size
        if sec_id == 5:
            count, j = read_leb(data, i)
            for _ in range(count):
                flag = data[j]
                j += 1
                _min, j = read_leb(data, j)
                if flag & 1:
                    max_pos = j
                    old, after = read_leb(data, j)
                    width = after - max_pos
                    if width != 3 or old not in (65536, 16384):
                        raise SystemExit(
                            f"unexpected memory maximum {old} ({width} bytes) in {path}"
                        )
                    # 16384 = 0x80 0x80 0x01; 65536 = 0x80 0x80 0x04
                    data[max_pos:max_pos + 3] = bytes([0x80, 0x80, 0x01])
                    found = True
                    print(f"{path}: max_pages {old} -> 16384 (1 GiB), min_pages {_min}")
        i = end
    if not found:
        raise SystemExit(f"no patchable memory maximum in {path}")
    path.write_bytes(data)

if __name__ == "__main__":
    if len(sys.argv) != 2:
        raise SystemExit("usage: patch_wasm_memory.py <file.wasm>")
    patch(Path(sys.argv[1]))
