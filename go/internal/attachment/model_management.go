package attachment

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

var ErrModelPermission = errors.New("只有所属图纸创建者或管理员可以管理 3D 文件；审核中、存档和借用件不能直接修改")

// Lock the owning drawing to serialize model writes and primary selection.
func AuthorizeModelWrite(ctx context.Context, tx pgx.Tx, drawingNo, partNo, userID string) error {
	var allowed bool
	err := tx.QueryRow(ctx, `SELECT d.status NOT IN ('archived', 'reviewing', 'disabled')
		AND (d.created_by = $3::uuid OR EXISTS (SELECT 1 FROM user_roles WHERE user_id = $3::uuid AND role = 'admin'))
		AND ($2 = '' OR EXISTS (SELECT 1 FROM drawing_part_relations r JOIN parts p ON p.id = r.part_id
			WHERE r.drawing_id = d.id AND r.status = 'active' AND r.relation_type = 'owned' AND p.normalized_part_no = $2))
		FROM drawings d WHERE d.drawing_no = $1 FOR UPDATE OF d`, drawingNo, normalizePartNo(partNo), userID).Scan(&allowed)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !allowed {
		return ErrModelPermission
	}
	return nil
}

func (repository *PGRepository) SetPrimaryModel(ctx context.Context, id string, expectedRevision int64, userID string) (Attachment, error) {
	item, err := repository.FindByID(ctx, id)
	if err != nil {
		return Attachment{}, err
	}
	if item.FileCategory != "model3d" {
		return Attachment{}, errors.New("只有 3D 文件可以设为主模型")
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return Attachment{}, err
	}
	defer tx.Rollback(ctx)
	if err = AuthorizeModelWrite(ctx, tx, item.DrawingNo, nullablePart(item.PartNo), userID); err != nil {
		return Attachment{}, err
	}
	var revision int64
	err = tx.QueryRow(ctx, `SELECT revision FROM attachments WHERE id = $1::uuid AND deleted_at IS NULL FOR UPDATE`, id).Scan(&revision)
	if err != nil {
		return Attachment{}, err
	}
	if revision != expectedRevision {
		return Attachment{}, ErrConflict
	}
	_, err = tx.Exec(ctx, `UPDATE attachments a SET is_primary_model = false, revision = a.revision + 1
		FROM attachments target WHERE target.id = $1::uuid AND a.id <> target.id AND a.is_primary_model AND a.deleted_at IS NULL
		AND (a.drawing_id = target.drawing_id OR a.part_id = target.part_id)`, id)
	if err != nil {
		return Attachment{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE attachments SET is_primary_model = true, file_category = 'model3d', revision = revision + 1 WHERE id = $1::uuid`, id)
	if err != nil {
		return Attachment{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_logs (actor_id, action, resource_type, resource_id, summary) VALUES ($1::uuid, 'edit', 'file', $2::uuid, $3)`, userID, id, "设置主模型 "+item.Name)
	if err != nil {
		return Attachment{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Attachment{}, err
	}
	return repository.FindByID(ctx, id)
}
