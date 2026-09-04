package drawing

import (
	"strings"

	"golang.org/x/text/unicode/norm"
)

// NormalizePartNo 是服务端唯一的零件图号规范化入口。
// NFKC 会把全角 ASCII、兼容字符转换为规范形式，再统一大小写和空白。
func NormalizePartNo(value string) string {
	return strings.ToUpper(strings.TrimSpace(norm.NFKC.String(value)))
}
