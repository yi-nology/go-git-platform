package github_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yi-nology/go-git-platform/backends/github"
	"github.com/yi-nology/go-git-platform/githubapp"
	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// TestGitHub_AppTokenSourceEndToEnd proves the intended wiring: an
// InstallationTokenSource feeding Config.TokenSource mints a JWT, exchanges
// it once, and then serves every API request with the cached installation
// token — one token-endpoint hit per validity window, not per request.
func TestGitHub_AppTokenSourceEndToEnd(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	keyPEM := string(pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key),
	}))

	var tokenFetches atomic.Int64
	var tokenBearer atomic.Value
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenFetches.Add(1)
		tokenBearer.Store(r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"token":"ghs_cached","expires_at":"%s"}`,
			time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
	}))
	defer tokenSrv.Close()

	var apiAuth atomic.Value
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiAuth.Store(r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer apiSrv.Close()

	src := githubapp.NewInstallationTokenSource(tokenSrv.URL, 42, 7, keyPEM)
	p, err := github.New(provider.Config{
		Platform:    model.PlatformGitHub,
		BaseURL:     apiSrv.URL,
		TokenSource: src,
	})
	if err != nil {
		t.Fatalf("github.New: %v", err)
	}

	for i := range 2 {
		repos, err := p.ListRepos(t.Context(), provider.ListRepoOptions{Page: 1, PerPage: 10})
		if err != nil {
			t.Fatalf("ListRepos #%d: %v", i+1, err)
		}
		if len(repos) != 0 {
			t.Fatalf("expected empty repo list, got %d", len(repos))
		}
	}

	if got := tokenFetches.Load(); got != 1 {
		t.Fatalf("token endpoint hit %d times, want exactly 1", got)
	}
	if b := tokenBearer.Load(); b == nil || len(fmt.Sprint(b)) < 20 {
		t.Fatalf("token exchange did not carry a JWT bearer: %v", b)
	}
	if got := apiAuth.Load(); got != "Bearer ghs_cached" {
		t.Fatalf("API authorization = %v, want the cached installation token", got)
	}
}
