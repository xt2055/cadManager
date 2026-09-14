package change

import (
	"context"
	"strings"
	"cadguanliq/internal/auth"
	"regexp"
	"testing"
)

// TestVerifyAlwaysRequiresReviewCenter 变更不再支持管理员直接验收发布。
//
// 完成路径改为在图纸审核中心按完整流程签署，通过后由 CompleteReview 发布；
// Verify 必须一律拒绝，否则会绕过节点签署直接发布本轮冻结版本。
func TestVerifyAlwaysRequiresReviewCenter(t *testing.T) {
	s := NewService(nil)
	cases := []struct {
		name       string
		submission string
	}{
		{"缺少提交轮次", ""},
		{"空白提交轮次", "  "},
		{"提供了提交轮次", "submission-1"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			_, err := s.Verify(context.Background(), auth.AuthUser{Roles: []string{"admin"}}, "request", DecisionInput{Opinion: "approve", SubmissionID: item.submission})
			if err == nil {
				t.Fatal("Verify 必须拒绝直接验收")
			}
			if !strings.Contains(err.Error(), "完整流程") {
				t.Fatalf("错误信息应说明须走完整审核流程，实际 %v", err)
			}
		})
	}
}

func TestStatusOpen(t *testing.T) {
	cases := map[Status]bool{
		StatusPendingApproval: true,
		StatusExecuting:       true,
		StatusPendingVerify:   true,
		StatusCompleted:       false,
		StatusRejected:        false,
		StatusCancelled:       false,
	}
	for status, want := range cases {
		if got := status.Open(); got != want {
			t.Errorf("Status(%s).Open() = %v, want %v", status, got, want)
		}
	}
}

func TestWaivedReason(t *testing.T) {
	if got := waivedReason(true, false, "  历史原因  "); got != "历史原因" {
		t.Errorf("waivedReason autoApprove no-verify = %q, want 历史原因", got)
	}
	if got := waivedReason(true, true, "原因"); got != "" {
		t.Errorf("waivedReason with verify required should be empty, got %q", got)
	}
	if got := waivedReason(false, false, "原因"); got != "" {
		t.Errorf("waivedReason without autoApprove should be empty, got %q", got)
	}
}

func TestGenerateRequestNo(t *testing.T) {
	pattern := regexp.MustCompile(`^CR\d{8}-[0-9a-f]{8}$`)
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		value, err := generateRequestNo()
		if err != nil {
			t.Fatalf("generateRequestNo error: %v", err)
		}
		if !pattern.MatchString(value) {
			t.Fatalf("request no %q does not match %s", value, pattern)
		}
		if seen[value] {
			t.Fatalf("duplicate request no generated: %s", value)
		}
		seen[value] = true
	}
}
