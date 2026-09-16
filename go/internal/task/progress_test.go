package task

import "testing"

// 进度是产品口径：百分比与阶段一起被断言，避免以后有人只改百分比
// 而让首页任务面板与任务管理台讲出两套不同的进度。
func TestDeriveProgressFromDrawingLifecycle(t *testing.T) {
	tests := []struct {
		name        string
		status      string
		fileCount   int
		reviewNode  string
		done, total int
		wantPercent int
		wantStage   string
		wantDone    bool
	}{
		{name: "草稿无文件", status: "draft", fileCount: 0, wantPercent: 0, wantStage: "待上传图纸"},
		{name: "草稿有文件", status: "draft", fileCount: 3, wantPercent: 30, wantStage: "编制中"},
		{name: "审核刚开始", status: "reviewing", fileCount: 2, reviewNode: "设计自检", done: 0, total: 3, wantPercent: 60, wantStage: "审核中"},
		{name: "审核进行一半", status: "reviewing", fileCount: 2, reviewNode: "主管批准", done: 2, total: 4, wantPercent: 75, wantStage: "审核中"},
		{name: "审核全部通过待发布", status: "reviewing", fileCount: 2, done: 3, total: 3, wantPercent: 90, wantStage: "审核中"},
		{name: "审核节点缺失时退回基准值", status: "reviewing", fileCount: 1, wantPercent: 60, wantStage: "审核中"},
		{name: "生产中即交付完成", status: "published", fileCount: 5, wantPercent: 100, wantStage: "生产中", wantDone: true},
		{name: "已存档即交付完成", status: "archived", fileCount: 5, wantPercent: 100, wantStage: "已存档", wantDone: true},
		{name: "已停用退回零进度", status: "disabled", fileCount: 4, wantPercent: 0, wantStage: "已停用"},
		{name: "未知状态不编造进度", status: "unknown", fileCount: 1, wantPercent: 0, wantStage: "状态未知"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Derive(tt.status, tt.fileCount, tt.reviewNode, tt.done, tt.total)
			if got.Percent != tt.wantPercent || got.Stage != tt.wantStage || got.Done != tt.wantDone {
				t.Fatalf("Derive() = {percent:%d stage:%q done:%v}, want {percent:%d stage:%q done:%v}",
					got.Percent, got.Stage, got.Done, tt.wantPercent, tt.wantStage, tt.wantDone)
			}
			// 每个阶段都必须告诉用户下一步做什么，否则进度只是一个没有行动指向的数字。
			if got.Detail == "" {
				t.Fatal("Derive() 必须给出说明文字")
			}
		})
	}
}

// 审核节点数多于节点总数时不产生超过刻度上限的百分比。
func TestDeriveReviewProgressIsClamped(t *testing.T) {
	got := Derive("reviewing", 1, "批准", 9, 2)
	if got.Percent != 90 {
		t.Fatalf("percent = %d, want 90", got.Percent)
	}
}
