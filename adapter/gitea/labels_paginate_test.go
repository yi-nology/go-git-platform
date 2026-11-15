package gitea_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/yi-nology/go-git-platform/adapter/gitea"
	"github.com/yi-nology/go-git-platform/adapter/internal/backendutil"
	"github.com/yi-nology/go-git-platform/contracttest"
	"github.com/yi-nology/go-git-platform/provider"
)

// TestGitea_ResolveLabelID_Paginates verifies that UpdateLabel resolves a
// label that sits on page 2, past the 100-label first page.
func TestGitea_ResolveLabelID_Paginates(t *testing.T) {
	page1 := buildLabelsPage(100, "l")
	var mu sync.Mutex
	var methods []string
	var pages []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		methods = append(methods, r.Method)
		if r.Method == http.MethodGet {
			pages = append(pages, r.URL.Query().Get("page"))
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Query().Get("page") == "2":
			_, _ = w.Write([]byte(`[{"id":42,"name":"target","color":"#4cc917"}]`))
		case r.Method == http.MethodGet:
			_, _ = w.Write([]byte(page1))
		default: // PATCH update after resolution
			_, _ = w.Write([]byte(`{"id":42,"name":"renamed","color":"#4cc917"}`))
		}
	}))
	defer srv.Close()
	wrapper := contracttest.VersionProxy(srv.URL, `{"version":"1.22.0"}`)
	defer wrapper.Close()

	p, err := gitea.New(provider.Config{BaseURL: wrapper.URL, Token: "t"})
	if err != nil {
		t.Fatalf("gitea.New: %v", err)
	}
	newName := "renamed"
	lm := p.(provider.LabelManager)
	if _, err := lm.UpdateLabel(context.Background(), "owner", "repo", "target", provider.UpdateLabelOptions{NewName: &newName}); err != nil {
		t.Fatalf("UpdateLabel should resolve %q on page 2, got: %v", "target", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if got := len(methods); got != 3 {
		t.Fatalf("expected exactly 3 requests (2 list GETs + 1 update), got %d: %v", got, methods)
	}
	if len(pages) != 2 || pages[0] != "" && pages[0] != "1" || pages[1] != "2" {
		t.Errorf("expected list GETs for pages 1 and 2 only, got %v", pages)
	}
	if methods[2] == http.MethodGet {
		t.Errorf("expected the final request to be the update verb, got GET")
	}
}

func buildLabelsPage(n int, prefix string) string {
	var b strings.Builder
	b.WriteByte('[')
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"id":` + strconv.Itoa(i+1) + `,"name":"` + prefix + strconv.Itoa(i) + `","color":"#4cc917"}`)
	}
	b.WriteByte(']')
	return b.String()
}

// TestGitea_ResolveLabelID_NotFoundStopsOnShortPage verifies the resolver
// terminates on a short page and reports a scan-limit error (distinct from a
// definitive 404) without further requests.
func TestGitea_ResolveLabelID_NotFoundStopsOnShortPage(t *testing.T) {
	page1 := buildLabelsPage(99, "l") // short page: resolver must stop here
	var gets int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			gets++
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(page1))
	}))
	defer srv.Close()
	wrapper := contracttest.VersionProxy(srv.URL, `{"version":"1.22.0"}`)
	defer wrapper.Close()
	p, err := gitea.New(provider.Config{BaseURL: wrapper.URL, Token: "t"})
	if err != nil {
		t.Fatalf("gitea.New: %v", err)
	}
	lm := p.(provider.LabelManager)
	err = lm.DeleteLabel(context.Background(), "owner", "repo", "missing")
	if err == nil {
		t.Fatal("expected scan-limit error, got nil")
	}
	if provider.IsNotFound(err) {
		t.Fatalf("scan-limit exhaustion must not surface as NotFound, got %v", err)
	}
	if !errors.Is(err, backendutil.ErrLabelScanLimit) {
		t.Fatalf("ErrLabelScanLimit sentinel must survive provider.Wrap, got %v", err)
	}
	if !strings.Contains(err.Error(), "scan limit") {
		t.Fatalf("expected scan-limit error message, got %v", err)
	}
	if gets != 1 {
		t.Fatalf("expected exactly 1 GET (short page terminates), got %d", gets)
	}
}
