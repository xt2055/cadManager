package attachment

import (
	"path/filepath"
	"regexp"
	"strings"
)

// Creo keeps the save counter after the extension (part.prt.12).
var creoVersionSuffix = regexp.MustCompile(`(?i)\.(prt|asm)\.\d+$`)

func FileFormat(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if match := creoVersionSuffix.FindStringSubmatch(name); len(match) > 1 {
		return match[1]
	}
	return strings.TrimPrefix(filepath.Ext(name), ".")
}

func FileCategory(name string) string {
	switch FileFormat(name) {
	case "z3prt", "z3asm", "z3", "step", "stp", "iges", "igs", "stl", "obj", "glb", "gltf", "sldprt", "sldasm", "prt", "asm", "catpart", "catproduct", "ipt", "iam", "x_t", "x_b", "sat", "sab", "jt", "3mf", "3dm", "ifc", "ply", "fbx":
		return "model3d"
	case "exb", "dwg", "dxf", "pdf", "slddrw", "catdrawing", "idw", "drw", "z3drw":
		return "drawing2d"
	default:
		return "other"
	}
}

func IsModelUpload(name, category string) bool {
	return FileCategory(name) == "model3d" || (category == "model3d" && FileFormat(name) == "zip")
}
