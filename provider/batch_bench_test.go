package provider

import (
	"context"
	"fmt"
	"testing"
)

// manyItemFakeProvider does no I/O: the benchmark measures batch
// orchestration overhead (goroutine scheduling, index bookkeeping), not
// platform latency.
type manyItemFakeProvider struct{ Provider }

func (manyItemFakeProvider) Platform() Platform { return PlatformGitHub }
func (manyItemFakeProvider) GetFileContent(ctx context.Context, owner, repo, path, ref string) (string, error) {
	return "c:" + path, nil
}

func benchmarkBatch(b *testing.B, n int, conc int) {
	p := manyItemFakeProvider{}
	paths := make([]string, n)
	for i := range paths {
		paths[i] = fmt.Sprintf("f%d", i)
	}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got := GetFileContents(ctx, p, "o", "r", "", paths, BatchOptions{Concurrency: conc})
		for j, r := range got {
			if r.Err != nil || r.Value != "c:"+paths[j] {
				b.Fatalf("item %d: %+v", j, r)
			}
		}
	}
}

func BenchmarkBatch1kItemsDefaultConcurrency(b *testing.B)  { benchmarkBatch(b, 1000, 4) }
func BenchmarkBatch10kItemsDefaultConcurrency(b *testing.B) { benchmarkBatch(b, 10000, 4) }
func BenchmarkBatch10kItemsMaxConcurrency(b *testing.B)     { benchmarkBatch(b, 10000, 16) }
