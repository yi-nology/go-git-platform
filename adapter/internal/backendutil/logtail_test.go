package backendutil

import (
	"strings"
	"testing"
)

func TestTailOfReader(t *testing.T) {
	// 短日志：全量保留、无截断标记
	got, trunc, err := TailOfReader(strings.NewReader("a\nb\nc"), 100)
	if err != nil || trunc || got != "a\nb\nc" {
		t.Fatalf("短日志应原样: %q trunc=%v err=%v", got, trunc, err)
	}
	// 长日志：保留尾部、标记截断
	long := strings.Repeat("x\n", 5000)
	got, trunc, err = TailOfReader(strings.NewReader(long), 100)
	if err != nil || !trunc {
		t.Fatalf("长日志应截断: trunc=%v err=%v", trunc, err)
	}
	if strings.Contains(got, strings.Repeat("x", 99)) {
		t.Fatalf("应只留尾部: len=%d", len(got))
	}
	if !strings.HasSuffix(got, "x") {
		t.Fatalf("尾部应完整: %q", got[len(got)-10:])
	}
}
