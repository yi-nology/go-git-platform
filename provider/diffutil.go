package provider

import (
	"fmt"
	"strings"

	"github.com/yi-nology/go-git-platform/model"
)

// CountDiffLines counts additions and deletions in a unified diff string.
func CountDiffLines(diff string) (additions, deletions int) {
	for _, line := range strings.Split(diff, "\n") {
		if line == "" {
			continue
		}
		if line[0] == '+' && !strings.HasPrefix(line, "+++") {
			additions++
		} else if line[0] == '-' && !strings.HasPrefix(line, "---") {
			deletions++
		}
	}
	return
}

// BuildRawDiff constructs a raw diff string from a list of ChangedFiles.
func BuildRawDiff(files []*model.ChangedFile) string {
	var b strings.Builder
	for _, f := range files {
		fmt.Fprintf(&b, "diff --git a/%s b/%s\n", f.OldPath, f.NewPath)
		if f.IsNew {
			b.WriteString("new file mode 100644\n")
		}
		if f.IsDeleted {
			b.WriteString("deleted file mode 100644\n")
		}
		if f.IsRenamed {
			fmt.Fprintf(&b, "rename from %s\nrename to %s\n", f.OldPath, f.NewPath)
		}
		if !f.IsNew {
			fmt.Fprintf(&b, "--- a/%s\n", f.OldPath)
		}
		if !f.IsDeleted {
			fmt.Fprintf(&b, "+++ b/%s\n", f.NewPath)
		}
		b.WriteString(f.Diff)
		b.WriteString("\n")
	}
	return b.String()
}

// SumDiffStats returns total additions and deletions from a list of ChangedFiles.
func SumDiffStats(files []*model.ChangedFile) (additions, deletions int) {
	for _, f := range files {
		additions += f.Additions
		deletions += f.Deletions
	}
	return
}
