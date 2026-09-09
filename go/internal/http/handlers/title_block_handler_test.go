package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cadguanliq/internal/titleblock"
)

type titleStoreStub struct {
	err       error
	saved     bool
	id        string
	versionID string
}

func (s *titleStoreStub) Get(context.Context, string, string, bool) (titleblock.Snapshot, error) {
	return titleblock.Snapshot{AttachmentID: "a"}, s.err
}
func (s *titleStoreStub) Save(_ context.Context, id, versionID, _ string, _ bool, _ titleblock.Payload) error {
	s.saved = true
	s.id = id
	s.versionID = versionID
	return s.err
}

const titleTestID = "11111111-1111-4111-8111-111111111111"
const titleUpperID = "AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA"

func TestTitleBlockHandler(t *testing.T) {
	for _, tc := range []struct {
		name          string
		method        string
		authenticated bool
		err           error
		status        int
	}{
		{"未登录", http.MethodGet, false, nil, 401}, {"读取", http.MethodGet, true, nil, 200},
		{"不存在", http.MethodGet, true, titleblock.ErrNotFound, 404}, {"保存", http.MethodPut, true, nil, 200},
		{"拒绝越权", http.MethodPut, true, titleblock.ErrForbidden, 403}, {"版本冲突", http.MethodPut, true, titleblock.ErrConflict, 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(map[string]any{"versionId": titleTestID, "payload": titleblock.Payload{Spaces: []titleblock.Space{}}})
			req := httptest.NewRequest(tc.method, "/api/cad/title-blocks/"+titleTestID, bytes.NewReader(raw))
			if tc.authenticated {
				req = req.WithContext(authenticatedGet("/").Context())
			}
			store := &titleStoreStub{err: tc.err}
			w := httptest.NewRecorder()
			TitleBlocks(store)(w, req)
			if w.Code != tc.status {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if !tc.authenticated && store.saved {
				t.Fatal("未认证请求不应写入")
			}
		})
	}
}

func TestTitleBlockCanonicalizesUUIDs(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{"versionId": titleUpperID, "payload": titleblock.Payload{Spaces: []titleblock.Space{}}})
	req := httptest.NewRequest(http.MethodPut, "/api/cad/title-blocks/"+titleUpperID, bytes.NewReader(raw)).WithContext(authenticatedGet("/").Context())
	store := &titleStoreStub{}
	w := httptest.NewRecorder()
	TitleBlocks(store)(w, req)
	if w.Code != http.StatusOK || store.id != "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" || store.versionID != "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" {
		t.Fatalf("UUID 未标准化：status=%d id=%q version=%q", w.Code, store.id, store.versionID)
	}
}

func TestTitleBlockRejectInvalidInput(t *testing.T) {
	for _, body := range []string{`{}`, `{"versionId":"bad"}`, `{"versionId":"` + titleTestID + `","payload":{"error":"` + strings.Repeat("x", 300000) + `"}}`} {
		req := httptest.NewRequest(http.MethodPut, "/api/cad/title-blocks/"+titleTestID, strings.NewReader(body)).WithContext(authenticatedGet("/").Context())
		store := &titleStoreStub{}
		w := httptest.NewRecorder()
		TitleBlocks(store)(w, req)
		if w.Code != 400 || store.saved {
			t.Fatalf("非法请求被接受：%d", w.Code)
		}
	}
}
