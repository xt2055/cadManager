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
	headerRow int
	idCol     int
	nameCol   int
	specCol   int
	qtyCol    int
	weightCol int
	remarkCol int
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
				if cm.specCol == 0 && (strings.Contains(clean, "规格") || strings.Contains(clean, "材质")) {
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

		if len(req.Items) == 0 {
			response.WriteError(writer, http.StatusBadRequest, "物料明细不能为空")
			return
		}

		var f *excelize.File
		var hasOriginal bool

		if req.StorageKey != "" && strings.HasSuffix(strings.ToLower(req.StorageKey), ".xlsx") {
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
			sheet := f.GetSheetList()[0]
			rows, _ := f.GetRows(sheet)
			colMap := findBomColumns(rows)

			if colMap != nil {
				// Find start of bottom signature footer
				firstFooterRow := len(rows) + 1
				for rIdx := colMap.headerRow; rIdx < len(rows); rIdx++ {
					if isBottomSignatureRow(rows[rIdx]) {
						firstFooterRow = rIdx + 1
						break
					}
				}

				idToRow := make(map[string]int)
				var availableDataRows []int
				for rIdx := colMap.headerRow; rIdx < len(rows) && (rIdx+1) < firstFooterRow; rIdx++ {
					rowNum := rIdx + 1
					row := rows[rIdx]
					var idVal string
					if colMap.idCol-1 < len(row) {
						idVal = strings.TrimSpace(row[colMap.idCol-1])
					}
					if idVal != "" {
						idToRow[idVal] = rowNum
					}
					availableDataRows = append(availableDataRows, rowNum)
				}

				for idx, item := range req.Items {
					var targetRow int
					if rowNum, ok := idToRow[item.ID]; ok {
						targetRow = rowNum
					} else if idx < len(availableDataRows) {
						targetRow = availableDataRows[idx]
					} else {
						// Need new row before footer
						targetRow = firstFooterRow
						_ = f.InsertRows(sheet, targetRow, 1)
						firstFooterRow++
					}

					setColVal := func(colNum int, val any) {
						if colNum > 0 {
							axis, axisErr := excelize.CoordinatesToCellName(colNum, targetRow)
							if axisErr == nil {
								_ = f.SetCellValue(sheet, axis, val)
							}
						}
					}

					setColVal(colMap.idCol, item.ID)
					setColVal(colMap.nameCol, item.Name)
					if colMap.specCol > 0 {
						setColVal(colMap.specCol, item.Spec)
					}
					if colMap.qtyCol > 0 {
						setColVal(colMap.qtyCol, item.Qty)
					}
					if colMap.weightCol > 0 {
						setColVal(colMap.weightCol, item.Weight)
					}
					if colMap.remarkCol > 0 {
						setColVal(colMap.remarkCol, item.Remark)
					}
				}
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
