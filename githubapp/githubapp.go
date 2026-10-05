// Package githubapp 提供 GitHub App 凭证能力：用 App 的 RSA 私钥签发 RS256
// JWT（第一层凭证），以及用该 JWT 换取 installation access token（第二层
// 凭证，实际调 GitHub API 用）。独立成包是为了让 git-sync-core、git-ferry
// 等下游共享同一实现，删掉各自手写的重复代码。
package githubapp

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/yi-nology/go-git-platform/transport"
)

// DefaultAPIBase 是 GitHub 公有 API 地址；GitHub Enterprise Server 传各自
// 的域名（如 https://ghe.example.com/api/v3）。
const DefaultAPIBase = "https://api.github.com"

// MintJWT 用 App 私钥签发 RS256 JWT，iss=app_id，iat=now-30（容忍调用方与
// GitHub 之间的时钟偏移），exp=now+9 分钟（GitHub 限制 JWT 有效期 ≤10 分钟，
// 留 1 分钟余量）。支持 PKCS#1 与 PKCS#8 两种 PEM RSA 私钥格式——GitHub
// App 下载的 .pem 是 PKCS#8，openssl genrsa 产出的是 PKCS#1，两者都常见。
func MintJWT(appID int64, privateKeyPEM string) (string, error) {
	if appID <= 0 {
		return "", fmt.Errorf("githubapp: app id must be positive, got %d", appID)
	}
	key, err := parseRSAPrivateKey(privateKeyPEM)
	if err != nil {
		return "", fmt.Errorf("githubapp: parse private key: %w", err)
	}
	now := time.Now()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payloadMap := map[string]any{
		"iss": fmt.Sprintf("%d", appID),
		"iat": now.Unix() - 30, // 时钟偏移容忍
		"exp": now.Add(9 * time.Minute).Unix(),
	}
	payloadJSON, err := json.Marshal(payloadMap)
	if err != nil {
		return "", fmt.Errorf("githubapp: marshal jwt payload: %w", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(payloadJSON)
	signingInput := header + "." + payload
	sum := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", fmt.Errorf("githubapp: sign jwt: %w", err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// FetchInstallationToken 用 JWT 换 installation access token（有效期约 1h）。
// apiBase 为空时取 DefaultAPIBase，尾部斜杠会被去掉。
//
// 请求走 transport.Client 而不是裸 http.Client，以复用平台统一的重试/日志/
// 钩子链路；access_tokens 是不幂等的写操作，默认重试配置只允许"请求根本
// 没发出去"（DNS/dial 失败）级别的安全重试，不会重放已到达服务端的 POST。
func FetchInstallationToken(ctx context.Context, apiBase string, appID, installationID int64, privateKeyPEM string) (string, error) {
	token, _, err := fetchInstallationToken(ctx, apiBase, appID, installationID, privateKeyPEM)
	return token, err
}

// fetchInstallationToken additionally reports the token's server-declared
// expiry (zero when the response omits expires_at) — the input the caching
// InstallationTokenSource needs.
func fetchInstallationToken(ctx context.Context, apiBase string, appID, installationID int64, privateKeyPEM string) (string, time.Time, error) {
	jwt, err := MintJWT(appID, privateKeyPEM)
	if err != nil {
		return "", time.Time{}, err
	}
	if apiBase == "" {
		apiBase = DefaultAPIBase
	}
	retry := transport.DefaultRetryConfig()
	client := transport.NewClient(strings.TrimRight(apiBase, "/"), transport.BearerToken{Token: jwt}, transport.WithRetry(&retry))

	resp, err := client.Do(ctx, &transport.Request{
		Method: http.MethodPost,
		Path:   fmt.Sprintf("/app/installations/%d/access_tokens", installationID),
		Headers: http.Header{
			"Accept":               {"application/vnd.github+json"},
			"X-GitHub-Api-Version": {"2022-11-28"},
		},
	})
	if err != nil {
		// transport 只把 >=400 包装成带 status 的错误；3xx 走下面统一判定，
		// 保证 >=300 一律报错且错误信息带 status。
		return "", time.Time{}, fmt.Errorf("githubapp: fetch installation token: %w", err)
	}
	if resp.StatusCode >= 300 {
		return "", time.Time{}, fmt.Errorf("githubapp: fetch installation token: status %d", resp.StatusCode)
	}
	var body struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		return "", time.Time{}, fmt.Errorf("githubapp: decode installation token: %w", err)
	}
	if body.Token == "" {
		return "", time.Time{}, fmt.Errorf("githubapp: installation token empty in response")
	}
	return body.Token, body.ExpiresAt, nil
}

// parseRSAPrivateKey 解析 PEM RSA 私钥，先试 PKCS#1 再试 PKCS#8。
func parseRSAPrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("unsupported private key: %w", err)
	}
	rsaKey, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not RSA")
	}
	return rsaKey, nil
}
