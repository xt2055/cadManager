package exb

import (
	"os"
	"testing"
)

func TestParseKnownSampleTitleBlock(t *testing.T) {
	path := `../../storage/attachments/2000W.02.03d(2000W斜撑油缸)/2000W.02.03d-01(缸体).exb`
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Skip("真实 EXB 样本不存在")
	}
	if err != nil {
		t.Fatal(err)
	}

	result, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]string{
		"单位名称": "泸州市巨力液压有限公司",
		"图纸名称": "缸体",
		"材料名称": "27SiMn组焊件",
		"图纸编号": "2000W.02.03C-01",
		"图纸比例": "1:3",
	}
	for key, value := range expected {
		if result.TitleBlock[key] != value {
			t.Fatalf("标题栏 %s = %q，期望 %q", key, result.TitleBlock[key], value)
		}
	}
}
