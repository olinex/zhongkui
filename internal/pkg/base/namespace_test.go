package base

import "testing"

// TestNamespaceScope 测试跨命名空间操作的作用域解析：租户命名空间并入内置系统命名空间，
// 空命名空间不追加系统作用域（保持既有「空命名空间不命中」语义），系统命名空间本身不重复。
func TestNamespaceScope(t *testing.T) {
	cases := []struct {
		name          string
		namespaceName string
		want          []string
	}{
		{name: "empty keeps empty only", namespaceName: "", want: []string{""}},
		{name: "system namespace stays single", namespaceName: SystemNamespace, want: []string{SystemNamespace}},
		{name: "tenant namespace appends system", namespaceName: "tenant-a", want: []string{"tenant-a", SystemNamespace}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NamespaceScope(tc.namespaceName)
			if len(got) != len(tc.want) {
				t.Fatalf("NamespaceScope(%q) = %v, want %v", tc.namespaceName, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("NamespaceScope(%q)[%d] = %q, want %q", tc.namespaceName, i, got[i], tc.want[i])
				}
			}
		})
	}
}
