package github

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/yi-nology/go-git-platform/provider"
)

// DownloadReleaseAsset implements provider.ReleaseAssetManager.
//
// SDK 内部用 bareDoUntilFound 处理资产端点的 302(GitHub 对
// Accept: application/octet-stream 返回重定向而非直接字节流),302 目标
// 是签名 URL:必须用无鉴权的裸 client 跟随,不能复用 SDK 自己的
// http.Client(其 CheckRedirect 被禁用)也不该带上 API Bearer。跟随用
// p.followClient —— 同 SkipTLS 策略,GHES 自签场景第二跳不再 x509。
// 跟随后的响应体经 CheckResponse 校验,非 2xx 直接报错;rc 即字节流,
// io.Copy 流式写入 w,不整读内存。
//
// 空响应体契约(#164):iface 文档承诺「无响应体时返回错误」。go-github v93
// 的 followRedirectsClient 非 nil 形态下 200 空体回传的是非 nil 的零长
// reader——rc==nil 守卫生产不可达,字节面复核(io.Copy n==0)是契约真正
// 能成立的落点;零字节资产与「无响应体」在 wire 上不可区分,按契约统一
// 报错,不静默成功(调用方拿 0 字节当成功会静默消费空)。
func (p *Provider) DownloadReleaseAsset(ctx context.Context, owner, repo string, assetID int64, w io.Writer) error {
	rc, redirectURL, err := p.client.Repositories.DownloadReleaseAsset(ctx, owner, repo, assetID, p.followClient)
	if err != nil {
		return provider.Wrap(provider.PlatformGitHub, "DownloadReleaseAsset", err)
	}
	// followRedirectsClient 非 nil 时 SDK 不会回传 redirectURL;rc 为空即
	// 既无字节流也无重定向(无 body),按契约报错而非静默成功。
	if rc == nil {
		return provider.Wrap(provider.PlatformGitHub, "DownloadReleaseAsset",
			fmt.Errorf("empty asset response: no body (redirect %q)", redirectURL))
	}
	defer func() { _ = rc.Close() }()
	n, err := io.Copy(w, rc)
	if err != nil {
		return provider.Wrap(provider.PlatformGitHub, "DownloadReleaseAsset", err)
	}
	// 字节面复核收口(#164):200 空体 → 0 字节写入 → 契约报错。
	if n == 0 {
		return provider.Wrap(provider.PlatformGitHub, "DownloadReleaseAsset",
			errors.New("empty asset response: 0-byte body"))
	}
	return nil
}

// compile-time guard
var _ provider.ReleaseAssetManager = (*Provider)(nil)
