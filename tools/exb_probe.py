#!/usr/bin/env python3
"""读取 CAXA EXB 内部流中的可识别数据。

该工具只读取 EXB，不修改原文件，不使用文件名推断业务字段，也不执行 OCR。
依赖：olefile。推荐使用 `uv run --with olefile python tools/exb_probe.py ...`。
"""

from __future__ import annotations

import argparse
import json
import re
import sys
import zlib
from pathlib import Path
from typing import Any

import olefile


ZLIB_HEADERS = {b"\x78\x01", b"\x78\x9c", b"\x78\xda"}
TEXT_PATTERN = re.compile(r"[\x20-\x7e\u3400-\u9fff]{2,}")
SCALE_PATTERN = re.compile(r"^\d+(?:\.\d+)?:\d+(?:\.\d+)?$")
DRAWING_NO_PATTERN = re.compile(r"^(?=.*\d)[A-Za-z0-9]+(?:[.-][A-Za-z0-9]+)+$")
STANDARD_PATTERN = re.compile(r"\b(?:GB|JB)(?:/[A-Z]+)?/[A-Z0-9.-]+", re.IGNORECASE)
MATERIAL_PATTERN = re.compile(r"^(?:\d+[A-Za-z]+\d*[A-Za-z0-9]*|[A-Za-z]+\d+[A-Za-z0-9]*)$")
TITLE_BLOCK_STREAM = "*BlockStg/*Blk_fffffffb"
TITLE_BLOCK_KEYS = (
    "单位名称",
    "图纸名称",
    "材料名称",
    "图纸编号",
    "图纸比例",
    "设计",
    "校对",
    "审核",
    "工艺",
    "标准",
    "批准",
    "标准化",
)

TITLE_BLOCK_KEY_ALIASES = {
    "图号": "图纸编号",
    "零件图号": "图纸编号",
    "零件号": "图纸编号",
    "零件代号": "图纸编号",
    "代号": "图纸编号",
    "设计_人员编号": "设计",
    "设计_日期": "设计日期",
    "校对_人员编号": "校对",
    "校对_日期": "校对日期",
    "审核_人员编号": "审核",
    "审核_日期": "审核日期",
    "工艺_人员编号": "工艺",
    "工艺_日期": "工艺日期",
    "标准化_人员编号": "标准化",
    "标准化_日期": "标准化日期",
    "批准_人员编号": "批准",
    "批准_日期": "批准日期",
}

COMMON_SURNAMES = set(
    "赵钱孙李周吴郑王冯陈褚卫蒋沈韩杨朱秦尤许何吕施张孔曹严华金魏陶姜戚谢邹喻柏水窦章云苏潘葛奚范彭郎鲁韦昌马苗凤花方俞任袁柳鲍史唐费廉岑薛雷贺倪汤滕殷罗毕郝邬安常乐于时傅皮卞齐康伍余元顾孟平黄和穆萧尹姚邵湛汪祁毛禹狄米贝明臧计伏成戴谈宋茅庞熊纪舒屈项祝董梁杜阮蓝闵席季麻强贾路娄危江童颜郭梅盛林刁钟徐邱骆高夏蔡田樊胡凌霍虞万支柯管卢莫经房裘缪干解应宗丁宣邓郁单杭洪包诸左石崔吉龚程嵇邢滑裴陆荣翁荀羊惠甄家封芮羿储靳汲邴糜松井段富巫乌焦巴弓牧山谷车侯宓蓬全班仰秋仲伊宫宁仇栾暴甘厉戎祖武符刘景詹束龙叶幸司韶郜黎蓟薄印宿白怀蒲从鄂索咸籍赖卓蔺屠蒙池乔阴胥能苍双闻莘党翟谭贡劳姬申扶堵冉宰郦雍璩桑桂濮牛寿通边扈燕冀郏浦尚农温别庄晏柴瞿阎充慕连茹习宦艾鱼容向古易慎戈廖庾终暨居衡步都耿满弘匡国文寇广禄阙东欧利师巩聂晁勾敖融冷辛阚那简饶空曾乜养鞠须丰巢关蒯相查后荆红游竺权逯盖益桓公"
)

def is_title_block_key(value: str) -> bool:
    return value in TITLE_BLOCK_KEYS or value in TITLE_BLOCK_KEY_ALIASES


def is_person_name(value: str) -> bool:
    value = value.strip()
    return (
        2 <= len(value) <= 4
        and all("\u4e00" <= char <= "\u9fff" for char in value)
        and value[0] in COMMON_SURNAMES
    )


def decompress_caxa_stream(data: bytes) -> tuple[bytes, str, int | None]:
    """解压 CAXA 流。当前文件格式通常在 20 字节头后放置 zlib 数据。"""

    candidates: list[int] = []
    if len(data) >= 22 and data[20:22] in ZLIB_HEADERS:
        candidates.append(20)
    candidates.extend(
        index
        for index in range(0, len(data) - 1)
        if data[index : index + 2] in ZLIB_HEADERS and index not in candidates
    )

    for offset in candidates:
        try:
            decoded = zlib.decompress(data[offset:])
        except zlib.error:
            continue
        if decoded:
            return decoded, "zlib", offset

    return data, "raw", None


def is_useful_text(value: str) -> bool:
    value = value.strip()
    if len(value) < 2:
        return False
    if not any(char.isalnum() or "\u3400" <= char <= "\u9fff" for char in value):
        return False
    # 过滤由二进制浮点数偶然解码出的极短噪声。
    meaningful = sum(
        1
        for char in value
        if char.isalnum() or "\u3400" <= char <= "\u9fff"
    )
    return meaningful >= 2


def extract_utf16_text(data: bytes) -> list[dict[str, Any]]:
    """提取 UTF-16LE 对象字符串，并保留解压流内偏移。"""

    result: list[dict[str, Any]] = []
    seen: set[tuple[int, str]] = set()
    # CAXA 记录中的 UTF-16LE 文本可能从奇数偏移开始，必须尝试两个对齐方式。
    for alignment in (0, 1):
        text = data[alignment:].decode("utf-16le", errors="ignore")
        for match in TEXT_PATTERN.finditer(text):
            value = match.group(0).strip()
            offset = alignment + match.start() * 2
            key = (offset, value)
            if is_useful_text(value) and key not in seen:
                seen.add(key)
                result.append({"offset": offset, "encoding": "utf-16le", "value": value})
    return result


def extract_ascii_text(data: bytes) -> list[dict[str, Any]]:
    """提取非 UTF-16 的 ASCII/UTF-8 片段，主要用于补充对象标识。"""

    result: list[dict[str, Any]] = []
    for match in re.finditer(rb"[\x20-\x7e]{3,}", data):
        value = match.group(0).decode("utf-8", errors="ignore").strip()
        if is_useful_text(value):
            result.append({"offset": match.start(), "encoding": "ascii", "value": value})
    return result


def unique(values: list[str]) -> list[str]:
    result: list[str] = []
    for value in values:
        if value and value not in result:
            result.append(value)
    return result


def classify_text(texts: list[dict[str, Any]]) -> dict[str, list[str]]:
    values = unique([item["value"] for item in texts])
    drawing_numbers: list[str] = []
    materials: list[str] = []
    scales: list[str] = []
    standards: list[str] = []
    keywords: list[str] = []

    for value in values:
        normalized = value.strip().rstrip(";")
        if SCALE_PATTERN.fullmatch(normalized):
            scales.append(normalized)
        if DRAWING_NO_PATTERN.fullmatch(normalized) and not normalized.isdigit():
            drawing_numbers.append(normalized)
        if MATERIAL_PATTERN.fullmatch(normalized) and any(char.isdigit() for char in normalized):
            materials.append(normalized)
        standards.extend(STANDARD_PATTERN.findall(value))
        if any(keyword in value for keyword in ("图号", "图名", "材料", "比例", "设计", "校对", "审核", "工艺", "标准", "批准", "明细")):
            keywords.append(value)

    return {
        "drawing_numbers": unique(drawing_numbers),
        "materials": unique(materials),
        "scales": unique(scales),
        "standards": unique(standards),
        "keywords": unique(keywords),
    }


def build_summary(texts: list[dict[str, Any]]) -> dict[str, Any]:
    """生成可供后端预览/确认使用的去重摘要，不替未知字段做猜测。"""

    values = unique([item["value"] for item in texts])
    summary = classify_text(texts)
    joined = "\n".join(values)

    labels = {
        "material_label": [value for value in values if value in ("材料", "材料名称")],
        "signer_labels": [
            value
            for value in values
            if any(keyword in value for keyword in ("设计", "校对", "审核", "工艺", "标准", "批准"))
        ],
    }
    detail_templates = [
        value
        for value in values
        if any(keyword in value for keyword in ("材料:", "规格:", "重量:", "体积:", "热处理:", "表面质量:", "备注:", "代号:"))
    ]
    object_names = [
        value
        for value in values
        if any(keyword in value for keyword in ("组焊件", "内螺纹", "缸体"))
    ]
    summary.update(
        {
            "object_names": object_names,
            "detail_templates": detail_templates,
            "labels": labels,
            "has_title_block_text": bool(labels["signer_labels"] or labels["material_label"]),
            "raw_text_count": len(texts),
            "unique_text_count": len(values),
            "contains_known_signer_values": any(
                keyword in joined for keyword in ("设计", "校对", "审核", "工艺", "批准")
            ),
        }
    )
    return summary


def extract_title_block_fields(texts: list[dict[str, Any]]) -> dict[str, str]:
    """从 CAXA 标题栏块中提取标签和值，不把全图文本当成业务字段。"""

    title_texts = [
        item
        for item in texts
        if item["stream"] == TITLE_BLOCK_STREAM and item["encoding"] == "utf-16le"
    ]
    title_texts.sort(key=lambda item: item["offset"])
    fields: dict[str, str] = {}

    index = 0
    while index < len(title_texts):
        item = title_texts[index]
        raw_key = item["value"].strip()
        key = TITLE_BLOCK_KEY_ALIASES.get(raw_key, raw_key)
        if key in TITLE_BLOCK_KEYS and key not in fields:
            # 优先从紧随当前键后面的有效值选取
            candidates: list[str] = []
            next_index = index + 1
            while next_index < len(title_texts):
                candidate = title_texts[next_index]
                value = candidate["value"].strip()
                normalized_candidate_key = TITLE_BLOCK_KEY_ALIASES.get(value, value)
                if normalized_candidate_key in TITLE_BLOCK_KEYS:
                    break
                # 键和值必须来自相同的 UTF-16 对齐方式，避免把错位二进制解析结果当成标题栏内容。
                if candidate["offset"] % 2 != item["offset"] % 2:
                    next_index += 1
                    continue
                if is_title_block_value(value, key):
                    if key in {"设计", "校对", "审核", "工艺", "标准化", "批准"} and not is_person_name(value):
                        next_index += 1
                        continue
                    candidates.append(value)
                next_index += 1
            if candidates:
                # CAXA 标题栏通常同时保存人员编号和姓名。姓名会在标题栏中重复出现，
                # 优先选择重复值，避免把损坏的编号文本误当成设计人。
                counts = {value: candidates.count(value) for value in candidates}
                repeated = [value for value in candidates if counts[value] >= 2]
                if repeated:
                    fields[key] = max(repeated, key=lambda value: (counts[value], len(value)))
                elif key not in {"设计", "校对", "审核", "工艺", "标准化", "批准"}:
                    fields[key] = max(candidates, key=title_block_value_score)
        index += 1

    return fields


def title_block_value_score(value: str) -> tuple[int, int, int]:
    """优先选择规范图号，避免将错位二进制文本作为标题栏值。"""
    normalized = value.strip()
    is_drawing_number = bool(DRAWING_NO_PATTERN.fullmatch(normalized))
    has_digit = any(char.isdigit() for char in normalized)
    has_separator = any(char in normalized for char in ".-_/")
    return (3 if is_drawing_number else 0, 1 if has_digit else 0, 1 if has_separator else 0)


def is_title_block_value(value: str, key: str) -> bool:
    if len(value) < 1 or len(value) > 120:
        return False
    if value in TITLE_BLOCK_KEYS:
        return False
    if any(char in value for char in ("Attribute", "Optional", "PickPt", "INNERPATH", "SOLID")):
        return False
    # CAXA 二进制记录按 UTF-16LE 扫描时会产生大量扩展区/错位乱码。
    if any("\u3400" <= char < "\u4e00" for char in value):
        return False
    if any(char in value for char in "㠀塶䘀輀䘔耀郯疐鰁䡽䉽戡倀㜀"):
        return False
    if key == "图纸比例":
        return bool(SCALE_PATTERN.fullmatch(value))
    if key == "图纸编号":
        return bool(DRAWING_NO_PATTERN.fullmatch(value))
    if key in {"图纸名称", "材料名称", "单位名称"}:
        return any(char.isalnum() or "\u4e00" <= char <= "\u9fff" for char in value)
    return any("\u4e00" <= char <= "\u9fff" or char.isalpha() for char in value)


def probe(path: Path, include_texts: bool = False) -> dict[str, Any]:
    streams: list[dict[str, Any]] = []
    all_texts: list[dict[str, Any]] = []

    with olefile.OleFileIO(path) as document:
        for stream_path in document.listdir(streams=True, storages=False):
            name = "/".join(stream_path)
            raw = document.openstream(stream_path).read()
            decoded, compression, zlib_offset = decompress_caxa_stream(raw)
            stream_texts = extract_utf16_text(decoded)
            stream_texts.extend(extract_ascii_text(decoded))
            stream_texts.sort(key=lambda item: (item["offset"], item["value"]))
            streams.append(
                {
                    "name": name,
                    "raw_size": len(raw),
                    "decoded_size": len(decoded),
                    "compression": compression,
                    "zlib_offset": zlib_offset,
                    "text_count": len(stream_texts),
                }
            )
            for item in stream_texts:
                all_texts.append({"stream": name, **item})

    fields = {
        "title_block": extract_title_block_fields(all_texts),
    }
    preview = next((item for item in streams if item["name"] == "*PreviewStream"), None)
    has_preview_image = False
    with olefile.OleFileIO(path) as document:
        if preview and document.exists(["*PreviewStream"]):
            try:
                p_stream = document.openstream(["*PreviewStream"]).read()
                bmp_idx = p_stream.find(b"BM")
                if bmp_idx != -1:
                    has_preview_image = True
            except Exception:
                pass

    result = {
        "file": str(path),
        "format": "OLE2/CFB",
        "ocr": False,
        "filename_fallback": False,
        "streams": streams,
        "fields": fields,
        "preview_stream": preview,
        "has_preview_image": has_preview_image,
    }
    if include_texts:
        result["texts"] = all_texts
    return result


def extract_preview_image(path: Path) -> bytes:
    """提取 EXB 内部内嵌的预览位图 (BMP)。"""
    ole = olefile.OleFileIO(path)
    if not ole.exists(["*PreviewStream"]):
        raise ValueError("EXB 文件中没有 *PreviewStream")
    stream = ole.openstream(["*PreviewStream"]).read()
    idx = stream.find(b"BM")
    if idx == -1:
        raise ValueError("预览流中未找到有效 BMP 图像")
    bmp = stream[idx:]
    if len(bmp) >= 6:
        import struct
        file_size, = struct.unpack("<I", bmp[2:6])
        if 54 <= file_size <= len(bmp):
            return bmp[:file_size]
    return bmp


def main() -> int:
    parser = argparse.ArgumentParser(description="读取 CAXA EXB 内部可识别文本与预览图")
    parser.add_argument("file", type=Path, help="EXB 文件路径")
    parser.add_argument("-o", "--output", type=Path, help="JSON 输出路径，默认输出到标准输出")
    parser.add_argument("--extract-preview", type=Path, help="提取预览图并保存为 BMP/PNG 文件")
    parser.add_argument("--include-texts", action="store_true", help="额外输出全部调试文本对象")
    args = parser.parse_args()

    if not args.file.is_file():
        print(f"文件不存在：{args.file}", file=sys.stderr)
        return 2

    if args.extract_preview:
        try:
            bmp_bytes = extract_preview_image(args.file)
            args.extract_preview.write_bytes(bmp_bytes)
            return 0
        except Exception as error:
            print(f"提取预览图失败：{error}", file=sys.stderr)
            return 1

    try:
        result = probe(args.file, include_texts=args.include_texts)
    except ImportError:
        print("缺少 olefile，请使用：uv run --with olefile python tools/exb_probe.py <文件>", file=sys.stderr)
        return 3
    except Exception as error:  # pragma: no cover - 命令行错误信息
        print(f"读取 EXB 失败：{error}", file=sys.stderr)
        return 1

    encoded = json.dumps(result, ensure_ascii=False, indent=2)
    if args.output:
        args.output.write_text(encoded + "\n", encoding="utf-8")
    else:
        sys.stdout.buffer.write(encoded.encode("utf-8") + b"\n")
        sys.stdout.buffer.flush()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
