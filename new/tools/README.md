# EXB 探针

`exb_probe.py` 用于从 CAXA `EXB` 文件内部读取能够直接识别的内容。

工具行为：

- 读取 OLE2/CFB 容器和内部流
- 解压 CAXA 流中的 `zlib` 数据
- 提取 UTF-16LE 和 ASCII 对象文本
- 只从标题栏块提取明确的标签和值
- 保留流名与解压后偏移，便于继续逆向对象结构
- 不读取文件名作为业务字段
- 不执行 OCR

运行方式：

```powershell
uv run --with olefile python tools/exb_probe.py "D:\path\drawing.exb" -o exb-result.json
```

输出中的 `fields.title_block` 只包含标题栏中实际读到的键值对。没有读到的字段不会被补写。

默认结果只输出标题栏键值。需要继续逆向时，增加 `--include-texts` 才会输出完整文本对象；这些文本不会被归类为业务字段。
