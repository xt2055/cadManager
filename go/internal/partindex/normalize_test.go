package partindex

import (
	"errors"
	"testing"
)

func TestNormalizeFieldsAndDate(t *testing.T) {
	fields, err := NormalizeFields(Fields{
		DrawingNo: "  J1233-03\t",
		PartName:  "主动  轴",
		Material:  " 40Cr ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if fields.DrawingNo != "J1233-03" || fields.PartName != "主动 轴" || fields.Material != "40Cr" {
		t.Fatalf("字段标准化错误：%+v", fields)
	}
	if got := ParseDrawingDate("2026-08-12"); got == nil || *got != "2026-08-12" {
		t.Fatalf("有效日期未解析：%v", got)
	}
	if ParseDrawingDate("2026/08/12") != nil || ParseDrawingDate("2026-02-30") != nil || ParseDrawingDate("0000-01-01") != nil {
		t.Fatal("无效日期被解析")
	}
	if got := ParseDrawingDate("0001-01-01"); got == nil || *got != "0001-01-01" {
		t.Fatalf("年份 0001 的有效日期未解析：%v", got)
	}
}

func TestNormalizeStringPreservesProjectSuffixAndFieldLimits(t *testing.T) {
	if got := NormalizeString("  J1233A  "); got != "J1233A" {
		t.Fatalf("项目编号后缀不应被解释或改写：%q", got)
	}
	if _, err := NormalizeFields(Fields{PartName: string(make([]rune, 501))}); err == nil {
		t.Fatal("超过字段上限的值被接受")
	}
}

func TestValidateConfirmation(t *testing.T) {
	if ValidateConfirmation(Fields{DrawingNo: "J1233-03", PartName: "主动轴"}) != nil {
		t.Fatal("完整确认字段被拒绝")
	}
	if ValidateConfirmation(Fields{DrawingNo: "J1233-03"}) == nil {
		t.Fatal("缺少零件名称的确认字段被接受")
	}
	if err := ValidateConfirmation(Fields{DrawingNo: "J1233-03"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("确认字段错误必须映射为 ErrInvalid：%v", err)
	}
}

func TestNormalizeUUID(t *testing.T) {
	got, err := NormalizeUUID(" AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA ")
	if err != nil || got != "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" {
		t.Fatalf("UUID 标准化失败：got=%q err=%v", got, err)
	}
	if _, err = NormalizeUUID("not-a-uuid"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("无效 UUID 未拒绝：%v", err)
	}
}
