package handlers

import (
	"context"
	"net/http"
	"testing"

	"cadguanliq/internal/attachment"
)

func TestCollectReplacedStorageKeys(t *testing.T) {
	document := map[string]any{
		"drawings": []any{
			map[string]any{
				"no": "JG-00",
				"files": []any{
					map[string]any{
						"name": "总图.dwg",
						"history": []any{
							map[string]any{"storageKey": "drawings/JG-00/总图.exb"},
						},
					},
				},
				"otherFiles": []any{
					map[string]any{
						"name": "说明.pdf",
					},
				},
			},
		},
		"structure": []any{
			map[string]any{
				"no": "JG-00-01",
				"files": []any{
					map[string]any{
						"name": "零件.dwg",
						"history": []any{
							map[string]any{"storageKey": "drawings/JG-00/history/零件/零件.exb"},
							map[string]any{"storageKey": "drawings/JG-00/history/零件/零件_旧.dwg"},
						},
					},
				},
			},
		},
	}

	replaced := collectReplacedStorageKeys(document["drawings"].([]any), document["structure"].([]any))
	for _, key := range []string{"drawings/JG-00/总图.exb", "drawings/JG-00/history/零件/零件.exb", "drawings/JG-00/history/零件/零件_旧.dwg"} {
		if !replaced[key] {
			t.Fatalf("collectReplacedStorageKeys() 缺少 %q", key)
		}
	}
	if len(replaced) != 3 {
		t.Fatalf("collectReplacedStorageKeys() = %v; want 3 keys", replaced)
	}
}

type fakeAttachmentRepo struct {
	attachment.Repository
	items []attachment.Attachment
}

func (repo *fakeAttachmentRepo) ListByDrawing(ctx context.Context, drawingNo string) ([]attachment.Attachment, error) {
	return repo.items, nil
}

func TestMergeStoredAttachmentsSkipsReplacedRecords(t *testing.T) {
	partNo := "JG-00-01"
	document := map[string]any{
		"drawings": []any{
			map[string]any{
				"no": "JG-00",
				"files": []any{
					map[string]any{
						"id":         "file-new",
						"name":       "总图.dwg",
						"storageKey": "drawings/JG-00/history/总图/总图_20260902_100000.dwg",
						"history": []any{
							map[string]any{"storageKey": "drawings/JG-00/总图.exb"},
						},
					},
				},
			},
		},
		"structure": []any{},
	}

	replacedRecord := attachment.Attachment{
		ID:         "att-old",
		StorageKey: "drawings/JG-00/总图.exb",
		Name:       "总图.exb",
		DrawingNo:  "JG-00",
		Role:       attachment.RoleAssembly,
	}
	currentRecord := attachment.Attachment{
		ID:         "att-new",
		StorageKey: "drawings/JG-00/history/总图/总图_20260902_100000.dwg",
		Name:       "总图_20260902_100000.dwg",
		DrawingNo:  "JG-00",
		Role:       attachment.RoleAssembly,
	}
	partRecord := attachment.Attachment{
		ID:         "att-part",
		StorageKey: "drawings/JG-00/零件.dwg",
		Name:       "零件.dwg",
		DrawingNo:  "JG-00",
		PartNo:     &partNo,
		Role:       attachment.RolePart,
	}
	repo := &fakeAttachmentRepo{items: []attachment.Attachment{replacedRecord, currentRecord, partRecord}}

	if err := mergeStoredAttachments((&http.Request{}).WithContext(context.Background()), document, repo); err != nil {
		t.Fatalf("mergeStoredAttachments() error = %v", err)
	}
	drawing := document["drawings"].([]any)[0].(map[string]any)
	files := drawing["files"].([]any)
	if len(files) != 1 {
		t.Fatalf("合并后文件数 = %d; want 1（被替换的旧记录应被跳过）", len(files))
	}
	merged := files[0].(map[string]any)
	if merged["id"] != "file-new" {
		t.Fatalf("合并保留的记录 id = %v; want file-new", merged["id"])
	}
	for _, rawFile := range files {
		file := rawFile.(map[string]any)
		if file["id"] == "att-old" {
			t.Fatalf("被替换的旧附件记录 att-old 不应合并进文件清单")
		}
	}
}

func TestUpsertStoredFileUpdatesSameAttachment(t *testing.T) {
	// 同一附件（原始键 K1）经本地编辑后 current 指向 DWG（K2）：
	// 前端清单已有原始形态条目时，应原地更新为当前形态，而不是追加出"原始 + 转换"两条。
	files := []any{
		map[string]any{
			"id":         "file-front",
			"name":       "铜套.exb",
			"storageKey": "drawings/JG-00/铜套.exb",
		},
	}
	stored := map[string]any{
		"id":                "att-1",
		"name":              "铜套.dwg",
		"rawName":           "铜套.exb",
		"size":              "1.2 MB",
		"storageKey":        "drawings/JG-00/铜套.dwg",
		"rawStorageKey":     "drawings/JG-00/铜套.exb",
		"currentStorageKey": "drawings/JG-00/铜套.dwg",
		"mimeType":          "application/acad",
		"previewable":       true,
	}

	result := upsertStoredFile(files, stored)
	if len(result) != 1 {
		t.Fatalf("upsert 后文件数 = %d; want 1（同一附件不应重复出现）", len(result))
	}
	file := result[0].(map[string]any)
	if file["name"] != "铜套.dwg" || file["storageKey"] != "drawings/JG-00/铜套.dwg" || file["rawStorageKey"] != "drawings/JG-00/铜套.exb" {
		t.Fatalf("upsert 后文件 = %v; want 更新为当前 DWG 形态", file)
	}
	if file["id"] != "file-front" {
		t.Fatalf("upsert 应保留前端文件 id; got %v", file["id"])
	}
}

func TestUpsertStoredFileDeduplicatesSameName(t *testing.T) {
	// 同名文件被重复上传时会留下两个存储键不同的记录：清单里只应展示一条。
	files := []any{
		map[string]any{"id": "file-1", "name": "标件.exb", "storageKey": "drawings/JG-00/标件.exb"},
	}
	stored := map[string]any{
		"id":            "att-2",
		"name":          "标件.exb",
		"storageKey":    "drawings/JG-00/标件_副本.exb",
		"rawStorageKey": "drawings/JG-00/标件_副本.exb",
		"previewable":   true,
	}

	result := upsertStoredFile(files, stored)
	if len(result) != 1 {
		t.Fatalf("同名文件合并后数量 = %d; want 1", len(result))
	}
	file := result[0].(map[string]any)
	if file["id"] != "file-1" {
		t.Fatalf("同名合并应保留前端条目; got %v", file["id"])
	}
	if file["storageKey"] != "drawings/JG-00/标件_副本.exb" {
		t.Fatalf("同名合并应更新为最新记录的存储键; got %v", file["storageKey"])
	}
}

func TestUpsertStoredFileAppendsNewAttachment(t *testing.T) {
	files := []any{
		map[string]any{"id": "file-a", "name": "其他.pdf", "storageKey": "drawings/JG-00/其他.pdf"},
	}
	stored := map[string]any{
		"id":            "att-new",
		"name":          "新零件.dwg",
		"storageKey":    "drawings/JG-00/新零件.dwg",
		"rawStorageKey": "drawings/JG-00/新零件.dwg",
		"previewable":   true,
	}

	result := upsertStoredFile(files, stored)
	if len(result) != 2 {
		t.Fatalf("upsert 后文件数 = %d; want 2", len(result))
	}
}

func TestMergeStoredAttachmentsKeepsCurrentStorageKeyFallback(t *testing.T) {
	// 记录 A 是原始 EXB，CurrentStorageKey 指向后台转换的 DWG（未替换场景）：
	// 没有任何 history 引用时应正常合并，且展示当前 DWG 名称。
	document := map[string]any{
		"drawings": []any{
			map[string]any{"no": "JG-00"},
		},
		"structure": []any{},
	}
	record := attachment.Attachment{
		ID:                "att-exb",
		StorageKey:        "drawings/JG-00/总图.exb",
		CurrentStorageKey: "drawings/JG-00/总图.dwg",
		CurrentName:       "总图.dwg",
		Name:              "总图.exb",
		DrawingNo:         "JG-00",
		Role:              attachment.RoleAssembly,
	}
	repo := &fakeAttachmentRepo{items: []attachment.Attachment{record}}

	if err := mergeStoredAttachments((&http.Request{}).WithContext(context.Background()), document, repo); err != nil {
		t.Fatalf("mergeStoredAttachments() error = %v", err)
	}
	drawing := document["drawings"].([]any)[0].(map[string]any)
	files := drawing["files"].([]any)
	if len(files) != 1 {
		t.Fatalf("合并后文件数 = %d; want 1", len(files))
	}
	merged := files[0].(map[string]any)
	if merged["name"] != "总图.dwg" || merged["rawName"] != "总图.exb" {
		t.Fatalf("合并文件 name/rawName = %v/%v; want 总图.dwg/总图.exb", merged["name"], merged["rawName"])
	}
}
