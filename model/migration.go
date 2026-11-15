package model

// MigrationInfo 迁移任务状态。json tag 与 GitHub 迁移 API 响应对齐。
type MigrationInfo struct {
	ID    int64  `json:"id"`
	State string `json:"state"`
	// ArchiveURL 归档下载地址(仅当 state 为 exported 时可用)。
	ArchiveURL string `json:"archive_url"`
}
