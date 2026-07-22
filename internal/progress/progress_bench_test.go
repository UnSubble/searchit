package progress_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/output/terminal"
	"github.com/unsubble/searchit/internal/progress"
	"github.com/unsubble/searchit/internal/stats"
)

func BenchmarkANSIRenderer_Render(b *testing.B) {
	c := stats.NewCollector()
	c.RecordRequestSent()
	c.RecordResponseReceived(200, 1024)
	c.SetActiveWorkers(10)
	c.SetQueuedJobs(50)
	c.RecordDiscovered()

	snap := c.Snapshot()
	tm := terminal.New(io.Discard)
	_ = tm.AcquireOwner(terminal.OwnerProgress)
	r := progress.NewANSIRenderer(tm, "https://target.local", nil, "Single target")
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
	tm := terminal.New(io.Discard)
	_ = tm.AcquireOwner(terminal.OwnerProgress)
	r := progress.NewANSIRenderer(tm, "https://target.local", nil, "Single target")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := progress.NewManager(tm, c, r, 1*time.Microsecond)
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
