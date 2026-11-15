package gitcode_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// TestGitCode_UpdateLabel_ColorWireFormat verifies the wire form of the color
// field: GitCode's label API uses '#'-prefixed colors (docs: create "eg:
// #fff"; update responses show "#ED4014"), so the backend must prepend '#'
// to the SDK's canonical '#' free form.
func TestGitCode_UpdateLabel_ColorWireFormat(t *testing.T) {
	var gotColor string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			raw, _ := io.ReadAll(r.Body)
			var m map[string]any
			_ = json.Unmarshal(raw, &m)
			gotColor, _ = m["color"].(string)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"name":"bug","color":"#4cc917"}`))
	}))
	defer srv.Close()

	p, err := provider.NewProvider(provider.Config{Platform: model.PlatformGitCode, BaseURL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	lm := p.(provider.LabelManager)
	color := "4cc917"
	if _, err := lm.UpdateLabel(context.Background(), "owner", "repo", "bug", provider.UpdateLabelOptions{Color: &color}); err != nil {
		t.Fatalf("UpdateLabel: %v", err)
	}
	if gotColor != "#4cc917" {
		t.Errorf("wire color = %q, want %q (GitCode expects '#'-prefixed colors)", gotColor, "#4cc917")
	}

	// Idempotence: a caller-supplied '#' must not be doubled.
	gotColor = ""
	prefixed := "#4cc917"
	if _, err := lm.UpdateLabel(context.Background(), "owner", "repo", "bug", provider.UpdateLabelOptions{Color: &prefixed}); err != nil {
		t.Fatalf("UpdateLabel (prefixed): %v", err)
	}
	if gotColor != "#4cc917" {
		t.Errorf("wire color = %q, want %q (prefix must not double)", gotColor, "#4cc917")
	}
}
