package provider

import (
	"testing"

	"github.com/yi-nology/go-git-platform/model"
)

func TestCountDiffLines(t *testing.T) {
	diff := `--- a/file.go
+++ b/file.go
@@ -1,3 +1,4 @@
 line1
+added1
+added2
-removed1
 context`
	additions, deletions := CountDiffLines(diff)
	if additions != 2 {
		t.Fatalf("expected 2 additions, got %d", additions)
	}
	if deletions != 1 {
		t.Fatalf("expected 1 deletions, got %d", deletions)
	}
}

func TestCountDiffLines_Empty(t *testing.T) {
	additions, deletions := CountDiffLines("")
	if additions != 0 || deletions != 0 {
		t.Fatalf("expected 0,0, got %d,%d", additions, deletions)
	}
}

func TestCountDiffLines_IgnoresHeaders(t *testing.T) {
	diff := `--- a/file.go
+++ b/file.go
+++ added/file.go`
	additions, deletions := CountDiffLines(diff)
	if additions != 0 {
		t.Fatalf("expected 0 additions, got %d", additions)
	}
	if deletions != 0 {
		t.Fatalf("expected 0 deletions, got %d", deletions)
	}
}

func TestBuildRawDiff(t *testing.T) {
	files := []*model.ChangedFile{
		{OldPath: "a.go", NewPath: "b.go", Diff: "+line\n", IsNew: true},
		{OldPath: "c.go", NewPath: "c.go", Diff: "-old\n+new\n"},
	}
	raw := BuildRawDiff(files)
	if raw == "" {
		t.Fatal("expected non-empty raw diff")
	}
	if !contains(raw, "diff --git") {
		t.Fatal("expected diff header")
	}
	if !contains(raw, "new file mode") {
		t.Fatal("expected new file mode for new file")
	}
}

func TestSumDiffStats(t *testing.T) {
	files := []*model.ChangedFile{
		{Additions: 5, Deletions: 2},
		{Additions: 3, Deletions: 1},
	}
	add, del := SumDiffStats(files)
	if add != 8 {
		t.Fatalf("expected 8 additions, got %d", add)
	}
	if del != 3 {
		t.Fatalf("expected 3 deletions, got %d", del)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
