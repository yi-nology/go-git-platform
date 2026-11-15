package provider

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/yi-nology/go-git-platform/model"
)

// giteeSign computes Gitee's webhook signature for a timestamp:
// Base64(HMAC-SHA256(key=secret, msg=timestamp+"\n"+secret)).
func giteeSign(secret, timestamp string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "\n" + secret))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func giteeRequest(token, timestamp string) *http.Request {
	r, _ := http.NewRequest(http.MethodPost, "/hook", nil)
	r.Header.Set("X-Gitee-Token", token)
	if timestamp != "" {
		r.Header.Set("X-Gitee-Timestamp", timestamp)
	}
	return r
}

func TestGiteeWebhookValidator_SignMode(t *testing.T) {
	const secret = "hook-secret"
	now := fmt.Sprintf("%d", time.Now().UnixMilli())
	v := GiteeWebhookValidator{}

	// Valid signature within the freshness window.
	if err := v.Validate(giteeRequest(giteeSign(secret, now), now), nil, secret); err != nil {
		t.Fatalf("valid sign rejected: %v", err)
	}

	// Forged with a different secret.
	if err := v.Validate(giteeRequest(giteeSign("other", now), now), nil, secret); err == nil {
		t.Fatal("forged signature accepted")
	}

	// Tampered timestamp: the signature no longer matches.
	if err := v.Validate(giteeRequest(giteeSign(secret, now), "999"), nil, secret); err == nil {
		t.Fatal("tampered timestamp accepted")
	}

	// Stale timestamp beyond MaxAge is rejected even with a valid signature.
	stale := fmt.Sprintf("%d", time.Now().Add(-30*time.Minute).UnixMilli())
	if err := v.Validate(giteeRequest(giteeSign(secret, stale), stale), nil, secret); err == nil {
		t.Fatal("stale signature accepted")
	}
	// ...but passes when the freshness bound is explicitly disabled.
	lax := GiteeWebhookValidator{MaxAge: -1}
	if err := lax.Validate(giteeRequest(giteeSign(secret, stale), stale), nil, secret); err != nil {
		t.Fatalf("MaxAge=-1 should disable the freshness check: %v", err)
	}

	// Future-dated timestamp inside the drift bound passes.
	future := fmt.Sprintf("%d", time.Now().Add(3*time.Minute).UnixMilli())
	if err := v.Validate(giteeRequest(giteeSign(secret, future), future), nil, secret); err != nil {
		t.Fatalf("future timestamp inside drift bound rejected: %v", err)
	}

	// Unparseable timestamp.
	if err := v.Validate(giteeRequest(giteeSign(secret, "abc"), "abc"), nil, secret); err == nil {
		t.Fatal("unparseable timestamp accepted")
	}

	// Missing headers.
	if err := v.Validate(giteeRequest("", now), nil, secret); err == nil {
		t.Fatal("missing token accepted")
	}
	if err := v.Validate(giteeRequest(giteeSign(secret, now), ""), nil, secret); err == nil {
		t.Fatal("missing timestamp accepted in sign mode")
	}

	// Empty secret.
	if err := v.Validate(giteeRequest(giteeSign("", now), now), nil, ""); err == nil {
		t.Fatal("empty secret accepted")
	}
}

func TestGiteeWebhookValidator_PasswordMode(t *testing.T) {
	const secret = "plain-password"
	v := GiteeWebhookValidator{AllowPasswordMode: true}

	// Password mode: token carries the password verbatim, no timestamp.
	if err := v.Validate(giteeRequest(secret, ""), nil, secret); err != nil {
		t.Fatalf("valid password rejected: %v", err)
	}
	if err := v.Validate(giteeRequest("wrong", ""), nil, secret); err == nil {
		t.Fatal("wrong password accepted")
	}

	// Password mode requires AllowPasswordMode: without it, a missing
	// timestamp is rejected (the default refuses the weaker mode).
	strict := GiteeWebhookValidator{}
	if err := strict.Validate(giteeRequest(secret, ""), nil, secret); err == nil {
		t.Fatal("password mode accepted without AllowPasswordMode")
	}
}

func TestGiteeWebhookValidator_DefaultRegistry(t *testing.T) {
	// The default registry must mount the Gitee-aware validator, not the
	// body-HMAC one that could never verify either Gitee mode.
	v := defaultWebhookRegistry.Get(model.PlatformGitee)
	if v == nil {
		t.Fatal("no validator registered for Gitee")
	}
	if _, ok := v.(GiteeWebhookValidator); !ok {
		t.Fatalf("Gitee validator = %T, want GiteeWebhookValidator", v)
	}

	// End-to-end through the registry with a fresh secret.
	const secret = "registry-secret"
	now := strconv.FormatInt(time.Now().UnixMilli(), 10)
	r := giteeRequest(giteeSign(secret, now), now)
	if err := defaultWebhookRegistry.Validate(model.PlatformGitee, r, nil, secret); err != nil {
		t.Fatalf("registry.Validate sign mode: %v", err)
	}
}

// ErrWebhookValidation must stay reachable through the error chain so
// callers can classify webhook failures with errors.Is.
func TestGiteeWebhookValidator_ErrorsAreClassified(t *testing.T) {
	v := GiteeWebhookValidator{}
	err := v.Validate(giteeRequest("", nowMs()), nil, "secret")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrWebhookValidation) {
		t.Fatalf("err = %v, want ErrWebhookValidation in chain", err)
	}
}

func nowMs() string { return strconv.FormatInt(time.Now().UnixMilli(), 10) }
