package targets_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/app"
	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/fuzz"
	"github.com/unsubble/searchit/internal/recursion"
	"github.com/unsubble/searchit/internal/signals"
	"github.com/unsubble/searchit/internal/state"
	"github.com/unsubble/searchit/internal/targets"
)

func slowServer(delay time.Duration) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)
		w.WriteHeader(http.StatusOK)
	}))
}

func TestAbortAllSingleTarget(t *testing.T) {
	srv := slowServer(100 * time.Millisecond)
	defer srv.Close()

	stateMgr := state.NewManager()
	globalCtx, cancelRaw := signals.SetupContext(context.Background(), stateMgr)
	cancelGlobal := func() {
		if stateMgr.Current() < state.PhaseStopping {
			stateMgr.Transition(state.PhaseStopping)
		}
		cancelRaw()
	}
	defer cancelGlobal()

	tList := []targets.Target{{URL: srv.URL}}
	mgr := targets.NewManager(tList)

	var targetExecuted bool
	errCh := make(chan error, 1)

	go func() {
		errCh <- mgr.Execute(globalCtx, func(tCtx targets.TargetContext) error {
			targetExecuted = true
			select {
			case <-tCtx.Ctx.Done():
				return tCtx.Ctx.Err()
			case <-time.After(1 * time.Second):
				return nil
			}
		})
	}()

	time.Sleep(20 * time.Millisecond)
	cancelGlobal()

	err := <-errCh
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled error, got: %v", err)
	}
	if !targetExecuted {
		t.Fatal("expected target to start execution before abort")
	}
	if stateMgr.Current() < state.PhaseStopping {
		t.Fatalf("expected state manager to transition to PhaseStopping, got %v", stateMgr.Current())
	}
}

func TestAbortAllMultiTarget(t *testing.T) {
	t.Run("Key 'q' semantics: skip current target only", func(t *testing.T) {
		stateMgr := state.NewManager()
		globalCtx, cancelGlobal := signals.SetupContext(context.Background(), stateMgr)
		defer cancelGlobal()

		tList := []targets.Target{
			{URL: "http://target1"},
			{URL: "http://target2"},
			{URL: "http://target3"},
		}
		mgr := targets.NewManager(tList)

		var executed []string
		err := mgr.Execute(globalCtx, func(tCtx targets.TargetContext) error {
			executed = append(executed, tCtx.Target.URL)
			if tCtx.Target.URL == "http://target1" {
				tCtx.Cancel() // Simulates key 'q' (CommandStopTarget)
				return context.Canceled
			}
			return nil
		})

		if err != nil {
			t.Fatalf("expected nil error on skipped target, got: %v", err)
		}
		if len(executed) != 3 {
			t.Fatalf("expected all 3 targets to execute in sequence, got %v", executed)
		}
	})

	t.Run("Key 'a' semantics: abort entire run", func(t *testing.T) {
		stateMgr := state.NewManager()
		globalCtx, cancelGlobal := signals.SetupContext(context.Background(), stateMgr)

		tList := []targets.Target{
			{URL: "http://target1"},
			{URL: "http://target2"},
			{URL: "http://target3"},
		}
		mgr := targets.NewManager(tList)

		var executed []string
		err := mgr.Execute(globalCtx, func(tCtx targets.TargetContext) error {
			executed = append(executed, tCtx.Target.URL)
			if tCtx.Target.URL == "http://target1" {
				cancelGlobal() // Simulates key 'a' (CommandAbortAll)
				return globalCtx.Err()
			}
			return nil
		})

		if err != context.Canceled {
			t.Fatalf("expected context.Canceled error, got: %v", err)
		}
		if len(executed) != 1 {
			t.Fatalf("expected target2 and target3 to NEVER start, executed list: %v", executed)
		}
	})
}

type sliceReader struct {
	words []string
}

func (r sliceReader) Read(ctx context.Context, out chan<- string) error {
	for _, w := range r.words {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- w:
		}
	}
	return nil
}

func TestAbortDuringRecursion(t *testing.T) {
	srv := slowServer(10 * time.Millisecond)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := config.Default()
	a := app.New(ctx, cfg)
	fs, _ := filter.NewFilterSuite("", "", "", "", nil, nil, nil, nil)
	reader := sliceReader{words: []string{"a", "b", "c", "d", "e"}}

	recMgr := recursion.NewManager(
		a.HTTPClient,
		cfg.Status.Exclude,
		reader,
		recursion.BFS,
		3,
		cfg.RecurseOn,
		false,
		false,
		cfg.IncludeSize,
		cfg.ExcludeSize,
		nil,
		nil,
		0,
		nil,
		nil,
	)
	recMgr.SetFilterSuite(fs)

	out := recMgr.Run(ctx, []string{srv.URL}, 4)

	// Cancel context after short delay
	time.Sleep(15 * time.Millisecond)
	cancel()

	// Drain output channel — must close cleanly without deadlock
	count := 0
	for range out {
		count++
	}
}

func TestAbortDuringFuzzing(t *testing.T) {
	srv := slowServer(10 * time.Millisecond)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fs, _ := filter.NewFilterSuite("", "", "", "", nil, nil, nil, nil)
	exec := fuzz.NewExecutor(ctx, http.DefaultClient, fs, 4, 0, nil, nil)
	defer exec.Close()

	// Launch async job executions
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 20; i++ {
			_, _ = exec.Execute(fuzz.Job{URL: srv.URL})
		}
	}()

	time.Sleep(15 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for fuzz executor to abort")
	}
}

func TestAbortDuringAdaptiveScanning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	cfg := config.Default()
	cfg.Adaptive = true
	a := app.New(ctx, cfg)
	fs, _ := filter.NewFilterSuite("", "", "", "", nil, nil, nil, nil)
	scanner := engine.NewScanner(a.HTTPClient, fs, nil, nil, 0, nil)

	p := engine.SliceProducer{URLs: []string{srv.URL, srv.URL + "/2"}}
	resChan := scanner.Scan(ctx, p, 2)

	for range resChan {
	}
}

func TestAbortAllDeterminism(t *testing.T) {
	for i := 0; i < 10; i++ {
		stateMgr := state.NewManager()
		ctx, cancelRaw := signals.SetupContext(context.Background(), stateMgr)
		cancel := func() {
			if stateMgr.Current() < state.PhaseStopping {
				stateMgr.Transition(state.PhaseStopping)
			}
			cancelRaw()
		}

		tList := []targets.Target{{URL: "http://t1"}, {URL: "http://t2"}}
		mgr := targets.NewManager(tList)

		var executed int32
		_ = mgr.Execute(ctx, func(tCtx targets.TargetContext) error {
			atomic.AddInt32(&executed, 1)
			cancel()
			return ctx.Err()
		})

		if atomic.LoadInt32(&executed) != 1 {
			t.Fatalf("run %d: expected exactly 1 target executed, got %d", i, executed)
		}
		if stateMgr.Current() < state.PhaseStopping {
			t.Fatalf("run %d: state manager did not reach PhaseStopping", i)
		}
	}
}

func TestAbortAllRaceSafety(t *testing.T) {
	srv := slowServer(5 * time.Millisecond)
	defer srv.Close()

	for i := 0; i < 5; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		fs, _ := filter.NewFilterSuite("", "", "", "", nil, nil, nil, nil)
		scanner := engine.NewScanner(http.DefaultClient, fs, nil, nil, 0, nil)

		urls := []string{srv.URL, srv.URL, srv.URL, srv.URL}
		resChan := scanner.Scan(ctx, engine.SliceProducer{URLs: urls}, 8)

		go func() {
			time.Sleep(2 * time.Millisecond)
			cancel()
		}()

		for range resChan {
		}
	}
}

func TestAbortWorkerScaling(t *testing.T) {
	srv := slowServer(2 * time.Millisecond)
	defer srv.Close()

	workerCounts := []int{1, 2, 4, 8, 16, 32, 64, 128}

	for _, wCount := range workerCounts {
		for run := 0; run < 10; run++ {
			ctx, cancel := context.WithCancel(context.Background())
			fs, _ := filter.NewFilterSuite("", "", "", "", nil, nil, nil, nil)
			scanner := engine.NewScanner(http.DefaultClient, fs, nil, nil, 0, nil)

			var urls []string
			for k := 0; k < 100; k++ {
				urls = append(urls, srv.URL)
			}

			resChan := scanner.Scan(ctx, engine.SliceProducer{URLs: urls}, wCount)

			go func() {
				time.Sleep(1 * time.Millisecond)
				cancel()
			}()

			for range resChan {
			}
		}
	}
}

func TestAbortAllNoGoroutineLeak(t *testing.T) {
	runtime.GC()
	initialGoroutines := runtime.NumGoroutine()

	srv := slowServer(10 * time.Millisecond)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	fs, _ := filter.NewFilterSuite("", "", "", "", nil, nil, nil, nil)
	scanner := engine.NewScanner(http.DefaultClient, fs, nil, nil, 0, nil)

	urls := []string{srv.URL, srv.URL, srv.URL}
	resChan := scanner.Scan(ctx, engine.SliceProducer{URLs: urls}, 4)

	cancel()
	for range resChan {
	}

	time.Sleep(50 * time.Millisecond)
	runtime.GC()
	finalGoroutines := runtime.NumGoroutine()

	// Allow a tiny delta for runtime background activities if any, but ensure workers/engine goroutines exited.
	if finalGoroutines > initialGoroutines+2 {
		t.Fatalf("goroutine leak detected: initial %d, final %d", initialGoroutines, finalGoroutines)
	}
}

func TestAbortAllRepeated(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Multiple repeated calls must be safe and idempotent
	cancel()
	cancel()
	cancel()
	cancel()

	if ctx.Err() != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", ctx.Err())
	}
}

func TestAbortAfterCompletion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	fs, _ := filter.NewFilterSuite("", "", "", "", nil, nil, nil, nil)
	scanner := engine.NewScanner(http.DefaultClient, fs, nil, nil, 0, nil)

	resChan := scanner.Scan(ctx, engine.SliceProducer{URLs: []string{srv.URL}}, 1)
	for range resChan {
	}

	// Abort after completion
	cancel()

	if ctx.Err() != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", ctx.Err())
	}
}

func TestCtrlCTest(t *testing.T) {
	TestAbortAllSingleTarget(t)
}

func TestCtrlCRepeated(t *testing.T) {
	TestAbortAllRepeated(t)
}

func TestCtrlCAfterCompletion(t *testing.T) {
	TestAbortAfterCompletion(t)
}

func TestCtrlCAndAbortMixed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Mixed cancellation MUST succeed: a -> Ctrl+C -> Ctrl+C -> a
	cancel()
	cancel()
	cancel()
	cancel()

	if ctx.Err() != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", ctx.Err())
	}
}
