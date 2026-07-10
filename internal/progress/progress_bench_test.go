package progress_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/progress"
	"github.com/unsubble/searchit/internal/stats"
)

func BenchmarkTextRenderer_Render(b *testing.B) {
	c := stats.NewCollector()
	c.RecordRequestSent()
	c.RecordResponseReceived(200, 1024)
	c.SetActiveWorkers(10)
	c.SetQueuedJobs(50)
	c.RecordDiscovered()

	snap := c.Snapshot()
	r := progress.NewTextRenderer(io.Discard, "https://target.local")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.Render(snap)
	}
}

func BenchmarkANSIRenderer_Render(b *testing.B) {
	c := stats.NewCollector()
	c.RecordRequestSent()
	c.RecordResponseReceived(200, 1024)
	c.SetActiveWorkers(10)
	c.SetQueuedJobs(50)
	c.RecordDiscovered()

	snap := c.Snapshot()
	r := progress.NewANSIRenderer(io.Discard, "https://target.local")
	// Pre-populate some discoveries
	for i := 0; i < 10; i++ {
		r.AddResult(200, "https://target.local/path")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.Render(snap)
	}
}

func BenchmarkManager_Tick(b *testing.B) {
	c := stats.NewCollector()
	r := &FakeRenderer{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := progress.NewManager(c, r, 1*time.Microsecond)
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			m.Start(ctx, nil)
			close(done)
		}()
		cancel()
		<-done
	}
}
