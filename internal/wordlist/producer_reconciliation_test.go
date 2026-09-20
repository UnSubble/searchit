package wordlist_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/stats"
	"github.com/unsubble/searchit/internal/wordlist"
)

type sliceReader struct {
	words []string
}

func (s sliceReader) Read(ctx context.Context, out chan<- string) error {
	for _, w := range s.words {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- w:
		}
	}
	return nil
}

func TestProducerPipelineReconciliation(t *testing.T) {
	stats.GlobalInstrumentation.Reset()
	atomic.StoreInt32(&stats.GlobalInstrumentation.Enabled, 1)
	defer stats.GlobalInstrumentation.Reset()

	collector := stats.NewCollector()
	collector.SetIsFinite(true)

	p := wordlist.Producer{
		BaseURL:   "http://example.com",
		Reader:    sliceReader{words: []string{"admin", "login", "dashboard"}},
		Collector: collector,
	}

	jobs := make(chan engine.Job, 10)
	var count int
	done := make(chan struct{})
	go func() {
		for range jobs {
			count++
		}
		close(done)
	}()

	if err := p.Produce(context.Background(), jobs); err != nil {
		t.Fatalf("producer failed: %v", err)
	}
	<-done

	if count != 3 {
		t.Fatalf("expected 3 jobs, got %d", count)
	}

	jobsProd := atomic.LoadInt64(&stats.GlobalInstrumentation.JobsProduced)
	jobsSub := atomic.LoadInt64(&stats.GlobalInstrumentation.JobsSubmitted)

	if jobsProd != 3 {
		t.Errorf("expected JobsProduced = 3, got %d", jobsProd)
	}
	if jobsSub != 3 {
		t.Errorf("expected JobsSubmitted = 3, got %d", jobsSub)
	}
	if jobsProd != jobsSub {
		t.Errorf("pipeline mismatch: JobsProduced (%d) != JobsSubmitted (%d)", jobsProd, jobsSub)
	}
}
