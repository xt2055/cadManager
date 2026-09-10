package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"cadguanliq/internal/audit"
	"cadguanliq/internal/auth"
	"cadguanliq/internal/http/middleware"
)

type fakeDrawingAuditRepository struct {
	filter audit.ListFilter
	page   audit.Page
}

func (repository *fakeDrawingAuditRepository) Create(context.Context, audit.CreateInput, string, string, string, string) (audit.Log, error) {
	return audit.Log{}, nil
}

func (repository *fakeDrawingAuditRepository) List(_ context.Context, filter audit.ListFilter) (audit.Page, error) {
	repository.filter = filter
	return repository.page, nil
}

type fakeAuditOptionRepository struct {
	filter audit.OptionFilter
	page   audit.OptionPage
}

func (repository *fakeAuditOptionRepository) Options(_ context.Context, filter audit.OptionFilter) (audit.OptionPage, error) {
	repository.filter = filter
	return repository.page, nil
}

type fakeAdminAuditRepository struct {
	filter audit.ListFilter
	page   audit.Page
}

func (repository *fakeAdminAuditRepository) ListAdmin(_ context.Context, filter audit.ListFilter) (audit.Page, error) {
	repository.filter = filter
	return repository.page, nil
}

func TestAuditLogOptionsBuildsScopedFilter(t *testing.T) {
	repository := &fakeAuditOptionRepository{page: audit.OptionPage{List: []audit.Option{}}}
	request := httptest.NewRequest(http.MethodGet, "/api/drawing-operation-logs/options?kind=drawing&keyword=JG&limit=30", nil)
	request = request.WithContext(context.WithValue(request.Context(), middleware.AuthUserContextKey, auth.AuthUser{ID: "user-1"}))
	response := httptest.NewRecorder()

	AuditLogOptions(repository, false).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if repository.filter.Kind != "drawing" || repository.filter.Keyword != "JG" || repository.filter.Limit != 30 || repository.filter.AdminOnly {
		t.Fatalf("筛选条件 = %+v", repository.filter)
	}
}

func TestAdminAuditLogOptionsScopesToAdminLogs(t *testing.T) {
	repository := &fakeAuditOptionRepository{page: audit.OptionPage{List: []audit.Option{}}}
	request := httptest.NewRequest(http.MethodGet, "/api/admin/audit-logs/options?kind=actor", nil)
	request = request.WithContext(context.WithValue(request.Context(), middleware.AuthUserContextKey, auth.AuthUser{ID: "admin-1", Roles: []string{"admin"}}))
	response := httptest.NewRecorder()

	AuditLogOptions(repository, true).ServeHTTP(response, request)

	if response.Code != http.StatusOK || !repository.filter.AdminOnly || repository.filter.Kind != "actor" {
		t.Fatalf("status/filter = %d/%+v", response.Code, repository.filter)
	}
}

func TestAuditLogOptionsRejectsInvalidKind(t *testing.T) {
	repository := &fakeAuditOptionRepository{}
	request := httptest.NewRequest(http.MethodGet, "/api/drawing-operation-logs/options?kind=unknown", nil)
	request = request.WithContext(context.WithValue(request.Context(), middleware.AuthUserContextKey, auth.AuthUser{ID: "user-1"}))
	response := httptest.NewRecorder()

	AuditLogOptions(repository, false).ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestDrawingOperationLogsBuildsServerFilter(t *testing.T) {
	repository := &fakeDrawingAuditRepository{page: audit.Page{List: []audit.Log{}, Total: 0}}
	request := httptest.NewRequest(http.MethodGet, "/api/drawing-operation-logs?page=3&page_size=50&action=edit&drawing_no=JG-00&actor=%E5%BC%A0%E5%B7%A5&keyword=%E8%AE%BE%E8%AE%A1", nil)
	request = request.WithContext(context.WithValue(request.Context(), middleware.AuthUserContextKey, auth.AuthUser{ID: "user-1"}))
	response := httptest.NewRecorder()

	DrawingOperationLogs(repository).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if repository.filter.Page != 3 || repository.filter.PageSize != 50 {
		t.Fatalf("分页 = %d/%d, want 3/50", repository.filter.Page, repository.filter.PageSize)
	}
	if repository.filter.Action != "edit" || repository.filter.DrawingNo != "JG-00" || repository.filter.Actor != "张工" || repository.filter.Keyword != "设计" {
		t.Fatalf("筛选条件 = %+v", repository.filter)
	}
}

func TestAdminOperationLogsBuildsServerFilter(t *testing.T) {
	repository := &fakeAdminAuditRepository{page: audit.Page{List: []audit.Log{}, Total: 0}}
	request := httptest.NewRequest(http.MethodGet, "/api/admin/audit-logs?page=2&page_size=50&action=delete&drawing_no=JG-00&target_type=file&actor_id=admin-1&result=failed&keyword=%E5%9B%9E%E9%80%80&from=2026-09-01&to=2026-09-03", nil)
	request = request.WithContext(context.WithValue(request.Context(), middleware.AuthUserContextKey, auth.AuthUser{ID: "admin-1", Roles: []string{"admin"}}))
	response := httptest.NewRecorder()

	AdminOperationLogs(repository).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if repository.filter.Page != 2 || repository.filter.PageSize != 50 {
		t.Fatalf("分页 = %d/%d, want 2/50", repository.filter.Page, repository.filter.PageSize)
	}
	if repository.filter.Action != "delete" || repository.filter.DrawingNo != "JG-00" || repository.filter.TargetType != "file" || repository.filter.ActorID != "admin-1" || repository.filter.Result != "failed" || repository.filter.Keyword != "回退" {
		t.Fatalf("筛选条件 = %+v", repository.filter)
	}
	if repository.filter.From.IsZero() || repository.filter.To.IsZero() || repository.filter.To.Sub(repository.filter.From).Hours() != 72 {
		t.Fatalf("日期范围 = %v - %v, want 72 hours", repository.filter.From, repository.filter.To)
	}
	var body struct {
		Code int        `json:"code"`
		Data audit.Page `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if body.Code != 0 {
		t.Fatalf("响应 code = %d, want 0", body.Code)
	}
}

func TestAdminOperationLogsRejectsInvalidResult(t *testing.T) {
	repository := &fakeAdminAuditRepository{}
	request := httptest.NewRequest(http.MethodGet, "/api/admin/audit-logs?result=unknown", nil)
	request = request.WithContext(context.WithValue(request.Context(), middleware.AuthUserContextKey, auth.AuthUser{Roles: []string{"admin"}}))
	response := httptest.NewRecorder()

	AdminOperationLogs(repository).ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}
