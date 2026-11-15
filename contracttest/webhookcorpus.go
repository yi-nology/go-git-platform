package contracttest

import (
	"bytes"
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/runtime"
)

// corpusSecret is the shared signing secret for every corpus fixture.
const corpusSecret = "corpus-secret"

// corpusUpdate regenerates golden files:
//
//	go test ./backends/<platform>/ -run WebhookCorpus -corpus-update
var corpusUpdate = flag.Bool("corpus-update", false, "rewrite webhook corpus golden files")

// WebhookParser is the surface RunWebhookCorpus exercises. Every backend's
// Provider implements it via WebhookManager; the narrow interface keeps
// the driver usable with zero-value backend structs (the parse path only
// needs Platform()).
type WebhookParser interface {
	ParseWebhookEvent(r *http.Request, secret string) (*model.NormalizedEvent, error)
}

// RunWebhookCorpus runs the golden-file webhook corpus under dir: every
// <event>.json fixture (raw POST body + required headers) is parsed by
// parser and the normalized event compared byte-for-byte against
// <event>.golden.json with the volatile fields (ID, Timestamp,
// RawPayload) cleared. The corpus is the cross-platform uniformity net:
// every backend must map its platform's wire events onto the same
// canonical vocabulary (cr./push/tag./branch./issue./comment.) with the
// same payload fields populated.
//
// Fixture format:
//
//	{
//	  "headers": {"X-Gitea-Event": "issues"},   // per-platform event headers
//	  "body": { ...raw webhook payload... }
//	}
//
// The driver signs requests with the platform's registered validator and
// corpusSecret, so signature validation runs end to end.
func RunWebhookCorpus(t *testing.T, platform model.Platform, parser WebhookParser, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read corpus dir %s: %v", dir, err)
	}
	v := runtime.DefaultWebhookRegistry().Get(platform)
	ran := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".golden.json") {
			continue
		}
		ran++
		t.Run(strings.TrimSuffix(name, ".json"), func(t *testing.T) {
			runCorpusFixture(t, v, parser, dir, name)
		})
	}
	if ran == 0 {
		t.Fatalf("no fixtures found in %s", dir)
	}
}

// runCorpusFixture executes one corpus fixture: decode, sign, parse,
// normalize, and compare against (or rewrite) the golden file.
func runCorpusFixture(t *testing.T, v runtime.WebhookValidator, parser WebhookParser, dir, name string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fx struct {
		Headers map[string]string `json:"headers"`
		Body    json.RawMessage   `json:"body"`
	}
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	body, err := json.Marshal(fx.Body)
	if err != nil {
		t.Fatalf("encode body: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, val := range fx.Headers {
		req.Header.Set(k, val)
	}
	if v != nil {
		signRequest(req, v, body, corpusSecret)
	}

	ev, err := parser.ParseWebhookEvent(req, corpusSecret)
	if err != nil {
		t.Fatalf("ParseWebhookEvent: %v", err)
	}
	if ev == nil {
		t.Fatal("ParseWebhookEvent returned a nil event (fixture dropped)")
	}
	got := normalizeForGolden(t, ev)

	goldenPath := filepath.Join(dir, strings.TrimSuffix(name, ".json")+".golden.json")
	if *corpusUpdate {
		if err := os.WriteFile(goldenPath, got, 0o600); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (run with -corpus-update to create): %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(want), bytes.TrimSpace(got)) {
		t.Fatalf("normalized event drifted from golden.\n--- golden ---\n%s\n--- got ---\n%s",
			strings.TrimSpace(string(want)), string(got))
	}
}

func normalizeForGolden(t *testing.T, ev *model.NormalizedEvent) []byte {
	t.Helper()
	ev.ID = ""
	ev.Timestamp = time.Time{}
	ev.RawPayload = nil
	out, err := json.MarshalIndent(ev, "", "  ")
	if err != nil {
		t.Fatalf("marshal normalized event: %v", err)
	}
	return append(out, '\n')
}
