package cadtext

import (
	"bytes"
	"testing"
)

func TestNormalizeDxfForCaxa(t *testing.T) {
	input := append([]byte("0\nSECTION\n  2\nHEADER\n  9\n$DWGCODEPAGE\n  3\nANSI_1252\n  1\n"), []byte("中文标题\n")...)

	output, err := NormalizeDxfForCaxa(input)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(output, []byte("ANSI_936")) {
		t.Fatalf("DXF codepage was not changed: %q", output)
	}
	if bytes.Contains(output, []byte("中文标题")) {
		t.Fatal("DXF should be encoded as ANSI_936 rather than UTF-8")
	}
}
