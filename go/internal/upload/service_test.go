package upload

import (
	"strings"
	"testing"
	"time"
)

func TestSafeNameRemovesPathTraversal(t *testing.T) {
	tests := map[string]string{
		`..\secret.txt`: "secret.txt",
		"/tmp/part.dwg": "part.dwg",
		"   ":           "file",
		".":             "file",
	}
	for input, expected := range tests {
		if actual := safeName(input); actual != expected {
			t.Fatalf("safeName(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestUniqueKeysPreservesOrderAndRemovesEmptyDuplicates(t *testing.T) {
	actual := uniqueKeys("a", "", "b", "a", "b", "c")
	want := []string{"a", "b", "c"}
	if len(actual) != len(want) {
		t.Fatalf("got %v, want %v", actual, want)
	}
	for index := range want {
		if actual[index] != want[index] {
			t.Fatalf("got %v, want %v", actual, want)
		}
	}
}

func TestIsCADIsCaseInsensitive(t *testing.T) {
	for _, name := range []string{"a.exb", "a.DWG", "a.dxf"} {
		if !isCAD(name) {
			t.Errorf("isCAD(%q) = false", name)
		}
	}
	if isCAD("a.pdf") {
		t.Error("isCAD(pdf) = true")
	}
}

func TestIntervalTextUsesSeconds(t *testing.T) {
	if got := intervalText(90 * time.Second); !strings.Contains(got, "90.000000 seconds") {
		t.Fatalf("intervalText = %q", got)
	}
}

func TestAbsoluteTTLIsLongerThanDefaultSessionTTL(t *testing.T) {
	if absoluteTTL <= 24*time.Hour {
		t.Fatalf("absoluteTTL = %s, want more than one day", absoluteTTL)
	}
}

func TestNormalizeSHA256CanonicalizesUppercase(t *testing.T) {
	input := " " + strings.Repeat("AB", 32) + " "
	got, err := normalizeSHA256(input)
	if err != nil {
		t.Fatalf("normalizeSHA256 returned error: %v", err)
	}
	if got != strings.ToLower(strings.TrimSpace(input)) {
		t.Fatalf("normalizeSHA256 = %q", got)
	}
}

func TestNormalizeSHA256RejectsMalformedValues(t *testing.T) {
	for _, input := range []string{"", "abc", strings.Repeat("g", 64), strings.Repeat("0", 63)} {
		if _, err := normalizeSHA256(input); err == nil {
			t.Errorf("normalizeSHA256(%q) accepted malformed hash", input)
		}
	}
}
