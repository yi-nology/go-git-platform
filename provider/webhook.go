package provider

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	oekiterrors "github.com/oekit/pkg/errors"
)

// WebhookValidator verifies the authenticity of an incoming webhook request.
// Implementations are stateless and safe for concurrent use.
//
// The signature scheme varies per platform:
//
//   - GitHub:    HMAC-SHA256 of the body, sent in X-Hub-Signature-256.
//   - GitLab:    static token compared in constant time against X-Gitlab-Token.
//   - Gitea /
//     Forgejo:   HMAC-SHA256 of the body, sent in X-Gitea-Signature.
//   - GitCode:   static password compared in constant time against
//     X-GitCode-Token (an HMAC sign mode exists on the platform;
//     its message format is not yet mirrored here).
//   - Tencent:   static token compared in constant time against X-Token.
//   - Gitee:     sign mode — Base64(HMAC-SHA256(secret, timestamp+"\n"+
//     secret)) in X-Gitee-Token with X-Gitee-Timestamp; or
//     password mode — the plain password in X-Gitee-Token.
type WebhookValidator interface {
	Name() string
	Validate(r *http.Request, body []byte, secret string) error
}

// SkipValidationSecret is the sentinel secret that makes
// WebhookEnvelope.Read skip the platform's signature validator.
//
// It exists for hosts that verify the webhook signature themselves and then
// reuse the SDK purely as a payload parser. Two situations need it:
//
//   - The platform's header convention differs from the SDK's model (e.g. a
//     GitLab-family payload delivered by a platform that brands its own token
//     header).
//   - The host validates the signature before dispatch, so the SDK's
//     validation would be a redundant second check on an already-trusted
//     request.
//
// The sentinel is intentionally unusable as a real secret: it contains a NUL
// byte, which no HTTP header value or config file can carry. It is honoured
// only inside WebhookEnvelope.Read; ValidateWebhookWithRegistry and
// Provider.ValidateWebhookSignature always run the real validator, so
// passing the sentinel there does NOT bypass signature checking.
const SkipValidationSecret = "\x00skip-signature-validation"

// ValidatorFunc adapts a plain function into a WebhookValidator.
type ValidatorFunc struct {
	N  string
	Fn func(r *http.Request, body []byte, secret string) error
}

// Name implements WebhookValidator.
func (v ValidatorFunc) Name() string { return v.N }

// Validate implements WebhookValidator.
func (v ValidatorFunc) Validate(r *http.Request, body []byte, secret string) error {
	return v.Fn(r, body, secret)
}

// WebhookValidatorRegistry indexes WebhookValidator implementations by
// platform. It is safe for concurrent use; registrations happen during
// package init and reads happen on the request path.
type WebhookValidatorRegistry struct {
	mu         sync.RWMutex
	validators map[Platform]WebhookValidator
}

// NewWebhookValidatorRegistry builds an empty registry.
func NewWebhookValidatorRegistry() *WebhookValidatorRegistry {
	return &WebhookValidatorRegistry{validators: map[Platform]WebhookValidator{}}
}

// Register associates a validator with a platform.
func (r *WebhookValidatorRegistry) Register(p Platform, v WebhookValidator) {
	if v == nil {
		return
	}
	r.mu.Lock()
	r.validators[p] = v
	r.mu.Unlock()
}

// Get returns the validator for the given platform, or nil if none is
// registered.
func (r *WebhookValidatorRegistry) Get(p Platform) WebhookValidator {
	r.mu.RLock()
	v := r.validators[p]
	r.mu.RUnlock()
	return v
}

// Validate looks up the validator for the given platform and runs it. A nil
// validator or missing platform produces ErrNotImplemented.
func (r *WebhookValidatorRegistry) Validate(p Platform, req *http.Request, body []byte, secret string) error {
	v := r.Get(p)
	if v == nil {
		return oekiterrors.Wrap(ErrNotImplemented, "no webhook validator", "platform", p)
	}
	return v.Validate(req, body, secret)
}

// defaultWebhookRegistry is populated by init functions in webhook_*.go.
var defaultWebhookRegistry = NewWebhookValidatorRegistry()

// DefaultWebhookRegistry returns the process-wide registry.
func DefaultWebhookRegistry() *WebhookValidatorRegistry { return defaultWebhookRegistry }

// HMACSHA256Validator verifies a "Header: sha256=<hex>" style signature
// using HMAC-SHA256 over the request body. The expected header is configurable
// so it works for GitHub (X-Hub-Signature-256), Gitea (X-Gitea-Signature),
// Gitee (X-Gitee-Token) and others.
type HMACSHA256Validator struct {
	Header string
}

// Name implements WebhookValidator.
func (HMACSHA256Validator) Name() string { return "hmac-sha256" }

// Validate implements WebhookValidator. The signature must be exactly
// "sha256=<hex>" or just "<hex>". Comparison is done in constant time.
func (h HMACSHA256Validator) Validate(r *http.Request, body []byte, secret string) error {
	if h.Header == "" {
		return oekiterrors.New("hmac-sha256 validator: empty header")
	}
	header := r.Header.Get(h.Header)
	if header == "" {
		return oekiterrors.Wrap(ErrWebhookValidation, "missing signature header", "header", h.Header)
	}
	if secret == "" {
		// An empty key makes HMAC verification trivially forgeable for
		// predictable payloads (e.g. push events on public repos), so refuse
		// it up front, mirroring StaticTokenValidator.
		return oekiterrors.Wrap(ErrWebhookValidation, "empty secret")
	}
	expected, err := decodeSignature(header, "sha256")
	if err != nil {
		return oekiterrors.Wrap(ErrWebhookValidation, "decode signature", "err", err)
	}
	actual := signBody(secret, body)
	if subtle.ConstantTimeCompare(actual, expected) != 1 {
		return oekiterrors.Wrap(ErrWebhookValidation, "signature mismatch")
	}
	return nil
}

// ForgejoWebhookValidator validates Forgejo's HMAC-SHA256 webhook
// signature. Current Forgejo sends X-Forgejo-Signature; releases forked
// from older Gitea send X-Gitea-Signature, so both are accepted —
// whichever header is present is used (the signature format itself is
// identical raw-hex HMAC-SHA256).
type ForgejoWebhookValidator struct{}

// Name implements WebhookValidator.
func (ForgejoWebhookValidator) Name() string { return "forgejo-hmac-sha256" }

// Validate implements WebhookValidator.
func (ForgejoWebhookValidator) Validate(r *http.Request, body []byte, secret string) error {
	for _, hdr := range []string{"X-Forgejo-Signature", "X-Gitea-Signature"} {
		if r.Header.Get(hdr) != "" {
			return HMACSHA256Validator{Header: hdr}.Validate(r, body, secret)
		}
	}
	return oekiterrors.Wrap(ErrWebhookValidation, "missing signature header", "header", "X-Forgejo-Signature")
}

// GitCodeWebhookValidator accepts GitCode's two webhook verification
// modes: an HMAC-SHA256 body signature (X-GitCode-Signature, falling back
// to the gitea-inherited X-Gitea-Signature) or the documented static
// password in X-GitCode-Token. Whichever header is present is used.
type GitCodeWebhookValidator struct{}

// Name implements WebhookValidator.
func (GitCodeWebhookValidator) Name() string { return "gitcode" }

// Validate implements WebhookValidator.
func (GitCodeWebhookValidator) Validate(r *http.Request, body []byte, secret string) error {
	for _, hdr := range []string{"X-GitCode-Signature", "X-Gitea-Signature"} {
		if r.Header.Get(hdr) != "" {
			return HMACSHA256Validator{Header: hdr}.Validate(r, body, secret)
		}
	}
	if r.Header.Get("X-GitCode-Token") != "" {
		return StaticTokenValidator{Header: "X-GitCode-Token"}.Validate(r, body, secret)
	}
	return oekiterrors.Wrap(ErrWebhookValidation, "missing GitCode signature or token header")
}

// StaticTokenValidator compares a static token header against the configured
// secret in constant time. Used for GitLab's X-Gitlab-Token.
type StaticTokenValidator struct {
	Header string
}

// Name implements WebhookValidator.
func (StaticTokenValidator) Name() string { return "static-token" }

// Validate implements WebhookValidator.
func (s StaticTokenValidator) Validate(r *http.Request, body []byte, secret string) error {
	if s.Header == "" {
		return oekiterrors.New("static-token validator: empty header")
	}
	got := r.Header.Get(s.Header)
	if got == "" {
		return oekiterrors.Wrap(ErrWebhookValidation, "missing token header", "header", s.Header)
	}
	if secret == "" {
		return oekiterrors.Wrap(ErrWebhookValidation, "empty secret")
	}
	if subtle.ConstantTimeCompare([]byte(got), []byte(secret)) != 1 {
		return oekiterrors.Wrap(ErrWebhookValidation, "token mismatch")
	}
	return nil
}

// DefaultGiteeSignMaxAge bounds the accepted X-Gitee-Timestamp drift in
// sign mode when GiteeWebhookValidator.MaxAge is zero.
const DefaultGiteeSignMaxAge = 10 * time.Minute

// GiteeWebhookValidator verifies Gitee webhooks. Gitee offers two security
// modes and both ride the X-Gitee-Token header:
//
//   - 签名密钥 (sign) mode: X-Gitee-Timestamp carries the send time in
//     unix millis and X-Gitee-Token carries
//     Base64(HMAC-SHA256(key=secret, msg=timestamp+"\n"+secret)).
//     This is the default mode this validator checks, including a
//     timestamp freshness bound (replay protection).
//   - 密码 (password) mode: X-Gitee-Token carries the configured password
//     verbatim, X-Gitee-Timestamp is absent, and the signature cannot be
//     computed. Opt in with AllowPasswordMode; the token is then compared
//     against the secret in constant time.
//
// The webhook body plays no part in either scheme (Gitee signs the
// timestamp, not the payload) — transport-level integrity is delegated to
// TLS.
type GiteeWebhookValidator struct {
	// MaxAge bounds |now - X-Gitee-Timestamp| in sign mode. Zero means
	// DefaultGiteeSignMaxAge; a negative value disables the freshness
	// check (not recommended — it enables indefinite replay).
	MaxAge time.Duration
	// AllowPasswordMode also accepts the plain-password form when
	// X-Gitee-Timestamp is absent. Enable only when the webhook is known
	// to be configured in password mode.
	AllowPasswordMode bool
}

// Name implements WebhookValidator.
func (GiteeWebhookValidator) Name() string { return "gitee-sign" }

// Validate implements WebhookValidator.
func (v GiteeWebhookValidator) Validate(r *http.Request, body []byte, secret string) error {
	if secret == "" {
		return oekiterrors.Wrap(ErrWebhookValidation, "empty secret")
	}
	token := r.Header.Get("X-Gitee-Token")
	if token == "" {
		return oekiterrors.Wrap(ErrWebhookValidation, "missing signature header", "header", "X-Gitee-Token")
	}
	ts := strings.TrimSpace(r.Header.Get("X-Gitee-Timestamp"))
	if ts == "" {
		// No timestamp: two body-independent modes remain. Password mode
		// sends the plain secret (opt-in), and the Gitee backend's own
		// scheme signs the request body with HMAC-SHA256 hex in
		// X-Gitee-Token. The body-signature check runs first so a secret
		// equal to another mode's token cannot cross-authenticate.
		if err := (HMACSHA256Validator{Header: "X-Gitee-Token"}).Validate(r, body, secret); err == nil {
			return nil
		}
		if !v.AllowPasswordMode {
			return oekiterrors.Wrap(ErrWebhookValidation, "missing X-Gitee-Timestamp header (password mode requires AllowPasswordMode)")
		}
		if subtle.ConstantTimeCompare([]byte(token), []byte(secret)) != 1 {
			return oekiterrors.Wrap(ErrWebhookValidation, "password mismatch")
		}
		return nil
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "\n" + secret))
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(token), []byte(want)) != 1 {
		return oekiterrors.Wrap(ErrWebhookValidation, "signature mismatch")
	}

	maxAge := v.MaxAge
	if maxAge == 0 {
		maxAge = DefaultGiteeSignMaxAge
	}
	if maxAge > 0 {
		ms, err := strconv.ParseInt(ts, 10, 64)
		if err != nil {
			return oekiterrors.Wrap(ErrWebhookValidation, "bad X-Gitee-Timestamp", "value", ts)
		}
		if drift := time.Since(time.UnixMilli(ms)); drift > maxAge || -drift > maxAge {
			return oekiterrors.Wrap(ErrWebhookValidation, "timestamp drift exceeds max age", "drift", drift.String(), "max_age", maxAge.String())
		}
	}
	return nil
}

// HmacValidator is a lower-level helper that supports arbitrary hash
// algorithms. It is intended for use by platform implementations that need
// something other than SHA-256.
type HmacValidator struct {
	Header    string
	Algorithm string // "sha1", "sha256", "sha512"
	Prefix    string // expected signature prefix ("sha256=")
}

// Name implements WebhookValidator.
func (h *HmacValidator) Name() string { return "hmac-" + h.Algorithm }

// Validate implements WebhookValidator.
func (h *HmacValidator) Validate(r *http.Request, body []byte, secret string) error {
	header := r.Header.Get(h.Header)
	if header == "" {
		return oekiterrors.Wrap(ErrWebhookValidation, "missing signature header", "header", h.Header)
	}
	if secret == "" {
		return oekiterrors.Wrap(ErrWebhookValidation, "empty secret")
	}
	algo := h.Algorithm
	if algo == "" {
		algo = "sha256"
	}
	expected, err := decodeSignature(header, h.Prefix)
	if err != nil {
		return oekiterrors.Wrap(ErrWebhookValidation, "decode signature", "err", err)
	}
	actual := signBodyHash(secret, body, algo)
	if subtle.ConstantTimeCompare(actual, expected) != 1 {
		return oekiterrors.Wrap(ErrWebhookValidation, "signature mismatch")
	}
	return nil
}

// signBody returns the HMAC-SHA256 of body keyed by secret, as raw bytes.
func signBody(secret string, body []byte) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return mac.Sum(nil)
}

// signBodyHash is the generic form. It returns the raw digest so callers can
// compare with decoded signatures in constant time.
func signBodyHash(secret string, body []byte, algo string) []byte {
	var h hash.Hash
	switch strings.ToLower(algo) {
	case "sha1":
		h = hmac.New(sha1.New, []byte(secret))
	case "sha256":
		h = hmac.New(sha256.New, []byte(secret))
	case "sha512":
		h = hmac.New(sha512.New, []byte(secret))
	default:
		h = hmac.New(sha256.New, []byte(secret))
	}
	h.Write(body)
	return h.Sum(nil)
}

// decodeSignature strips an optional "<algo>=" prefix and returns the raw
// signature bytes. Both standard hex encoding and the GitHub-style prefixed
// form are supported.
func decodeSignature(header, expectedPrefix string) ([]byte, error) {
	header = strings.TrimSpace(header)
	if expectedPrefix != "" && strings.HasPrefix(header, expectedPrefix+"=") {
		header = strings.TrimPrefix(header, expectedPrefix+"=")
	}
	if i := strings.IndexByte(header, '='); i >= 0 {
		// Allow the header to carry a "algo=hex" prefix; ignore the algo
		// portion and decode the hex part.
		header = header[i+1:]
	}
	return hex.DecodeString(header)
}

// ReadAndRestoreBody reads the full body of r and replaces it with a fresh
// NopCloser so that downstream readers (signature verification, JSON
// decoding) can still consume it. Use it at the entry point of webhook
// handlers that need to inspect the raw bytes.
func ReadAndRestoreBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

// ValidateWebhookWithRegistry validates req's webhook signature with the
// default registry entry for platform — the single implementation per
// signature scheme (STRATEGY dispatch). The body is read and restored so
// callers can keep parsing the payload afterwards. Platforms without a
// registered validator yield ErrNotImplemented; an empty secret is rejected
// (an empty HMAC key is trivially forgeable on predictable payloads).
//
// Backend WebhookManager implementations delegate here instead of keeping an
// inline copy of their platform's crypto, which had already drifted from the
// registry (empty-secret semantics) before v0.77.0.
func ValidateWebhookWithRegistry(p Platform, r *http.Request, secret string) error {
	body, err := ReadAndRestoreBody(r)
	if err != nil {
		return oekiterrors.Wrap(ErrWebhookValidation, "read request body", "err", err)
	}
	return defaultWebhookRegistry.Validate(p, r, body, secret)
}

// WebhookEnvelope carries one platform's webhook dialect for the shared
// envelope steps every ParseWebhookEvent must run before its payload switch
// (validate → read/restore body → event header → event skeleton). Payload
// decoding and the event-type switch stay per-platform: those are the
// dialect, not the envelope.
type WebhookEnvelope struct {
	// Platform selects the registered signature validator.
	Platform Platform
	// EventHeaders are checked in order for the event-type header; the first
	// non-empty value wins (multi-header dialects list fallbacks). Leave
	// empty when the platform dispatches on a payload field (e.g. GitLab's
	// object_kind) instead of a header.
	EventHeaders []string
	// IDPrefix prefixes generated event IDs ("<prefix>-<unixnano>-<key>").
	// Empty falls back to the platform name — the ID is **never empty**
	// (see NewEvent).
	IDPrefix string
	// IDHeaders are checked in order for the platform's **delivery ID**
	// (e.g. GitHub's "X-GitHub-Delivery"); the first non-empty value wins.
	//
	// The delivery ID is a true idempotency key: the platform repeats the
	// same value when it redelivers an event, so callers can deduplicate
	// properly. Without it the ID is synthesized locally and differs on
	// every delivery (no deduplication possible).
	IDHeaders []string
}

// Read validates the signature via the platform's registered validator,
// restores the body for downstream parsing, and returns the event-type
// header ("" when EventHeaders is empty or unmatched) plus the raw payload.
// Error ops mirror the historical hand-written sequence: body-read failures
// surface under ParseWebhookEvent, validation failures under
// ValidateWebhookSignature.
//
// Passing secret == SkipValidationSecret skips the signature validation
// step: see that constant for the hosts that need parse-only access.
func (e WebhookEnvelope) Read(r *http.Request, secret string) (string, []byte, error) {
	body, err := ReadAndRestoreBody(r)
	if err != nil {
		// Historical ops: the body is read as part of validation, so a read
		// failure surfaces under ValidateWebhookSignature.
		return "", nil, Wrap(e.Platform, "ValidateWebhookSignature", err)
	}
	if secret != SkipValidationSecret {
		if err := defaultWebhookRegistry.Validate(e.Platform, r, body, secret); err != nil {
			return "", nil, Wrap(e.Platform, "ValidateWebhookSignature", err)
		}
	}
	for _, h := range e.EventHeaders {
		if v := r.Header.Get(h); v != "" {
			return v, body, nil
		}
	}
	return "", body, nil
}

// NewEvent builds the event skeleton: Source, Timestamp, RawPayload — plus
// ID "<prefix>-<unixnano>-<idKey>". idKey is the platform's payload identifier
// for the event (issue number, MR IID, ...). Actor/Repo remain the caller's:
// they come from the payload the mapper decodes.
//
// The ID is **never empty**: without an explicit IDPrefix the platform name is
// used as the prefix. An empty event ID is not merely "missing data" — callers
// commonly key their dedup table and their unique index on it, and an empty
// key makes every later event collide with the first one (silently dropping
// all events after the first).
func (e WebhookEnvelope) NewEvent(idKey int64, body []byte) *NormalizedEvent {
	ev := &NormalizedEvent{
		Source:     e.Platform,
		Timestamp:  time.Now(),
		RawPayload: json.RawMessage(body),
	}
	ev.ID = fmt.Sprintf("%s-%d-%d", e.idPrefix(), time.Now().UnixNano(), idKey)
	return ev
}

// NewEventWithID behaves like NewEvent but prefers the platform's delivery ID
// (X-GitHub-Delivery and friends) as the event ID.
//
// The delivery ID is a real idempotency key: the platform repeats it when it
// redelivers the same event, so a caller keying dedup on the event ID can
// actually suppress the duplicate. Falls back to NewEvent's synthesized ID
// when the platform sends no such header.
func (e WebhookEnvelope) NewEventWithID(r *http.Request, idKey int64, body []byte) *NormalizedEvent {
	ev := e.NewEvent(idKey, body)
	if r == nil {
		return ev
	}
	for _, h := range e.IDHeaders {
		if v := strings.TrimSpace(r.Header.Get(h)); v != "" {
			ev.ID = e.idPrefix() + "-" + v
			break
		}
	}
	return ev
}

// idPrefix 返回事件 ID 的前缀:显式 IDPrefix 优先,否则用平台名
// (保证 ID 前缀非空且可读)。
func (e WebhookEnvelope) idPrefix() string {
	if e.IDPrefix != "" {
		return e.IDPrefix
	}
	return string(e.Platform)
}

// Pre-registered validators for the platforms the SDK ships with. These are
// registered in init() below.
func init() {
	defaultWebhookRegistry.Register(PlatformGitHub, HMACSHA256Validator{Header: "X-Hub-Signature-256"})
	defaultWebhookRegistry.Register(PlatformGitea, HMACSHA256Validator{Header: "X-Gitea-Signature"})
	defaultWebhookRegistry.Register(PlatformForgejo, ForgejoWebhookValidator{})
	defaultWebhookRegistry.Register(PlatformGitee, GiteeWebhookValidator{})
	// GitCode documents its webhook password in X-GitCode-Token
	// (docs.gitcode.com "配置 WebHook"); the X-Token body-HMAC scheme
	// registered previously matched no documented mode. An HMAC sign mode
	// exists but its exact message format is not yet verified against the
	// docs — the documented password scheme is registered until then.
	defaultWebhookRegistry.Register(PlatformGitCode, GitCodeWebhookValidator{})
	defaultWebhookRegistry.Register(PlatformTencentCode, StaticTokenValidator{Header: "X-Token"})
	defaultWebhookRegistry.Register(PlatformGitLab, StaticTokenValidator{Header: "X-Gitlab-Token"})
}

// --- Canonical webhook event types ---
//
// These constants define the normalized event type vocabulary. Each backend's
// ParseWebhookEvent must produce these values so consumers can switch on event
// types without knowing the source platform.

const (
	// CR (change request / pull request / merge request) actions.
	CRActionOpened   = "opened"
	CRActionClosed   = "closed"
	CRActionMerged   = "merged"
	CRActionReopened = "reopened"
	CRActionUpdated  = "updated"

	// Event type prefixes.
	EventTypeCR      = "cr."
	EventTypePush    = "push"
	EventTypeTag     = "tag."
	EventTypeBranch  = "branch."
	EventTypeIssue   = "issue."
	EventTypeComment = "comment."

	// Comment actions. GitHub delivers created/edited/deleted; the other
	// platforms only notify on creation today.
	CommentActionCreated = "created"
	CommentActionEdited  = "edited"
	CommentActionDeleted = "deleted"
)

// NormalizeCRAction maps platform-specific PR/MR action strings to the
// canonical vocabulary. Each backend should call this instead of ad-hoc
// string mapping.
//
// Mappings:
//
//	GitHub:   "opened" → opened, "closed" (+merged) → merged/closed,
//	          "reopened" → reopened, "synchronize"/"edited" → updated
//	GitLab:   "open" → opened, "close" → closed, "merge" → merged,
//	          "reopen" → reopened, "update" → updated
//	Gitee:    "open" → opened, "close" (+merged check) → merged/closed
//	GitCode:  "opened" → opened, "closed" (+merged check) → merged/closed
//	Gitea:    "opened" → opened, "closed" (+merged check) → merged/closed
func NormalizeCRAction(action string, merged bool) string {
	switch strings.ToLower(action) {
	case "opened", "open":
		return CRActionOpened
	case "closed", "close":
		if merged {
			return CRActionMerged
		}
		return CRActionClosed
	case "merged", "merge":
		return CRActionMerged
	case "reopened", "reopen":
		return CRActionReopened
	case "synchronize", "sync", "edited", "edit", "update":
		return CRActionUpdated
	default:
		return action
	}
}

// NormalizeTagAction maps platform-specific tag event actions to the
// canonical vocabulary. The canonical full event type is "tag.created":
// GitLab/Gitea/Tencent signal a tag with a tag_push hook (a tag being
// pushed is the tag coming into existence), and GitHub's create/delete
// events carry ref_type "tag".
func NormalizeTagAction(action string) string {
	switch strings.ToLower(action) {
	case "push", "pushed", "created", "create":
		return "created"
	default:
		return action
	}
}

// NormalizeBranchAction maps platform-specific branch event actions.
func NormalizeBranchAction(action string) string {
	switch strings.ToLower(action) {
	case "created", "create":
		return "created"
	case "deleted", "delete":
		return "deleted"
	default:
		return action
	}
}

// NormalizeIssueAction maps platform-specific issue event actions.
func NormalizeIssueAction(action string) string {
	switch strings.ToLower(action) {
	case "opened", "open":
		return "opened"
	case "closed", "close":
		return "closed"
	case "reopened", "reopen":
		return "reopened"
	case "edited", "edit", "updated", "update":
		return "updated"
	default:
		return action
	}
}
