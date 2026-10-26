package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDecodeFileContent 按平台的 Encoding 解码文件内容。
//
// 回归背景(P1):平台的"取内容"端点有两种形态 —— raw(直接给字节)与 JSON(base64 字段)。
// **不按 Encoding 解码 ⇒ 调用方拿到 base64 串**:按 GitHub 直觉读文件全错
// (解析 YAML/JSON 直接失败),而调用方无从察觉。
//
// 这条用例是**反向验证**:把解码分支去掉,本用例会变红。
func TestDecodeFileContent(t *testing.T) {
	// base64（GitHub/Gitee/GitCode/工蜂 的 JSON 形态）
	got, err := DecodeFileContent(PlatformGitee, "base64", "aGVsbG8K")
	require.NoError(t, err)
	assert.Equal(t, "hello\n", got, "base64 必须解码成真实内容")

	// base64 内容里的换行（平台会按 60 字符折行）
	got2, err := DecodeFileContent(PlatformGitCode, "base64", "aGVsbG8K\nd29ybGQ=\n")
	require.NoError(t, err)
	assert.Equal(t, "hello\nworld", got2, "折行的 base64 也要能解")

	// 大小写不敏感
	got3, err := DecodeFileContent(PlatformGitee, "BASE64", "aGVsbG8K")
	require.NoError(t, err)
	assert.Equal(t, "hello\n", got3)

	// 非 base64（raw 端点）⇒ 原样返回
	raw, err := DecodeFileContent(PlatformGitLab, "", "plain content")
	require.NoError(t, err)
	assert.Equal(t, "plain content", raw)

	// 非法 base64 ⇒ 明确报错（不能静默把 base64 串当内容返回）
	_, err = DecodeFileContent(PlatformGitee, "base64", "!!!not-base64!!!")
	require.Error(t, err, "非法 base64 必须报错,不能静默返回原文")
}
