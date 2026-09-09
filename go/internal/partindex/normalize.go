package partindex

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var fieldKeys = map[string]bool{
	"drawingNo": true, "partName": true, "material": true, "designer": true,
	"checker": true, "approver": true, "drawingDateRaw": true, "scale": true,
	"sheetSize": true, "process": true, "standard": true, "company": true,
}

var partIndexUUIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func NormalizeUUID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if !partIndexUUIDPattern.MatchString(value) {
		return "", ErrInvalid
	}
	return strings.ToLower(value), nil
}

func NormalizeString(value string) string {
	return strings.Join(strings.FieldsFunc(strings.TrimSpace(value), unicode.IsSpace), " ")
}

func normalizeField(value string) (string, error) {
	value = NormalizeString(value)
	if utf8.RuneCountInString(value) > 500 {
		return "", ErrInvalid
	}
	return value, nil
}

func NormalizeFields(input Fields) (Fields, error) {
	var err error
	for _, pointer := range []*string{
		&input.DrawingNo, &input.PartName, &input.Material, &input.Designer,
		&input.Checker, &input.Approver, &input.DrawingDateRaw, &input.Scale,
		&input.SheetSize, &input.Process, &input.Standard, &input.Company,
	} {
		*pointer, err = normalizeField(*pointer)
		if err != nil {
			return Fields{}, err
		}
	}
	return input, nil
}

func EmptyFields() Fields { return Fields{} }

func FieldsEqual(left, right Fields) bool { return left == right }

func IsCompleteManualFields(fields Fields) bool {
	// Every field is serialized in Fields. The function documents that empty
	// strings are valid; callers use the struct rather than a sparse map.
	return true
}

func ValidateConfirmation(fields Fields) error {
	if NormalizeString(fields.DrawingNo) == "" || NormalizeString(fields.PartName) == "" {
		return fmt.Errorf("确认信息时标题栏图号和零件名称不能为空: %w", ErrInvalid)
	}
	return nil
}

func ParseDrawingDate(raw string) *string {
	if raw == "" {
		return nil
	}
	parsed, err := time.Parse("2006-01-02", raw)
	if err != nil || parsed.Year() < 1 || parsed.Year() > 9999 || parsed.Format("2006-01-02") != raw {
		return nil
	}
	value := parsed.Format("2006-01-02")
	return &value
}

func statusOf(extraction string, hasManual, confirmed bool, confirmedSnapshot, sourceSnapshot int64) string {
	if confirmed && confirmedSnapshot != sourceSnapshot {
		return StatusRecheck
	}
	if confirmed {
		return StatusConfirmed
	}
	if hasManual {
		return StatusNeedsConfirmation
	}
	if extraction == ExtractionFailed {
		return StatusFailed
	}
	if extraction == ExtractionPending {
		return StatusPending
	}
	return StatusNeedsConfirmation
}
