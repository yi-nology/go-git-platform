// Command gensurface regenerates the v1 API golden file
// docs/api-surface-v1.txt from the 18 frozen packages of spec §3.1.
//
// Usage:
//
//	go run ./internal/tools/gensurface            # write docs/api-surface-v1.txt
//	go run ./internal/tools/gensurface -stdout    # print the golden text (guard test path)
//
// This is an explicit action by design: the golden file is never hooked into
// the go:generate mechanism (spec §3.1 discipline — a v1-freeze API change is
// a conscious golden regeneration plus a CHANGELOG entry, and the guard test
// in internal/apisurface compares the live surface against the committed
// file).
//
// The extraction core lives in surface.go, shared with the guard test —
// there is no second extraction implementation in the repo.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	stdout := flag.Bool("stdout", false, "print the golden text to stdout instead of writing docs/api-surface-v1.txt")
	flag.Parse()

	text, err := RenderGolden()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if *stdout {
		fmt.Print(text)
		return
	}

	root, err := repoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	out := filepath.Join(root, "docs", "api-surface-v1.txt")
	if err := os.WriteFile(out, []byte(text), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "gensurface: write:", err)
		os.Exit(1)
	}
	fmt.Println("gensurface: wrote", out)
}
