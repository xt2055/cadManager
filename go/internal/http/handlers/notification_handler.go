package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"cadguanliq/internal/http/middleware"
	"cadguanliq/internal/response"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type notificationItem struct {
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`
	Title      string     `json:"title"`
	Content    string     `json:"content"`
	SenderName string     `json:"senderName"`
	DrawingID  string     `json:"drawingId"`
	// DrawingNo 与 Target 供前端直接跳转：待办审核需要图纸编号才能打开审核工作台。
	DrawingNo string     `json:"drawingNo"`
	Target    string     `json:"target"`
	CreatedAt time.Time  `json:"createdAt"`
	ReadAt    *time.Time `json:"readAt"`
}

// Notifications scopes every read and mutation to the authenticated recipient.
func Notifications(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := middleware.UserFromContext(r.Context())
		if !ok {
			response.WriteError(w, 401, "请先登录")
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/api/notifications")
		if path == "" && r.Method == http.MethodGet {
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			if page < 1 {
				page = 1
			}
			if page > 1000000 {
				response.WriteError(w, 400, "页码无效")
				return
			}
			unreadOnly := r.URL.Query().Get("unread") == "1"
			tx, err := pool.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
			if err != nil {
				response.WriteError(w, 500, "读取通知失败")
				return
			}
			defer tx.Rollback(r.Context())
			var total, unread int
			err = tx.QueryRow(r.Context(), `SELECT count(*) FILTER (WHERE NOT $2 OR read_at IS NULL), count(*) FILTER (WHERE read_at IS NULL) FROM notifications WHERE recipient_id=$1::uuid`, user.ID, unreadOnly).Scan(&total, &unread)
			if err != nil {
				response.WriteError(w, 500, "读取通知失败")
				return
			}
			rows, err := tx.Query(r.Context(), `SELECT n.id::text,n.kind,n.title,n.content,COALESCE(u.display_name,u.account,'系统'),COALESCE(n.drawing_id::text,''),COALESCE(d.drawing_no,''),CASE WHEN n.event_key LIKE 'review-turn:%' THEN 'review-workspace' ELSE '' END,n.created_at,n.read_at
    FROM notifications n LEFT JOIN users u ON u.id=n.sender_id LEFT JOIN drawings d ON d.id=n.drawing_id
    WHERE n.recipient_id=$1::uuid AND (NOT $2 OR n.read_at IS NULL)
    ORDER BY n.created_at DESC,n.id DESC LIMIT 20 OFFSET $3`, user.ID, unreadOnly, (page-1)*20)
			if err != nil {
				response.WriteError(w, 500, "读取通知失败")
				return
			}
			items := make([]notificationItem, 0)
			for rows.Next() {
				var item notificationItem
				if err = rows.Scan(&item.ID, &item.Kind, &item.Title, &item.Content, &item.SenderName, &item.DrawingID, &item.DrawingNo, &item.Target, &item.CreatedAt, &item.ReadAt); err != nil {
					break
				}
				items = append(items, item)
			}
			rows.Close()
			if err != nil || rows.Err() != nil {
				response.WriteError(w, 500, "读取通知失败")
				return
			}
			if err = tx.Commit(r.Context()); err != nil {
				response.WriteError(w, 500, "读取通知失败")
				return
			}
			response.WriteData(w, 200, map[string]any{"items": items, "total": total, "unread": unread, "page": page, "pageSize": 20})
			return
		}
		if r.Method != http.MethodPost {
			response.WriteError(w, 405, "method not allowed")
			return
		}
		if path == "/read-all" {
			if _, err := pool.Exec(r.Context(), `UPDATE notifications SET read_at=now() WHERE recipient_id=$1::uuid AND read_at IS NULL`, user.ID); err != nil {
				response.WriteError(w, 500, "标记已读失败")
				return
			}
			response.WriteData(w, 200, map[string]bool{"ok": true})
			return
		}
		segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
		if len(segments) != 2 || segments[1] != "read" || !notificationUUID(segments[0]) {
			response.WriteError(w, 404, "通知不存在")
			return
		}
		result, err := pool.Exec(r.Context(), `UPDATE notifications SET read_at=COALESCE(read_at,now()) WHERE id=$1::uuid AND recipient_id=$2::uuid`, segments[0], user.ID)
		if err != nil {
			response.WriteError(w, 500, "标记已读失败")
			return
		}
		if result.RowsAffected() == 0 {
			response.WriteError(w, 404, "通知不存在")
			return
		}
		response.WriteData(w, 200, map[string]bool{"ok": true})
	}
}

type sendNotificationInput struct {
	Title        string   `json:"title"`
	Content      string   `json:"content"`
	RecipientIDs []string `json:"recipientIds"`
	Broadcast    bool     `json:"broadcast"`
}

func notificationUUID(value string) bool {
	var id pgtype.UUID
	return len(value) == 36 && id.Scan(value) == nil && id.Valid
}

func (input *sendNotificationInput) validate() error {
	input.Title = strings.TrimSpace(input.Title)
	input.Content = strings.TrimSpace(input.Content)
	if input.Title == "" || utf8.RuneCountInString(input.Title) > 120 {
		return errors.New("通知标题需为 1–120 个字")
	}
	if input.Content == "" || utf8.RuneCountInString(input.Content) > 5000 {
		return errors.New("通知正文需为 1–5000 个字")
	}
	if input.Broadcast && len(input.RecipientIDs) > 0 {
		return errors.New("群发全体时请勿指定收件人")
	}
	if !input.Broadcast && (len(input.RecipientIDs) == 0 || len(input.RecipientIDs) > 1000) {
		return errors.New("请选择 1–1000 位收件人")
	}
	seen := map[string]bool{}
	ids := make([]string, 0, len(input.RecipientIDs))
	for _, id := range input.RecipientIDs {
		id = strings.ToLower(strings.TrimSpace(id))
		if !notificationUUID(id) {
			return errors.New("收件人无效")
		}
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	input.RecipientIDs = ids
	return nil
}

func SendNotifications(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := middleware.UserFromContext(r.Context())
		if !ok {
			response.WriteError(w, 401, "请先登录")
			return
		}
		if !hasAdminRole(user.Roles) {
			response.WriteError(w, 403, "只有管理员可以发送通知")
			return
		}
		if r.Method != http.MethodPost {
			response.WriteError(w, 405, "method not allowed")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 128*1024)
		var input sendNotificationInput
		if err := decodeJSON(r, &input); err != nil {
			response.WriteError(w, 400, "通知参数格式无效")
			return
		}
		if err := input.validate(); err != nil {
			response.WriteError(w, 400, err.Error())
			return
		}
		tx, err := pool.Begin(r.Context())
		if err != nil {
			response.WriteError(w, 500, "发送通知失败")
			return
		}
		defer tx.Rollback(r.Context())
		// Lock the selected users while validating and inserting so a disabled or missing
		// recipient cannot silently turn a targeted send into a partial delivery.
		rows, err := tx.Query(r.Context(), `SELECT id::text FROM users WHERE status='active' AND ($1 OR id=ANY($2::uuid[])) FOR SHARE`, input.Broadcast, input.RecipientIDs)
		if err != nil {
			response.WriteError(w, 500, "读取收件人失败")
			return
		}
		ids := make([]string, 0)
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				break
			}
			ids = append(ids, id)
		}
		rows.Close()
		if err != nil || rows.Err() != nil {
			response.WriteError(w, 500, "读取收件人失败")
			return
		}
		if len(ids) == 0 || (!input.Broadcast && len(ids) != len(input.RecipientIDs)) {
			response.WriteError(w, 400, "收件人不存在或已停用，请刷新后重试")
			return
		}
		_, err = tx.Exec(r.Context(), `INSERT INTO notifications(recipient_id,sender_id,kind,title,content,event_key)
   SELECT id,$2::uuid,'announcement',$3,$4,'manual:' || gen_random_uuid()::text FROM unnest($1::uuid[]) AS id`, ids, user.ID, input.Title, input.Content)
		if err != nil {
			response.WriteError(w, 500, "发送通知失败")
			return
		}
		if err = tx.Commit(r.Context()); err != nil {
			response.WriteError(w, 500, "发送通知失败")
			return
		}
		response.WriteData(w, 201, map[string]int{"sent": len(ids)})
	}
}
