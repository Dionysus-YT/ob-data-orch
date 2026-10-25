package executionargv

import "testing"

func Test普通导入严格布尔参数只接受同令牌值(t *testing.T) {
	base := []string{"--host", "synthetic.invalid", "--port", "2883", "--user", "synthetic", "--database", "synthetic", "--file-path", `E:\synthetic`, "--all", "--csv", "--no-sys"}
	for _, tc := range []struct {
		extra []string
		valid bool
	}{
		{[]string{"--strict=true"}, true}, {[]string{"--strict=false"}, true},
		{[]string{"--strict", "true"}, false}, {[]string{"--strict="}, false},
		{[]string{"--strict=invalid"}, false}, {[]string{"--strict=true", "--strict=false"}, false},
		{[]string{"--strict=true", "--password=synthetic-rejected"}, false},
	} {
		if got := ValidNormalImport(append(append([]string(nil), base...), tc.extra...)); got != tc.valid {
			t.Fatal("布尔执行边界不一致")
		}
	}
}
