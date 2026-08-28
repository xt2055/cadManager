package cadtext

import (
	"bytes"
	"fmt"
	"os"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// NormalizeDxfForCaxa 将 UTF-8 DXF 转为 CAXA 常用的 ANSI_936 编码。
func NormalizeDxfForCaxa(content []byte) ([]byte, error) {
	if len(content) == 0 || !utf8.Valid(content) {
		return content, nil
	}
	if !bytes.Contains(content, []byte("SECTION")) && !bytes.Contains(content, []byte("ENTITIES")) {
		return content, nil
	}

	// dxfOut 返回 UTF-8 文本，而 CAXA 按 DXF 头部代码页解释中文。
	// 无论原头部是 ANSI_1252、UTF-8 还是缺少代码页声明，都统一写成 ANSI_936。
	content = bytes.Replace(content, []byte("ANSI_1252"), []byte("ANSI_936"), 1)
	content = bytes.Replace(content, []byte("ANSI_1200"), []byte("ANSI_936"), 1)
	content = bytes.Replace(content, []byte("UTF-8"), []byte("ANSI_936"), 1)
	if !bytes.Contains(content, []byte("$DWGCODEPAGE")) {
		content = insertCodePageHeader(content)
	}
	encoded, _, err := transform.Bytes(simplifiedchinese.GB18030.NewEncoder(), content)
	if err != nil {
		return nil, fmt.Errorf("DXF 中文编码转换失败: %w", err)
	}
	return encoded, nil
}

func insertCodePageHeader(content []byte) []byte {
	for _, newline := range [][]byte{[]byte("\r\n"), []byte("\n")} {
		marker := append([]byte("  2"), newline...)
		marker = append(marker, append([]byte("HEADER"), newline...)...)
		if index := bytes.Index(content, marker); index >= 0 {
			insert := append([]byte("  9"), newline...)
			insert = append(insert, append([]byte("$DWGCODEPAGE"), newline...)...)
			insert = append(insert, append([]byte("  3"), newline...)...)
			insert = append(insert, append([]byte("ANSI_936"), newline...)...)
			position := index + len(marker)
			return append(append(content[:position], insert...), content[position:]...)
		}
	}
	return content
}

func NormalizeDxfFileForCaxa(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取 DXF 文件失败: %w", err)
	}
	normalized, err := NormalizeDxfForCaxa(content)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, normalized, 0o600); err != nil {
		return fmt.Errorf("写入 CAXA 兼容 DXF 失败: %w", err)
	}
	return nil
}
