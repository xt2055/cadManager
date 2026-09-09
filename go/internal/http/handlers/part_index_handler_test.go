package handlers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cadguanliq/internal/partindex"
)

const partIndexTestID = "22222222-2222-4222-8222-222222222222"
const partIndexUpperID = "AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA"
const partIndexEmptyFields = `{"drawingNo":"","partName":"","material":"","designer":"","checker":"","approver":"","drawingDateRaw":"","scale":"","sheetSize":"","process":"","standard":"","company":""}`

type partIndexStoreStub struct {
	err       error
	saved     bool
	editInput partindex.EditInput
	detailID  string
	editID    string
	backfill  partindex.BackfillInput
	result    partindex.BackfillResult
}

func (s *partIndexStoreStub) List(context.Context, partindex.ListFilter, string, bool) (partindex.Page, error) {
	return partindex.Page{List: []partindex.Item{}, Page: 1, PageSize: 20}, s.err
}

func (s *partIndexStoreStub) Detail(_ context.Context, attachmentID string, _ string, _ bool) (partindex.Detail, error) {
	s.detailID = attachmentID
	return partindex.Detail{}, s.err
}

func (s *partIndexStoreStub) Options(context.Context, string, string, int, string, bool) (partindex.OptionPage, error) {
	return partindex.OptionPage{}, s.err
}

func (s *partIndexStoreStub) Edit(_ context.Context, attachmentID string, _ string, _ bool, input partindex.EditInput) (partindex.Detail, error) {
	s.saved = true
	s.editID = attachmentID
	s.editInput = input
	return partindex.Detail{}, s.err
}

func (s *partIndexStoreStub) Rebuild(context.Context, string, string, bool, partindex.RebuildInput) (string, partindex.Detail, error) {
	return "updated", partindex.Detail{}, s.err
}

func (s *partIndexStoreStub) Backfill(_ context.Context, input partindex.BackfillInput) (partindex.BackfillResult, error) {
	s.backfill = input
	return s.result, s.err
}

func TestPartIndexHandler(t *testing.T) {
	t.Run("未登录不能查询", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/api/part-indexes", nil)
		writer := httptest.NewRecorder()
		PartIndexes(&partIndexStoreStub{})(writer, request)
		if writer.Code != http.StatusUnauthorized {
			t.Fatalf("status=%d body=%s", writer.Code, writer.Body.String())
		}
	})

	t.Run("列表查询", func(t *testing.T) {
		request := authenticatedGet("/api/part-indexes?page=1&page_size=20")
		writer := httptest.NewRecorder()
		PartIndexes(&partIndexStoreStub{})(writer, request)
		if writer.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", writer.Code, writer.Body.String())
		}
	})

	t.Run("不存在映射为404", func(t *testing.T) {
		request := authenticatedGet("/api/part-indexes/" + partIndexTestID)
		writer := httptest.NewRecorder()
		PartIndexes(&partIndexStoreStub{err: partindex.ErrNotFound})(writer, request)
		if writer.Code != http.StatusNotFound {
			t.Fatalf("status=%d body=%s", writer.Code, writer.Body.String())
		}
	})

	t.Run("保存参数无效", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPatch, "/api/part-indexes/"+partIndexTestID, bytes.NewBufferString(`{`)).WithContext(authenticatedGet("/").Context())
		writer := httptest.NewRecorder()
		store := &partIndexStoreStub{}
		PartIndexes(store)(writer, request)
		if writer.Code != http.StatusBadRequest || store.saved {
			t.Fatalf("status=%d saved=%t body=%s", writer.Code, store.saved, writer.Body.String())
		}
	})

	t.Run("保存前校验版本 ID 和未知字段", func(t *testing.T) {
		badVersion := `{"versionId":"not-a-uuid","expectedRevision":0,"expectedSnapshotRevision":0,"action":"save","selectedSpaceId":null,"fields":` + partIndexEmptyFields + `}`
		request := httptest.NewRequest(http.MethodPatch, "/api/part-indexes/"+partIndexTestID, bytes.NewBufferString(badVersion)).WithContext(authenticatedGet("/").Context())
		writer := httptest.NewRecorder()
		store := &partIndexStoreStub{}
		PartIndexes(store)(writer, request)
		if writer.Code != http.StatusBadRequest || store.saved {
			t.Fatalf("错误版本不应进入存储层：status=%d saved=%t body=%s", writer.Code, store.saved, writer.Body.String())
		}

		unknownField := `{"versionId":"` + partIndexTestID + `","expectedRevision":0,"expectedSnapshotRevision":0,"action":"save","selectedSpaceId":null,"fields":{"drawingNo":"","partName":"","material":"","designer":"","checker":"","approver":"","drawingDateRaw":"","scale":"","sheetSize":"","process":"","standard":"","company":"","unexpected":""}}`
		request = httptest.NewRequest(http.MethodPatch, "/api/part-indexes/"+partIndexTestID, bytes.NewBufferString(unknownField)).WithContext(authenticatedGet("/").Context())
		writer = httptest.NewRecorder()
		store = &partIndexStoreStub{}
		PartIndexes(store)(writer, request)
		if writer.Code != http.StatusBadRequest || store.saved {
			t.Fatalf("未知字段不应进入存储层：status=%d saved=%t body=%s", writer.Code, store.saved, writer.Body.String())
		}
	})

	t.Run("字段值必须是字符串，空字符串仍然允许保存", func(t *testing.T) {
		nullField := `{"versionId":"` + partIndexTestID + `","expectedRevision":0,"expectedSnapshotRevision":0,"action":"save","selectedSpaceId":null,"fields":{"drawingNo":"","partName":"","material":null,"designer":"","checker":"","approver":"","drawingDateRaw":"","scale":"","sheetSize":"","process":"","standard":"","company":""}}`
		request := httptest.NewRequest(http.MethodPatch, "/api/part-indexes/"+partIndexTestID, bytes.NewBufferString(nullField)).WithContext(authenticatedGet("/").Context())
		writer := httptest.NewRecorder()
		store := &partIndexStoreStub{}
		PartIndexes(store)(writer, request)
		if writer.Code != http.StatusBadRequest || store.saved {
			t.Fatalf("null 字段不应进入存储层：status=%d saved=%t body=%s", writer.Code, store.saved, writer.Body.String())
		}

		valid := `{"versionId":"` + partIndexTestID + `","expectedRevision":0,"expectedSnapshotRevision":0,"action":"save","selectedSpaceId":null,"fields":` + partIndexEmptyFields + `}`
		request = httptest.NewRequest(http.MethodPatch, "/api/part-indexes/"+partIndexTestID, bytes.NewBufferString(valid)).WithContext(authenticatedGet("/").Context())
		writer = httptest.NewRecorder()
		store = &partIndexStoreStub{}
		PartIndexes(store)(writer, request)
		if writer.Code != http.StatusOK || !store.saved || store.editInput.Fields == nil || store.editInput.Fields.Material != "" {
			t.Fatalf("空字符串字段应进入存储层：status=%d input=%+v body=%s", writer.Code, store.editInput, writer.Body.String())
		}
	})

	t.Run("确认缺少图号或名称映射为400", func(t *testing.T) {
		body := `{"versionId":"` + partIndexTestID + `","expectedRevision":0,"expectedSnapshotRevision":0,"action":"confirm","selectedSpaceId":null,"fields":` + partIndexEmptyFields + `}`
		request := httptest.NewRequest(http.MethodPatch, "/api/part-indexes/"+partIndexTestID, bytes.NewBufferString(body)).WithContext(authenticatedGet("/").Context())
		writer := httptest.NewRecorder()
		store := &partIndexStoreStub{err: partindex.ValidateConfirmation(partindex.Fields{})}
		PartIndexes(store)(writer, request)
		if writer.Code != http.StatusBadRequest {
			t.Fatalf("确认字段缺失应返回400：status=%d body=%s", writer.Code, writer.Body.String())
		}
	})

	t.Run("附件与版本 UUID 统一为小写", func(t *testing.T) {
		body := `{"versionId":"` + partIndexUpperID + `","expectedRevision":0,"expectedSnapshotRevision":0,"action":"save","selectedSpaceId":null,"fields":` + partIndexEmptyFields + `}`
		request := httptest.NewRequest(http.MethodPatch, "/api/part-indexes/"+partIndexUpperID, bytes.NewBufferString(body)).WithContext(authenticatedGet("/").Context())
		writer := httptest.NewRecorder()
		store := &partIndexStoreStub{}
		PartIndexes(store)(writer, request)
		if writer.Code != http.StatusOK || store.editID != "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" || store.editInput.VersionID != "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" {
			t.Fatalf("UUID 未标准化：status=%d attachment=%q version=%q", writer.Code, store.editID, store.editInput.VersionID)
		}
	})

	t.Run("reset 不提交布局和字段", func(t *testing.T) {
		body := `{"versionId":"` + partIndexTestID + `","expectedRevision":0,"expectedSnapshotRevision":0,"action":"reset"}`
		request := httptest.NewRequest(http.MethodPatch, "/api/part-indexes/"+partIndexTestID, bytes.NewBufferString(body)).WithContext(authenticatedGet("/").Context())
		writer := httptest.NewRecorder()
		store := &partIndexStoreStub{}
		PartIndexes(store)(writer, request)
		if writer.Code != http.StatusOK || !store.saved || store.editInput.Fields != nil || store.editInput.SelectedSpaceID != nil {
			t.Fatalf("reset 请求不符合契约：status=%d input=%+v body=%s", writer.Code, store.editInput, writer.Body.String())
		}
	})

	t.Run("补建返回可定位的失败详情并规范化游标 UUID", func(t *testing.T) {
		store := &partIndexStoreStub{result: partindex.BackfillResult{
			Failed: 1,
			Errors: []partindex.BackfillError{{
				AttachmentID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
				Message:      "标题栏快照投影或索引写入失败，请重新提取标题栏后重试",
			}},
		}}
		request := httptest.NewRequest(http.MethodPost, "/api/admin/part-indexes/backfill",
			bytes.NewBufferString(`{"afterAttachmentId":"`+partIndexUpperID+`","limit":50}`))
		writer := httptest.NewRecorder()
		PartIndexBackfill(store)(writer, request)
		if writer.Code != http.StatusOK ||
			store.backfill.AfterAttachmentID != "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" ||
			!strings.Contains(writer.Body.String(), store.result.Errors[0].AttachmentID) ||
			!strings.Contains(writer.Body.String(), store.result.Errors[0].Message) {
			t.Fatalf("补建失败详情或游标不正确：status=%d input=%+v body=%s", writer.Code, store.backfill, writer.Body.String())
		}
	})
}
