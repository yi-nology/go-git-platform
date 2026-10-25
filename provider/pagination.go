package provider

import (
	"net/http"
	"strconv"

	"github.com/oekit/pkg/pagination"
)

const (
	DefaultPage    = 1
	DefaultPerPage = 20
	MaxPerPage     = 100
)

// pagePolicy 本仓分页口径：默认每页 DefaultPerPage、上限 MaxPerPage、超限钳到上限。
// 口径数字仍由本包的导出常量给出（对外 API 不变），归一化逻辑单源于
// oekit/pkg/pagination.Policy。
var pagePolicy = pagination.Policy{
	DefaultPageSize: DefaultPerPage,
	MaxPageSize:     MaxPerPage,
}

// NormalizePageOpts applies default values for page/perPage.
func NormalizePageOpts(page, perPage int) (int, int) {
	return pagePolicy.Normalize(page, perPage)
}

// ParseTotalCountHeader reads X-Total-Count or X-Total from response headers.
// Falls back to the provided default if neither header is present or valid.
func ParseTotalCountHeader(headers http.Header, fallback int) int {
	for _, key := range []string{"X-Total-Count", "X-Total"} {
		if v := headers.Get(key); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				return n
			}
		}
	}
	return fallback
}
