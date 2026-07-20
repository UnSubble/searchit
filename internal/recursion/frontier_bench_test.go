package recursion

import (
	"testing"

	"github.com/unsubble/searchit/internal/engine"
)

const (
	smallSize  = 1_000
	mediumSize = 10_000
)

func benchmarkBatch(b *testing.B, strategy Strategy, size int) {
	b.Helper()
	b.ReportAllocs()

	job := engine.Job{
		URL: "https://example.com/admin",
	}

	for i := 0; i < b.N; i++ {
		f := NewFrontierWithCapacity(strategy, size)

		for i := 0; i < size; i++ {
			job.Depth = uint16(i % 10)
			f.Push(job)
		}

		for i := 0; i < size; i++ {
			if _, ok := f.Pop(); !ok {
				b.Fatal("frontier became empty unexpectedly")
			}
		}
	}
}

func benchmarkMixed(b *testing.B, strategy Strategy, size int) {
	b.Helper()
	b.ReportAllocs()

	job := engine.Job{
		URL: "https://example.com/admin",
	}

	for i := 0; i < b.N; i++ {
		f := NewFrontierWithCapacity(strategy, size)

		for i := 0; i < size; i++ {
			job.Depth = uint16(i % 10)
			f.Push(job)

			if i%2 == 0 {
				if _, ok := f.Pop(); !ok {
					b.Fatal("frontier became empty unexpectedly")
				}
			}
		}

		for f.Len() > 0 {
			f.Pop()
		}
	}
}

func benchmarkGrow(b *testing.B, strategy Strategy) {
	b.Helper()
	b.ReportAllocs()

	job := engine.Job{
		URL: "https://example.com/admin",
	}

	size := DefaultJobBuffer * 4

	for i := 0; i < b.N; i++ {
		f := NewFrontierWithCapacity(strategy, size)

		for i := 0; i < size; i++ {
			job.Depth = uint16(i)
			f.Push(job)
		}

		for f.Len() > 0 {
			f.Pop()
		}
	}
}

func BenchmarkFrontier_BFS_1K(b *testing.B) {
	benchmarkBatch(b, BFS, smallSize)
}

func BenchmarkFrontier_BFS_10K(b *testing.B) {
	benchmarkBatch(b, BFS, mediumSize)
}

func BenchmarkFrontier_DFS_1K(b *testing.B) {
	benchmarkBatch(b, DFS, smallSize)
}

func BenchmarkFrontier_DFS_10K(b *testing.B) {
	benchmarkBatch(b, DFS, mediumSize)
}

func BenchmarkFrontier_BFS_Mixed_10K(b *testing.B) {
	benchmarkMixed(b, BFS, mediumSize)
}

func BenchmarkFrontier_DFS_Mixed_10K(b *testing.B) {
	benchmarkMixed(b, DFS, mediumSize)
}

func BenchmarkFrontier_BFS_Grow(b *testing.B) {
	benchmarkGrow(b, BFS)
}

func BenchmarkFrontier_DFS_Grow(b *testing.B) {
	benchmarkGrow(b, DFS)
}

func TestFrontier_GrowPreservesBFSOrder(t *testing.T) {
	f := NewFrontier(BFS)

	n := DefaultJobBuffer * 3

	for i := 0; i < n; i++ {
		f.Push(engine.Job{
			Depth: uint16(i),
		})
	}

	for i := 0; i < n; i++ {
		job, ok := f.Pop()
		if !ok {
			t.Fatal("unexpected empty frontier")
		}

		if job.Depth != uint16(i) {
			t.Fatalf("got %d, want %d", job.Depth, i)
		}
	}
}

func TestFrontier_GrowPreservesDFSOrder(t *testing.T) {
	f := NewFrontier(DFS)

	n := DefaultJobBuffer * 3

	for i := 0; i < n; i++ {
		f.Push(engine.Job{
			Depth: uint16(i),
		})
	}

	for i := n - 1; i >= 0; i-- {
		job, ok := f.Pop()
		if !ok {
			t.Fatal("unexpected empty frontier")
		}

		if job.Depth != uint16(i) {
			t.Fatalf("got %d, want %d", job.Depth, i)
		}
	}
}
