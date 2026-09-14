package dbtest

import "testing"

// TestFixtureBootstrapsMigratedSchema 验证测试基建本身：迁移全部应用后可写入数据。
func TestFixtureBootstrapsMigratedSchema(t *testing.T) {
	db := New(t)
	fixture := db.Seed(t)
	if fixture.Drawing == "" || fixture.Author == "" {
		t.Fatalf("测试数据不完整: %+v", fixture)
	}
	// 迁移 000041 建立的关键表必须存在，否则后续约束测试无从谈起。
	for _, table := range []string{"lifecycle_documents", "patent_records", "patent_events", "drawing_release_snapshots", "change_submission_documents"} {
		var exists bool
		if err := db.Pool.QueryRow(t.Context(), `SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema=current_schema() AND table_name=$1)`, table).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Fatalf("迁移后缺少表 %s", table)
		}
	}
}
