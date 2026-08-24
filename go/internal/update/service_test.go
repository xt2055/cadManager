package update

import "testing"

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name     string
		left     string
		right    string
		expected int
	}{
		{name: "newer", left: "0.2.0", right: "0.1.9", expected: 1},
		{name: "same with v prefix", left: "v0.1.0", right: "0.1.0", expected: 0},
		{name: "older", left: "0.1.0", right: "0.1.1", expected: -1},
		{name: "release beats prerelease", left: "1.0.0", right: "1.0.0-rc1", expected: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if actual := CompareVersions(test.left, test.right); actual != test.expected {
				t.Fatalf("CompareVersions(%q, %q) = %d, expected %d", test.left, test.right, actual, test.expected)
			}
		})
	}
}
