package partindex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"cadguanliq/internal/logging"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

const candidateCTE = `WITH candidate AS (
	SELECT
		a.id::text AS attachment_id,
		a.current_version_id::text AS version_id,
		av.original_name AS file_name,
		a.part_id::text AS part_id,
		COALESCE(p.part_no,'') AS registered_part_no,
		COALESCE(pi.revision,0) AS index_revision,
		COALESCE(pi.source_snapshot_revision,0) AS source_snapshot_revision,
		COALESCE(t.revision,0) AS snapshot_revision,
			COALESCE(pi.extraction_status,'pending') AS extraction_status,
			COALESCE(pi.extraction_error,'') AS extraction_error,
		pi.selected_space_id,
		COALESCE(pi.selection_mode,'auto') AS selection_mode,
		COALESCE(pi.auto_fields,'{}'::jsonb) AS auto_fields,
		COALESCE(pi.manual_fields,'{}'::jsonb) AS manual_fields,
		COALESCE(pi.drawing_no,'') AS drawing_no,
		COALESCE(pi.part_name,'') AS part_name,
		COALESCE(pi.material,'') AS material,
		COALESCE(pi.designer,'') AS designer,
		COALESCE(pi.checker,'') AS checker,
		COALESCE(pi.approver,'') AS approver,
		COALESCE(pi.drawing_date_raw,'') AS drawing_date_raw,
		pi.drawing_date::text AS drawing_date,
		COALESCE(pi.scale,'') AS scale,
		COALESCE(pi.sheet_size,'') AS sheet_size,
		pi.metadata_json,
		pi.confirmed_by::text AS confirmed_by,
		CASE WHEN pi.confirmed_at IS NULL THEN NULL
			 ELSE to_char(pi.confirmed_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"') END AS confirmed_at,
		pi.confirmed_snapshot_revision,
		pi.edited_by::text AS edited_by,
		CASE WHEN pi.edited_at IS NULL THEN NULL
			 ELSE to_char(pi.edited_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"') END AS edited_at,
		CASE WHEN pi.created_at IS NULL THEN NULL
			 ELSE to_char(pi.created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"') END AS created_at,
		CASE WHEN pi.updated_at IS NULL THEN NULL
			 ELSE to_char(pi.updated_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"') END AS updated_at,
		t.payload AS source_payload,
		to_char(av.created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"') AS version_created_at,
		($1::boolean OR a.uploaded_by::text=$2 OR d.created_by::text=$2 OR p.created_by::text=$2) IS TRUE AS can_write,
		CASE
			WHEN pi.confirmed_at IS NOT NULL
				 AND pi.confirmed_snapshot_revision IS DISTINCT FROM COALESCE(t.revision,0) THEN 'recheck'
			WHEN pi.confirmed_at IS NOT NULL THEN 'confirmed'
			WHEN COALESCE(pi.manual_fields,'{}'::jsonb) <> '{}'::jsonb THEN 'needs_confirmation'
			WHEN COALESCE(pi.extraction_status,'pending') = 'failed' THEN 'failed'
			WHEN COALESCE(pi.extraction_status,'pending') = 'pending' THEN 'pending'
			ELSE 'needs_confirmation'
		END AS status
	FROM attachments a
	JOIN attachment_versions av
	  ON av.id=a.current_version_id
	 AND av.attachment_id=a.id
	 AND av.deleted_at IS NULL
	LEFT JOIN drawings d ON d.id=a.drawing_id
	LEFT JOIN parts p ON p.id=a.part_id
	LEFT JOIN attachment_title_blocks t
	  ON t.attachment_id=a.id AND t.version_id=a.current_version_id
	LEFT JOIN part_indexes pi
	  ON pi.attachment_id=a.id AND pi.version_id=a.current_version_id
	WHERE a.deleted_at IS NULL
	  AND a.file_role='part'
	  AND lower(right(av.original_name,4)) IN ('.dwg','.dxf','.exb')
		  AND (
			d.id IS NOT NULL OR EXISTS (
		  SELECT 1 FROM drawing_part_relations r
		  JOIN drawings relation_drawing ON relation_drawing.id=r.drawing_id
		  WHERE r.part_id=a.part_id AND r.status='active'
		)
	  )
)`

type candidateRow struct {
	Item
	IndexRevision             int64
	SourceSnapshotRevision    int64
	SnapshotRevision          int64
	SelectedSpaceID           *string
	SelectionMode             string
	AutoRaw                   []byte
	ManualRaw                 []byte
	Checker                   string
	Approver                  string
	Scale                     string
	SheetSize                 string
	MetadataRaw               []byte
	ExtractionError           string
	ConfirmedBy               *string
	ConfirmedAt               *string
	ConfirmedSnapshotRevision *int64
	EditedBy                  *string
	EditedAt                  *string
	CreatedAt                 *string
	UpdatedAt                 *string
	SourcePayloadRaw          []byte
}

type rowScanner interface{ Scan(...any) error }

func scanCandidate(row rowScanner) (candidateRow, error) {
	var item candidateRow
	var partID *string
	var drawingDate *string
	err := row.Scan(
		&item.AttachmentID, &item.VersionID, &item.FileName, &partID, &item.RegisteredPartNo,
		&item.IndexRevision, &item.SourceSnapshotRevision, &item.SnapshotRevision,
		&item.ExtractionStatus, &item.ExtractionError, &item.SelectedSpaceID, &item.SelectionMode,
		&item.AutoRaw, &item.ManualRaw, &item.DrawingNo, &item.PartName, &item.Material,
		&item.Designer, &item.Checker, &item.Approver, &item.DrawingDateRaw, &drawingDate,
		&item.Scale, &item.SheetSize, &item.MetadataRaw, &item.ConfirmedBy, &item.ConfirmedAt,
		&item.ConfirmedSnapshotRevision, &item.EditedBy, &item.EditedAt, &item.CreatedAt,
		&item.UpdatedAt, &item.SourcePayloadRaw, &item.VersionCreatedAt, &item.CanWrite, &item.Status,
	)
	if err != nil {
		return candidateRow{}, err
	}
	item.PartID = partID
	item.DrawingDate = drawingDate
	return item, nil
}

const candidateColumns = `attachment_id,version_id,file_name,part_id,registered_part_no,
	index_revision,source_snapshot_revision,snapshot_revision,extraction_status,extraction_error,
	selected_space_id,selection_mode,auto_fields,manual_fields,drawing_no,part_name,material,designer,
	checker,approver,drawing_date_raw,drawing_date,scale,sheet_size,metadata_json,confirmed_by,confirmed_at,
	confirmed_snapshot_revision,edited_by,edited_at,created_at,updated_at,source_payload,version_created_at,
	can_write,status`

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	return strings.ReplaceAll(value, `_`, `\_`)
}

func normalizeKeyword(value string) (string, error) {
	value = NormalizeString(value)
	if len([]rune(value)) > 100 {
		return "", ErrInvalid
	}
	return value, nil
}

func validateFilter(filter ListFilter) (ListFilter, error) {
	if filter.Page < 1 || filter.PageSize < 1 || filter.PageSize > 100 {
		return ListFilter{}, ErrInvalid
	}
	var err error
	filter.Keyword, err = normalizeKeyword(filter.Keyword)
	if err != nil {
		return ListFilter{}, err
	}
	for _, pointer := range []*string{&filter.Material, &filter.Designer} {
		*pointer, err = normalizeField(*pointer)
		if err != nil {
			return ListFilter{}, err
		}
	}
	if filter.ProjectID != "" {
		filter.ProjectID, err = NormalizeUUID(filter.ProjectID)
		if err != nil {
			return ListFilter{}, err
		}
	}
	if filter.DateFrom != "" && ParseDrawingDate(filter.DateFrom) == nil {
		return ListFilter{}, ErrInvalid
	}
	if filter.DateTo != "" && ParseDrawingDate(filter.DateTo) == nil {
		return ListFilter{}, ErrInvalid
	}
	if filter.DateFrom != "" && filter.DateTo != "" && filter.DateFrom > filter.DateTo {
		return ListFilter{}, ErrInvalid
	}
	if filter.Status != "" && !validStatus(filter.Status) {
		return ListFilter{}, ErrInvalid
	}
	return filter, nil
}

func validStatus(value string) bool {
	switch value {
	case StatusPending, StatusFailed, StatusNeedsConfirmation, StatusConfirmed, StatusRecheck:
		return true
	default:
		return false
	}
}

func candidateFilters(filter ListFilter, startArgument int) (string, []any) {
	args := make([]any, 0, 8)
	next := startArgument
	add := func(value any) string {
		args = append(args, value)
		token := fmt.Sprintf("$%d", next)
		next++
		return token
	}
	keyword := add("%" + strings.ToLower(escapeLike(filter.Keyword)) + "%")
	project := add(filter.ProjectID)
	material := add(filter.Material)
	designer := add(filter.Designer)
	dateFrom := add(filter.DateFrom)
	dateTo := add(filter.DateTo)
	status := add(filter.Status)
	where := ` WHERE (` + keyword + `='%' OR lower(c.drawing_no) LIKE ` + keyword + ` ESCAPE '\'
		OR lower(c.part_name) LIKE ` + keyword + ` ESCAPE '\'
		OR lower(c.material) LIKE ` + keyword + ` ESCAPE '\'
		OR lower(c.designer) LIKE ` + keyword + ` ESCAPE '\'
		OR EXISTS (
			SELECT 1 FROM (
				SELECT d.id, d.project FROM attachments a JOIN drawings d ON d.id=a.drawing_id WHERE a.id=c.attachment_id::uuid
				UNION
				SELECT d.id, d.project FROM attachments a JOIN drawing_part_relations r ON r.part_id=a.part_id AND r.status='active'
					JOIN drawings d ON d.id=r.drawing_id WHERE a.id=c.attachment_id::uuid
			) candidate_projects WHERE lower(candidate_projects.project) LIKE ` + keyword + ` ESCAPE '\'
		))
		AND (` + project + `='' OR EXISTS (
			SELECT 1 FROM (
				SELECT d.id::text AS id FROM attachments a JOIN drawings d ON d.id=a.drawing_id WHERE a.id=c.attachment_id::uuid
				UNION
				SELECT d.id::text AS id FROM attachments a JOIN drawing_part_relations r ON r.part_id=a.part_id AND r.status='active'
					JOIN drawings d ON d.id=r.drawing_id WHERE a.id=c.attachment_id::uuid
			) candidate_projects WHERE candidate_projects.id=` + project + `
		))
		AND (` + material + `='' OR c.material=` + material + `)
		AND (` + designer + `='' OR c.designer=` + designer + `)
		AND (` + dateFrom + `='' OR c.drawing_date >= ` + dateFrom + `)
		AND (` + dateTo + `='' OR c.drawing_date <= ` + dateTo + `)
		AND (` + status + `='' OR c.status=` + status + `)`
	return where, args
}

func (r *Repository) List(ctx context.Context, filter ListFilter, userID string, admin bool) (Page, error) {
	filter, err := validateFilter(filter)
	if err != nil {
		return Page{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return Page{}, err
	}
	defer tx.Rollback(ctx)
	where, args := candidateFilters(filter, 3)
	allArgs := append([]any{admin, userID}, args...)
	var total int
	if err = tx.QueryRow(ctx, candidateCTE+` SELECT count(*) FROM candidate c`+where, allArgs...).Scan(&total); err != nil {
		return Page{}, err
	}
	offset := (filter.Page - 1) * filter.PageSize
	allArgs = append(allArgs, filter.PageSize, offset)
	rows, err := tx.Query(ctx, candidateCTE+` SELECT `+candidateColumns+` FROM candidate c`+where+
		` ORDER BY c.version_created_at DESC, c.attachment_id ASC LIMIT $10 OFFSET $11`, allArgs...)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	list := make([]Item, 0)
	ids := make([]string, 0)
	for rows.Next() {
		row, scanErr := scanCandidate(rows)
		if scanErr != nil {
			return Page{}, scanErr
		}
		list = append(list, row.Item)
		ids = append(ids, row.AttachmentID)
	}
	if err = rows.Err(); err != nil {
		return Page{}, err
	}
	projects, err := r.projectsByAttachment(ctx, tx, ids)
	if err != nil {
		return Page{}, err
	}
	for index := range list {
		list[index].Projects = projects[list[index].AttachmentID]
		list[index].ProjectCount = len(list[index].Projects)
	}
	if err = tx.Commit(ctx); err != nil {
		return Page{}, err
	}
	return Page{List: list, Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
}

func (r *Repository) candidateByAttachment(ctx context.Context, tx pgx.Tx, attachmentID, userID string, admin bool) (candidateRow, error) {
	row, err := scanCandidate(tx.QueryRow(ctx, candidateCTE+` SELECT `+candidateColumns+` FROM candidate c WHERE c.attachment_id=$3::text`, admin, userID, attachmentID))
	if errors.Is(err, pgx.ErrNoRows) {
		return candidateRow{}, ErrNotFound
	}
	return row, err
}

func (r *Repository) Detail(ctx context.Context, attachmentID, userID string, admin bool) (Detail, error) {
	attachmentID, err := NormalizeUUID(attachmentID)
	if err != nil {
		return Detail{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return Detail{}, err
	}
	defer tx.Rollback(ctx)
	row, err := r.candidateByAttachment(ctx, tx, attachmentID, userID, admin)
	if err != nil {
		return Detail{}, err
	}
	projects, err := r.projectsByAttachment(ctx, tx, []string{attachmentID})
	if err != nil {
		return Detail{}, err
	}
	row.Projects = projects[attachmentID]
	row.ProjectCount = len(row.Projects)
	detail, err := detailFromRow(row)
	if err != nil {
		return Detail{}, err
	}
	if len(row.Projects) > 0 {
		detail.DefaultProjectDrawingNo = row.Projects[0].DrawingNo
	}
	if err = tx.Commit(ctx); err != nil {
		return Detail{}, err
	}
	return detail, nil
}

func detailFromRow(row candidateRow) (Detail, error) {
	autoFields, err := decodeFields(row.AutoRaw)
	if err != nil {
		return Detail{}, err
	}
	var manual *Fields
	if string(row.ManualRaw) != "{}" && string(row.ManualRaw) != "null" && len(row.ManualRaw) > 0 {
		value, decodeErr := decodeFields(row.ManualRaw)
		if decodeErr != nil {
			return Detail{}, decodeErr
		}
		manual = &value
	}
	fields := autoFields
	if manual != nil {
		fields = *manual
	}
	if row.IndexRevision == 0 {
		fields = Fields{}
		autoFields = Fields{}
	}
	payload, err := normalizedSourcePayload(row.SourcePayloadRaw)
	if err != nil {
		return Detail{}, err
	}
	payloadState := sourcePayload{}
	if len(row.SourcePayloadRaw) > 0 {
		_ = json.Unmarshal(row.SourcePayloadRaw, &payloadState)
	}
	selectedMissing := row.SelectionMode == "manual" && row.SelectedSpaceID != nil && !payloadHasSpace(payloadState, *row.SelectedSpaceID)
	return Detail{
		Item:                      row.Item,
		Revision:                  row.IndexRevision,
		SnapshotRevision:          row.SnapshotRevision,
		SourceSnapshotRevision:    row.SourceSnapshotRevision,
		SelectedSpaceID:           row.SelectedSpaceID,
		SelectionMode:             row.SelectionMode,
		SelectedSpaceMissing:      selectedMissing,
		HasManualFields:           manual != nil,
		Fields:                    fields,
		AutoFields:                autoFields,
		ExtractionError:           row.ExtractionError,
		ConfirmedBy:               row.ConfirmedBy,
		ConfirmedAt:               row.ConfirmedAt,
		ConfirmedSnapshotRevision: row.ConfirmedSnapshotRevision,
		EditedBy:                  row.EditedBy,
		EditedAt:                  row.EditedAt,
		CreatedAt:                 row.CreatedAt,
		UpdatedAt:                 row.UpdatedAt,
		SourcePayload:             payload,
	}, nil
}

func (r *Repository) projectsByAttachment(ctx context.Context, tx pgx.Tx, ids []string) (map[string][]Project, error) {
	result := make(map[string][]Project, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	rows, err := tx.Query(ctx, `SELECT attachment_id, drawing_id, project_code, project_name, drawing_no, relation_type
		FROM (
			SELECT a.id::text AS attachment_id, d.id::text AS drawing_id, d.project AS project_code,
				d.name AS project_name, d.drawing_no, 'direct'::text AS relation_type
			FROM attachments a
			JOIN drawings d ON d.id=a.drawing_id
			WHERE a.id = ANY($1::uuid[])
			UNION ALL
			SELECT a.id::text, d.id::text, d.project, d.name, d.drawing_no, r.relation_type
			FROM attachments a
			JOIN drawing_part_relations r ON r.part_id=a.part_id AND r.status='active'
			JOIN drawings d ON d.id=r.drawing_id
			WHERE a.id = ANY($1::uuid[])
		) relations`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type key struct{ attachmentID, drawingID string }
	projects := map[key]*Project{}
	for rows.Next() {
		var attachmentID, drawingID, relationType string
		var project Project
		if err = rows.Scan(&attachmentID, &drawingID, &project.ProjectCode, &project.ProjectName, &project.DrawingNo, &relationType); err != nil {
			return nil, err
		}
		project.DrawingID = drawingID
		groupKey := key{attachmentID: attachmentID, drawingID: drawingID}
		if existing, ok := projects[groupKey]; ok {
			existing.RelationTypes = appendUnique(existing.RelationTypes, relationType)
		} else {
			project.RelationTypes = []string{relationType}
			projects[groupKey] = &project
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	for groupKey, project := range projects {
		project.RelationTypes = orderedRelationTypes(project.RelationTypes)
		result[groupKey.attachmentID] = append(result[groupKey.attachmentID], *project)
	}
	for attachmentID := range result {
		sort.Slice(result[attachmentID], func(left, right int) bool {
			a, b := result[attachmentID][left], result[attachmentID][right]
			if a.ProjectCode != b.ProjectCode {
				return a.ProjectCode < b.ProjectCode
			}
			if a.DrawingNo != b.DrawingNo {
				return a.DrawingNo < b.DrawingNo
			}
			return a.DrawingID < b.DrawingID
		})
	}
	return result, nil
}

func appendUnique(values []string, value string) []string {
	for _, item := range values {
		if item == value {
			return values
		}
	}
	return append(values, value)
}

func orderedRelationTypes(values []string) []string {
	priority := map[string]int{"direct": 0, "owned": 1, "borrowed": 2}
	sort.Slice(values, func(left, right int) bool { return priority[values[left]] < priority[values[right]] })
	return values
}

func (r *Repository) Options(ctx context.Context, kind, keyword string, limit int, userID string, admin bool) (OptionPage, error) {
	keyword, err := normalizeKeyword(keyword)
	if err != nil || limit < 1 || limit > 100 {
		return OptionPage{}, ErrInvalid
	}
	like := "%" + strings.ToLower(escapeLike(keyword)) + "%"
	var query string
	switch kind {
	case "material":
		query = candidateCTE + ` SELECT c.material, c.material FROM candidate c
			WHERE c.material <> '' AND ($3='%' OR lower(c.material) LIKE $3 ESCAPE '\')
			GROUP BY c.material ORDER BY c.material ASC LIMIT $4`
	case "designer":
		query = candidateCTE + ` SELECT c.designer, c.designer FROM candidate c
			WHERE c.designer <> '' AND ($3='%' OR lower(c.designer) LIKE $3 ESCAPE '\')
			GROUP BY c.designer ORDER BY c.designer ASC LIMIT $4`
	case "project":
		query = candidateCTE + ` SELECT p.drawing_id, p.project_code || ' / ' || p.project_name || ' / ' || p.drawing_no
			FROM candidate c
			JOIN LATERAL (
				SELECT d.id::text AS drawing_id,d.project AS project_code,d.name AS project_name,d.drawing_no
				FROM attachments a JOIN drawings d ON d.id=a.drawing_id WHERE a.id=c.attachment_id::uuid
				UNION
				SELECT d.id::text,d.project,d.name,d.drawing_no
				FROM attachments a JOIN drawing_part_relations r ON r.part_id=a.part_id AND r.status='active'
					JOIN drawings d ON d.id=r.drawing_id WHERE a.id=c.attachment_id::uuid
			) p ON true
			WHERE ($3='%' OR lower(p.project_code || ' / ' || p.project_name || ' / ' || p.drawing_no) LIKE $3 ESCAPE '\')
			GROUP BY p.drawing_id,p.project_code,p.project_name,p.drawing_no
			ORDER BY p.project_code,p.project_name,p.drawing_no,p.drawing_id LIMIT $4`
	default:
		return OptionPage{}, ErrInvalid
	}
	rows, err := r.pool.Query(ctx, query, admin, userID, like, limit+1)
	if err != nil {
		return OptionPage{}, err
	}
	defer rows.Close()
	result := OptionPage{List: make([]Option, 0)}
	for rows.Next() {
		var option Option
		if err = rows.Scan(&option.Value, &option.Label); err != nil {
			return OptionPage{}, err
		}
		if len(result.List) < limit {
			result.List = append(result.List, option)
		} else {
			result.HasMore = true
		}
	}
	if err = rows.Err(); err != nil {
		return OptionPage{}, err
	}
	return result, nil
}

func canWriteTx(ctx context.Context, tx pgx.Tx, attachmentID, userID string, admin bool) (versionID string, allowed, eligible bool, err error) {
	err = tx.QueryRow(ctx, `SELECT a.current_version_id::text,
		($3::boolean OR a.uploaded_by::text=$2 OR d.created_by::text=$2 OR p.created_by::text=$2) IS TRUE,
		  (a.file_role='part' AND lower(right(av.original_name,4)) IN ('.dwg','.dxf','.exb') AND
			  (d.id IS NOT NULL OR EXISTS (
			SELECT 1 FROM drawing_part_relations r JOIN drawings rd ON rd.id=r.drawing_id
			WHERE r.part_id=a.part_id AND r.status='active'
		  ))) AS eligible
		FROM attachments a
		JOIN attachment_versions av ON av.id=a.current_version_id AND av.attachment_id=a.id AND av.deleted_at IS NULL
		LEFT JOIN drawings d ON d.id=a.drawing_id
		LEFT JOIN parts p ON p.id=a.part_id
		WHERE a.id=$1::uuid AND a.deleted_at IS NULL
		FOR UPDATE OF a`, attachmentID, userID, admin).Scan(&versionID, &allowed, &eligible)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, false, ErrNotFound
	}
	return versionID, allowed, eligible, err
}

func (r *Repository) Edit(ctx context.Context, attachmentID, userID string, admin bool, input EditInput) (Detail, error) {
	if input.Action != "save" && input.Action != "confirm" && input.Action != "reset" {
		return Detail{}, ErrInvalid
	}
	if input.ExpectedRevision < 0 || input.ExpectedSnapshotRevision < 0 || strings.TrimSpace(input.VersionID) == "" {
		return Detail{}, ErrInvalid
	}
	if input.Action == "reset" && (input.Fields != nil || input.SelectedSpaceID != nil) {
		return Detail{}, ErrInvalid
	}
	if input.Action != "reset" && input.Fields == nil {
		return Detail{}, ErrInvalid
	}
	if input.Action != "reset" && input.SelectedSpaceID != nil && strings.TrimSpace(*input.SelectedSpaceID) == "" {
		return Detail{}, ErrInvalid
	}
	var err error
	attachmentID, err = NormalizeUUID(attachmentID)
	if err != nil {
		return Detail{}, err
	}
	input.VersionID, err = NormalizeUUID(input.VersionID)
	if err != nil {
		return Detail{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Detail{}, err
	}
	defer tx.Rollback(ctx)
	versionID, allowed, eligible, err := canWriteTx(ctx, tx, attachmentID, userID, admin)
	if err != nil {
		return Detail{}, err
	}
	if !eligible {
		return Detail{}, ErrNotFound
	}
	if !allowed {
		return Detail{}, ErrForbidden
	}
	if !strings.EqualFold(versionID, input.VersionID) {
		return Detail{}, ErrConflict
	}
	rawPayload, snapshotRevision, hasSnapshot, err := currentSnapshot(ctx, tx, attachmentID, versionID)
	if err != nil {
		return Detail{}, err
	}
	if snapshotRevision != input.ExpectedSnapshotRevision {
		return Detail{}, ErrConflict
	}
	old, err := loadIndexState(ctx, tx, attachmentID, versionID, true)
	if err != nil {
		return Detail{}, err
	}
	if old.Revision != input.ExpectedRevision {
		return Detail{}, ErrConflict
	}
	state := old
	if !state.Exists {
		state.SelectionMode = "auto"
	}
	state.SourceSnapshotRevision = snapshotRevision
	payload := sourcePayload{}
	if hasSnapshot {
		payload, err = decodePayload(rawPayload)
		if err != nil {
			return Detail{}, err
		}
	}
	switch input.Action {
	case "reset":
		state.ManualFields = nil
		state.ConfirmedBy, state.ConfirmedAt, state.ConfirmedSnapshotRevision = nil, nil, nil
		state.SelectionMode = "auto"
		state.SelectedSpaceID = nil
	case "save", "confirm":
		fields, normalizeErr := NormalizeFields(*input.Fields)
		if normalizeErr != nil {
			return Detail{}, normalizeErr
		}
		if input.SelectedSpaceID != nil && hasSnapshot && !payloadHasSpace(payload, *input.SelectedSpaceID) {
			return Detail{}, ErrInvalid
		}
		state.SelectionMode = "manual"
		state.SelectedSpaceID = input.SelectedSpaceID
		state.ManualFields = &fields
		state.ConfirmedBy, state.ConfirmedAt, state.ConfirmedSnapshotRevision = nil, nil, nil
		if input.Action == "confirm" {
			if err = ValidateConfirmation(fields); err != nil {
				return Detail{}, err
			}
			state.ConfirmedBy = &userID
			state.ConfirmedSnapshotRevision = &snapshotRevision
			now := time.Now().UTC().Format(time.RFC3339)
			state.ConfirmedAt = &now
		}
	}
	if hasSnapshot {
		state, err = computeAutomatic(payload, state)
		if err != nil {
			return Detail{}, err
		}
	} else {
		state.ExtractionStatus, state.ExtractionError, state.AutoFields = ExtractionPending, "", Fields{}
	}
	effective := effectiveFields(state)
	_, err = upsertState(ctx, tx, attachmentID, versionID, old, state, effective, &userID)
	if err != nil {
		return Detail{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Detail{}, err
	}
	return r.Detail(ctx, attachmentID, userID, admin)
}

func (r *Repository) Rebuild(ctx context.Context, attachmentID, userID string, admin bool, input RebuildInput) (string, Detail, error) {
	if input.ExpectedSnapshotRevision < 0 || input.VersionID == "" {
		return "", Detail{}, ErrInvalid
	}
	var err error
	attachmentID, err = NormalizeUUID(attachmentID)
	if err != nil {
		return "", Detail{}, err
	}
	input.VersionID, err = NormalizeUUID(input.VersionID)
	if err != nil {
		return "", Detail{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", Detail{}, err
	}
	defer tx.Rollback(ctx)
	versionID, allowed, eligible, err := canWriteTx(ctx, tx, attachmentID, userID, admin)
	if err != nil {
		return "", Detail{}, err
	}
	if !eligible {
		return "", Detail{}, ErrNotFound
	}
	if !allowed {
		return "", Detail{}, ErrForbidden
	}
	if !strings.EqualFold(versionID, input.VersionID) {
		return "", Detail{}, ErrConflict
	}
	_, snapshotRevision, hasSnapshot, err := currentSnapshot(ctx, tx, attachmentID, versionID)
	if err != nil {
		return "", Detail{}, err
	}
	if !hasSnapshot {
		return "", Detail{}, ErrSnapshotRequired
	}
	if snapshotRevision != input.ExpectedSnapshotRevision {
		return "", Detail{}, ErrConflict
	}
	result, err := SyncCurrentTx(ctx, tx, attachmentID)
	if err != nil {
		return "", Detail{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", Detail{}, err
	}
	detail, err := r.Detail(ctx, attachmentID, userID, admin)
	return result, detail, err
}

func (r *Repository) Backfill(ctx context.Context, input BackfillInput) (BackfillResult, error) {
	if input.Limit < 1 || input.Limit > 100 {
		return BackfillResult{}, ErrInvalid
	}
	if input.AfterAttachmentID != "" {
		var err error
		input.AfterAttachmentID, err = NormalizeUUID(input.AfterAttachmentID)
		if err != nil {
			return BackfillResult{}, err
		}
	}
	rows, err := r.pool.Query(ctx, `SELECT a.id::text
		FROM attachments a
		JOIN attachment_versions av ON av.id=a.current_version_id AND av.attachment_id=a.id AND av.deleted_at IS NULL
		JOIN attachment_title_blocks t ON t.attachment_id=a.id AND t.version_id=a.current_version_id
		LEFT JOIN drawings d ON d.id=a.drawing_id
		WHERE a.deleted_at IS NULL AND a.file_role='part'
		  AND lower(right(av.original_name,4)) IN ('.dwg','.dxf','.exb')
		  AND (d.id IS NOT NULL OR EXISTS (
			SELECT 1 FROM drawing_part_relations r JOIN drawings d ON d.id=r.drawing_id
			WHERE r.part_id=a.part_id AND r.status='active'
		  ))
		  AND ($1='' OR a.id::text > $1)
		ORDER BY a.id ASC LIMIT $2`, input.AfterAttachmentID, input.Limit+1)
	if err != nil {
		return BackfillResult{}, err
	}
	defer rows.Close()
	ids := make([]string, 0, input.Limit+1)
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return BackfillResult{}, err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		return BackfillResult{}, err
	}
	result := BackfillResult{Errors: make([]BackfillError, 0)}
	if len(ids) > input.Limit {
		result.HasMore = true
		ids = ids[:input.Limit]
	}
	for _, attachmentID := range ids {
		result.Scanned++
		cursor := attachmentID
		result.NextCursor = &cursor
		tx, beginErr := r.pool.Begin(ctx)
		if beginErr != nil {
			recordBackfillFailure(&result, attachmentID, "begin", beginErr)
			continue
		}
		syncResult, syncErr := SyncCurrentTx(ctx, tx, attachmentID)
		if syncErr == nil {
			syncErr = tx.Commit(ctx)
		} else {
			_ = tx.Rollback(ctx)
		}
		if syncErr != nil {
			recordBackfillFailure(&result, attachmentID, "sync", syncErr)
			continue
		}
		switch syncResult {
		case "created":
			result.Created++
		case "updated":
			result.Updated++
		case "skipped":
			result.Skipped++
		default:
			result.Unchanged++
		}
	}
	if !result.HasMore {
		result.NextCursor = nil
	}
	return result, nil
}

// recordBackfillFailure keeps the client response useful without exposing
// database details. The full cause is retained in the server log together
// with the attachment and operation stage for an administrator to diagnose.
func recordBackfillFailure(result *BackfillResult, attachmentID, stage string, err error) {
	logging.Errorf("[零件索引补建] attachment=%s stage=%s err=%v", attachmentID, stage, err)
	message := "标题栏快照投影或索引写入失败，请重新提取标题栏后重试"
	if stage == "begin" {
		message = "无法创建数据库事务，请稍后重试"
	}
	result.Failed++
	result.Errors = append(result.Errors, BackfillError{AttachmentID: attachmentID, Message: message})
}
