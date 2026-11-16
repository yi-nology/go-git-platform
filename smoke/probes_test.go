package smoke

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/yi-nology/go-git-platform/adapter/all" // register every platform
	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

func TestProbesRegistry_SixFamiliesReadOnly(t *testing.T) {
	want := []string{"repo", "cr", "webhook", "pagination", "errors", "connection"}
	for _, f := range want {
		if _, ok := Probes[f]; !ok {
			t.Errorf("family %q missing from Probes registry", f)
		}
	}
	if len(Probes) != len(want) {
		t.Errorf("Probes has %d entries, want exactly %d (register new families here consciously)", len(Probes), len(want))
	}
}

func TestParseFamilies(t *testing.T) {
	// 计划骨架笔误修正：`in, want []string` 会把 in 也声明成 []string，
	// 而 ParseFamilies 收 string——字段改为 `in string; want []string`。
	cases := []struct {
		in   string
		want []string
	}{
		{"", []string{"repo"}}, // 缺省保持现状
		{"all", []string{"repo", "cr", "webhook", "pagination", "errors", "connection"}},
		{"repo,cr", []string{"repo", "cr"}},
	}
	for _, c := range cases {
		got := ParseFamilies(c.in)
		if len(got) != len(c.want) {
			t.Fatalf("ParseFamilies(%q)=%v want %v", c.in, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("ParseFamilies(%q)[%d]=%q want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}

// TestProbes_AllSixFamilies_OnGitHubStub drives every registry family
// through the real GitHub adapter against an httptest stub (spec §6.3:
// 六 family 单测绿，httptest 假 provider 面). It proves the wiring end to
// end — registry → probe → provider → transport → wire parse — without
// live secrets, and records every observed request so the wire-level
// read-only red line (spec §2) is machine-checked: only GETs.
func TestProbes_AllSixFamilies_OnGitHubStub(t *testing.T) {
	const goodToken = "smoke-good-token"

	var (
		mu      sync.Mutex
		methods []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		methods = append(methods, r.Method)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		// Bad credentials (the errors probe's 401/403 leg) → 401 for
		// every route; the good token falls through to the stub data.
		if r.Header.Get("Authorization") != "Bearer "+goodToken {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"Bad credentials"}`))
			return
		}

		path := strings.TrimPrefix(r.URL.Path, "/api/v3")
		page := r.URL.Query().Get("page")
		switch path {
		case "/user": // TestConnection
			_, _ = w.Write([]byte(`{"login":"smoke-user","id":1}`))
		case "/orgs/acme/repos": // personal owner → GitHub 404s the org endpoint
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Not Found"}`))
		case "/users/acme/repos": // ListRepos fallback (page-aware)
			switch page {
			case "", "1":
				_, _ = w.Write([]byte(`[{"id":11,"name":"target","full_name":"acme/target","default_branch":"main"},` +
					`{"id":12,"name":"other","full_name":"acme/other","default_branch":"main"}]`))
			case "2":
				_, _ = w.Write([]byte(`[{"id":13,"name":"third","full_name":"acme/third","default_branch":"main"}]`))
			default:
				_, _ = w.Write([]byte(`[]`))
			}
		case "/repos/acme/target": // GetRepo
			_, _ = w.Write([]byte(`{"id":11,"name":"target","full_name":"acme/target","default_branch":"main"}`))
		case "/repos/acme/target/pulls": // ListCRs (page-aware)
			switch page {
			case "", "1":
				_, _ = w.Write([]byte(`[{"number":1,"title":"first","state":"open","head":{"ref":"feature-x","sha":"aaa"},` +
					`"base":{"ref":"main","sha":"bbb"},"user":{"login":"alice","id":7}},` +
					`{"number":2,"title":"second","state":"closed","merged":false,"head":{"ref":"feature-y","sha":"ccc"},` +
					`"base":{"ref":"main","sha":"bbb"},"user":{"login":"bob","id":8}}]`))
			default:
				_, _ = w.Write([]byte(`[]`))
			}
		case "/repos/acme/target/pulls/1": // GetCR
			_, _ = w.Write([]byte(`{"number":1,"title":"first","state":"open","head":{"ref":"feature-x","sha":"aaa"},` +
				`"base":{"ref":"main","sha":"bbb"},"user":{"login":"alice","id":7}}`))
		case "/repos/acme/target/hooks": // ListWebhooks (AllPages probes page 2)
			if page == "" || page == "1" {
				_, _ = w.Write([]byte(`[{"id":555,"url":"http://hooks.example.test/555","events":["push"],"active":true}]`))
			} else {
				_, _ = w.Write([]byte(`[]`))
			}
		default: // nonexistent repos (ghost leg) and unknown routes → 404
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Not Found"}`))
		}
	}))
	defer srv.Close()

	p, err := provider.NewProvider(provider.Config{
		Platform: model.PlatformGitHub,
		BaseURL:  srv.URL,
		Token:    goodToken,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	base := Options{Owner: "acme", Repo: "target", URL: srv.URL, Token: goodToken}
	for _, fam := range ParseFamilies("all") {
		probe, ok := Probes[fam]
		if !ok {
			t.Fatalf("family %q missing from Probes registry", fam)
		}
		t.Run(fam, func(t *testing.T) {
			var logs []string
			opts := base
			opts.Logf = func(format string, args ...any) {
				logs = append(logs, fmt.Sprintf(format, args...))
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := probe(ctx, p, opts); err != nil {
				t.Fatalf("probe %q against httptest stub: %v", fam, err)
			}
			if len(logs) == 0 {
				t.Errorf("probe %q produced no log lines — Options.Logf wiring broken", fam)
			}
		})
	}

	mu.Lock()
	defer mu.Unlock()
	if len(methods) == 0 {
		t.Fatal("stub observed no requests — probes never hit the wire")
	}
	for _, m := range methods {
		if m != http.MethodGet {
			t.Errorf("read-only red line violated: non-GET request observed on the wire: %s", m)
		}
	}
	t.Logf("wire read-only proof: %d requests, all GET", len(methods))
}
