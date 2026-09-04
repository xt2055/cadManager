package drawing

import "testing"

func TestNormalizePartNo(t *testing.T) {
	got := NormalizePartNo("  ｊｇ９０６１ｄ－５０－２８－００  ")
	want := "JG9061D-50-28-00"
	if got != want {
		t.Fatalf("NormalizePartNo() = %q, want %q", got, want)
	}
}
