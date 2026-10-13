package tencentcode

import (
	"errors"

	gongfeng "github.com/studyzy/gongfeng-sdk-go"
	"github.com/yi-nology/go-git-platform/provider"
)

// init registers TencentCODE's SDK error extractor: gongfeng's single
// *ErrorResponse exposes the status only through Response *http.Response.
func init() {
	provider.RegisterStatusExtractor(provider.PlatformTencentCode, func(err error) (int, bool) {
		var er *gongfeng.ErrorResponse
		if errors.As(err, &er) && er.Response != nil {
			return er.Response.StatusCode, true
		}
		return 0, false
	})
}
