//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	// 后端注册：provider.NewProvider 走 init() 注册表，e2e 只消费。
	_ "github.com/yi-nology/go-git-platform/adapter/all"

	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// 镜像钉（T4 Step1 现场实测）：gitea.dev/sdk v1.5.1 的 version 门最大认到
// 29.0.0（version.go: version28_0_0/version29_0_0），gitea/gitea:28.4 实测
// /api/v1/version = 28.4.0；codeberg.org/mvdkleijn/forgejo-sdk/forgejo/v3
// v3.0.0 的门 version1_22_0 与 forgejo/forgejo:17.0.1 实测 version 串
// "17.0.1+gitea-1.22.0" 逐字对齐。Makefile 的 GITEA_IMAGE/FORGEJO_IMAGE 与
// 这里保持同一钉值（测试内缺省为兜底，make e2e 会显式注入同值）。
const (
	giteaImageDefault   = "gitea/gitea:28.4"
	forgejoImageDefault = "codeberg.org/forgejo/forgejo:17.0.1"
	// 沙箱凭据：仅存在于一次性容器与进程内，仓库零 secrets（spec §5）。
	// 密码满足 Gitea/Forgejo 缺省密码策略（≥8 位 + 字符类别）。
	adminUser  = "e2erunner"
	adminPass  = "E2ePassw0rd!"
	adminEmail = "e2e@example.local"

	readyTimeout = 120 * time.Second
	readyPoll    = 1 * time.Second
)

// platformSpec 描述一个被测平台的双场合接入面。
type platformSpec struct {
	kind       string         // "gitea" | "forgejo"
	platform   model.Platform // provider 注册名
	urlEnv     string         // CI 模式的 URL 注入
	tokenEnv   string         // CI 模式的 token 注入
	imageEnv   string         // 本地模式镜像覆盖（Makefile 注入）
	imageDeflt string         // 本地模式镜像钉缺省
	envPrefix  string         // 镜像的 environment-to-ini 前缀（GITEA__/FORGEJO__）
}

var (
	giteaSpec = platformSpec{
		kind:       "gitea",
		platform:   model.PlatformGitea,
		urlEnv:     "E2E_GITEA_URL",
		tokenEnv:   "E2E_GITEA_TOKEN",
		imageEnv:   "GITEA_IMAGE",
		imageDeflt: giteaImageDefault,
		envPrefix:  "GITEA__",
	}
	forgejoSpec = platformSpec{
		kind:       "forgejo",
		platform:   model.PlatformForgejo,
		urlEnv:     "E2E_FORGEJO_URL",
		tokenEnv:   "E2E_FORGEJO_TOKEN",
		imageEnv:   "FORGEJO_IMAGE",
		imageDeflt: forgejoImageDefault,
		envPrefix:  "FORGEJO__",
	}
)

// requestCounter 是零写断言的 wire 级计数器：provider.Config.Hooks 的
// Response hook 按 HTTP 方法分桶（GET/HEAD=读，POST/PUT/PATCH/DELETE=写）。
// backendutil.NewTransportClient 把 cfg.Hooks 桥进 transport（site-verified），
// gitea/forgejo 两后端构造均走该路径；SDK 不配 Retry（RetryConfig nil），
// 计数即真实请求数。
type requestCounter struct {
	reads  int64
	writes int64
}

func (c *requestCounter) hook(_ context.Context, _ *http.Request, resp *http.Response, _ time.Duration, _ error) {
	if resp == nil {
		return
	}
	switch resp.Request.Method {
	case http.MethodGet, http.MethodHead:
		c.reads++
	default:
		c.writes++
	}
}

// newProvider 构造被测 provider（挂请求计数 hook），并断言实现被测接口。
func newProvider(t *testing.T, spec platformSpec, baseURL, token string) (provider.Provider, *requestCounter) {
	t.Helper()
	counter := &requestCounter{}
	hooks := &provider.Hooks{}
	hooks.AddResponseHook(counter.hook)
	p, err := provider.NewProvider(provider.Config{
		Platform: spec.platform,
		BaseURL:  baseURL,
		Token:    token,
		Hooks:    hooks,
	})
	if err != nil {
		t.Fatalf("newProvider(%s): %v", spec.kind, err)
	}
	// 被测面接口断言（brief Step2）：缺能力即测试环境错，不是红。
	for name, ok := range map[string]bool{
		"IssueManager":            interfaceOK[provider.IssueManager](p),
		"LabelManager":            interfaceOK[provider.LabelManager](p),
		"ChangeRequestManager":    interfaceOK[provider.ChangeRequestManager](p),
		"BranchProtectionManager": interfaceOK[provider.BranchProtectionManager](p),
		"RepoManager":             interfaceOK[provider.RepoManager](p),
		"BranchManager":           interfaceOK[provider.BranchManager](p),
		"FileManager":             interfaceOK[provider.FileManager](p),
	} {
		if !ok {
			t.Fatalf("provider %s lacks %s", spec.kind, name)
		}
	}
	return p, counter
}

// interfaceOK 泛型接口断言助手（map 字面量里不好内联写，单点收口）。
func interfaceOK[T any](p provider.Provider) bool {
	_, ok := p.(T)
	return ok
}

// startPlatform 按 brief Step2 起平台：CI 模式复用 env 端点；本地模式
// docker run 一次性镜像并引导 admin token。返回 baseURL、token 与本地
// 容器名（CI 模式容器名为空——容器编排层归 Actions services 管）。
func startPlatform(t *testing.T, spec platformSpec) (baseURL, token, container string) {
	t.Helper()

	url := os.Getenv(spec.urlEnv)
	if url != "" {
		// CI 模式：services 已就绪并由编排层引导，直接复用；token 必须同给。
		token := os.Getenv(spec.tokenEnv)
		if token == "" {
			t.Fatalf("%s set without %s: CI harness contract requires both", spec.urlEnv, spec.tokenEnv)
		}
		url = strings.TrimRight(url, "/")
		if err := pollVersion(url, readyTimeout); err != nil {
			t.Fatalf("%s endpoint %s not ready: %v", spec.kind, url, err)
		}
		return url, token, ""
	}

	// 本地模式：需要 docker；没有则按契约 skip（不红）。
	if !dockerAvailable() {
		t.Skipf("%s unset and docker unavailable — no venue for %s write roundtrip (CI runs it via e2e.yml services)", spec.urlEnv, spec.kind)
	}

	image := os.Getenv(spec.imageEnv)
	if image == "" {
		image = spec.imageDeflt
	}
	port := freePort(t)
	name := fmt.Sprintf("e2e-%s-%d", spec.kind, time.Now().UnixNano())
	// unattended：install lock 环境变量跳过安装向导（两镜像现场实测生效）。
	runDocker(t, "run", "-d", "--name", name,
		"-p", strconv.Itoa(port)+":3000",
		"-e", spec.envPrefix+"security__INSTALL_LOCK=true",
		"-e", spec.envPrefix+"server__ROOT_URL=http://localhost:"+strconv.Itoa(port)+"/",
		image)
	baseURL = "http://localhost:" + strconv.Itoa(port)
	t.Cleanup(func() {
		if t.Failed() {
			logContainerTail(t, name)
		}
		// t.Cleanup 兜底一次性容器回收（红线：不留驻写面）。
		out, err := exec.Command("docker", "rm", "-f", name).CombinedOutput()
		if err != nil {
			t.Logf("cleanup docker rm -f %s failed: %v (%s)", name, err, strings.TrimSpace(string(out)))
		}
	})

	if err := pollVersion(baseURL, readyTimeout); err != nil {
		t.Fatalf("%s container %s never became ready: %v", spec.kind, name, err)
	}

	// 引导 admin：官方镜像内置 CLI（-u git 以镜像内运行用户身份执行；
	// 现场实测 /app/gitea/gitea 在 gitea:28.4 与 forgejo:17.0.1 均存在）。
	out, err := exec.Command("docker", "exec", "-u", "git", name,
		"/app/gitea/gitea", "admin", "user", "create", "--admin",
		"--username", adminUser, "--password", adminPass,
		"--email", adminEmail, "--must-change-password=false").CombinedOutput()
	if err != nil {
		t.Fatalf("%s admin bootstrap failed: %v (%s)", spec.kind, err, strings.TrimSpace(string(out)))
	}

	token = createToken(t, spec.kind, baseURL, "e2e-harness")
	return baseURL, token, name
}

// createToken 用 admin 基本认证换一枚 scoped token（现场实测命令：
// POST /api/v1/users/<user>/tokens, scopes ["all"]；返回体 sha1 即 token）。
func createToken(t *testing.T, kind, baseURL, name string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	body := fmt.Sprintf(`{"name":%q,"scopes":["all"]}`, name)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		baseURL+"/api/v1/users/"+adminUser+"/tokens", strings.NewReader(body))
	if err != nil {
		t.Fatalf("token request build: %v", err)
	}
	req.SetBasicAuth(adminUser, adminPass)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s token create: %v", kind, err)
	}
	defer resp.Body.Close() //nolint:errcheck // read-side cleanup
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("%s token create: HTTP %d", kind, resp.StatusCode)
	}
	var created struct {
		SHA1 string `json:"sha1"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("%s token decode: %v", kind, err)
	}
	if created.SHA1 == "" {
		t.Fatalf("%s token create: empty sha1", kind)
	}
	return created.SHA1
}

// pollVersion 轮询 GET <base>/api/v1/version 至 200（120s 超时 1s 间隔，
// brief Step2 的就绪契约）。
func pollVersion(baseURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 3 * time.Second}
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := client.Get(baseURL + "/api/v1/version")
		if err == nil {
			resp.Body.Close() //nolint:errcheck // probe cleanup
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(readyPoll)
	}
	return fmt.Errorf("timeout after %s: %v", timeout, lastErr)
}

// dockerAvailable 探测本机 docker（docker info 退出码为准）。
func dockerAvailable() bool {
	return exec.Command("docker", "info", "--format", "{{.ServerVersion}}").Run() == nil
}

// freePort 取一个空闲端口（listen :0 后关闭；竞窗可忽略——端口随即交给 docker）。
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		t.Fatalf("freePort close: %v", err)
	}
	return port
}

// runDocker 执行 docker 子命令，失败即致命（容器编排失败没有可测面）。
func runDocker(t *testing.T, args ...string) {
	t.Helper()
	out, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("docker %s failed: %v (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
}

// logContainerTail 失败诊断（brief Step3.5）：t.Logf 容器日志尾部。
func logContainerTail(t *testing.T, name string) {
	t.Helper()
	out, err := exec.Command("docker", "logs", "--tail", "40", name).CombinedOutput()
	if err != nil {
		t.Logf("docker logs %s failed: %v", name, err)
		return
	}
	t.Logf("docker logs %s (tail):\n%s", name, strings.TrimSpace(string(out)))
}

// smokeTestConnection 是对 wire 的 TestConnection 冒烟（token 有效性快失败）。
// 仓库落位命名空间由 createRepoForTest 从 FullName 回填，不在此解析。
func smokeTestConnection(t *testing.T, p provider.Provider, kind string) {
	t.Helper()
	tc, err := p.TestConnection(t.Context())
	if err != nil {
		t.Fatalf("%s TestConnection: %v", kind, err)
	}
	if tc == nil || !tc.Connected || tc.UserName == "" {
		t.Fatalf("%s TestConnection: not connected (%+v)", kind, tc)
	}
}

// createRepoForTest 建本轮测试仓库（AutoInit 产 main 分支，CR/分支保护都要
// 它）。owner 传空串 = 个人命名空间——site-verified 于
// giteafamily.CreateRepo：owner 非空走 CreateOrgRepo（org 管线，个人名下
// 仓库必须 owner==""）。返回实际落位命名空间，t.Cleanup 尽力删净
// （一次性容器内写，删除失败仅记日志）。
func createRepoForTest(t *testing.T, p provider.Provider, owner, name string) string {
	t.Helper()
	rm, ok := p.(provider.RepoManager)
	if !ok {
		t.Fatalf("provider lacks RepoManager")
	}
	repo, err := rm.CreateRepo(t.Context(), owner, provider.CreateRepoOptions{
		Name:     name,
		AutoInit: true,
	})
	if err != nil {
		t.Fatalf("create repo (owner=%q) %s: %v", owner, name, err)
	}
	if repo.Name != name {
		t.Fatalf("create repo returned %q, want %q", repo.Name, name)
	}
	// 个人命名空间回填（owner=="" 时仓库落在 token identity 名下）。
	if owner == "" {
		if repo.FullName == "" {
			t.Fatalf("create repo %s: empty FullName, cannot resolve owner", name)
		}
		owner = strings.SplitN(repo.FullName, "/", 2)[0]
	}
	t.Cleanup(func() {
		if err := rm.DeleteRepo(t.Context(), owner, name); err != nil {
			t.Logf("cleanup delete repo %s/%s: %v", owner, name, err)
		}
	})
	return owner
}

// requireErrNotFound 断言错误分类走哨兵 errors.Is（规则二：禁止文本匹配）。
func requireErrNotFound(t *testing.T, op string, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected ErrNotFound, got nil", op)
	}
	if !provider.IsNotFound(err) {
		t.Fatalf("%s: expected ErrNotFound chain, got %v", op, err)
	}
}
