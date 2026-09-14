package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cadguanliq/internal/dbtest"
)

func TestNotificationPermissionsAndValidation(t *testing.T) {
	for _, handler := range []http.Handler{Notifications(nil), SendNotifications(nil)} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/notifications", nil))
		if w.Code != 401 {
			t.Fatalf("anonymous access: %d", w.Code)
		}
	}
	w := postJSON(t, SendNotifications(nil), patentActor{ID: "user", Roles: []string{"designer"}}, "POST", "/api/admin/notifications", map[string]any{})
	if w.Code != 403 {
		t.Fatalf("non-admin send: %d", w.Code)
	}
	for _, input := range []sendNotificationInput{
		{Title: " ", Content: "内容", Broadcast: true},
		{Title: "标题", Content: " ", Broadcast: true},
		{Title: strings.Repeat("字", 121), Content: "内容", Broadcast: true},
		{Title: "标题", Content: strings.Repeat("字", 5001), Broadcast: true},
		{Title: "标题", Content: "内容"},
		{Title: "标题", Content: "内容", RecipientIDs: []string{"bad-id"}},
		{Title: "标题", Content: "内容", Broadcast: true, RecipientIDs: []string{"bad-id"}},
	} {
		if err := input.validate(); err == nil {
			t.Fatalf("accepted invalid input: %+v", input)
		}
	}
}

func TestNotificationInboxAndSending(t *testing.T) {
	db := dbtest.New(t)
	fx := seedPatentUsers(t, db)
	admin := patentActor{ID: fx.Admin, Roles: []string{"admin"}}
	owner := patentActor{ID: fx.Owner, Roles: []string{"designer"}}
	outsider := patentActor{ID: fx.Outsider, Roles: []string{"designer"}}
	send := SendNotifications(db.Pool)
	inbox := Notifications(db.Pool)
	input := sendNotificationInput{Title: "测试通知", Content: "请核对图纸", RecipientIDs: []string{fx.Owner, fx.Owner}}
	w := postJSON(t, send, admin, "POST", "/api/admin/notifications", input)
	if w.Code != 201 {
		t.Fatalf("send: %d %s", w.Code, w.Body.String())
	}
	if got := db.ScanString(t, `SELECT count(*)::text FROM notifications`); got != "1" {
		t.Fatalf("duplicate recipients: %s", got)
	}
	id := db.ScanString(t, `SELECT id::text FROM notifications`)
	w = postJSON(t, inbox, outsider, "POST", "/api/notifications/"+id+"/read", nil)
	if w.Code != 404 {
		t.Fatalf("cross-user mark read: %d", w.Code)
	}
	readInbox := func(actor patentActor, path string) (total, unread int, items []notificationItem) {
		t.Helper()
		w := postJSON(t, inbox, actor, "GET", path, nil)
		if w.Code != 200 {
			t.Fatalf("list: %d %s", w.Code, w.Body.String())
		}
		var body struct {
			Data struct {
				Total  int                `json:"total"`
				Unread int                `json:"unread"`
				Items  []notificationItem `json:"items"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body.Data.Total, body.Data.Unread, body.Data.Items
	}
	if total, unread, items := readInbox(outsider, "/api/notifications"); total != 0 || unread != 0 || len(items) != 0 {
		t.Fatal("another user's inbox leaked")
	}
	if total, unread, items := readInbox(owner, "/api/notifications"); total != 1 || unread != 1 || len(items) != 1 || items[0].Content != input.Content {
		t.Fatal("missing inbox notification")
	}
	for i := 0; i < 2; i++ {
		w = postJSON(t, inbox, owner, "POST", "/api/notifications/"+id+"/read", nil)
		if w.Code != 200 {
			t.Fatalf("idempotent read: %d", w.Code)
		}
	}
	if total, unread, _ := readInbox(owner, "/api/notifications?unread=1"); total != 0 || unread != 0 {
		t.Fatal("read item remains unread")
	}
	db.Exec(t, `UPDATE users SET status='disabled' WHERE id=$1::uuid`, fx.Outsider)
	input.RecipientIDs = []string{fx.Owner, fx.Outsider}
	w = postJSON(t, send, admin, "POST", "/api/admin/notifications", input)
	if w.Code != 400 {
		t.Fatalf("disabled recipient: %d", w.Code)
	}
	if got := db.ScanString(t, `SELECT count(*)::text FROM notifications`); got != "1" {
		t.Fatalf("partial delivery: %s", got)
	}
	input.Broadcast = true
	input.RecipientIDs = nil
	w = postJSON(t, send, admin, "POST", "/api/admin/notifications", input)
	if w.Code != 201 {
		t.Fatalf("broadcast: %s", w.Body.String())
	}
	if got := db.ScanString(t, `SELECT count(*)::text FROM notifications WHERE recipient_id=$1::uuid`, fx.Outsider); got != "0" {
		t.Fatal("broadcast included disabled user")
	}
	if got := db.ScanString(t, `SELECT count(*)::text FROM notifications`); got != "4" {
		t.Fatalf("broadcast count: %s", got)
	}
	db.Exec(t, `INSERT INTO notifications(recipient_id,kind,title,event_key) SELECT $1::uuid,'announcement','分页','page:'||n FROM generate_series(1,25) n`, fx.Owner)
	if total, unread, items := readInbox(owner, "/api/notifications?page=2"); total != 27 || unread != 26 || len(items) != 7 {
		t.Fatalf("pagination: %d %d %d", total, unread, len(items))
	}
	w = postJSON(t, inbox, owner, "POST", "/api/notifications/read-all", nil)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if _, unread, _ := readInbox(owner, "/api/notifications"); unread != 0 {
		t.Fatal("read-all failed")
	}
	if _, unread, _ := readInbox(admin, "/api/notifications"); unread != 1 {
		t.Fatal("read-all changed another user's inbox")
	}
}
