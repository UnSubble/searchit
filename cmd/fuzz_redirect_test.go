package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestFuzzRedirectMatrix(t *testing.T) {
	// 1. Mutual Exclusion: --follow-redirects and --only-redirects conflict
	t.Run("FlagConflict_FollowThenOnly", func(t *testing.T) {
		var reqCount atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqCount.Add(1)
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		tmpDir := t.TempDir()
		wlPath := filepath.Join(tmpDir, "wl.txt")
		_ = os.WriteFile(wlPath, []byte("test\n"), 0644)

		cmd, _ := NewFuzzCmd()
		_, _, err := executeCmd(cmd, []string{
			"-u", srv.URL + "/FUZZ", "-w", wlPath, "--follow-redirects", "--only-redirects",
		})
		if err == nil {
			t.Fatal("expected error for --follow-redirects with --only-redirects, got nil")
		}
		if !strings.Contains(err.Error(), "--follow-redirects cannot be used with --only-redirects") {
			t.Errorf("unexpected error message: %v", err)
		}
		if reqCount.Load() != 0 {
			t.Errorf("expected 0 requests sent before flag validation failure, got %d", reqCount.Load())
		}
	})

	t.Run("FlagConflict_OnlyThenFollow", func(t *testing.T) {
		var reqCount atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqCount.Add(1)
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		tmpDir := t.TempDir()
		wlPath := filepath.Join(tmpDir, "wl.txt")
		_ = os.WriteFile(wlPath, []byte("test\n"), 0644)

		cmd, _ := NewFuzzCmd()
		_, _, err := executeCmd(cmd, []string{
			"-u", srv.URL + "/FUZZ", "-w", wlPath, "--only-redirects", "--follow-redirects",
		})
		if err == nil {
			t.Fatal("expected error for --only-redirects with --follow-redirects, got nil")
		}
		if !strings.Contains(err.Error(), "--follow-redirects cannot be used with --only-redirects") {
			t.Errorf("unexpected error message: %v", err)
		}
		if reqCount.Load() != 0 {
			t.Errorf("expected 0 requests sent before flag validation failure, got %d", reqCount.Load())
		}
	})

	// 2. 302 -> 200 with --only-redirects
	//    /login -> 302 /dashboard
	//    /dashboard -> 200
	// Expected: exactly one result, reporting 200 and the final URL
	t.Run("302_To_200_OnlyRedirects", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/login":
				http.Redirect(w, r, "/dashboard", http.StatusFound)
			case "/dashboard":
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("dashboard ok"))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer srv.Close()

		tmpDir := t.TempDir()
		wlPath := filepath.Join(tmpDir, "wl.txt")
		_ = os.WriteFile(wlPath, []byte("login\n"), 0644)

		cmd, _ := NewFuzzCmd()
		stdout, _, err := executeCmd(cmd, []string{
			"-u", srv.URL + "/FUZZ", "-w", wlPath, "--only-redirects",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(stdout, "200") || !strings.Contains(stdout, "/dashboard") {
			t.Errorf("expected final 200 /dashboard in output, got:\n%s", stdout)
		}
		if strings.Contains(stdout, "302") {
			t.Errorf("expected intermediate 302 to NOT be in output, got:\n%s", stdout)
		}
	})

	// 3. Direct 200 (no redirect) with --only-redirects
	//    Should NOT be emitted in output
	t.Run("NoRedirect_WithOnlyRedirects_Suppressed", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("direct ok"))
		}))
		defer srv.Close()

		tmpDir := t.TempDir()
		wlPath := filepath.Join(tmpDir, "wl.txt")
		_ = os.WriteFile(wlPath, []byte("direct\n"), 0644)

		cmd, _ := NewFuzzCmd()
		stdout, _, err := executeCmd(cmd, []string{
			"-u", srv.URL + "/FUZZ", "-w", wlPath, "--only-redirects",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if strings.Contains(stdout, "200") || strings.Contains(stdout, "/direct") {
			t.Errorf("expected non-redirected result to be suppressed, got:\n%s", stdout)
		}
	})

	// 4. 302 -> 404 with --only-redirects and --mc 404
	//    Final status is 404. With --mc 404, it should be emitted as 404.
	t.Run("302_To_404_WithMC", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/old":
				http.Redirect(w, r, "/missing", http.StatusFound)
			case "/missing":
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte("not found"))
			default:
				w.WriteHeader(http.StatusOK)
			}
		}))
		defer srv.Close()

		tmpDir := t.TempDir()
		wlPath := filepath.Join(tmpDir, "wl.txt")
		_ = os.WriteFile(wlPath, []byte("old\n"), 0644)

		cmd, _ := NewFuzzCmd()
		stdout, _, err := executeCmd(cmd, []string{
			"-u", srv.URL + "/FUZZ", "-w", wlPath, "--only-redirects", "--mc", "404",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(stdout, "404") || !strings.Contains(stdout, "/missing") {
			t.Errorf("expected 404 /missing in output, got:\n%s", stdout)
		}
		if strings.Contains(stdout, "302") {
			t.Errorf("expected intermediate 302 to NOT be in output, got:\n%s", stdout)
		}
	})

	// 5. 302 -> 200 with --fc 200
	//    Final status is 200. With --fc 200, it should be filtered out.
	t.Run("302_To_200_FilteredByFC", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/login":
				http.Redirect(w, r, "/dashboard", http.StatusFound)
			case "/dashboard":
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("dashboard ok"))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer srv.Close()

		tmpDir := t.TempDir()
		wlPath := filepath.Join(tmpDir, "wl.txt")
		_ = os.WriteFile(wlPath, []byte("login\n"), 0644)

		cmd, _ := NewFuzzCmd()
		stdout, _, err := executeCmd(cmd, []string{
			"-u", srv.URL + "/FUZZ", "-w", wlPath, "--only-redirects", "--fc", "200",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if strings.Contains(stdout, "200") || strings.Contains(stdout, "/dashboard") {
			t.Errorf("expected 200 to be filtered by --fc 200, got:\n%s", stdout)
		}
	})

	// 6. Multi-hop redirect chain:
	//    /a -> 301 /b -> 302 /c -> 200
	// Expected: exactly one result: 200 /c
	t.Run("MultiHopChain_Fuzz", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/a":
				http.Redirect(w, r, "/b", http.StatusMovedPermanently)
			case "/b":
				http.Redirect(w, r, "/c", http.StatusFound)
			case "/c":
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("c reached"))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer srv.Close()

		tmpDir := t.TempDir()
		wlPath := filepath.Join(tmpDir, "wl.txt")
		_ = os.WriteFile(wlPath, []byte("a\n"), 0644)

		cmd, _ := NewFuzzCmd()
		stdout, _, err := executeCmd(cmd, []string{
			"-u", srv.URL + "/FUZZ", "-w", wlPath, "--only-redirects",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(stdout, "200") || !strings.Contains(stdout, "/c") {
			t.Errorf("expected final 200 /c in output, got:\n%s", stdout)
		}
		if strings.Contains(stdout, "301") || strings.Contains(stdout, "302") {
			t.Errorf("intermediate redirect statuses must not be in output, got:\n%s", stdout)
		}
	})

	// 7. Max redirects limit respected
	t.Run("MaxRedirectsLimit", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/start":
				http.Redirect(w, r, "/hop1", http.StatusFound)
			case "/hop1":
				http.Redirect(w, r, "/hop2", http.StatusFound)
			case "/hop2":
				http.Redirect(w, r, "/final", http.StatusFound)
			case "/final":
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("final reached"))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer srv.Close()

		tmpDir := t.TempDir()
		wlPath := filepath.Join(tmpDir, "wl.txt")
		_ = os.WriteFile(wlPath, []byte("start\n"), 0644)

		// With max-redirects 1, the 3-hop chain must exceed the limit and not emit 200
		cmd, _ := NewFuzzCmd()
		stdout, _, _ := executeCmd(cmd, []string{
			"-u", srv.URL + "/FUZZ", "-w", wlPath, "--only-redirects", "--max-redirects", "1",
		})
		if strings.Contains(stdout, "200") || strings.Contains(stdout, "/final") {
			t.Errorf("expected chain exceeding max-redirects 1 to NOT emit 200 finding, got:\n%s", stdout)
		}

		// With max-redirects 5, the 3-hop chain succeeds and emits final 200
		cmd2, _ := NewFuzzCmd()
		stdout2, _, err2 := executeCmd(cmd2, []string{
			"-u", srv.URL + "/FUZZ", "-w", wlPath, "--only-redirects", "--max-redirects", "5",
		})
		if err2 != nil {
			t.Fatalf("unexpected error with max-redirects 5: %v", err2)
		}
		if !strings.Contains(stdout2, "200") || !strings.Contains(stdout2, "/final") {
			t.Errorf("expected max-redirects 5 to successfully reach /final with 200, got:\n%s", stdout2)
		}
	})
}
