package backendutil

import (
	"io"

	"github.com/oekit/pkg/bufiox"
)

// CILogTailBytes 失败日志尾部保留量：CI 失败的根因几乎总在日志尾部
// （报错与堆栈），头部只有无关的 setup 输出。20KB 对 LLM 归因足够。
const CILogTailBytes = 20_000

// TailOfReader 读流并保留尾部 budget 字节。
// 委托 bufiox.TailLines（pkg#198 收编）：行粒度、头部按行丢弃、
// 单行超预算整行保留、\r\n 归一——语义单源在上游，此处仅保留
// CI 日志线的领域入口名。
func TailOfReader(r io.Reader, budget int) (string, bool, error) {
	return bufiox.TailLines(r, budget)
}

// IsFailedCIConclusion reports whether a CI job conclusion counts as a real
// failure worth attributing: failure / timed_out (ran and failed) and
// startup_failure (misconfigured, never ran) are attributable; canceled and
// skipped are collateral or not-executed and are not.
func IsFailedCIConclusion(c string) bool {
	switch c {
	case "failure", "timed_out", "startup_failure":
		return true
	}
	return false
}
