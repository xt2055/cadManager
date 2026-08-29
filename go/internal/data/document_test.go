package data

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFindRootDrawingNo(t *testing.T) {
	drawings := []compatibilityDrawing{
		{No: "JG9055e-50/32-00"},
		{No: "PRJ-2026-782"},
		{No: "2000W.02.03c"},
	}

	cases := []struct {
		name     string
		parentNo string
		expected string
	}{
		{"标题栏真值前缀规则", "PRJ-2026-782-01", "PRJ-2026-782"},
		{"一级零件直接等于总图号", "JG9055e-50/32-00", "JG9055e-50/32-00"},
		{"文件名变体一级零件", "JG9055e-5032-01", "JG9055e-50/32-00"},
		{"文件名变体二级零件", "JG9055e-5032-01-1", "JG9055e-50/32-00"},
		{"标题栏真值二级零件", "JG9055e-50/32-01-2", "JG9055e-50/32-00"},
		{"大小写不敏感", "2000W.02.03C-01", "2000W.02.03c"},
		{"不同版本号不混淆", "2000W.02.03D-01", ""},
		{"无关图号", "OTHER-99-01", ""},
		{"空父级", "", ""},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if actual := findRootDrawingNo(drawings, testCase.parentNo); actual != testCase.expected {
				t.Fatalf("findRootDrawingNo(%q) = %q，期望 %q", testCase.parentNo, actual, testCase.expected)
			}
		})
	}
}

func TestFindRootDrawingNoPrefersLongestMatch(t *testing.T) {
	drawings := []compatibilityDrawing{
		{No: "JG-001"},
		{No: "JG-001-02"},
	}
	if actual := findRootDrawingNo(drawings, "JG-001-02-1"); actual != "JG-001-02" {
		t.Fatalf("findRootDrawingNo = %q，期望最长匹配 JG-001-02", actual)
	}
}

// 使用真实数据库文档复现保存流程；事务结束时回滚，不写入任何数据。
func TestSyncCompatibilityRecordsWithRealDocument(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, "postgres://cadguanliq_app:5201314520xt@127.0.0.1:5432/cadguanliq?sslmode=disable")
	if err != nil {
		t.Skipf("本地数据库不可用: %v", err)
	}
	defer pool.Close()

	repository := NewDocumentRepository(pool)
	document, _, err := repository.Load(ctx)
	if err != nil {
		t.Fatalf("加载真实文档失败: %v", err)
	}

	drawings, _ := document["drawings"].([]any)
	document["drawings"] = append(drawings, map[string]any{
		"no": "JG9055e-50/32-00", "name": "2000W卡杆油缸", "kind": "总图",
		"project": "JG9055e-5032-00", "material": "—", "status": "draft", "ver": "v1.0",
	})
	structure, _ := document["structure"].([]any)
	document["structure"] = append(structure,
		map[string]any{"no": "JG9055e-5032-01", "name": "缸体", "parentNo": "JG9055e-50/32-00", "project": "JG9055e-5032-00", "qty": float64(1), "material": "27SiMn"},
		map[string]any{"no": "JG9055e-5032-01-1", "name": "耳环缸头", "parentNo": "JG9055e-5032-01", "project": "JG9055e-5032-00", "qty": float64(1), "material": "27SiMn"},
	)

	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("序列化文档失败: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("开启事务失败: %v", err)
	}
	defer tx.Rollback(ctx)

	if err := syncCompatibilityRecords(ctx, tx, raw, ""); err != nil {
		t.Fatalf("syncCompatibilityRecords 报错: %v", err)
	}
}
