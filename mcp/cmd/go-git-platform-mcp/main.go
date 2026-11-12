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
//	--toolsets    comma-separated subset (run with --help for the live
//	              list; names come straight from the toolset registry)
//	--http        serve streamable HTTP on this address instead of stdio.
//	              ":port" binds 127.0.0.1; a non-loopback host additionally
//	              requires --http-token or the server refuses to start.
//	              The MCP endpoint lives at /mcp.
//	--http-token  require "Authorization: Bearer <token>" on the HTTP
//	              endpoint (required for non-loopback binds)
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
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	oekiterrors "github.com/oekit/pkg/errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/oekit/concurrency/async"
	"github.com/oekit/logx"

	"github.com/yi-nology/go-git-platform/provider"

	mcpserver "github.com/yi-nology/go-git-platform/mcp"

	// register every shipped backend
	_ "github.com/yi-nology/go-git-platform/backends/all"
)

// logger is the binary's service logger (kv style). The mcp server is a
// deployable service, so it owns a service name — SDK packages inside the
// process stay on component-attributed loggers instead.
var logger = logx.WithService("go-git-platform-mcp")

func main() {
	if err := run(); err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
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
		toolsets  = flag.String("toolsets", "", "comma-separated subset of "+strings.Join(mcpserver.ToolsetNames(), ",")+" (empty = all)")
		httpAddr  = flag.String("http", "", "serve streamable HTTP on this address instead of stdio: \":port\" binds loopback only; non-loopback requires --http-token")
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
		return oekiterrors.Wrap(err, "build provider")
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
		return oekiterrors.Wrap(err, "build mcp server")
	}

	if *httpAddr != "" {
		listen, host, err := resolveListenAddr(*httpAddr, *httpToken != "")
		if err != nil {
			return err
		}
		if *skipTLS && !isLoopbackHost(host) {
			logger.Warn("skip-tls with non-loopback HTTP listener: token and git traffic travel in plaintext on the network",
				"addr", listen)
		}
		return serveHTTP(listen, *httpToken, srv)
	}

	ctx := context.Background()
	if err := srv.Run(ctx, &mcp.StdioTransport{}); err != nil {
		return oekiterrors.Wrap(err, "mcp server")
	}
	return nil
}

// resolveListenAddr normalizes the --http listen address and enforces the
// auth policy: a bare ":port" binds loopback only; an explicit non-loopback
// host (wildcard, public IP, or hostname) refuses to start without
// --http-token, so an anonymous remote client can never reach the git
// credentials this process holds.
func resolveListenAddr(addr string, hasToken bool) (listen, host string, err error) {
	h, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", "", fmt.Errorf("invalid --http address %q: %w", addr, err)
	}
	if port == "" {
		return "", "", fmt.Errorf("invalid --http address %q: missing port", addr)
	}
	if h == "" {
		h = "127.0.0.1"
	}
	if !isLoopbackHost(h) && !hasToken {
		return "", "", fmt.Errorf("--http address %q is not loopback: refusing to start without --http-token (anonymous remote clients would share this process's git credentials)", addr)
	}
	return net.JoinHostPort(h, port), h, nil
}

// isLoopbackHost reports whether host is a loopback literal (IP or
// localhost name). Hostname form is intentionally conservative: only
// "localhost" and "*.localhost" qualify; any other name must carry a token.
func isLoopbackHost(host string) bool {
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	return h == "localhost" || strings.HasSuffix(h, ".localhost")
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

	logger.Info("streamable HTTP listening",
		"version", mcpserver.Version(),
		"url", "http://"+addr+"/mcp",
		"auth", map[bool]string{true: "bearer token", false: "NONE — set --http-token"}[bearer != ""])
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
	async.GoSafeWithErrChannel(errCh, func() error { return server.ListenAndServe() })
	select {
	case err := <-errCh:
		if err != nil && !oekiterrors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}
