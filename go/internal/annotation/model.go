package annotation

import (
	"errors"
	"math"
	"regexp"
	"strings"
)

var ErrForbidden = errors.New("当前节点不属于你或已签署，批注仅可查看")
var ErrConflict = errors.New("批注已在其他窗口更新，请保留未保存内容后重新读取")
var ErrNotFound = errors.New("本次审核没有该文件的固定版本，无法添加批注")

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}
type Mark struct {
	ID     string  `json:"id"`
	Kind   string  `json:"kind"`
	Layout string  `json:"layout"`
	Points []Point `json:"points"`
	Text   string  `json:"text"`
	Color  string  `json:"color"`
	Width  float64 `json:"width"`
}
type Content struct {
	SchemaVersion int    `json:"schemaVersion"`
	Marks         []Mark `json:"marks"`
}
type Document struct {
	ID         string  `json:"id"`
	NodeID     string  `json:"nodeId"`
	NodeName   string  `json:"nodeName"`
	AuthorID   string  `json:"authorId"`
	AuthorName string  `json:"authorName"`
	Revision   int64   `json:"revision"`
	UpdatedAt  string  `json:"updatedAt"`
	Content    Content `json:"content"`
}
type Workspace struct {
	CaseID       string     `json:"caseId"`
	AttachmentID string     `json:"attachmentId"`
	VersionID    string     `json:"versionId"`
	NodeID       string     `json:"nodeId"`
	NodeName     string     `json:"nodeName"`
	CanEdit      bool       `json:"canEdit"`
	Documents    []Document `json:"documents"`
}
type SaveInput struct {
	CaseID       string  `json:"caseId"`
	AttachmentID string  `json:"attachmentId"`
	VersionID    string  `json:"versionId"`
	NodeID       string  `json:"nodeId"`
	Revision     int64   `json:"revision"`
	Content      Content `json:"content"`
}
type Template struct {
	ID       string `json:"id"`
	OwnerID  string `json:"ownerId"`
	Category string `json:"category"`
	Text     string `json:"text"`
}

type ReviewFile struct {
	AttachmentID string `json:"attachmentId"`
	VersionID string `json:"versionId"`
	Name string `json:"name"`
	Version string `json:"version"`
	MarkCount int `json:"markCount"`
}

var colorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func ValidID(id string) bool { return uuidPattern.MatchString(id) }
func Validate(content Content) error {
	if content.SchemaVersion != 1 || len(content.Marks) > 1000 {
		return errors.New("批注格式不支持或超过 1000 条")
	}
	ids := map[string]bool{}
	total := 0
	for _, m := range content.Marks {
		if m.ID == "" || len(m.ID) > 80 || ids[m.ID] || strings.TrimSpace(m.Layout) == "" || len(m.Layout) > 200 {
			return errors.New("批注标识或布局无效")
		}
		ids[m.ID] = true
		if !colorPattern.MatchString(m.Color) || math.IsNaN(m.Width) || math.IsInf(m.Width, 0) || m.Width < 1 || m.Width > 12 || len([]rune(m.Text)) > 2000 {
			return errors.New("批注文字或样式无效")
		}
		n := len(m.Points)
		total += n
		switch m.Kind {
		case "pen":
			if n < 2 || n > 10000 {
				return errors.New("笔画点数无效")
			}
		case "rect", "ellipse", "arrow":
			if n != 2 {
				return errors.New("图形需要两个定位点")
			}
		case "text", "check", "cross":
			if n != 1 {
				return errors.New("标记需要一个定位点")
			}
		default:
			return errors.New("不支持的批注工具")
		}
		if m.Kind == "text" && strings.TrimSpace(m.Text) == "" {
			return errors.New("文字批注不能为空")
		}
		for _, p := range m.Points {
			if math.IsNaN(p.X) || math.IsNaN(p.Y) || math.IsInf(p.X, 0) || math.IsInf(p.Y, 0) || math.Abs(p.X) > 1e12 || math.Abs(p.Y) > 1e12 {
				return errors.New("批注坐标无效")
			}
		}
	}
	if total > 100000 {
		return errors.New("笔画点数过多，请分次整理批注")
	}
	return nil
}
