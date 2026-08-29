package versioning

import "testing"

func TestNextWorkingVersion(t *testing.T) {
	tests := []struct {
		name   string
		latest string
		base   string
		want   string
		valid  bool
	}{
		{name: "首个工作版本", base: "A-001", want: "A-001-w001", valid: true},
		{name: "递增工作版本", latest: "A-001-w009", base: "A-001", want: "A-001-w010", valid: true},
		{name: "缺少后缀", latest: "A-001", base: "A-001", valid: false},
		{name: "超过范围", latest: "A-001-w999", base: "A-001", valid: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := nextWorkingVersion(Version{Version: test.latest}, test.base)
			if test.valid {
				if err != nil || got != test.want {
					t.Fatalf("nextWorkingVersion() = %q, %v; want %q", got, err, test.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("nextWorkingVersion() error = nil; want error")
			}
		})
	}
}
