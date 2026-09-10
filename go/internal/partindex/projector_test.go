package partindex

import (
	"encoding/json"
	"testing"
)

func TestAutomaticSelection(t *testing.T) {
	t.Run("仅一个有值空间时自动选择", func(t *testing.T) {
		selected, fields, status, extractionError, err := automaticSelection(sourcePayload{
			Spaces: []sourceSpace{
				{ID: "model", Fields: []sourceField{{Key: "name", Value: ""}}},
				{ID: "layout-1", Fields: []sourceField{
					{Key: "number", Value: " J1233-03 "},
					{Key: "name", Value: " 主动轴 "},
					{Key: "material", Value: " 40Cr "},
				}},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if selected == nil || *selected != "layout-1" {
			t.Fatalf("selected=%v", selected)
		}
		if status != ExtractionExtracted || extractionError != "" {
			t.Fatalf("status=%q error=%q", status, extractionError)
		}
		if fields.DrawingNo != "J1233-03" || fields.PartName != "主动轴" || fields.Material != "40Cr" {
			t.Fatalf("fields=%+v", fields)
		}
	})

	t.Run("多个有值空间时不擅自选择", func(t *testing.T) {
		selected, fields, status, extractionError, err := automaticSelection(sourcePayload{
			Spaces: []sourceSpace{
				{ID: "layout-1", Fields: []sourceField{{Key: "name", Value: "主动轴"}}},
				{ID: "layout-2", Fields: []sourceField{{Key: "name", Value: "从动轴"}}},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if selected != nil || fields != (Fields{}) || status != ExtractionExtracted || extractionError != "" {
			t.Fatalf("selected=%v fields=%+v status=%q error=%q", selected, fields, status, extractionError)
		}
	})

	t.Run("候选值可使空间参与选择但不能自动写入字段", func(t *testing.T) {
		selected, fields, status, extractionError, err := automaticSelection(sourcePayload{
			Spaces: []sourceSpace{{ID: "layout-1", Fields: []sourceField{{Key: "name", Candidates: []string{"主动轴"}}}}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if selected == nil || *selected != "layout-1" || fields.PartName != "" || status != ExtractionExtracted || extractionError != "" {
			t.Fatalf("selected=%v fields=%+v status=%q error=%q", selected, fields, status, extractionError)
		}
	})

	t.Run("解析错误保留为失败状态", func(t *testing.T) {
		selected, fields, status, extractionError, err := automaticSelection(sourcePayload{Error: "  CAD 数据解析失败  "})
		if err != nil {
			t.Fatal(err)
		}
		if selected != nil || fields != (Fields{}) || status != ExtractionFailed || extractionError != "CAD 数据解析失败" {
			t.Fatalf("selected=%v fields=%+v status=%q error=%q", selected, fields, status, extractionError)
		}
	})
}

func TestComputeAutomaticRetainsManualSpace(t *testing.T) {
	state := indexState{
		SelectionMode:   "manual",
		SelectedSpaceID: stringPointer("layout-2"),
	}
	next, err := computeAutomatic(sourcePayload{Spaces: []sourceSpace{
		{ID: "layout-1", Fields: []sourceField{{Key: "name", Value: "主动轴"}}},
		{ID: "layout-2", Fields: []sourceField{{Key: "name", Value: "从动轴"}}},
	}}, state)
	if err != nil {
		t.Fatal(err)
	}
	if next.SelectedSpaceID == nil || *next.SelectedSpaceID != "layout-2" || next.AutoFields.PartName != "从动轴" {
		t.Fatalf("state=%+v", next)
	}
}

func TestComputeAutomaticUsesUpdatedManualSpace(t *testing.T) {
	state := indexState{
		SelectionMode:   "manual",
		SelectedSpaceID: stringPointer("layout-1"),
	}
	// 模拟保存时先应用用户新选择的 layout-2，再刷新自动字段。
	state.SelectedSpaceID = stringPointer("layout-2")
	next, err := computeAutomatic(sourcePayload{Spaces: []sourceSpace{
		{ID: "layout-1", Fields: []sourceField{{Key: "name", Value: "旧布局零件"}}},
		{ID: "layout-2", Fields: []sourceField{{Key: "name", Value: "新布局零件"}}},
	}}, state)
	if err != nil {
		t.Fatal(err)
	}
	if next.SelectedSpaceID == nil || *next.SelectedSpaceID != "layout-2" || next.AutoFields.PartName != "新布局零件" {
		t.Fatalf("自动字段必须跟随新选择的布局：%+v", next)
	}
}

func TestComputeAutomaticRetainsMissingManualSpaceAndManualEmptyFields(t *testing.T) {
	manual := Fields{}
	state := indexState{
		SelectionMode:   "manual",
		SelectedSpaceID: stringPointer("deleted-layout"),
		ManualFields:    &manual,
	}
	next, err := computeAutomatic(sourcePayload{Spaces: []sourceSpace{
		{ID: "layout-1", Fields: []sourceField{{Key: "name", Value: "主动轴"}}},
	}}, state)
	if err != nil {
		t.Fatal(err)
	}
	if next.SelectedSpaceID == nil || *next.SelectedSpaceID != "deleted-layout" || next.AutoFields != (Fields{}) {
		t.Fatalf("人工选中的消失空间不应被擅自替换：%+v", next)
	}
	if got := effectiveFields(next); got != (Fields{}) {
		t.Fatalf("人工空字段必须覆盖自动字段：%+v", got)
	}
}

func TestStatusPriority(t *testing.T) {
	complete := Fields{DrawingNo: "J1233-03", PartName: "主动轴"}
	if got := statusOf(ExtractionFailed, complete, true, true, 1, 2); got != StatusRecheck {
		t.Fatalf("历史确认后快照变化应优先复核，got=%q", got)
	}
	if got := statusOf(ExtractionFailed, complete, true, false, 0, 0); got != StatusFailed {
		t.Fatalf("提取失败不能被人工修订状态掩盖，got=%q", got)
	}
	if got := statusOf(ExtractionPending, Fields{}, false, false, 0, 0); got != StatusPending {
		t.Fatalf("待提取状态错误，got=%q", got)
	}
	if got := statusOf(ExtractionExtracted, Fields{DrawingNo: "J1233-03"}, false, false, 0, 0); got != StatusNeedsConfirmation {
		t.Fatalf("缺少零件名称应提示检查，got=%q", got)
	}
	if got := statusOf(ExtractionExtracted, complete, false, false, 0, 0); got != StatusRecognized {
		t.Fatalf("完整自动识别结果应直接可用，got=%q", got)
	}
	if got := statusOf(ExtractionExtracted, complete, true, false, 0, 0); got != StatusEdited {
		t.Fatalf("完整人工修订结果应标记为已修订，got=%q", got)
	}
}

func TestValidateFilterAcceptsAutomaticAndEditedStatuses(t *testing.T) {
	for _, status := range []string{StatusRecognized, StatusEdited} {
		if _, err := validateFilter(ListFilter{Page: 1, PageSize: 20, Status: status}); err != nil {
			t.Fatalf("status=%q should be accepted: %v", status, err)
		}
	}
}

func TestNormalizedSourcePayloadRepairsLegacyNullCollections(t *testing.T) {
	payload, err := normalizedSourcePayload([]byte(`{"spaces":[{"id":"model","fields":[{"key":"name","value":"主动轴","candidates":null}],"warnings":null}]}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Spaces []struct {
			Fields []struct {
				Candidates []string `json:"candidates"`
			} `json:"fields"`
			Warnings []string `json:"warnings"`
		} `json:"spaces"`
	}
	if err = json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Spaces) != 1 || decoded.Spaces[0].Warnings == nil || decoded.Spaces[0].Fields[0].Candidates == nil {
		t.Fatalf("旧快照未标准化为数组：%s", raw)
	}
}

func stringPointer(value string) *string { return &value }
