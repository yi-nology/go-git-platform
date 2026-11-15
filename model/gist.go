package model

import (
	"time"
)

// GistFile 单个 gist 文件的元数据与内容。json tag 与 GitHub /gists 响应对齐,
// 与消费方(git-sync-service githubapi.Gist 内嵌结构)字段保持兼容。
type GistFile struct {
	Filename string `json:"filename"`
	Language string `json:"language"`
	RawURL   string `json:"raw_url"`
	Size     int64  `json:"size"`
	Content  string `json:"content"`
}

// Gist GitHub Gist 元数据。json tag 与 GitHub API 对齐,便于消费方直接
// 序列化落盘备份(下游 BackupGists 即整体 json.MarshalIndent)。
type Gist struct {
	ID          string              `json:"id"`
	Description string              `json:"description"`
	Public      bool                `json:"public"`
	HTMLURL     string              `json:"html_url"`
	CreatedAt   time.Time           `json:"created_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
	Files       map[string]GistFile `json:"files"`
}
