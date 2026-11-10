package transport

import (
	"context"
	"net/http"
)

// Auth style values understood by TokenSourceAuth. They mirror the
// provider-level TokenStyle vocabulary so backends can forward their
// platform default directly.
const (
	// AuthStyleBearer is "Authorization: Bearer <token>" — GitHub,
	// GitCode, Gitee, GitLab OAuth-style tokens.
	AuthStyleBearer = "bearer"
	// AuthStylePrivate is the "PRIVATE-TOKEN: <token>" header — GitLab
	// (default), Tencent Code.
	AuthStylePrivate = "private"
	// AuthStyleToken is the legacy "Authorization: token <token>" header —
	// Gitea, Forgejo.
	AuthStyleToken = "token"
)

// TokenSource supplies the access token for outgoing requests. It is
// consulted on every request, so implementations may rotate or refresh the
// credential behind the scenes (OAuth device-flow tokens, GitHub App
// installation tokens, expiring CI credentials) without rebuilding clients.
// Implementations must be safe for concurrent use.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// StaticTokenSource returns a TokenSource that always returns token. It is
// the drop-in equivalent of a static AuthStrategy value.
func StaticTokenSource(token string) TokenSource {
	return staticTokenSource{token: token}
}

type staticTokenSource struct{ token string }

func (s staticTokenSource) Token(context.Context) (string, error) {
	return s.token, nil
}

// TokenSourceAuth is an AuthStrategy backed by a TokenSource. The token is
// fetched per request via ApplyContext, which the transport pipeline calls
// with the request's context; a refreshable source therefore takes effect
// on the very next request after rotation. Style selects the header scheme
// (AuthStyleBearer by default).
type TokenSourceAuth struct {
	Source TokenSource
	Style  string
}

// ApplyContext fetches a token from the source and applies it to req.
func (a TokenSourceAuth) ApplyContext(ctx context.Context, req *http.Request) error {
	if a.Source == nil {
		return nil
	}
	tok, err := a.Source.Token(ctx)
	if err != nil {
		return err
	}
	if tok == "" {
		// An empty token must not leave whatever credential the wrapped
		// SDK already placed on the request in place — clear the auth
		// headers this style owns instead of sending a stale secret.
		req.Header.Del("Authorization")
		req.Header.Del("PRIVATE-TOKEN")
		return nil
	}
	switch a.Style {
	case AuthStylePrivate:
		req.Header.Set("PRIVATE-TOKEN", tok)
	case AuthStyleToken:
		req.Header.Set("Authorization", "token "+tok)
	default:
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	return nil
}

// Apply implements AuthStrategy for callers that invoke it directly
// without a context. No context means no cancellation/refresh coordination;
// the transport pipeline itself always goes through ApplyContext. The
// refresh error is returned, not discarded — a caller stuck on the plain
// Apply path must see the failure instead of sending an unauthenticated
// request (issue #68: the pre-fix `_ =` swallowed TokenSource errors).
func (a TokenSourceAuth) Apply(req *http.Request) error {
	return a.ApplyContext(context.Background(), req)
}

// contextAuthStrategy is implemented by AuthStrategies that need the
// request context (e.g. to refresh an expiring token). When present it
// takes precedence over the plain Apply method.
type contextAuthStrategy interface {
	ApplyContext(ctx context.Context, req *http.Request) error
}

// applyAuth applies a to req, preferring the context-aware path so token
// refreshes are honored, and propagates auth acquisition failures instead
// of sending an unauthenticated request.
func applyAuth(ctx context.Context, a AuthStrategy, req *http.Request) error {
	if a == nil {
		return nil
	}
	if ca, ok := a.(contextAuthStrategy); ok {
		return ca.ApplyContext(ctx, req)
	}
	return a.Apply(req)
}
