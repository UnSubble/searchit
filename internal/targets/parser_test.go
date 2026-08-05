package targets_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/unsubble/searchit/internal/stats"
	"github.com/unsubble/searchit/internal/targets"
)

func TestParse_TableDriven(t *testing.T) {
	tmpDir := t.TempDir()
	urlFilePath := filepath.Join(tmpDir, "urls.txt")
	err := os.WriteFile(urlFilePath, []byte("http://target1.local\n\n# comment\nhttp://target2.local\n"), 0644)
	if err != nil {
		t.Fatalf("failed to create temp url file: %v", err)
	}

	tests := []struct {
		name       string
		opts       targets.ParseOptions
		wantLen    int
		wantErr    bool
		checkFirst func(t *testing.T, target targets.Target)
	}{
		{
			name:    "Single URL",
			opts:    targets.ParseOptions{URL: "http://example.com"},
			wantLen: 1,
			wantErr: false,
			checkFirst: func(t *testing.T, target targets.Target) {
				if target.ID != 1 || target.URL != "http://example.com" {
					t.Errorf("unexpected target: %+v", target)
				}
			},
		},
		{
			name:    "Multiple Comma Separated URLs",
			opts:    targets.ParseOptions{URL: "http://a.com, http://b.com , http://c.com"},
			wantLen: 3,
			wantErr: false,
			checkFirst: func(t *testing.T, target targets.Target) {
				if target.ID != 1 || target.URL != "http://a.com" {
					t.Errorf("unexpected first target: %+v", target)
				}
			},
		},
		{
			name:    "URL File Parsing",
			opts:    targets.ParseOptions{URLFile: urlFilePath},
			wantLen: 2,
			wantErr: false,
			checkFirst: func(t *testing.T, target targets.Target) {
				if target.ID != 1 || target.URL != "http://target1.local" {
					t.Errorf("unexpected first target: %+v", target)
				}
			},
		},
		{
			name:    "Missing Target Error",
			opts:    targets.ParseOptions{},
			wantLen: 0,
			wantErr: true,
		},
		{
			name:    "Nonexistent URL File",
			opts:    targets.ParseOptions{URLFile: filepath.Join(tmpDir, "nonexistent.txt")},
			wantLen: 0,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := targets.Parse(tc.opts)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Parse() error = %v, wantErr = %v", err, tc.wantErr)
			}
			if len(res) != tc.wantLen {
				t.Fatalf("expected %d targets, got %d", tc.wantLen, len(res))
			}
			if tc.checkFirst != nil && len(res) > 0 {
				tc.checkFirst(t, res[0])
			}
		})
	}
}

func TestManager_Execute_Lifecycle(t *testing.T) {
	targetList := []targets.Target{
		{ID: 1, URL: "http://t1.local"},
		{ID: 2, URL: "http://t2.local"},
		{ID: 3, URL: "http://t3.local"},
	}

	// 1. Success through all targets
	mgr := targets.NewManager(targetList)
	visited := make([]int, 0)
	err := mgr.Execute(context.Background(), func(tCtx targets.TargetContext) error {
		visited = append(visited, tCtx.Target.ID)
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(visited) != 3 {
		t.Fatalf("expected 3 visited targets, got %v", visited)
	}

	// 2. Skip single target via local context cancellation
	visited = nil
	err = mgr.Execute(context.Background(), func(tCtx targets.TargetContext) error {
		visited = append(visited, tCtx.Target.ID)
		if tCtx.Target.ID == 2 {
			tCtx.Cancel() // simulate user pressing 'q' on target 2
			return context.Canceled
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error on local skip, got: %v", err)
	}
	if len(visited) != 3 {
		t.Fatalf("expected all 3 targets to be processed despite skip on 2, got %v", visited)
	}

	// 3. Global abortion terminates entire manager
	globalCtx, globalCancel := context.WithCancel(context.Background())
	visited = nil
	err = mgr.Execute(globalCtx, func(tCtx targets.TargetContext) error {
		visited = append(visited, tCtx.Target.ID)
		if tCtx.Target.ID == 1 {
			globalCancel() // simulate user pressing 'a' or SIGINT
			return context.Canceled
		}
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled on global abort, got: %v", err)
	}
	if len(visited) != 1 {
		t.Fatalf("expected only 1 target before abort, got %v", visited)
	}

	// 4. Fatal error propagation
	fatalErr := errors.New("network catastrophic failure")
	err = mgr.Execute(context.Background(), func(tCtx targets.TargetContext) error {
		return fatalErr
	})
	if !errors.Is(err, fatalErr) {
		t.Fatalf("expected fatal error returned, got: %v", err)
	}
}

func TestGlobalSummary(t *testing.T) {
	summary := targets.NewGlobalSummary(5)

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			summary.AddSnapshot(stats.Snapshot{
				RequestsSent: 100,
				Discovered:   5,
			})
		}(i)
	}
	wg.Wait()

	if summary.TargetsTotal != 5 {
		t.Errorf("expected TargetsTotal = 5, got %d", summary.TargetsTotal)
	}
	if summary.TargetsRun != 3 {
		t.Errorf("expected TargetsRun = 3, got %d", summary.TargetsRun)
	}
	if summary.TotalJobs != 300 {
		t.Errorf("expected TotalJobs = 300, got %d", summary.TotalJobs)
	}
	if summary.TotalFound != 15 {
		t.Errorf("expected TotalFound = 15, got %d", summary.TotalFound)
	}
	if summary.Duration() <= 0 {
		t.Errorf("expected positive duration, got %v", summary.Duration())
	}
}
