package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"cadguanliq/internal/attachment"
	"cadguanliq/internal/response"
	"cadguanliq/internal/storage"

	"github.com/xuri/excelize/v2"
)

type BomItemPayload struct {
	No        int     `json:"no"`
	ID        string  `json:"id"`
	DrawingNo string  `json:"drawingNo"`
	Name      string  `json:"name"`
	Spec      string  `json:"spec"`
	Qty       int     `json:"qty"`
	Weight    float64 `json:"weight"`
	Remark    string  `json:"remark"`
}

type ExportBomRequest struct {
	DrawingNo  string           `json:"drawingNo"`
	StorageKey string           `json:"storageKey"`
	Items      []BomItemPayload `json:"items"`
}

type bomColumnMap struct {
	headerRow    int
	noCol        int
	totalCol     int
	idCol        int
	nameCol      int
	specCol      int
	materialCol  int
	dimensionCol int
	qtyCol       int
	weightCol    int
	remarkCol    int
}

func isBottomSignatureRow(row []string) bool {
	for _, cell := range row {
		clean := strings.ReplaceAll(strings.TrimSpace(cell), " ", "")
		clean = strings.ReplaceAll(clean, "　", "")
		if strings.Contains(clean, "总重") ||
			strings.Contains(clean, "编制：") || strings.Contains(clean, "编制:") ||
			strings.Contains(clean, "审核：") || strings.Contains(clean, "审核:") ||
			strings.Contains(clean, "批准：") || strings.Contains(clean, "批准:") ||
			strings.Contains(clean, "发放：") || strings.Contains(clean, "发放:") ||
			strings.Contains(clean, "下料组") ||
			strings.Contains(clean, "半成品库") ||
			strings.Contains(clean, "物流：") || strings.Contains(clean, "物流:") ||
			strings.Contains(clean, "总装：") || strings.Contains(clean, "总装:") {
			return true
		}
	}
	return false
}

func findBomColumns(rows [][]string) *bomColumnMap {
	for rIdx, row := range rows {
		if rIdx > 10 {
			break
		}
		var idCol, nameCol int
		for cIdx, val := range row {
			clean := strings.ReplaceAll(strings.TrimSpace(val), " ", "")
			clean = strings.ReplaceAll(clean, "\n", "")
			clean = strings.ReplaceAll(clean, "\r", "")
			clean = strings.ReplaceAll(clean, "\t", "")
			if strings.Contains(clean, "零件代号") || strings.Contains(clean, "图号") || strings.Contains(clean, "零件号") {
				idCol = cIdx + 1
			}
			if strings.Contains(clean, "零件名称") || clean == "名称" {
				nameCol = cIdx + 1
			}
		}

		if idCol > 0 && nameCol > 0 {
			cm := &bomColumnMap{
				headerRow: rIdx + 1,
				idCol:     idCol,
				nameCol:   nameCol,
			}
			for cIdx, val := range row {
				clean := strings.ReplaceAll(strings.TrimSpace(val), " ", "")
				clean = strings.ReplaceAll(clean, "\n", "")
				clean = strings.ReplaceAll(clean, "\r", "")
				clean = strings.ReplaceAll(clean, "\t", "")
				colNum := cIdx + 1
				if clean == "序号" {
					cm.noCol = colNum
				}
				if strings.Contains(clean, "总重") || strings.Contains(clean, "合重") {
					cm.totalCol = colNum
					continue
				}
				if strings.Contains(clean, "下料尺寸") {
					cm.dimensionCol = colNum
				}
				if cm.materialCol == 0 && (clean == "材质" || clean == "材料") {
					cm.materialCol = colNum
				}
				if cm.specCol == 0 && strings.Contains(clean, "规格") {
					cm.specCol = colNum
				}
				if cm.qtyCol == 0 && (strings.Contains(clean, "单支数量") || strings.Contains(clean, "数量")) {
					cm.qtyCol = colNum
				}
				if cm.weightCol == 0 && (strings.Contains(clean, "毛坯重量") || strings.Contains(clean, "单重") || strings.Contains(clean, "重量")) {
					cm.weightCol = colNum
				}
				if cm.remarkCol == 0 && (strings.Contains(clean, "备注") || strings.Contains(clean, "说明")) {
					cm.remarkCol = colNum
				}
			}
			if cm.specCol == 0 {
				cm.specCol = cm.materialCol
				cm.materialCol = 0
			}
			return cm
		}
	}
	return nil
}

func ExportBOM(attachmentRepo attachment.Repository, objectStorage storage.ObjectStorage) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.WriteError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		var req ExportBomRequest
		if err := json.NewDecoder(request.Body).Decode(&req); err != nil {
			response.WriteError(writer, http.StatusBadRequest, "请求参数解析失败")
			return
		}

		if req.Items == nil {
			response.WriteError(writer, http.StatusBadRequest, "物料明细不能为空")
			return
		}

		var f *excelize.File
		var hasOriginal bool

		if req.StorageKey != "" {
			rc, _, readErr := objectStorage.Open(request.Context(), req.StorageKey)
			if readErr == nil && rc != nil {
				data, readAllErr := io.ReadAll(rc)
				_ = rc.Close()
				if readAllErr == nil && len(data) > 0 {
					openedFile, openErr := excelize.OpenReader(bytes.NewReader(data))
					if openErr == nil {
						f = openedFile
						hasOriginal = true
					}
				}
			}
		}
		if req.StorageKey != "" && !hasOriginal {
			response.WriteError(writer, http.StatusUnprocessableEntity, "无法读取原始 Excel 模板，请检查附件后重试")
			return
		}

		if !hasOriginal || f == nil {
			f = excelize.NewFile()
			sheet := f.GetSheetList()[0]
			_ = f.SetSheetName(sheet, "备料明细")
			sheet = "备料明细"

			headerStyle, _ := f.NewStyle(&excelize.Style{
				Font:      &excelize.Font{Bold: true, Color: "#FFFFFF"},
				Fill:      excelize.Fill{Type: "pattern", Color: []string{"#34495E"}, Pattern: 1},
				Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
				Border: []excelize.Border{
					{Type: "left", Color: "#D0D5DD", Style: 1},
					{Type: "top", Color: "#D0D5DD", Style: 1},
					{Type: "bottom", Color: "#D0D5DD", Style: 1},
					{Type: "right", Color: "#D0D5DD", Style: 1},
				},
			})
			dataStyle, _ := f.NewStyle(&excelize.Style{
				Alignment: &excelize.Alignment{Vertical: "center"},
				Border: []excelize.Border{
					{Type: "left", Color: "#E5E7EB", Style: 1},
					{Type: "top", Color: "#E5E7EB", Style: 1},
					{Type: "bottom", Color: "#E5E7EB", Style: 1},
					{Type: "right", Color: "#E5E7EB", Style: 1},
				},
			})

			headers := []string{"序号", "图号/标准号", "名称", "规格/材质", "数量", "单重(kg)", "备注"}
			for cIdx, h := range headers {
				colName, _ := excelize.ColumnNumberToName(cIdx + 1)
				_ = f.SetCellValue(sheet, fmt.Sprintf("%s1", colName), h)
				_ = f.SetCellStyle(sheet, fmt.Sprintf("%s1", colName), fmt.Sprintf("%s1", colName), headerStyle)
			}
			_ = f.SetRowHeight(sheet, 1, 26)

			for idx, item := range req.Items {
				r := idx + 2
				_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", r), item.No)
				_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", r), item.ID)
				_ = f.SetCellValue(sheet, fmt.Sprintf("C%d", r), item.Name)
				_ = f.SetCellValue(sheet, fmt.Sprintf("D%d", r), item.Spec)
				_ = f.SetCellValue(sheet, fmt.Sprintf("E%d", r), item.Qty)
				_ = f.SetCellValue(sheet, fmt.Sprintf("F%d", r), item.Weight)
				_ = f.SetCellValue(sheet, fmt.Sprintf("G%d", r), item.Remark)

				for c := 1; c <= 7; c++ {
					colName, _ := excelize.ColumnNumberToName(c)
					_ = f.SetCellStyle(sheet, fmt.Sprintf("%s%d", colName, r), fmt.Sprintf("%s%d", colName, r), dataStyle)
				}
				_ = f.SetRowHeight(sheet, r, 22)
			}

			_ = f.SetColWidth(sheet, "A", "A", 8)
			_ = f.SetColWidth(sheet, "B", "B", 24)
			_ = f.SetColWidth(sheet, "C", "C", 22)
			_ = f.SetColWidth(sheet, "D", "D", 20)
			_ = f.SetColWidth(sheet, "E", "E", 10)
			_ = f.SetColWidth(sheet, "F", "F", 14)
			_ = f.SetColWidth(sheet, "G", "G", 18)
		} else {
			defer f.Close()
			if err := updateBomTemplate(f, req.Items); err != nil {
				response.WriteError(writer, http.StatusUnprocessableEntity, err.Error())
				return
			}
		}

		var buf bytes.Buffer
		if err := f.Write(&buf); err != nil {
			response.WriteError(writer, http.StatusInternalServerError, "生成 Excel 文件失败")
			return
		}

		drawingName := strings.ReplaceAll(req.DrawingNo, "/", "_")
		drawingName = strings.ReplaceAll(drawingName, "\\", "_")
		if drawingName == "" {
			drawingName = "图纸"
		}
		fileName := fmt.Sprintf("%s_备料明细表.xlsx", drawingName)

		writer.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		writer.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileName))
		writer.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write(buf.Bytes())
	}
}

// Update values in the source workbook so widths, styles, merges and print settings survive.
func bomTemplateSpecValues(cm *bomColumnMap, original []string, spec string) (map[int]any, error) {
	values := map[int]any{}
	if cm.materialCol == 0 && cm.dimensionCol == 0 {
		values[cm.specCol] = spec
		return values, nil
	}
	get := func(col int) string {
		if col <= 0 || col > len(original) {
			return ""
		}
		return strings.TrimSpace(strings.ReplaceAll(original[col-1], "\n", " "))
	}
	raw := func(col int) string {
		if col <= 0 || col > len(original) {
			return ""
		}
		return original[col-1]
	}
	var parts []string
	var columns []int
	for _, col := range []int{cm.materialCol, cm.specCol} {
		if col <= 0 {
			continue
		}
		values[col] = raw(col)
		if value := get(col); value != "" && value != "/" {
			parts = append(parts, value)
			columns = append(columns, col)
		}
	}
	var dimensions []string
	if cm.dimensionCol > 0 {
		for col := cm.dimensionCol; col < cm.dimensionCol+3; col++ {
			values[col] = raw(col)
			if value := get(col); value != "" && value != "/" && value != "0" {
				dimensions = append(dimensions, value)
			}
		}
	}
	dimensionText := strings.Join(dimensions, "×")
	if dimensionText != "" {
		parts = append(parts, dimensionText)
	}
	originalSpec := strings.Join(parts, " ")
	if originalSpec == "" {
		originalSpec = "—"
	}
	if strings.Join(strings.Fields(spec), " ") == strings.Join(strings.Fields(originalSpec), " ") {
		return values, nil
	}
	// A combined UI specification must be split back into the source columns.
	tokens := strings.Fields(spec)
	baseCount := len(columns)
	if len(tokens) != baseCount && len(tokens) != baseCount+1 {
		return nil, fmt.Errorf("规格“%s”无法对应原表的材质、规格和尺寸列，请按原有字段顺序填写", spec)
	}
	for i, col := range columns {
		values[col] = tokens[i]
	}
	if len(tokens) == baseCount+1 {
		if cm.dimensionCol == 0 {
			return nil, fmt.Errorf("原表没有独立尺寸列")
		}
		if tokens[baseCount] != dimensionText {
			dims := strings.FieldsFunc(tokens[baseCount], func(r rune) bool { return r == '×' || r == 'x' || r == 'X' })
			if len(dims) != 3 {
				return nil, fmt.Errorf("修改尺寸时请填写完整的外径×内径×长度（缺省项填0）")
			}
			for i, value := range dims {
				values[cm.dimensionCol+i] = value
			}
		}
	} else if dimensionText != "" {
		return nil, fmt.Errorf("规格中缺少原表尺寸，请保留尺寸或填写完整的外径×内径×长度")
	}
	return values, nil
}

func updateBomTemplate(f *excelize.File, items []BomItemPayload) error {
	for _, sheet := range f.GetSheetList() {
		rows, err := f.GetRows(sheet)
		if err != nil {
			return err
		}
		cm := findBomColumns(rows)
		if cm == nil {
			continue
		}
		footer := len(rows) + 1
		for i := cm.headerRow; i < len(rows); i++ {
			if isBottomSignatureRow(rows[i]) {
				footer = i + 1
				break
			}
		}
		start := cm.headerRow + 1
		// Vertically merged identity headers span the entire multi-row header.
		merges, err := f.GetMergeCells(sheet)
		if err != nil {
			return err
		}
		for _, merge := range merges {
			col, row, err := excelize.CellNameToCoordinates(merge.GetStartAxis())
			if err != nil {
				return err
			}
			_, endRow, err := excelize.CellNameToCoordinates(merge.GetEndAxis())
			if err != nil {
				return err
			}
			if row == cm.headerRow && (col == cm.idCol || col == cm.nameCol) && endRow >= start {
				start = endRow + 1
			}
		}
		for start < footer {
			row := rows[start-1]
			text := strings.Join(row, "")
			if !strings.Contains(text, "外径") && !strings.Contains(text, "内径") {
				break
			}
			start++
		}
		capacity := footer - start
		sourceCapacity := capacity
		if capacity < 0 {
			return fmt.Errorf("无法识别模板明细区域")
		}
		originalRows := make(map[string][]string)
		for r := start; r < footer; r++ {
			if cm.idCol <= len(rows[r-1]) {
				originalRows[strings.TrimSpace(rows[r-1][cm.idCol-1])] = rows[r-1]
			}
		}
		if capacity == 0 && len(items) > 0 {
			return fmt.Errorf("原始模板缺少可复用的明细行")
		}
		for capacity < len(items) {
			if err := f.DuplicateRowTo(sheet, footer-1, footer); err != nil {
				return err
			}
			footer++
			capacity++
		}
		for i := 0; i < capacity; i++ {
			r := start + i
			values := map[int]any{}
			if i < len(items) {
				item := items[i]
				values = map[int]any{cm.noCol: i + 1, cm.idCol: item.ID, cm.nameCol: item.Name,
					cm.qtyCol: item.Qty, cm.weightCol: item.Weight,
					cm.totalCol: item.Weight * float64(item.Qty), cm.remarkCol: item.Remark}
				original := originalRows[item.ID]
				if original == nil && sourceCapacity > 0 {
					original = rows[start+min(i, sourceCapacity-1)-1]
				}
				specValues, err := bomTemplateSpecValues(cm, original, item.Spec)
				if err != nil {
					return err
				}
				for col, value := range specValues {
					values[col] = value
				}
			} else {
				// Keep empty template rows and their formatting, but remove deleted material values.
				for c := 1; c <= len(rows[cm.headerRow-1]); c++ {
					values[c] = ""
				}
			}
			for col, value := range values {
				if col <= 0 {
					continue
				}
				axis, err := excelize.CoordinatesToCellName(col, r)
				if err != nil {
					return err
				}
				if err := f.SetCellValue(sheet, axis, value); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return fmt.Errorf("未能识别原始模板的明细表头，无法保留原格式生成打印文件")
}
