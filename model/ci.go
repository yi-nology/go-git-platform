package model

// JobLog is one failed CI job's identifying metadata plus the tail of its
// execution log, sized for LLM consumption rather than archival.
type JobLog struct {
	Name      string `json:"name"`
	Stage     string `json:"stage"`
	WebURL    string `json:"web_url,omitempty"`
	Trace     string `json:"trace"` // 失败日志尾部（后端截断，量级 ~20KB）
	Truncated bool   `json:"truncated"`
}
