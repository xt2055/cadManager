package attachment

import "testing"

func TestModelFormatClassification(t *testing.T) {
	for _, name := range []string{"零件.Z3PRT", "assembly.z3", "a.STEP", "part.prt.12", "assembly.ASM.1", "part.sldprt", "a.CATProduct", "a.x_t", "mesh.glb"} {
		if FileCategory(name) != "model3d" {
			t.Errorf("model not recognized: %s", name)
		}
	}
	for _, name := range []string{"a.exb", "a.DWG", "a.pdf"} {
		if FileCategory(name) != "drawing2d" {
			t.Errorf("2D not recognized: %s", name)
		}
	}
	for _, name := range []string{"a.zip", "a.z3prt.exe", "a.prt.backup", "README"} {
		if FileCategory(name) != "other" {
			t.Errorf("unexpected model: %s", name)
		}
	}
	if !IsModelUpload("assembly.zip", "model3d") || IsModelUpload("assembly.zip", "other") || IsModelUpload("script.exe", "model3d") {
		t.Fatal("assembly packages require explicit model classification; executable files must be rejected")
	}
}
