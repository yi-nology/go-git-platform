package gitbackend

import (
	"context"
	"strings"
)

// --- Remote operations ---

func (b *NativeGitBackend) AddRemote(ctx context.Context, repoPath, name, url string) error {
	if err := checkNameArgs("AddRemote", name, url); err != nil {
		return err
	}
	_, stderr, err := b.runGit(ctx, repoPath, []string{"remote", "add", "--", name, url}, AuthConfig{})
	if err != nil {
		return newGitError("AddRemote", repoPath, stderr, err)
	}
	return nil
}

func (b *NativeGitBackend) RemoveRemote(ctx context.Context, repoPath, name string) error {
	if err := checkNameArg("RemoveRemote", name); err != nil {
		return err
	}
	_, stderr, err := b.runGit(ctx, repoPath, []string{"remote", "remove", "--", name}, AuthConfig{})
	if err != nil {
		return newGitError("RemoveRemote", repoPath, stderr, err)
	}
	return nil
}

func (b *NativeGitBackend) GetRemoteURL(ctx context.Context, repoPath, name string) (string, error) {
	if err := checkNameArg("GetRemoteURL", name); err != nil {
		return "", err
	}
	stdout, stderr, err := b.runGit(ctx, repoPath, []string{"remote", "get-url", "--", name}, AuthConfig{})
	if err != nil {
		return "", newGitError("GetRemoteURL", repoPath, stderr, err)
	}
	return strings.TrimSpace(stdout), nil
}

func (b *NativeGitBackend) GetRemotes(ctx context.Context, repoPath string) ([]string, error) {
	stdout, stderr, err := b.runGit(ctx, repoPath, []string{"remote"}, AuthConfig{})
	if err != nil {
		return nil, newGitError("GetRemotes", repoPath, stderr, err)
	}
	var names []string
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			names = append(names, line)
		}
	}
	return names, nil
}

// --- Tag operations ---

func (b *NativeGitBackend) CreateTag(ctx context.Context, repoPath, name, ref string) error {
	if err := checkNameArgs("CreateTag", name, ref); err != nil {
		return err
	}
	// `--` 让 name/ref 按位置解析：name="-d"/ref="-f" 旧实现会反转成
	// 删/改语义（issue #109）
	args := []string{"tag", "--", name}
	if ref != "" {
		args = append(args, ref)
	}
	_, stderr, err := b.runGit(ctx, repoPath, args, AuthConfig{})
	if err != nil {
		if strings.Contains(stderr, "already exists") {
			return newGitError("CreateTag", repoPath, stderr, ErrTagExists)
		}
		return newGitError("CreateTag", repoPath, stderr, err)
	}
	return nil
}

func (b *NativeGitBackend) DeleteTag(ctx context.Context, repoPath, name string) error {
	if err := checkNameArg("DeleteTag", name); err != nil {
		return err
	}
	_, stderr, err := b.runGit(ctx, repoPath, []string{"tag", "-d", "--", name}, AuthConfig{})
	if err != nil {
		return newGitError("DeleteTag", repoPath, stderr, err)
	}
	return nil
}

func (b *NativeGitBackend) PushTag(ctx context.Context, repoPath, remote, name string, auth AuthConfig) error {
	// refspec 带 refs/tags/ 前缀不会被解析成 flag，remote 校验即可
	if err := checkNameArg("PushTag", remote); err != nil {
		return err
	}
	_, stderr, err := b.runGit(ctx, repoPath, []string{"push", remote, "refs/tags/" + name}, auth)
	if err != nil {
		return newGitError("PushTag", repoPath, stderr, err)
	}
	return nil
}

func (b *NativeGitBackend) GetTagList(ctx context.Context, repoPath string) ([]TagInfo, error) {
	// 记录以 %00（NUL）开头、字段 NUL 分隔、记录之间是 for-each-ref 默认
	// 插入的 "\n"（落在末位 %(contents) 里，随其尾部换行一起 TrimRight）。
	// for-each-ref 的 %xx 转义支持内嵌 NUL（git for-each-ref(1)：
	// "%xx where xx are hex digits"），旧实现「不支持 %x00」的注释弄混了
	// 语法（%x00 四位形态确实不存在，%00 两位十六进制形态可用）。
	// %(contents) 是完整消息（annotated = tag 消息全文，lightweight =
	// commit 消息全文）——旧实现 %(subject) 只有主题行，与 gogit 后端
	// 的全文口径分裂（issue #103）。消息置于记录末位，任意文本整体吸收。
	// Author 字段：annotated tag 的身份在 tagger（%(authorname) 恒空），
	// lightweight tag 在 commit author——条件格式取先命中者。
	stdout, stderr, err := b.runGit(ctx, repoPath, []string{
		"for-each-ref", "--format=%00%(refname:short)%00%(objectname)%00%(if)%(taggername)%(then)%(taggername)%(else)%(authorname)%(end)%00%(contents)",
		"refs/tags/",
	}, AuthConfig{})
	if err != nil {
		return nil, newGitError("GetTagList", repoPath, stderr, err)
	}

	fields := strings.Split(stdout, "\x00")
	var tags []TagInfo
	for i := 1; i+3 < len(fields); i += 4 {
		tags = append(tags, TagInfo{
			Name:    fields[i],
			Hash:    fields[i+1],
			Author:  fields[i+2],
			Message: strings.TrimRight(fields[i+3], "\n"),
		})
	}
	return tags, nil
}
