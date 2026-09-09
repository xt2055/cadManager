package titleblock

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"cadguanliq/internal/partindex"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound  = errors.New("附件不存在")
	ErrForbidden = errors.New("只有管理员、上传人或图纸创建人可以保存提取信息")
	ErrConflict  = errors.New("文件版本已变化，请重新提取")
	ErrInvalid   = errors.New("标题栏提取数据格式无效或超过限制")
)

type Field struct {
	Key        string   `json:"key"`
	Label      string   `json:"label"`
	Value      string   `json:"value"`
	Source     string   `json:"source"`
	Candidates []string `json:"candidates"`
}
type Space struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	TextCount int      `json:"textCount"`
	Fields    []Field  `json:"fields"`
	Warnings  []string `json:"warnings"`
}
type Payload struct {
	Spaces []Space `json:"spaces"`
	Error  string  `json:"error,omitempty"`
}
type Snapshot struct {
	AttachmentID     string   `json:"attachmentId"`
	VersionID        string   `json:"versionId"`
	FileName         string   `json:"fileName"`
	CanWrite         bool     `json:"canWrite"`
	HasPrevious      bool     `json:"hasPrevious"`
	Payload          *Payload `json:"payload"`
	ExtractedAt      string   `json:"extractedAt"`
	SnapshotRevision int64    `json:"snapshotRevision"`
}
type Store interface {
	Get(context.Context, string, string, bool) (Snapshot, error)
	Save(context.Context, string, string, string, bool, Payload) error
}

// normalizePayload keeps the API contract stable for both newly saved and
// historical snapshots: every collection is encoded as an array, never null.
func normalizePayload(payload Payload) Payload {
	if payload.Spaces == nil {
		payload.Spaces = []Space{}
	}
	for spaceIndex := range payload.Spaces {
		space := &payload.Spaces[spaceIndex]
		if space.Fields == nil {
			space.Fields = []Field{}
		}
		if space.Warnings == nil {
			space.Warnings = []string{}
		}
		for fieldIndex := range space.Fields {
			if space.Fields[fieldIndex].Candidates == nil {
				space.Fields[fieldIndex].Candidates = []string{}
			}
		}
	}
	return payload
}

func Validate(payload Payload) error {
	if len(payload.Spaces) > 64 || utf8.RuneCountInString(payload.Error) > 1000 || (payload.Error != "" && len(payload.Spaces) != 0) {
		return ErrInvalid
	}
	keys := map[string]bool{"number": true, "name": true, "designer": true, "checker": true, "process": true, "standard": true, "approver": true, "date": true, "material": true, "scale": true, "company": true}
	seenSpaces := map[string]bool{}
	for _, space := range payload.Spaces {
		if space.ID == "" || len(space.ID) > 128 || seenSpaces[space.ID] || len(space.Name) > 256 || space.TextCount < 0 || space.TextCount > 200000 || len(space.Fields) > 11 || len(space.Warnings) > 16 {
			return ErrInvalid
		}
		seenSpaces[space.ID] = true
		for _, warning := range space.Warnings {
			if utf8.RuneCountInString(warning) > 500 {
				return ErrInvalid
			}
		}
		seen := map[string]bool{}
		for _, field := range space.Fields {
			if !keys[field.Key] || seen[field.Key] || utf8.RuneCountInString(field.Label) > 30 || utf8.RuneCountInString(field.Value) > 500 || utf8.RuneCountInString(field.Source) > 100 || len(field.Candidates) > 32 {
				return ErrInvalid
			}
			seen[field.Key] = true
			for _, value := range field.Candidates {
				if utf8.RuneCountInString(value) > 500 {
					return ErrInvalid
				}
			}
		}
	}
	return nil
}

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

const attachmentQuery = `SELECT a.id::text, a.current_version_id::text, av.original_name,
 ($3 OR a.uploaded_by::text=$2 OR d.created_by::text=$2 OR p.created_by::text=$2) IS TRUE
 FROM attachments a
 JOIN attachment_versions av ON av.id=a.current_version_id AND av.attachment_id=a.id AND av.deleted_at IS NULL
 LEFT JOIN drawings d ON d.id=a.drawing_id
 LEFT JOIN parts p ON p.id=a.part_id
 WHERE a.id=$1::uuid AND a.deleted_at IS NULL`

func (r *Repository) Get(ctx context.Context, id, userID string, admin bool) (Snapshot, error) {
	var out Snapshot
	var raw []byte
	err := r.pool.QueryRow(ctx, `SELECT s.*, t.payload, COALESCE(to_char(t.extracted_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),''),
	 COALESCE(t.revision,0),
	 EXISTS(SELECT 1 FROM attachment_title_blocks previous WHERE previous.attachment_id=s.id::uuid AND previous.version_id<>s.current_version_id::uuid)
		 FROM (`+attachmentQuery+`) s
		 LEFT JOIN attachment_title_blocks t ON t.attachment_id=s.id::uuid AND t.version_id=s.current_version_id::uuid`, id, userID, admin).Scan(&out.AttachmentID, &out.VersionID, &out.FileName, &out.CanWrite, &raw, &out.ExtractedAt, &out.SnapshotRevision, &out.HasPrevious)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	if len(raw) > 0 {
		out.Payload = &Payload{}
		if err = json.Unmarshal(raw, out.Payload); err != nil {
			return out, err
		}
		*out.Payload = normalizePayload(*out.Payload)
	}
	return out, nil
}

func (r *Repository) Save(ctx context.Context, id, versionID, userID string, admin bool, payload Payload) error {
	id = strings.ToLower(strings.TrimSpace(id))
	versionID = strings.ToLower(strings.TrimSpace(versionID))
	payload = normalizePayload(payload)
	if err := Validate(payload); err != nil {
		return err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var foundID, current, name string
	var allowed bool
	err = tx.QueryRow(ctx, attachmentQuery+` FOR UPDATE OF a`, id, userID, admin).Scan(&foundID, &current, &name, &allowed)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	if !strings.EqualFold(current, versionID) {
		return ErrConflict
	}
	if !strings.HasSuffix(strings.ToLower(name), ".exb") && !strings.HasSuffix(strings.ToLower(name), ".dwg") && !strings.HasSuffix(strings.ToLower(name), ".dxf") {
		return ErrInvalid
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO attachment_title_blocks(attachment_id,version_id,payload,extracted_by)
	VALUES($1::uuid,$2::uuid,$3::jsonb,$4::uuid)
	ON CONFLICT(attachment_id,version_id) DO UPDATE SET
		payload=EXCLUDED.payload,
		extracted_by=EXCLUDED.extracted_by,
		extracted_at=now(),
		revision=attachment_title_blocks.revision+1
	WHERE attachment_title_blocks.payload IS DISTINCT FROM EXCLUDED.payload`, id, versionID, raw, userID)
	if err != nil {
		return err
	}
	if _, err = partindex.SyncCurrentTx(ctx, tx, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
