package contracttest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// NotesHarnessConfig carries the fixtures a backend needs to auto-mount
// the CR-note (merge-request note) read/edit suite via Harness.Notes.
//
// NoteManager is an optional capability interface with GitLab-family
// semantics: MR notes live in their own iid namespace, so the
// IssueManager comment tools 404 for merge requests there. GitHub-family
// platforms (an issue IS the PR) are covered by IssueManager and are
// expected NOT to provide this config.
type NotesHarnessConfig struct {
	// ListResponse is the JSON array served for the notes-list GET (and
	// the empty terminal page beyond page 1). Required shape: at least
	// one note with id/body/author.username.
	ListResponse string
	// MutateResponse is the JSON object returned by the note-edit PUT.
	MutateResponse string
}

// testNotesSuite auto-mounts RunNotesSuite from a main Harness with the
// same bidirectional drift checks as the other capability suites.
func testNotesSuite(t *testing.T, h Harness) {
	declared := func() bool {
		if h.NewProvider == nil {
			return false
		}
		srv := httptest.NewServer(stubHandler(h))
		defer srv.Close()
		p := h.NewProvider(t, baseCfg(h, srv.URL))
		_, ok := p.(provider.NoteManager)
		return ok
	}()
	switch {
	case h.Notes == nil && !declared:
		t.Skipf("%s declares no NoteManager capability", h.Name)
	case h.Notes == nil:
		t.Errorf("%s implements provider.NoteManager but Harness provides no config", h.Name)
	case !declared:
		t.Errorf("%s Harness provides Notes config but platform does not implement NoteManager", h.Name)
	default:
		RunNotesSuite(t, NotesHarness{
			Name: h.Name, Platform: h.Platform, NewProvider: h.NewProvider,
			ListResponse: h.Notes.ListResponse, MutateResponse: h.Notes.MutateResponse,
		})
	}
}

// NotesHarness bundles inputs for the CR-note suite.
type NotesHarness struct {
	Name           string
	Platform       provider.Platform
	NewProvider    func(t *testing.T, cfg provider.Config) provider.Provider
	ListResponse   string
	MutateResponse string
}

// RunNotesSuite tests CR-comment update and full listing. The mock routes
// by method only: GET serves the notes array (page-aware so AllPages
// walkers stop), PUT returns the mutated note — both platforms'
// note-edit verbs are PUT.
func RunNotesSuite(t *testing.T, h NotesHarness) {
	newNM := func(t *testing.T) provider.NoteManager {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch r.Method {
			case http.MethodGet:
				if beyondFirstPage(r) {
					_, _ = w.Write([]byte(`[]`))
					return
				}
				_, _ = w.Write([]byte(h.ListResponse))
			case http.MethodPut:
				_, _ = w.Write([]byte(h.MutateResponse))
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		}))
		t.Cleanup(srv.Close)
		p := h.NewProvider(t, provider.Config{Platform: h.Platform, BaseURL: srv.URL, Token: "test"})
		nm, ok := p.(provider.NoteManager)
		if !ok {
			t.Fatalf("%s does not implement provider.NoteManager", h.Name)
		}
		return nm
	}

	t.Run("UpdateNote", func(t *testing.T) {
		nm := newNM(t)
		comment, err := nm.UpdateNote(context.Background(), "owner", "repo", "1", "7", "rewritten")
		if err != nil {
			t.Fatalf("UpdateNote: %v", err)
		}
		if comment == nil || comment.Body == "" {
			t.Fatalf("expected a parsed comment, got %+v", comment)
		}
	})

	t.Run("ListNotes", func(t *testing.T) {
		nm := newNM(t)
		notes, err := nm.ListNotes(context.Background(), "owner", "repo", "1")
		if err != nil {
			t.Fatalf("ListNotes: %v", err)
		}
		if len(notes) == 0 {
			t.Fatal("expected at least one note")
		}
		for _, n := range notes {
			if n.Body == "" {
				t.Errorf("note %+v carries no body", n)
			}
		}
	})
}
