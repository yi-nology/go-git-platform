package githubapp

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// testKeyPEM 为每个测试生成一把临时 RSA 私钥并导出为 PEM。
// pkcs8=false 导出 PKCS#1（"RSA PRIVATE KEY"），true 导出 PKCS#8（"PRIVATE KEY"）。
func testKeyPEM(t *testing.T, pkcs8 bool) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generate test key: %v", err)
	}
	var block *pem.Block
	if pkcs8 {
		der, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			t.Fatalf("marshal pkcs8: %v", err)
		}
		block = &pem.Block{Type: "PRIVATE KEY", Bytes: der}
	} else {
		block = &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}
	}
	return string(pem.EncodeToMemory(block))
}

func decodeSegment(t *testing.T, seg string) map[string]any {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(seg)
	if err != nil {
		t.Fatalf("decode segment %q: %v", seg, err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal segment %q: %v", raw, err)
	}
	return m
}

func TestMintJWT_ValidToken(t *testing.T) {
	before := time.Now()
	token, err := MintJWT(12345, testKeyPEM(t, false))
	if err != nil {
		t.Fatal(err)
	}
	after := time.Now()

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 dot-separated segments, got %d (%q)", len(parts), token)
	}

	header := decodeSegment(t, parts[0])
	if header["alg"] != "RS256" {
		t.Errorf("expected alg RS256, got %v", header["alg"])
	}
	if header["typ"] != "JWT" {
		t.Errorf("expected typ JWT, got %v", header["typ"])
	}

	payload := decodeSegment(t, parts[1])
	if payload["iss"] != "12345" {
		t.Errorf("expected iss \"12345\", got %v", payload["iss"])
	}
	iat, ok := payload["iat"].(float64)
	if !ok {
		t.Fatalf("iat missing or not a number: %v", payload["iat"])
	}
	// iat = 签发时刻 - 30s（时钟偏移容忍），允许几秒的执行抖动。
	wantIAT := float64(before.Unix() - 30)
	if iat < wantIAT-5 || iat > float64(after.Unix()-30)+5 {
		t.Errorf("iat out of range: got %v, want ~%v", iat, wantIAT)
	}
	exp, ok := payload["exp"].(float64)
	if !ok {
		t.Fatalf("exp missing or not a number: %v", payload["exp"])
	}
	// exp = 签发时刻 + 9 分钟：早于 +8min 或晚于 +10min 都是错的。
	minExp := float64(before.Add(8 * time.Minute).Unix())
	maxExp := float64(after.Add(10 * time.Minute).Unix())
	if exp < minExp || exp > maxExp {
		t.Errorf("exp out of range: got %v, want within [%v, %v]", exp, minExp, maxExp)
	}

	// 签名段必须非空（RS256 产出 ~128 字节的 base64url）。
	if parts[2] == "" {
		t.Error("signature segment is empty")
	}
}

func TestMintJWT_PKCS8Key(t *testing.T) {
	token, err := MintJWT(42, testKeyPEM(t, true))
	if err != nil {
		t.Fatalf("PKCS#8 key must be accepted: %v", err)
	}
	if len(strings.Split(token, ".")) != 3 {
		t.Fatalf("expected a 3-segment JWT, got %q", token)
	}
}

func TestMintJWT_InvalidPEM(t *testing.T) {
	if _, err := MintJWT(1, "not a pem at all"); err == nil {
		t.Fatal("expected error for invalid PEM")
	}
	// 合法 PEM 但不是 RSA（EC 密钥的 DER 放进 PEM 块也会失败于解析阶段）。
	if _, err := MintJWT(1, string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: []byte("garbage")}))); err == nil {
		t.Fatal("expected error for corrupt key bytes")
	}
}

func TestMintJWT_AppIDZero(t *testing.T) {
	if _, err := MintJWT(0, testKeyPEM(t, false)); err == nil {
		t.Fatal("expected error for appID=0")
	}
	if _, err := MintJWT(-1, testKeyPEM(t, false)); err == nil {
		t.Fatal("expected error for negative appID")
	}
}

func TestFetchInstallationToken_Success(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotAccept, gotAPIVersion string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotAccept = r.Header.Get("Accept")
		gotAPIVersion = r.Header.Get("X-GitHub-Api-Version")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"ghs_testtoken","expires_at":"2030-01-02T03:04:05Z"}`))
	}))
	defer srv.Close()

	token, err := FetchInstallationToken(context.Background(), srv.URL+"/", 42, 7, testKeyPEM(t, false))
	if err != nil {
		t.Fatal(err)
	}
	if token != "ghs_testtoken" {
		t.Errorf("expected token ghs_testtoken, got %q", token)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if gotPath != "/app/installations/7/access_tokens" {
		t.Errorf("unexpected path %q", gotPath)
	}
	// Authorization 必须是 Bearer + 三段式 JWT（即 MintJWT 的产物）。
	if !strings.HasPrefix(gotAuth, "Bearer ") {
		t.Errorf("expected Bearer authorization, got %q", gotAuth)
	} else if jwt := strings.TrimPrefix(gotAuth, "Bearer "); len(strings.Split(jwt, ".")) != 3 {
		t.Errorf("expected a 3-segment JWT bearer token, got %q", jwt)
	}
	if gotAccept != "application/vnd.github+json" {
		t.Errorf("unexpected Accept %q", gotAccept)
	}
	if gotAPIVersion != "2022-11-28" {
		t.Errorf("unexpected X-GitHub-Api-Version %q", gotAPIVersion)
	}
}

func TestFetchInstallationToken_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Bad credentials"}`))
	}))
	defer srv.Close()

	_, err := FetchInstallationToken(context.Background(), srv.URL, 42, 7, testKeyPEM(t, false))
	if err == nil {
		t.Fatal("expected error for non-2xx response")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error must carry the status code, got %v", err)
	}
}

func TestFetchInstallationToken_EmptyToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"expires_at":"2030-01-02T03:04:05Z"}`))
	}))
	defer srv.Close()

	_, err := FetchInstallationToken(context.Background(), srv.URL, 42, 7, testKeyPEM(t, false))
	if err == nil {
		t.Fatal("expected error when token field is empty")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error should mention the empty token, got %v", err)
	}
}
