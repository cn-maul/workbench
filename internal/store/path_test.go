//go:build windows

package store

import "testing"

// 回归：Windows 路径不区分大小写。此前 SamePath/Nested 用大小写敏感比较，
// 「换大小写重选同一目录」会被判为迁移 → 自我复制 → 删掉正在使用的 index.db。
func TestSamePathAndNestedCaseInsensitive(t *testing.T) {
	cases := []struct {
		name      string
		a, b      string
		same, nst bool
	}{
		{"同目录大小写不同", `D:\我的工作台`, `d:\我的工作台`, true, true},
		{"同目录混合大小写", `C:\WS\Sub`, `c:\ws\sub`, true, true},
		{"不同目录", `D:\a`, `D:\b`, false, false},
		{"子目录嵌套", `D:\ws\child`, `D:\ws`, false, true},
		{"父目录不嵌套", `D:\ws`, `D:\ws\child`, false, false},
		{"尾部斜杠", `D:\ws\`, `D:\WS`, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SamePath(c.a, c.b); got != c.same {
				t.Errorf("SamePath(%q, %q) = %v, 期望 %v", c.a, c.b, got, c.same)
			}
			if got := Nested(c.a, c.b); got != c.nst {
				t.Errorf("Nested(%q, %q) = %v, 期望 %v", c.a, c.b, got, c.nst)
			}
		})
	}
}
