package contracttest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/yi-nology/go-git-platform/provider"
)

// families_contract.go mounts the remaining core-manager behavior suites
// (issue #63): BranchManager, CommitManager, DiffManager, and FileManager.
// All four are core interfaces composed into provider.Provider, so the
// configs are mandatory like Releases/CRs; individual fixture fields may be
// empty, which skips their subtest with a named message so a wiring gap is
// visible without failing unrelated families.
//
// Assertions are tolerant of the platforms' wire vocabularies (branch keys,
// commit id vs sha, files vs diffs endpoint spellings) but pin the parsed
// provider-side values, so a mapping regression fails here for every
// affected backend at once.

// --- BranchManager ---

// BranchHarnessConfig carries the branch suite fixtures. ListResponse's
// first branch must be named "main"; MutateResponse is the create/delete
// response body in the platform's own shape.
type BranchHarnessConfig struct {
	ListResponse   string
	MutateResponse string
}

type BranchHarness struct {
	Name        string
	Platform    provider.Platform
	NewProvider newProviderFn
	BranchHarnessConfig
}

func testBranchSuite(t *testing.T, h Harness) {
	if h.Branches == nil {
		t.Fatalf("%s provides no Branches config — BranchManager is a core interface, every backend must wire the branch suite", h.Name)
	}
	RunBranchSuite(t, BranchHarness{
		Name:                h.Name,
		Platform:            h.Platform,
		NewProvider:         h.NewProvider,
		BranchHarnessConfig: *h.Branches,
	})
}

// RunBranchSuite executes the branch contract suite: list parses, create
// mutates via a state-changing verb, delete issues a DELETE.
func RunBranchSuite(t *testing.T, h BranchHarness) {
	newBM := func(t *testing.T) (provider.BranchManager, *[]recordedRequest) {
		srv, requests := keywordStubServer(keywordStub{
			list:   h.ListResponse,
			mutate: h.MutateResponse,
			// GitHub's CreateBranch pre-resolves the ref through ListCommits;
			// serve a commit-shaped array for any non-branch GET.
			get:        `[{"sha":"abc123","commit":{"message":"commit msg"}}]`,
			pathIsGet:  func(path string) bool { return !strings.Contains(path, "branches") },
			pathIsList: func(path string) bool { return strings.Contains(path, "branches") },
		})
		t.Cleanup(srv.Close)
		p := h.NewProvider(t, provider.Config{Platform: h.Platform, BaseURL: srv.URL, Token: "test"})
		bm, ok := p.(provider.BranchManager)
		if !ok {
			t.Fatalf("%s does not implement provider.BranchManager", h.Name)
		}
		return bm, requests
	}

	t.Run("List_Parses", func(t *testing.T) {
		bm, _ := newBM(t)
		branches, err := bm.ListBranches(context.Background(), "owner", "repo")
		if err != nil {
			t.Fatalf("ListBranches: %v", err)
		}
		if len(branches) == 0 {
			t.Fatal("expected at least one branch")
		}
		if branches[0].Name != "main" {
			t.Errorf("expected first branch %q, got %q", "main", branches[0].Name)
		}
	})

	t.Run("Create_Mutates", func(t *testing.T) {
		bm, requests := newBM(t)
		b, err := bm.CreateBranch(context.Background(), "owner", "repo", "feature-y", "main")
		if err != nil || b == nil {
			t.Fatalf("CreateBranch: branch=%v err=%v", b, err)
		}
		assertVerb(t, requests, http.MethodPost, http.MethodPut)
	})

	t.Run("Delete_Deletes", func(t *testing.T) {
		bm, requests := newBM(t)
		if err := bm.DeleteBranch(context.Background(), "owner", "repo", "feature-y"); err != nil {
			t.Fatalf("DeleteBranch: %v", err)
		}
		for i := range *requests {
			if (*requests)[i].Method == http.MethodDelete {
				return
			}
		}
		t.Errorf("expected a DELETE on the wire, recorded %s", methodsOf(*requests))
	})
}

// --- CommitManager ---

// CommitHarnessConfig carries the commit suite fixtures. GetResponse's
// commit must carry sha/id "abc123" and message "commit msg".
type CommitHarnessConfig struct {
	GetResponse     string
	ListResponse    string
	CompareResponse string // empty = the Compare subtest skips
}

type CommitHarness struct {
	Name        string
	Platform    provider.Platform
	NewProvider newProviderFn
	CommitHarnessConfig
}

func testCommitSuite(t *testing.T, h Harness) {
	if h.Commits == nil {
		t.Fatalf("%s provides no Commits config — CommitManager is a core interface, every backend must wire the commit suite", h.Name)
	}
	RunCommitSuite(t, CommitHarness{
		Name:                h.Name,
		Platform:            h.Platform,
		NewProvider:         h.NewProvider,
		CommitHarnessConfig: *h.Commits,
	})
}

// RunCommitSuite executes the commit contract suite: get/list parse the
// unified CommitInfo, compare parses either commits or files.
func RunCommitSuite(t *testing.T, h CommitHarness) {
	newCM := func(t *testing.T) (provider.CommitManager, *[]recordedRequest) {
		srv, requests := keywordStubServer(keywordStub{
			get:        h.GetResponse,
			list:       h.ListResponse,
			alt:        h.CompareResponse,
			pathIsGet:  func(path string) bool { return strings.Contains(path, "abc123") },
			pathIsAlt:  func(path string) bool { return strings.Contains(path, "compare") },
			pathIsList: func(path string) bool { return strings.Contains(path, "commits") },
		})
		t.Cleanup(srv.Close)
		p := h.NewProvider(t, provider.Config{Platform: h.Platform, BaseURL: srv.URL, Token: "test"})
		cm, ok := p.(provider.CommitManager)
		if !ok {
			t.Fatalf("%s does not implement provider.CommitManager", h.Name)
		}
		return cm, requests
	}

	t.Run("Get_Parses", func(t *testing.T) {
		if h.GetResponse == "" {
			t.Skipf("%s harness wires no commit GetResponse fixture", h.Name)
		}
		cm, _ := newCM(t)
		c, err := cm.GetCommit(context.Background(), "owner", "repo", "abc123")
		if err != nil {
			t.Fatalf("GetCommit: %v", err)
		}
		if c.SHA != "abc123" {
			t.Errorf("expected sha %q, got %q", "abc123", c.SHA)
		}
		if c.Message != "commit msg" {
			t.Errorf("expected message %q, got %q", "commit msg", c.Message)
		}
	})

	t.Run("List_Parses", func(t *testing.T) {
		if h.ListResponse == "" {
			t.Skipf("%s harness wires no commit ListResponse fixture", h.Name)
		}
		cm, _ := newCM(t)
		commits, err := cm.ListCommits(context.Background(), "owner", "repo", provider.ListCommitsOptions{Branch: "main", Page: 1, PerPage: 10})
		if err != nil {
			t.Fatalf("ListCommits: %v", err)
		}
		if len(commits) == 0 || commits[0].SHA == "" {
			t.Fatalf("expected at least one parsed commit, got %v", commits)
		}
	})

	t.Run("Compare_Parses", func(t *testing.T) {
		if h.CompareResponse == "" {
			t.Skipf("%s harness wires no CompareResponse fixture", h.Name)
		}
		cm, _ := newCM(t)
		cr, err := cm.CompareCommits(context.Background(), "owner", "repo", "main", "feature-x")
		if err != nil {
			t.Fatalf("CompareCommits: %v", err)
		}
		if cr == nil || (len(cr.Commits) == 0 && len(cr.Files) == 0) {
			t.Fatalf("expected commits or files in the compare result, got %+v", cr)
		}
	})

	t.Run("List_MalformedSince_Rejected", func(t *testing.T) {
		assertMalformedTimestampRejected(t, newCM, "Since", "2026-01-01")
	})

	t.Run("List_MalformedUntil_Rejected", func(t *testing.T) {
		assertMalformedTimestampRejected(t, newCM, "Until", "not-a-date")
	})
}

// assertMalformedTimestampRejected pins the timestamp-option contract of
// issue #52: the SAME malformed value must produce the same outcome on every
// backend — an ErrInvalidInput rejection before the wire, never a silently
// unfiltered (full history) result with err == nil. A date-only string is
// exactly the hand-written shape incremental syncs naturally produce.
func assertMalformedTimestampRejected(t *testing.T, newCM func(*testing.T) (provider.CommitManager, *[]recordedRequest), field, value string) {
	t.Helper()
	cm, requests := newCM(t)
	opts := provider.ListCommitsOptions{}
	switch field {
	case "Since":
		opts.Since = value
	default:
		opts.Until = value
	}
	_, err := cm.ListCommits(context.Background(), "owner", "repo", opts)
	if err == nil {
		t.Fatalf("malformed %s=%q must error, got nil — the filter was silently dropped and the full history served", field, value)
	}
	if !provider.IsInvalidInput(err) {
		t.Fatalf("malformed %s = %v, want an error wrapping ErrInvalidInput", field, err)
	}
	if got := countGETs(requests); got != 0 {
		t.Fatalf("rejection must happen before the wire, recorded %d GETs", got)
	}
}

// --- DiffManager (GetCRDiff / GetCRFiles / CreateNote) ---

// DiffHarnessConfig carries the diff suite fixtures. FilesResponse is the
// changed-file array served for both the files endpoint and the JSON diffs
// endpoint (every platform serves GetCRDiff and GetCRFiles from one
// endpoint); first file must be "a.txt" carrying "@@" in its diff.
// DiffRaw is the raw unified-diff text for platforms whose GetCRDiff reads a
// text endpoint (gitea family).
type DiffHarnessConfig struct {
	FilesResponse string
	DiffRaw       string
	// ChangesResponse serves the wrapped-object shape of a changes endpoint
	// (gongfeng's merge_request/{id}/changes returns {"files":[…]} instead
	// of a bare array); it takes priority over FilesResponse when the path
	// addresses changes.
	ChangesResponse string
	// FilesWithoutHunks declares that the platform's changed-file endpoint
	// carries no hunk text (gitea family: filename/status/additions only),
	// so the hunk assertion relaxes to additions/deletions.
	FilesWithoutHunks bool
}

type DiffHarness struct {
	Name        string
	Platform    provider.Platform
	NewProvider newProviderFn
	DiffHarnessConfig
}

func testDiffSuite(t *testing.T, h Harness) {
	if h.Diffs == nil {
		t.Fatalf("%s provides no Diffs config — DiffManager is a core interface, every backend must wire the diff suite", h.Name)
	}
	RunDiffSuite(t, DiffHarness{
		Name:              h.Name,
		Platform:          h.Platform,
		NewProvider:       h.NewProvider,
		DiffHarnessConfig: *h.Diffs,
	})
}

// RunDiffSuite executes the diff contract suite — the #44 silent-truncation
// family: GetCRFiles and GetCRDiff must parse and normalize the changed-file
// set, and CreateNote must carry the note body.
func RunDiffSuite(t *testing.T, h DiffHarness) {
	newDM := func(t *testing.T) (provider.DiffManager, *[]recordedRequest) {
		srv, requests := keywordStubServer(keywordStub{
			list:    h.FilesResponse,
			raw:     h.DiffRaw,
			changes: h.ChangesResponse,
			pathIsList: func(path string) bool {
				return strings.Contains(path, "/files") || strings.Contains(path, "diffs") || strings.Contains(path, "changes")
			},
			pathIsChanges: func(path string) bool { return strings.Contains(path, "changes") },
			pathIsRaw:     func(path string) bool { return strings.HasSuffix(path, ".diff") },
		})
		t.Cleanup(srv.Close)
		p := h.NewProvider(t, provider.Config{Platform: h.Platform, BaseURL: srv.URL, Token: "test"})
		dm, ok := p.(provider.DiffManager)
		if !ok {
			t.Fatalf("%s does not implement provider.DiffManager", h.Name)
		}
		return dm, requests
	}

	t.Run("GetCRFiles_Parses", func(t *testing.T) {
		if h.FilesResponse == "" {
			t.Skipf("%s harness wires no FilesResponse fixture", h.Name)
		}
		dm, _ := newDM(t)
		assertDiffFilesParses(t, h, dm)
	})

	t.Run("GetCRDiff_Parses", func(t *testing.T) {
		if h.FilesResponse == "" && h.DiffRaw == "" {
			t.Skipf("%s harness wires no diff fixtures", h.Name)
		}
		dm, _ := newDM(t)
		assertDiffDiffParses(t, dm)
	})

	t.Run("CreateNote_CarriesBody", func(t *testing.T) {
		dm, requests := newDM(t)
		assertDiffNoteWire(t, dm, requests)
	})
}

// --- FileManager ---

// FileHarnessConfig carries the file suite fixtures. GetResponse carries the
// base64 content of "hello world" under the platform's content/encoding
// keys (all seven platforms decode via provider.DecodeFileContent).
type FileHarnessConfig struct {
	GetResponse string
	// ContentIsRaw declares that the platform's content endpoint returns the
	// raw bytes (gitea family) instead of an envelope carrying base64.
	ContentIsRaw bool
}

type FileHarness struct {
	Name        string
	Platform    provider.Platform
	NewProvider newProviderFn
	FileHarnessConfig
}

func testFileSuite(t *testing.T, h Harness) {
	if h.Files == nil {
		t.Fatalf("%s provides no Files config — FileManager is a core interface, every backend must wire the file suite", h.Name)
	}
	RunFileSuite(t, FileHarness{
		Name:              h.Name,
		Platform:          h.Platform,
		NewProvider:       h.NewProvider,
		FileHarnessConfig: *h.Files,
	})
}

// RunFileSuite executes the file contract suite: GetFileContent must decode
// the platform's content encoding into the raw bytes.
func RunFileSuite(t *testing.T, h FileHarness) {
	newFM := func(t *testing.T) (provider.FileManager, *[]recordedRequest) {
		srv, requests := keywordStubServer(keywordStub{
			get: h.GetResponse,
			pathIsGet: func(path string) bool {
				return strings.Contains(path, "contents") || strings.Contains(path, "repository/files") || strings.Contains(path, "blobs")
			},
		})
		t.Cleanup(srv.Close)
		p := h.NewProvider(t, provider.Config{Platform: h.Platform, BaseURL: srv.URL, Token: "test"})
		fm, ok := p.(provider.FileManager)
		if !ok {
			t.Fatalf("%s does not implement provider.FileManager", h.Name)
		}
		return fm, requests
	}

	t.Run("Get_ParsesContent", func(t *testing.T) {
		if h.GetResponse == "" {
			t.Skipf("%s harness wires no File GetResponse fixture", h.Name)
		}
		if h.ContentIsRaw {
			// Raw-content platforms read the bytes straight off the wire:
			// serve plain text instead of the JSON envelope.
			srv, _ := keywordStubServer(keywordStub{
				list: "hello world",
				pathIsList: func(path string) bool {
					return strings.Contains(path, "contents") || strings.Contains(path, "repository/files") || strings.Contains(path, "blobs") || strings.Contains(path, "raw")
				},
			})
			defer srv.Close()
			p := h.NewProvider(t, provider.Config{Platform: h.Platform, BaseURL: srv.URL, Token: "test"})
			fm := p.(provider.FileManager)
			content, err := fm.GetFileContent(context.Background(), "owner", "repo", "README.md", "main")
			if err != nil {
				t.Fatalf("GetFileContent: %v", err)
			}
			if strings.TrimSpace(content) != "hello world" {
				t.Errorf("expected raw content %q, got %q", "hello world", content)
			}
			return
		}
		fm, _ := newFM(t)
		content, err := fm.GetFileContent(context.Background(), "owner", "repo", "README.md", "main")
		if err != nil {
			t.Fatalf("GetFileContent: %v", err)
		}
		if strings.TrimSpace(content) != "hello world" {
			t.Errorf("expected decoded content %q, got %q — the platform's content encoding was not decoded", "hello world", content)
		}
	})
}

// --- shared keyword-routed stub ---

// keywordStub describes a path-keyword-routed mock: verb-based for mutations
// (POST/PUT serve mutate, DELETE serves 204), keyword-based for GETs (get >
// alt > raw > list priority).
type keywordStub struct {
	get           string
	alt           string
	raw           string
	list          string
	mutate        string
	changes       string
	pathIsGet     func(path string) bool
	pathIsAlt     func(path string) bool
	pathIsRaw     func(path string) bool
	pathIsList    func(path string) bool
	pathIsChanges func(path string) bool
}

// keywordStubServer returns a recording mock routed by keywordStub.
func keywordStubServer(cfg keywordStub) (*httptest.Server, *[]recordedRequest) {
	var mu sync.Mutex
	var requests []recordedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := decodeRecordedBody(r)
		mu.Lock()
		requests = append(requests, recordedRequest{Method: r.Method, Path: r.URL.RequestURI(), Body: body})
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(mutationBody(cfg.mutate)))
		case http.MethodPatch, http.MethodPut:
			_, _ = w.Write([]byte(cfg.mutate))
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			_, _ = w.Write([]byte(resolveKeywordBody(cfg, r.URL.Path)))
		}
	}))
	return srv, &requests
}

// mutationBody is the POST response body: the platform's mutate fixture or
// the generic note id.
func mutationBody(mutate string) string {
	if mutate != "" {
		return mutate
	}
	return `{"id":1}`
}

// resolveKeywordBody routes a GET by the keywordStub's predicates, with the
// documented priority (changes > get > alt > raw > list/default).
func resolveKeywordBody(cfg keywordStub, path string) string {
	switch {
	case cfg.pathIsChanges != nil && cfg.pathIsChanges(path) && cfg.changes != "":
		return cfg.changes
	case cfg.pathIsGet != nil && cfg.pathIsGet(path) && cfg.get != "":
		return cfg.get
	case cfg.pathIsAlt != nil && cfg.pathIsAlt(path) && cfg.alt != "":
		return cfg.alt
	case cfg.pathIsRaw != nil && cfg.pathIsRaw(path) && cfg.raw != "":
		return cfg.raw
	default:
		return cfg.list
	}
}

// assertDiffFilesParses checks GetCRFiles parses and normalizes the first
// changed file, relaxing the hunk assertion on no-hunk platforms.
func assertDiffFilesParses(t *testing.T, h DiffHarness, dm provider.DiffManager) {
	t.Helper()
	files, err := dm.GetCRFiles(context.Background(), "owner", "repo", "1")
	if err != nil {
		t.Fatalf("GetCRFiles: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("expected at least one changed file")
	}
	if files[0].NewPath != "a.txt" {
		t.Errorf("expected new path %q, got %q", "a.txt", files[0].NewPath)
	}
	if h.FilesWithoutHunks {
		if files[0].Diff != "" {
			t.Errorf("platform serves no hunks but Diff was set: %q", files[0].Diff)
		}
		return
	}
	if !strings.Contains(files[0].Diff, "@@") {
		t.Errorf("expected a hunk in the file diff, got %q", files[0].Diff)
	}
}

// assertDiffDiffParses checks GetCRDiff returns a parsed merge diff carrying
// either raw unified diff text or per-file hunks.
func assertDiffDiffParses(t *testing.T, dm provider.DiffManager) {
	t.Helper()
	md, err := dm.GetCRDiff(context.Background(), "owner", "repo", "1")
	if err != nil {
		t.Fatalf("GetCRDiff: %v", err)
	}
	if md == nil || len(md.Files) == 0 {
		t.Fatalf("expected a parsed merge diff, got %+v", md)
	}
	if md.RawDiff == "" && md.Files[0].Diff == "" {
		t.Error("expected a raw diff or per-file hunks, got neither")
	}
}

// assertDiffNoteWire checks CreateNote posts the note body and returns a
// non-empty id.
func assertDiffNoteWire(t *testing.T, dm provider.DiffManager, requests *[]recordedRequest) {
	t.Helper()
	id, err := dm.CreateNote(context.Background(), "owner", "repo", "1", "looks good")
	if err != nil || id == "" {
		t.Fatalf("CreateNote: id=%q err=%v", id, err)
	}
	assertBodyHas(t, requests, http.MethodPost, "", "body", "looks good")
}
