package partindex

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

type sourceField struct {
	Key        string   `json:"key"`
	Value      string   `json:"value"`
	Candidates []string `json:"candidates"`
}

type sourceSpace struct {
	ID     string        `json:"id"`
	Name   string        `json:"name"`
	Fields []sourceField `json:"fields"`
}

type sourcePayload struct {
	Spaces []sourceSpace `json:"spaces"`
	Error  string        `json:"error"`
}

type indexState struct {
	Exists                    bool
	Revision                  int64
	SourceSnapshotRevision    int64
	ExtractionStatus          string
	ExtractionError           string
	SelectedSpaceID           *string
	SelectionMode             string
	AutoFields                Fields
	ManualFields              *Fields
	ConfirmedBy               *string
	ConfirmedAt               *string
	ConfirmedSnapshotRevision *int64
	EditedBy                  *string
	EditedAt                  *string
	CreatedAt                 *string
	UpdatedAt                 *string
}

func decodeFields(raw []byte) (Fields, error) {
	if len(raw) == 0 || string(raw) == "{}" || string(raw) == "null" {
		return Fields{}, nil
	}
	var fields Fields
	if err := json.Unmarshal(raw, &fields); err != nil {
		return Fields{}, err
	}
	return NormalizeFields(fields)
}

func encodeFields(fields Fields) ([]byte, error) { return json.Marshal(fields) }

func decodePayload(raw []byte) (sourcePayload, error) {
	if len(raw) == 0 {
		return sourcePayload{}, nil
	}
	var payload sourcePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return sourcePayload{}, err
	}
	return payload, nil
}

// normalizedSourcePayload prevents legacy JSON null collections from breaking
// clients that iterate title-block spaces, fields, candidates, or warnings.
func normalizedSourcePayload(raw []byte) (any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}
	root, ok := decoded.(map[string]any)
	if !ok {
		return map[string]any{"spaces": []any{}}, nil
	}
	rawSpaces, ok := root["spaces"].([]any)
	if !ok {
		root["spaces"] = []any{}
		return root, nil
	}
	spaces := make([]any, 0, len(rawSpaces))
	for _, rawSpace := range rawSpaces {
		space, ok := rawSpace.(map[string]any)
		if !ok {
			continue
		}
		rawFields, ok := space["fields"].([]any)
		if !ok {
			rawFields = []any{}
		}
		fields := make([]any, 0, len(rawFields))
		for _, rawField := range rawFields {
			field, ok := rawField.(map[string]any)
			if !ok {
				continue
			}
			if _, ok := field["key"].(string); !ok {
				field["key"] = ""
			}
			if _, ok := field["value"].(string); !ok {
				field["value"] = ""
			}
			rawCandidates, ok := field["candidates"].([]any)
			if !ok {
				rawCandidates = []any{}
			}
			candidates := make([]any, 0, len(rawCandidates))
			for _, rawCandidate := range rawCandidates {
				if candidate, ok := rawCandidate.(string); ok {
					candidates = append(candidates, candidate)
				}
			}
			field["candidates"] = candidates
			fields = append(fields, field)
		}
		space["fields"] = fields
		rawWarnings, ok := space["warnings"].([]any)
		if !ok {
			rawWarnings = []any{}
		}
		warnings := make([]any, 0, len(rawWarnings))
		for _, rawWarning := range rawWarnings {
			if warning, ok := rawWarning.(string); ok {
				warnings = append(warnings, warning)
			}
		}
		space["warnings"] = warnings
		spaces = append(spaces, space)
	}
	root["spaces"] = spaces
	return root, nil
}

func validSpaces(payload sourcePayload) []sourceSpace {
	result := make([]sourceSpace, 0, len(payload.Spaces))
	for _, space := range payload.Spaces {
		for _, field := range space.Fields {
			if NormalizeString(field.Value) != "" {
				result = append(result, space)
				break
			}
			for _, candidate := range field.Candidates {
				if NormalizeString(candidate) != "" {
					result = append(result, space)
					break
				}
			}
			if len(result) > 0 && result[len(result)-1].ID == space.ID {
				break
			}
		}
	}
	return result
}

func findSpace(payload sourcePayload, id string) (sourceSpace, bool) {
	for _, space := range payload.Spaces {
		if space.ID == id {
			return space, true
		}
	}
	return sourceSpace{}, false
}

func fieldsFromSpace(space sourceSpace) (Fields, error) {
	var fields Fields
	for _, field := range space.Fields {
		value := NormalizeString(field.Value)
		switch field.Key {
		case "number":
			fields.DrawingNo = value
		case "name":
			fields.PartName = value
		case "material":
			fields.Material = value
		case "designer":
			fields.Designer = value
		case "checker":
			fields.Checker = value
		case "approver":
			fields.Approver = value
		case "date":
			fields.DrawingDateRaw = value
		case "scale":
			fields.Scale = value
		case "process":
			fields.Process = value
		case "standard":
			fields.Standard = value
		case "company":
			fields.Company = value
		}
	}
	return NormalizeFields(fields)
}

func automaticSelection(payload sourcePayload) (*string, Fields, string, string, error) {
	if payload.Error != "" {
		return nil, Fields{}, ExtractionFailed, NormalizeString(payload.Error), nil
	}
	spaces := validSpaces(payload)
	if len(spaces) != 1 {
		return nil, Fields{}, ExtractionExtracted, "", nil
	}
	fields, err := fieldsFromSpace(spaces[0])
	if err != nil {
		return nil, Fields{}, "", "", err
	}
	id := spaces[0].ID
	return &id, fields, ExtractionExtracted, "", nil
}

func projectFields(fields Fields) (drawingDate *string, metadata []byte, err error) {
	drawingDate = ParseDrawingDate(fields.DrawingDateRaw)
	metadata, err = json.Marshal(map[string]string{
		"process": fields.Process, "standard": fields.Standard, "company": fields.Company,
	})
	return
}

func loadIndexState(ctx context.Context, tx pgx.Tx, attachmentID, versionID string, lock bool) (indexState, error) {
	query := `SELECT revision, source_snapshot_revision, extraction_status, extraction_error,
	 selected_space_id, selection_mode, auto_fields, manual_fields,
	 confirmed_by::text,
	 COALESCE(to_char(confirmed_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),''),
	 confirmed_snapshot_revision, edited_by::text,
	 COALESCE(to_char(edited_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),''),
	 COALESCE(to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),''),
	 COALESCE(to_char(updated_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),'')
	 FROM part_indexes WHERE attachment_id=$1::uuid AND version_id=$2::uuid`
	if lock {
		query += ` FOR UPDATE`
	}
	var state indexState
	var autoRaw, manualRaw []byte
	var selected, confirmedBy, confirmedAt, editedBy, editedAt, createdAt, updatedAt *string
	var confirmedRevision *int64
	err := tx.QueryRow(ctx, query, attachmentID, versionID).Scan(
		&state.Revision, &state.SourceSnapshotRevision, &state.ExtractionStatus, &state.ExtractionError,
		&selected, &state.SelectionMode, &autoRaw, &manualRaw,
		&confirmedBy, &confirmedAt, &confirmedRevision, &editedBy, &editedAt, &createdAt, &updatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return indexState{}, nil
	}
	if err != nil {
		return indexState{}, err
	}
	state.Exists = true
	state.SelectedSpaceID = selected
	state.ConfirmedBy, state.ConfirmedAt, state.ConfirmedSnapshotRevision = confirmedBy, confirmedAt, confirmedRevision
	state.EditedBy, state.EditedAt, state.CreatedAt, state.UpdatedAt = editedBy, editedAt, createdAt, updatedAt
	state.AutoFields, err = decodeFields(autoRaw)
	if err != nil {
		return indexState{}, err
	}
	if string(manualRaw) != "{}" && string(manualRaw) != "null" && len(manualRaw) > 0 {
		manual, decodeErr := decodeFields(manualRaw)
		if decodeErr != nil {
			return indexState{}, decodeErr
		}
		state.ManualFields = &manual
	}
	return state, nil
}

func currentSnapshot(ctx context.Context, tx pgx.Tx, attachmentID, versionID string) ([]byte, int64, bool, error) {
	var payload []byte
	var revision int64
	err := tx.QueryRow(ctx, `SELECT payload, revision FROM attachment_title_blocks
		WHERE attachment_id=$1::uuid AND version_id=$2::uuid`, attachmentID, versionID).Scan(&payload, &revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, false, nil
	}
	if err != nil {
		return nil, 0, false, err
	}
	return payload, revision, true, nil
}

func upsertState(ctx context.Context, tx pgx.Tx, attachmentID, versionID string, old indexState, state indexState, fields Fields, editor *string) (string, error) {
	autoRaw, err := encodeFields(state.AutoFields)
	if err != nil {
		return "", err
	}
	manualRaw := []byte("{}")
	if state.ManualFields != nil {
		manualRaw, err = encodeFields(*state.ManualFields)
		if err != nil {
			return "", err
		}
	}
	drawingDate, metadata, err := projectFields(fields)
	if err != nil {
		return "", err
	}
	if !old.Exists {
		_, err = tx.Exec(ctx, `INSERT INTO part_indexes(
			attachment_id,version_id,revision,source_snapshot_revision,extraction_status,extraction_error,
			selected_space_id,selection_mode,auto_fields,manual_fields,drawing_no,part_name,material,designer,
			checker,approver,drawing_date_raw,drawing_date,scale,sheet_size,metadata_json,
			confirmed_by,confirmed_at,confirmed_snapshot_revision,edited_by,edited_at)
			VALUES($1::uuid,$2::uuid,1,$3,$4,$5,$6,$7,$8::jsonb,$9::jsonb,$10,$11,$12,$13,$14,$15,$16,$17::date,$18,$19,$20::jsonb,
			$21::uuid,CASE WHEN $22='' THEN NULL ELSE $22::timestamptz END,$23,$24::uuid,CASE WHEN $24::uuid IS NULL THEN NULL ELSE now() END)`,
			attachmentID, versionID, state.SourceSnapshotRevision, state.ExtractionStatus, state.ExtractionError,
			state.SelectedSpaceID, state.SelectionMode, autoRaw, manualRaw, fields.DrawingNo, fields.PartName,
			fields.Material, fields.Designer, fields.Checker, fields.Approver, fields.DrawingDateRaw, drawingDate,
			fields.Scale, fields.SheetSize, metadata, state.ConfirmedBy, dereference(state.ConfirmedAt),
			state.ConfirmedSnapshotRevision, editor)
		if err != nil {
			return "", err
		}
		return "created", nil
	}
	// The database performs the final null-safe comparison. No-op rebuilds do
	// not alter row revision or timestamps.
	command, err := tx.Exec(ctx, `UPDATE part_indexes SET
		revision=revision+1, source_snapshot_revision=$3, extraction_status=$4, extraction_error=$5,
		selected_space_id=$6, selection_mode=$7, auto_fields=$8::jsonb, manual_fields=$9::jsonb,
		drawing_no=$10, part_name=$11, material=$12, designer=$13, checker=$14, approver=$15,
		drawing_date_raw=$16, drawing_date=$17::date, scale=$18, sheet_size=$19, metadata_json=$20::jsonb,
		confirmed_by=$21::uuid, confirmed_at=CASE WHEN $22='' THEN NULL ELSE $22::timestamptz END,
		confirmed_snapshot_revision=$23, edited_by=COALESCE($24::uuid,edited_by),
		edited_at=CASE WHEN $24::uuid IS NULL THEN edited_at ELSE now() END, updated_at=now()
		WHERE attachment_id=$1::uuid AND version_id=$2::uuid AND (
		  source_snapshot_revision, extraction_status, extraction_error, selected_space_id, selection_mode,
		  auto_fields, manual_fields, drawing_no, part_name, material, designer, checker, approver,
		  drawing_date_raw, drawing_date, scale, sheet_size, metadata_json,
		  confirmed_by, confirmed_at, confirmed_snapshot_revision
		) IS DISTINCT FROM (
		  $3,$4,$5,$6,$7,$8::jsonb,$9::jsonb,$10,$11,$12,$13,$14,$15,$16,$17::date,$18,$19,$20::jsonb,
		  $21::uuid,CASE WHEN $22='' THEN NULL ELSE $22::timestamptz END,$23
		)`,
		attachmentID, versionID, state.SourceSnapshotRevision, state.ExtractionStatus, state.ExtractionError,
		state.SelectedSpaceID, state.SelectionMode, autoRaw, manualRaw, fields.DrawingNo, fields.PartName,
		fields.Material, fields.Designer, fields.Checker, fields.Approver, fields.DrawingDateRaw, drawingDate,
		fields.Scale, fields.SheetSize, metadata, state.ConfirmedBy, dereference(state.ConfirmedAt),
		state.ConfirmedSnapshotRevision, editor)
	if err != nil {
		return "", err
	}
	if command.RowsAffected() == 0 {
		return "unchanged", nil
	}
	return "updated", nil
}

func dereference(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func effectiveFields(state indexState) Fields {
	if state.ManualFields != nil {
		return *state.ManualFields
	}
	return state.AutoFields
}

func computeAutomatic(payload sourcePayload, state indexState) (indexState, error) {
	state.ExtractionStatus = ExtractionPending
	state.ExtractionError = ""
	state.AutoFields = Fields{}
	if payload.Spaces == nil && payload.Error == "" {
		return state, nil
	}
	if state.SelectionMode == "manual" {
		if state.SelectedSpaceID == nil {
			if payload.Error != "" {
				state.ExtractionStatus = ExtractionFailed
				state.ExtractionError = NormalizeString(payload.Error)
			} else {
				state.ExtractionStatus = ExtractionExtracted
			}
			return state, nil
		}
		if space, ok := findSpace(payload, *state.SelectedSpaceID); ok {
			fields, err := fieldsFromSpace(space)
			if err != nil {
				return state, err
			}
			state.AutoFields = fields
		}
		if payload.Error != "" {
			state.ExtractionStatus = ExtractionFailed
			state.ExtractionError = NormalizeString(payload.Error)
		} else {
			state.ExtractionStatus = ExtractionExtracted
		}
		return state, nil
	}
	selected, fields, extraction, extractionError, err := automaticSelection(payload)
	if err != nil {
		return state, err
	}
	state.SelectedSpaceID = selected
	state.AutoFields = fields
	state.ExtractionStatus = extraction
	state.ExtractionError = extractionError
	return state, nil
}

// SyncCurrentTx projects the current title-block snapshot to the current
// attachment version. The caller owns the surrounding transaction.
func SyncCurrentTx(ctx context.Context, tx pgx.Tx, attachmentID string) (string, error) {
	var versionID, role, fileName string
	var eligible bool
	err := tx.QueryRow(ctx, `SELECT a.current_version_id::text, a.file_role, av.original_name,
			(a.file_role='part' AND lower(right(av.original_name,4)) IN ('.dwg','.dxf','.exb') AND
			  (d.id IS NOT NULL OR EXISTS (
			SELECT 1 FROM drawing_part_relations r
			JOIN drawings d ON d.id=r.drawing_id
			WHERE r.part_id=a.part_id AND r.status='active'
		  ))) AS eligible
			FROM attachments a
			JOIN attachment_versions av ON av.id=a.current_version_id AND av.attachment_id=a.id AND av.deleted_at IS NULL
			LEFT JOIN drawings d ON d.id=a.drawing_id
			WHERE a.id=$1::uuid AND a.deleted_at IS NULL
		FOR UPDATE OF a`, attachmentID).Scan(&versionID, &role, &fileName, &eligible)
	if errors.Is(err, pgx.ErrNoRows) {
		return "skipped", nil
	}
	if err != nil {
		return "", err
	}
	if !eligible {
		return "skipped", nil
	}
	old, err := loadIndexState(ctx, tx, attachmentID, versionID, true)
	if err != nil {
		return "", err
	}
	raw, snapshotRevision, hasSnapshot, err := currentSnapshot(ctx, tx, attachmentID, versionID)
	if err != nil {
		return "", err
	}
	if !hasSnapshot && !old.Exists {
		return "unchanged", nil
	}
	state := old
	if !state.Exists {
		state.SelectionMode = "auto"
	}
	state.SourceSnapshotRevision = snapshotRevision
	if hasSnapshot {
		payload, decodeErr := decodePayload(raw)
		if decodeErr != nil {
			return "", decodeErr
		}
		state, err = computeAutomatic(payload, state)
		if err != nil {
			return "", err
		}
	} else {
		state.ExtractionStatus, state.ExtractionError, state.AutoFields = ExtractionPending, "", Fields{}
	}
	fields := effectiveFields(state)
	return upsertState(ctx, tx, attachmentID, versionID, old, state, fields, nil)
}

func payloadHasSpace(payload sourcePayload, id string) bool {
	if id == "" {
		return false
	}
	_, ok := findSpace(payload, id)
	return ok
}

func normalizePayloadError(value string) string {
	return strings.TrimSpace(value)
}
