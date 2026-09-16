package handlers

import (
	"reflect"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestUpdateBomTemplateMergedHeaderAndSeparateSpecification(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	sheet := "Sheet1"
	f.SetCellValue(sheet, "A1", "物料需求表")
	header := []any{"零件代号", "零件名称", "下料尺寸", "", "", "材质", "规格", "单支数量", "计划数量", "炉号", "实际下料完成时间", "外协自制", "领取人材料/图纸", "要求入库时间", "入库时间", "单重", "备注"}
	f.SetSheetRow(sheet, "A2", &header)
	f.MergeCell(sheet, "C2", "E2")
	for _, col := range []string{"A", "B", "F", "G", "H", "I", "J", "K", "L", "M", "N", "O", "P", "Q"} {
		f.MergeCell(sheet, col+"2", col+"3")
	}
	subheader := []any{"外径（长）", "内径（宽）", "长度（高）"}
	f.SetSheetRow(sheet, "C3", &subheader)
	row := []any{"01", "法兰缸体", 80, 0, 50, "27SiMn", "圆钢", 100, 200, "炉号保留", "", "", "", "", "", 2, ""}
	f.SetSheetRow(sheet, "A4", &row)
	f.SetCellValue(sheet, "A5", "编制：张工")
	before, _ := f.GetRows(sheet)
	if err := updateBomTemplate(f, []BomItemPayload{{ID: "01", Name: "修改名称", Spec: "27SiMn 圆钢 80×50", Qty: 3, Weight: 2}}); err != nil {
		t.Fatal(err)
	}
	after, _ := f.GetRows(sheet)
	if !reflect.DeepEqual(before[:3], after[:3]) {
		t.Fatal("merged header was overwritten")
	}
	for axis, want := range map[string]string{"A4": "01", "B4": "修改名称", "C4": "80", "D4": "0", "E4": "50", "F4": "27SiMn", "G4": "圆钢", "H4": "3", "I4": "200"} {
		if got, _ := f.GetCellValue(sheet, axis); got != want {
			t.Fatalf("%s = %q, want %q", axis, got, want)
		}
	}
	if err := updateBomTemplate(f, []BomItemPayload{{ID: "01", Name: "修改名称", Spec: "20 无缝管 90×7×60", Qty: 3, Weight: 2}}); err != nil {
		t.Fatal(err)
	}
	for axis, want := range map[string]string{"C4": "90", "D4": "7", "E4": "60", "F4": "20", "G4": "无缝管"} {
		if got, _ := f.GetCellValue(sheet, axis); got != want {
			t.Fatalf("%s = %q, want %q", axis, got, want)
		}
	}
}

func TestUpdateBomTemplate(t *testing.T) {
	for _, count := range []int{0, 1, 3} {
		t.Run(string(rune('0'+count)), func(t *testing.T) {
			f := excelize.NewFile()
			defer f.Close()
			f.SetSheetName("Sheet1", "封面")
			f.SetCellValue("封面", "A1", "封面保留")
			f.NewSheet("备料")
			sheet := "备料"
			header := []any{"序号", "图号", "名称", "规格", "数量", "单重", "总重", "备注"}
			f.SetSheetRow(sheet, "A2", &header)
			for _, axis := range []string{"A3", "A4"} {
				row := []any{1, "旧图号", "旧名称", "钢", 9, 8, 72, "旧备注"}
				f.SetSheetRow(sheet, axis, &row)
			}
			f.SetCellValue(sheet, "A5", "编制：张工")
			f.MergeCell(sheet, "A5", "D5")
			style, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Color: "FF0000"}, Border: []excelize.Border{{Type: "bottom", Color: "000000", Style: 1}}})
			f.SetCellStyle(sheet, "A3", "H4", style)
			f.SetRowHeight(sheet, 4, 31)
			f.SetColWidth(sheet, "B", "B", 27)
			orientation := "landscape"
			f.SetPageLayout(sheet, &excelize.PageLayoutOptions{Orientation: &orientation})
			items := make([]BomItemPayload, count)
			for i := range items {
				items[i] = BomItemPayload{ID: "新图号", Name: "修改名称", Qty: 2, Weight: 3}
			}
			if err := updateBomTemplate(f, items); err != nil {
				t.Fatal(err)
			}
			buf, err := f.WriteToBuffer()
			if err != nil {
				t.Fatal(err)
			}
			out, err := excelize.OpenReader(buf)
			if err != nil {
				t.Fatal(err)
			}
			defer out.Close()
			for i := 0; i < count; i++ {
				axis, _ := excelize.CoordinatesToCellName(3, i+3)
				if value, _ := out.GetCellValue(sheet, axis); value != "修改名称" {
					t.Fatalf("%s = %q", axis, value)
				}
				if got, _ := out.GetCellStyle(sheet, axis); got != style {
					t.Fatalf("lost style at %s", axis)
				}
			}
			if count < 2 {
				axis, _ := excelize.CoordinatesToCellName(3, count+3)
				if value, _ := out.GetCellValue(sheet, axis); value != "" {
					t.Fatalf("deleted row remains: %q", value)
				}
			}
			footer := "A5"
			if count == 3 {
				footer = "A6"
				if h, _ := out.GetRowHeight(sheet, 5); h != 31 {
					t.Fatalf("new row height = %v", h)
				}
			}
			if value, _ := out.GetCellValue(sheet, footer); value != "编制：张工" {
				t.Fatalf("footer lost: %q", value)
			}
			if merges, _ := out.GetMergeCells(sheet); len(merges) != 1 {
				t.Fatal("footer merge lost")
			}
			if width, _ := out.GetColWidth(sheet, "B"); width != 27 {
				t.Fatal("column width lost")
			}
			if layout, _ := out.GetPageLayout(sheet); layout.Orientation == nil || *layout.Orientation != orientation {
				t.Fatal("print orientation lost")
			}
		})
	}
}

func TestUpdateBomTemplateRejectsUnknownLayout(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	if err := updateBomTemplate(f, []BomItemPayload{{ID: "new"}}); err == nil {
		t.Fatal("must not return unchanged source")
	}
}
