package cmd_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unsubble/searchit/cmd"
)

func verifyCLINoGoroutineLeak(t *testing.T, initial int, client *http.Client, srv *httptest.Server, label string) {
	t.Helper()
	if srv != nil {
		srv.Close()
	}
	if client != nil {
		if tr, ok := client.Transport.(*http.Transport); ok {
			tr.CloseIdleConnections()
		}
	}
	http.DefaultTransport.(*http.Transport).CloseIdleConnections()

	deadline := time.Now().Add(1 * time.Second)
	var final int
	for time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(30 * time.Millisecond)
		final = runtime.NumGoroutine()
		if final <= initial+3 {
			return
		}
	}

	buf := make([]byte, 2*1024*1024)
	n := runtime.Stack(buf, true)
	t.Fatalf("[%s] goroutine leak detected in CLI: started with %d, ended with %d\n%s", label, initial, final, string(buf[:n]))
}

func TestFuzzCLI_Adaptive_NormalCompletion(t *testing.T) {
	initialGoroutines := runtime.NumGoroutine()

	var reqCount int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&reqCount, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "words.txt")
	_ = os.WriteFile(wlPath, []byte("admin\nlogin\napi\n"), 0644)

	stdout, stderr, err := executeFuzzCmd([]string{
		"-u", srv.URL + "/FUZZ",
		"-w", wlPath,
		"--adaptive",
		"-q",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr)
	}

	if atomic.LoadInt64(&reqCount) < 3 {
		t.Errorf("expected at least 3 requests, got %d", reqCount)
	}
	_ = stdout

	verifyCLINoGoroutineLeak(t, initialGoroutines, srv.Client(), srv, "CLI_Adaptive_NormalCompletion")
}

func TestFuzzCLI_Adaptive_ContextCancellation(t *testing.T) {
	initialGoroutines := runtime.NumGoroutine()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))

	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "words.txt")
	content := ""
	for i := 0; i < 50; i++ {
		content += "word\n"
	}
	_ = os.WriteFile(wlPath, []byte(content), 0644)

	ctx, cancel := context.WithCancel(context.Background())
	fuzzCmd, _ := cmd.NewFuzzCmd()
	fuzzCmd.SetArgs([]string{
		"-u", srv.URL + "/FUZZ",
		"-w", wlPath,
		"--adaptive",
		"-q",
		"-t", "2",
	})

	done := make(chan error, 1)
	go func() {
		done <- fuzzCmd.ExecuteContext(ctx)
	}()

	time.Sleep(80 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("ExecuteContext timed out after cancellation")
	}

	verifyCLINoGoroutineLeak(t, initialGoroutines, srv.Client(), srv, "CLI_Adaptive_ContextCancellation")
}

func TestFuzzCLI_Adaptive_EmptyWordlist(t *testing.T) {
	initialGoroutines := runtime.NumGoroutine()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "empty.txt")
	_ = os.WriteFile(wlPath, []byte(""), 0644)

	_, stderr, err := executeFuzzCmd([]string{
		"-u", srv.URL + "/FUZZ",
		"-w", wlPath,
		"--adaptive",
		"-q",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr)
	}

	verifyCLINoGoroutineLeak(t, initialGoroutines, srv.Client(), srv, "CLI_Adaptive_EmptyWordlist")
}

func TestFuzzCLI_Adaptive_ConcurrencyScaling(t *testing.T) {
	for _, tc := range []int{1, 4, 16, 64} {
		t.Run(fmt.Sprintf("threads_%d", tc), func(t *testing.T) {
			var inFlight int64
			var maxInFlight int64
			barrier := make(chan struct{})
			var releaseOnce sync.Once

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/robots.txt" || r.URL.Path == "/sitemap.xml" {
					w.WriteHeader(http.StatusNotFound)
					return
				}

				cur := atomic.AddInt64(&inFlight, 1)
				for {
					old := atomic.LoadInt64(&maxInFlight)
					if cur <= old || atomic.CompareAndSwapInt64(&maxInFlight, old, cur) {
						break
					}
				}

				if cur >= int64(tc) {
					releaseOnce.Do(func() {
						close(barrier)
					})
				}

				select {
				case <-barrier:
				case <-time.After(3 * time.Second):
					releaseOnce.Do(func() {
						close(barrier)
					})
				}

				atomic.AddInt64(&inFlight, -1)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ok"))
			}))
			defer srv.Close()
			defer srv.Client().CloseIdleConnections()

			tmpDir := t.TempDir()
			wlPath := filepath.Join(tmpDir, "words.txt")
			f, err := os.Create(wlPath)
			if err != nil {
				t.Fatal(err)
			}
			numWords := tc * 3
			if numWords < 10 {
				numWords = 10
			}
			for i := 0; i < numWords; i++ {
				fmt.Fprintf(f, "word%d\n", i)
			}
			_ = f.Close()

			_, _, err = executeFuzzCmd([]string{
				"-u", srv.URL + "/FUZZ",
				"-w", wlPath,
				"--adaptive",
				"-t", fmt.Sprintf("%d", tc),
				"-q",
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			measured := atomic.LoadInt64(&maxInFlight)
			if measured < int64(tc) {
				t.Errorf("expected at least %d concurrent requests, got %d", tc, measured)
			}
		})
	}
}
