package handlers

import "testing"

func TestResolvePartNo(t *testing.T) {
	tests := []struct {
		name       string
		filename   string
		titleBlock map[string]string
		want       string
		source     string
	}{
		{name: "文件名优先避免标题栏误读", filename: "A-001.dwg", titleBlock: map[string]string{"图号": "B-002"}, want: "A-001", source: "filename"},
		{name: "标题栏缺失回退文件名", filename: "A-001-02（支架）.dwg", titleBlock: map[string]string{}, want: "A-001-02", source: "filename"},
		{name: "无法识别返回空", filename: "支架.dwg", titleBlock: map[string]string{}, want: "", source: "none"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, source := resolvePartNo(test.filename, test.titleBlock, false)
			if got != test.want || source != test.source {
				t.Fatalf("resolvePartNo() = %q, %q; want %q, %q", got, source, test.want, test.source)
			}
		})
	}
}

func TestResolvePartNoRejectsTitleBlockNoise(t *testing.T) {
	filename := "JG9055e-5032-01(缸体).exb"
	for _, value := range []string{"4-MN-12WD", "0.02", "GB1235-76"} {
		got, source := resolvePartNo(filename, map[string]string{"图号": value}, false)
		if got != "JG9055e-5032-01" || source != "filename" {
			t.Fatalf("resolvePartNo(%q) = %q, %q; want filename fallback", value, got, source)
		}
	}
}

func TestResolvePartNoTitleBlockOnly(t *testing.T) {
	got, source := resolvePartNo("JG9055e-5032-00(总图).exb", map[string]string{"图号": "2000W.02.03E-01-3"}, true)
	if got != "2000W.02.03E-01-3" || source != "titleBlock" {
		t.Fatalf("resolvePartNo() = %q, %q; want title block value", got, source)
	}
	got, source = resolvePartNo("JG9055e-5032-00(总图).exb", map[string]string{"图号": "0.02"}, true)
	if got != "" || source != "none" {
		t.Fatalf("resolvePartNo() = %q, %q; want empty result for noise", got, source)
	}
}

func TestIsLikelyDrawingNoRejectsDimensions(t *testing.T) {
	for _, value := range []string{"4-MN-12WD", "0.02", "GB1235-76", "1.2:1"} {
		if isLikelyDrawingNo(value) {
			t.Fatalf("isLikelyDrawingNo(%q) = true", value)
		}
	}
	for _, value := range []string{"2000W.02.03E-01-3", "JG9055e-5032-01", "JG9055e-50/32-00"} {
		if !isLikelyDrawingNo(value) {
			t.Fatalf("isLikelyDrawingNo(%q) = false", value)
		}
	}
}
