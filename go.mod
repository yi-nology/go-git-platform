module github.com/yi-nology/go-git-platform

go 1.27.3

require (
	codeberg.org/mvdkleijn/forgejo-sdk/forgejo/v3 v3.0.0
	gitea.dev/sdk v1.5.1
	github.com/go-git/go-git/v5 v5.19.3
	github.com/google/go-github/v93 v93.0.0
	github.com/next-bin/go-gitee v0.0.0-20260521141902-36ce436eee18
	github.com/oekit/concurrency v0.33.0
	github.com/oekit/logx v0.9.2
	github.com/oekit/pkg v0.44.0
	github.com/stretchr/testify v1.12.1
	// pinned v0.15.0：v0.16.0 起 SDK 内建路径段转义（其 issue #6，pathf 统一收口），
	// 与本仓 #84 后置补丁的预转义（backendutil.Esc / escPath）叠加成双重转义
	// （wire 里 owner/repo 变 %2523，escape_wire_test 当场抓红）。撤预转义需逐点
	// 区分 path/query 两面（gitcode 后端 84 处 Esc 调用点），属专门收编批，不随
	// 依赖地板批量升。升级前先撤 Esc 预转义并核对 ref 等查询参数的转义口径。
	github.com/yi-nology/go-gitcode v0.15.0
	github.com/yi-nology/go-gongfeng v0.3.1
	gitlab.com/gitlab-org/api/client-go/v3 v3.47.0
	golang.org/x/crypto v0.58.1-0.20261009181718-2f1f834330b2
	golang.org/x/oauth2 v0.38.0
	golang.org/x/time v0.16.0
)

require (
	dario.cat/mergo v1.0.2 // indirect
	github.com/42wim/httpsig v1.3.0 // indirect
	github.com/Microsoft/go-winio v0.6.4 // indirect
	github.com/ProtonMail/go-crypto v1.5.2 // indirect
	github.com/cloudflare/circl v1.6.5 // indirect
	github.com/cyphar/filepath-securejoin v0.7.0 // indirect
	github.com/davidmz/go-pageant v1.0.2 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/go-fed/httpsig v1.1.0 // indirect
	github.com/go-git/gcfg v1.5.1-0.20230307220236-3a3c6141e376 // indirect
	github.com/go-git/go-billy/v5 v5.9.2 // indirect
	github.com/go-openapi/errors v0.22.9 // indirect
	github.com/go-openapi/jsonpointer v1.0.2 // indirect
	github.com/go-openapi/strfmt v0.27.4 // indirect
	github.com/go-openapi/swag v0.29.2 // indirect
	github.com/go-openapi/swag/cmdutils v0.29.2 // indirect
	github.com/go-openapi/swag/conv v0.29.2 // indirect
	github.com/go-openapi/swag/fileutils v0.29.2 // indirect
	github.com/go-openapi/swag/jsonutils v0.29.2 // indirect
	github.com/go-openapi/swag/loading v0.29.2 // indirect
	github.com/go-openapi/swag/mangling v0.29.2 // indirect
	github.com/go-openapi/swag/netutils v0.29.2 // indirect
	github.com/go-openapi/swag/pools v0.29.2 // indirect
	github.com/go-openapi/swag/stringutils v0.29.2 // indirect
	github.com/go-openapi/swag/typeutils v0.29.2 // indirect
	github.com/go-openapi/swag/yamlutils v0.29.2 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8 // indirect
	github.com/google/go-querystring v1.2.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.8 // indirect
	github.com/hashicorp/go-version v1.9.0 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/kevinburke/ssh_config v1.6.0 // indirect
	github.com/oklog/ulid/v2 v2.1.2 // indirect
	github.com/pjbgf/sha1cd v0.7.0 // indirect
	github.com/sergi/go-diff v1.4.0 // indirect
	github.com/skeema/knownhosts v1.3.3 // indirect
	github.com/xanzy/ssh-agent v0.3.3 // indirect
	go.yaml.in/yaml/v3 v3.0.6 // indirect
	golang.org/x/net v0.62.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
	golang.org/x/text v0.44.0 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
)
