package stats_test

import (
	"testing"

	"github.com/unsubble/searchit/internal/stats"
)

func BenchmarkCollector_Increment(b *testing.B) {
	c := stats.NewCollector()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.RecordRequestSent()
	}
}

func BenchmarkCollector_StatusUpdate(b *testing.B) {
	c := stats.NewCollector()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.RecordResponseReceived(200, 128)
	}
}

func BenchmarkCollector_Snapshot(b *testing.B) {
	c := stats.NewCollector()
	c.RecordRequestSent()
	c.RecordResponseReceived(200, 1024)
	c.RecordResponseReceived(404, 512)
	c.RecordResponseReceived(302, 256)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Snapshot()
	}
}
