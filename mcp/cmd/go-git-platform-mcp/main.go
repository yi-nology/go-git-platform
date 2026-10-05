// Command go-git-platform-mcp runs a Model Context Protocol server over
// go-git-platform, exposing one tool surface for GitHub, GitLab, Gitea,
// Forgejo, Gitee, GitCode, and Tencent Code.
//
// Usage:
//
//	go-git-platform-mcp --platform gitea --base-url https://gitea.example.com --token-env GITEA_TOKEN
//
// Flags:
//
//	--platform    one of: github, gitlab, gitea, forgejo, gitee, gitcode, tencentcode
//	--base-url    instance base URL; empty uses the platform's public host
//	--token       API token (prefer --token-env in shells; stdin tokens are
//	              also accepted via GIT_PLATFORM_TOKEN when the flag is unset)
//	--token-env   name of the environment variable holding the token
//	--read-only   mount only read tools (recommended for untrusted agents)
//	--toolsets    comma-separated subset: core,crs,issues,status,search,releases
//	--http        serve streamable HTTP on this address (e.g. ":8080")
//	              instead of stdio; the MCP endpoint lives at /mcp
//	--http-token  require "Authorization: Bearer <token>" on the HTTP
//	              endpoint (strongly recommended when binding non-localhost)
//
// The default transport is MCP over stdio, so a typical client config is:
//
//	{"command": "go-git-platform-mcp", "args": ["--platform", "gitea", "--token-env", "GITEA_TOKEN"]}
//
// For remote/HTTP deployments, front the server with TLS (the listener is
// plain HTTP) and set --http-token so the MCP endpoint is not anonymous.
package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	mcpserver "github.com/yi-nology/go-git-platform/mcp"
	"github.com/yi-nology/go-git-platform/provider"

	// register every shipped backend
	_ "github.com/yi-nology/go-git-platform/backends/all"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var (
		platform  = flag.String("platform", os.Getenv("GIT_PLATFORM"), "github|gitlab|gitea|forgejo|gitee|gitcode|tencentcode")
		baseURL   = flag.String("base-url", os.Getenv("GIT_PLATFORM_URL"), "instance base URL (empty = public host)")
		token     = flag.String("token", "", "API token (prefer --token-env)")
		tokenEnv  = flag.String("token-env", "GIT_PLATFORM_TOKEN", "environment variable holding the API token")
		skipTLS   = flag.Bool("skip-tls", false, "skip TLS verification for self-hosted instances (not recommended)")
		readOnly  = flag.Bool("read-only", false, "mount read tools only")
		toolsets  = flag.String("toolsets", "", "comma-separated subset of core,crs,issues,status,search,releases (empty = all)")
		httpAddr  = flag.String("http", "", "serve streamable HTTP on this address (e.g. \":8080\") instead of stdio")
		httpToken = flag.String("http-token", "", "require Authorization: Bearer <token> on the HTTP endpoint")
	)
	flag.Parse()

	if *platform == "" {
		return fmt.Errorf("--platform (or GIT_PLATFORM) is required: github, gitlab, gitea, forgejo, gitee, gitcode, tencentcode")
	}
	tok := *token
	if tok == "" {
		tok = os.Getenv(*tokenEnv)
	}
	if tok == "" {
		return fmt.Errorf("no token: pass --token or set %s", *tokenEnv)
	}

	cfg := provider.Config{
		Platform: provider.Platform(*platform),
		BaseURL:  *baseURL,
		Token:    tok,
		SkipTLS:  *skipTLS,
	}
	p, err := provider.NewProvider(cfg)
	if err != nil {
		return fmt.Errorf("build provider: %w", err)
	}

	opts := mcpserver.Options{ReadOnly: *readOnly}
	if *toolsets != "" {
		for _, t := range strings.Split(*toolsets, ",") {
			if t = strings.TrimSpace(t); t != "" {
				opts.Toolsets = append(opts.Toolsets, t)
			}
		}
	}

	srv, err := mcpserver.NewServer(p, opts)
	if err != nil {
		return fmt.Errorf("build mcp server: %w", err)
	}

	if *httpAddr != "" {
		return serveHTTP(*httpAddr, *httpToken, srv)
	}

	ctx := context.Background()
	if err := srv.Run(ctx, &mcp.StdioTransport{}); err != nil {
		return fmt.Errorf("mcp server: %w", err)
	}
	return nil
}

// serveHTTP runs the MCP endpoint as streamable HTTP at /mcp. The stateful
// server instance is shared across sessions, matching the SDK's remote
// deployment pattern. When bearer is non-empty, requests without a matching
// Authorization header are rejected before any MCP traffic.
func serveHTTP(addr, bearer string, srv *mcp.Server) error {
	mux := http.NewServeMux()
	mux.Handle("/mcp", mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil))

	var handler http.Handler = mux
	if bearer != "" {
		expected := []byte("Bearer " + bearer)
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), expected) != 1 {
				w.Header().Set("WWW-Authenticate", "Bearer")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			mux.ServeHTTP(w, r)
		})
	}

	log.Printf("go-git-platform-mcp %s: streamable HTTP on http://%s/mcp (auth: %s)",
		mcpserver.Version(), addr, map[bool]string{true: "bearer token", false: "NONE — set --http-token"}[bearer != ""])
	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second, // Slowloris bound; MCP bodies are small JSON-RPC frames
	}

	// Graceful shutdown: SIGINT/SIGTERM stop accepting and give in-flight
	// tool calls a grace window instead of dropping them mid-request.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errCh := make(chan error, 1)
	go func() { errCh <- server.ListenAndServe() }()
	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}
