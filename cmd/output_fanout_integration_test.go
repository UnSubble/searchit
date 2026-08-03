package cmd

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/output"
	"github.com/unsubble/searchit/internal/stats"
)

func TestOutputPipeline_SinkInvariants(t *testing.T) {
	t.Run("Stats and Yield Invariants", func(t *testing.T) {
		col := stats.NewCollector()
		var yieldCount int32
		var termReceived int32
		var fileReceived int32

		var termBuf bytes.Buffer
		var fileBuf bytes.Buffer

		termFmttr := output.NewTextFormatter(&termBuf, false, false, false)
		fileFmttr := output.NewTextFormatter(&fileBuf, false, false, false)

		emitResult := func(r engine.Result) {
			if r.Accepted {
				col.RecordDiscovered()
				atomic.AddInt32(&yieldCount, 1)

				if termFmttr != nil {
					_ = termFmttr.Print(r)
					atomic.AddInt32(&termReceived, 1)
				}
				if fileFmttr != nil {
					_ = fileFmttr.Print(r)
					atomic.AddInt32(&fileReceived, 1)
				}
			}
		}

		res1 := engine.Result{URL: "http://example.com/a", StatusCode: 200, Accepted: true}
		res2 := engine.Result{URL: "http://example.com/b", StatusCode: 200, Accepted: true}
		res3 := engine.Result{URL: "http://example.com/c", StatusCode: 404, Accepted: false}

		emitResult(res1)
		emitResult(res2)
		emitResult(res3)

		// 1. Accepted findings increment Results
		snap := col.Snapshot()
		if snap.Discovered != 2 {
			t.Errorf("expected 2 discovered resources in stats, got %d", snap.Discovered)
		}

		// 2. Yield invoked exactly once per accepted finding
		if yieldCount != 2 {
			t.Errorf("expected yield count 2, got %d", yieldCount)
		}

		// 3. Terminal & File sinks both receive every accepted finding
		if termReceived != 2 {
			t.Errorf("expected terminal sink to receive 2 findings, got %d", termReceived)
		}
		if fileReceived != 2 {
			t.Errorf("expected file sink to receive 2 findings, got %d", fileReceived)
		}

		// 4. Simultaneous outputs match
		if !strings.Contains(termBuf.String(), "/a") || !strings.Contains(termBuf.String(), "/b") {
			t.Errorf("terminal buffer missing findings: %s", termBuf.String())
		}
		if !strings.Contains(fileBuf.String(), "/a") || !strings.Contains(fileBuf.String(), "/b") {
			t.Errorf("file buffer missing findings: %s", fileBuf.String())
		}
	})
}

func TestOutputPipeline_FanOutScenarios(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/", "/robots.txt", "/sitemap.xml":
			w.WriteHeader(http.StatusOK)
		case "/found1", "/found2":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("found item"))
		case "/recurse":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`<a href="/recurse/child">child</a>`))
		case "/recurse/child":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	wordlistFile := filepath.Join(tmpDir, "words.txt")
	_ = os.WriteFile(wordlistFile, []byte("found1\nfound2\n"), 0644)

	t.Run("Terminal Only (no -o, no -q)", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var outBuf bytes.Buffer
		cmd, _ := NewScanCmd()
		cmd.SetOut(&outBuf)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{
			"-u", srv.URL,
			"-w", wordlistFile,
			"--no-progress",
		})

		err := cmd.ExecuteContext(ctx)
		if err != nil {
			t.Fatalf("scan failed: %v", err)
		}

		stdout := outBuf.String()
		if !strings.Contains(stdout, "/found1") || !strings.Contains(stdout, "/found2") {
			t.Errorf("expected stdout to contain both findings, got:\n%s", stdout)
		}
		if strings.Count(stdout, "/found1") != 1 || strings.Count(stdout, "/found2") != 1 {
			t.Errorf("expected findings exactly once in stdout, got:\n%s", stdout)
		}
	})

	t.Run("Terminal + File (-o, no -q)", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		outFile := filepath.Join(tmpDir, "term_file.txt")

		var outBuf bytes.Buffer
		cmd, _ := NewScanCmd()
		cmd.SetOut(&outBuf)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{
			"-u", srv.URL,
			"-w", wordlistFile,
			"-o", outFile,
			"--no-progress",
		})

		err := cmd.ExecuteContext(ctx)
		if err != nil {
			t.Fatalf("scan failed: %v", err)
		}

		stdout := outBuf.String()
		fileData, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("failed to read output file: %v", err)
		}
		fileStr := string(fileData)

		if !strings.Contains(stdout, "/found1") || !strings.Contains(stdout, "/found2") {
			t.Errorf("expected stdout to contain findings when -o is set, got:\n%s", stdout)
		}
		if strings.Count(stdout, "/found1") != 1 || strings.Count(stdout, "/found2") != 1 {
			t.Errorf("expected findings exactly once in stdout, got:\n%s", stdout)
		}

		if !strings.Contains(fileStr, "/found1") || !strings.Contains(fileStr, "/found2") {
			t.Errorf("expected output file to contain findings, got:\n%s", fileStr)
		}
		if strings.Count(fileStr, "/found1") != 1 || strings.Count(fileStr, "/found2") != 1 {
			t.Errorf("expected findings exactly once in file, got:\n%s", fileStr)
		}
	})

	t.Run("Quiet Mode (-q + -o)", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		outFile := filepath.Join(tmpDir, "quiet_file.txt")

		var outBuf bytes.Buffer
		cmd, _ := NewScanCmd()
		cmd.SetOut(&outBuf)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{
			"-u", srv.URL,
			"-w", wordlistFile,
			"-o", outFile,
			"-q",
			"--no-progress",
		})

		err := cmd.ExecuteContext(ctx)
		if err != nil {
			t.Fatalf("scan failed: %v", err)
		}

		stdout := outBuf.String()
		fileData, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("failed to read output file: %v", err)
		}
		fileStr := string(fileData)

		// Under new UX spec: -q outputs links-only to terminal stdout AND to file.
		if !strings.Contains(stdout, "/found1") || !strings.Contains(stdout, "/found2") {
			t.Errorf("expected links-only findings in stdout for -q + -o, got:\n%s", stdout)
		}
		if strings.Contains(stdout, "[+]") {
			t.Errorf("expected no [+] formatting in stdout for -q + -o, got:\n%s", stdout)
		}

		if !strings.Contains(fileStr, "/found1") || !strings.Contains(fileStr, "/found2") {
			t.Errorf("expected file output for -q + -o, got:\n%s", fileStr)
		}
		if strings.Contains(fileStr, "[+]") {
			t.Errorf("expected no [+] formatting in file for -q + -o, got:\n%s", fileStr)
		}
	})

	t.Run("Adaptive Fuzz + -o", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		outFile := filepath.Join(tmpDir, "adaptive_fuzz.txt")

		var outBuf bytes.Buffer
		cmd, _ := NewFuzzCmd()
		cmd.SetOut(&outBuf)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{
			"fuzz",
			"-u", srv.URL + "/FUZZ",
			"-w", wordlistFile,
			"-o", outFile,
			"--adaptive",
			"--no-progress",
		})

		err := cmd.ExecuteContext(ctx)
		if err != nil {
			t.Fatalf("fuzz command failed: %v", err)
		}

		stdout := outBuf.String()
		fileData, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("failed to read output file: %v", err)
		}
		fileStr := string(fileData)

		if !strings.Contains(stdout, "/found1") || !strings.Contains(stdout, "/found2") {
			t.Errorf("expected stdout in adaptive fuzz mode with -o, got:\n%s", stdout)
		}
		if !strings.Contains(fileStr, "/found1") || !strings.Contains(fileStr, "/found2") {
			t.Errorf("expected output file in adaptive fuzz mode with -o, got:\n%s", fileStr)
		}
	})

	t.Run("Recursive Scan + -o", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		outFile := filepath.Join(tmpDir, "recursive_scan.txt")
		recWordlist := filepath.Join(tmpDir, "rec_words.txt")
		_ = os.WriteFile(recWordlist, []byte("recurse\nchild\n"), 0644)

		var outBuf bytes.Buffer
		cmd, _ := NewScanCmd()
		cmd.SetOut(&outBuf)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{
			"-u", srv.URL,
			"-w", recWordlist,
			"-o", outFile,
			"--recursive",
			"--max-depth", "2",
			"--no-progress",
		})

		err := cmd.ExecuteContext(ctx)
		if err != nil {
			t.Fatalf("recursive scan failed: %v", err)
		}

		stdout := outBuf.String()
		fileData, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("failed to read output file: %v", err)
		}
		fileStr := string(fileData)

		if !strings.Contains(stdout, "/recurse") || !strings.Contains(stdout, "/recurse/child") {
			t.Errorf("expected stdout in recursive scan with -o, got:\n%s", stdout)
		}
		if !strings.Contains(fileStr, "/recurse") || !strings.Contains(fileStr, "/recurse/child") {
			t.Errorf("expected output file in recursive scan with -o, got:\n%s", fileStr)
		}
	})
}
