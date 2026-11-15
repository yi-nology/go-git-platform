package gitlab

import "testing"

// v0.66.0：parseDiffHunk——new→old 行映射（上下文行同步推进、+ 行 old=0、
// - 行不占 new 号、\ 行忽略、hunk 间重置、文件头忽略）。
func TestParseDiffHunk(t *testing.T) {
	const diff = `--- a/a.go
+++ b/a.go
@@ -10,4 +10,5 @@ func A()
 context-old-10
-removed-11
+added-new-11
+added-new-12
 context-old-12
@@ -100,1 +102,1 @@
ctx-only
`
	h := parseDiffHunk(diff).newToOld
	if h[10] != 10 {
		t.Fatalf("上下文行 new=10 应映射 old=10: %d", h[10])
	}
	if v, ok := h[11]; !ok || v != 0 {
		t.Fatalf("新增行 new=11 应存在且 old=0: %d %v", v, ok)
	}
	if v, ok := h[12]; !ok || v != 0 {
		t.Fatalf("新增行 new=12 应存在且 old=0: %d %v", v, ok)
	}
	if h[13] != 12 {
		t.Fatalf("上下文行 new=13 应映射 old=12: %d", h[13])
	}
	if h[102] != 100 {
		t.Fatalf("第二 hunk 上下文行 new=102 应映射 old=100（hunk 头声明的偏移）: %d", h[102])
	}
	if _, ok := h[100]; ok {
		t.Fatal("文件头/删除行不应出现在 new 映射（old=11 未消费前 hunk1 无 100）")
	}
	if h[11-1] != 10 && h[11-1] != 0 {
		t.Fatalf("new=10 已断言，防御性冗余")
	}
}
