package model

// ReleaseAsset Release 附件元数据。json tag 与 GitHub API 对齐,与消费方
// (git-sync-service githubapi.ReleaseAsset)字段保持兼容,Size 放大到 int64。
type ReleaseAsset struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	ContentType        string `json:"content_type"`
	BrowserDownloadURL string `json:"browser_download_url"`
	URL                string `json:"url"`
}
