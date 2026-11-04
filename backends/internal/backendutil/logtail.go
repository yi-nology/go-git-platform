package backendutil

import (
	"bufio"
	"io"
)

// CILogTailBytes 失败日志尾部保留量：CI 失败的根因几乎总在日志尾部
// （报错与堆栈），头部只有无关的 setup 输出。20KB 对 LLM 归因足够。
const CILogTailBytes = 20_000

// TailOfReader 读流并保留尾部 budget 字节（行粒度扫描避免整文驻留内存；
// CI trace 可能数十 MB）。GitLab trace / GitHub presigned job log /
// Gitea job log 三条 CI 日志线的共享尾部截断。
func TailOfReader(r io.Reader, budget int) (string, bool, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	var lines []string
	total := 0
	for sc.Scan() {
		lines = append(lines, sc.Text())
		total += len(sc.Text()) + 1
		for total > budget*4 { // 行数软上限：超 4 倍预算从头部丢弃
			total -= len(lines[0]) + 1
			lines = lines[1:]
		}
	}
	if err := sc.Err(); err != nil {
		return "", false, err
	}
	joined := ""
	for i := len(lines) - 1; i >= 0; i-- {
		if joined == "" {
			joined = lines[i]
			continue
		}
		if len(joined)+len(lines[i])+1 > budget {
			return joined, true, nil
		}
		joined = lines[i] + "\n" + joined
	}
	return joined, false, nil
}
