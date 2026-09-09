package drawing

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBOMItemJSONSeparatesDatabaseRowIDFromItemCode(t *testing.T) {
	payload, err := json.Marshal(BOMItem{
		RowID:  "2c5df30c-d51f-43d8-8d31-123456789abc",
		ID:     "2000W.02.03d-01",
		ItemNo: 1,
		Name:   "缸体",
	})
	if err != nil {
		t.Fatalf("marshal BOM item: %v", err)
	}
	body := string(payload)
	if !strings.Contains(body, `"rowId":"2c5df30c-d51f-43d8-8d31-123456789abc"`) {
		t.Fatalf("missing database row id: %s", body)
	}
	if !strings.Contains(body, `"id":"2000W.02.03d-01"`) {
		t.Fatalf("missing item code: %s", body)
	}
}
