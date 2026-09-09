package change

import (
	"context"
	"errors"
	"cadguanliq/internal/auth"
	"regexp"
	"testing"
)

func TestVerifyRequiresSubmissionBeforeDatabaseAccess(t *testing.T) {
	s := NewService(nil)
	for _, id := range []string{"", "  "} {
		_, err := s.Verify(context.Background(), auth.AuthUser{Roles: []string{"admin"}}, "request", DecisionInput{Opinion: "approve", SubmissionID: id})
		if !errors.Is(err, ErrStaleSubmit) { t.Fatalf("missing submission: %v", err) }
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
