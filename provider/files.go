package provider

import (
	"encoding/base64"
	"strings"

	oekiterrors "github.com/oekit/pkg/errors"
)

// DecodeFileContent 按平台返回的 Encoding 解码文件内容。
//
// 平台的「取文件内容」端点有两种形态:raw(直接给字节)与 JSON(base64 字段)。
// **不按 Encoding 解码 ⇒ 调用方拿到 base64 串** —— 按 GitHub 直觉读文件会全错
// (解析 YAML/JSON 直接失败),而调用方无从察觉。
//
// 只认 `base64`(大小写不敏感);其它 encoding 原样返回。
func DecodeFileContent(platform Platform, encoding, content string) (string, error) {
	if !strings.EqualFold(strings.TrimSpace(encoding), "base64") {
		return content, nil
	}
	cleaned := strings.NewReplacer("\n", "", "\r", "").Replace(content)
	decoded, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil {
		return "", Wrap(platform, "DecodeFileContent", oekiterrors.Wrap(err, "decode base64 content"))
	}
	return string(decoded), nil
}
