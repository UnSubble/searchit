package recursion_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/recursion"
	"github.com/unsubble/searchit/internal/status"
	"github.com/unsubble/searchit/internal/wordlist"
)

func TestWordlistError_MissingFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	missingPath := filepath.Join(tmpDir, "missing.txt")
	reader := wordlist.FileReader{Path: missingPath}

	recurseOnPolicy, _ := status.Parse("200,301,302,403")

	// Verify NewDirectoryGenerator fails immediately
	gen, err := recursion.NewDirectoryGenerator(
		context.Background(),
		reader,
		srv.URL,
		nil,
		1,
		"",
		nil,
		nil,
		false,
		false,
		false,
		false,
		false,
		nil,
		make(map[string]struct{}),
		nil,
		nil,
		nil,
		nil,
	)
	if err == nil {
		t.Fatalf("expected error from NewDirectoryGenerator with missing wordlist, got nil")
	}
	if gen != nil {
		t.Fatalf("expected DirectoryGenerator to be nil when LoadWords fails, got non-nil: %+v", gen)
	}

	// Verify Manager.Run propagates the error instead of finishing with Candidates: 1
	m := recursion.NewManager(
		http.DefaultClient,
		nil,
		reader,
		recursion.BFS,
		3,
		recurseOnPolicy,
		false,
		false,
		0,
		nil,
		nil,
		100,
	)

	runErr := m.Run(context.Background(), context.Background(), []string{srv.URL}, 2, func(r engine.Result) {}, nil)
	if runErr == nil {
		t.Fatalf("expected error from Manager.Run with missing wordlist, got nil")
	}
	if !os.IsNotExist(runErr) && !errors.Is(runErr, os.ErrNotExist) && !strings.Contains(runErr.Error(), "cannot find the file specified") && !strings.Contains(runErr.Error(), "no such file or directory") {
		t.Fatalf("expected file not found error, got: %v", runErr)
	}
}

func TestWordlistError_EmptyFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	emptyPath := filepath.Join(tmpDir, "empty.txt")
	_ = os.WriteFile(emptyPath, []byte("# comment line only\n\n"), 0644)
	reader := wordlist.FileReader{Path: emptyPath}

	recurseOnPolicy, _ := status.Parse("200,301,302,403")

	gen, err := recursion.NewDirectoryGenerator(
		context.Background(),
		reader,
		srv.URL,
		nil,
		1,
		"",
		nil,
		nil,
		false,
		false,
		false,
		false,
		false,
		nil,
		make(map[string]struct{}),
		nil,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("expected nil error for empty but valid wordlist, got: %v", err)
	}
	if gen == nil {
		t.Fatalf("expected non-nil DirectoryGenerator for empty but valid wordlist")
	}
	_, ok := gen.Next()
	if ok {
		t.Fatalf("expected Next() to return false for empty wordlist")
	}

	m := recursion.NewManager(
		http.DefaultClient,
		nil,
		reader,
		recursion.BFS,
		3,
		recurseOnPolicy,
		false,
		false,
		0,
		nil,
		nil,
		100,
	)

	runErr := m.Run(context.Background(), context.Background(), []string{srv.URL}, 2, func(r engine.Result) {}, nil)
	if runErr != nil {
		t.Fatalf("expected nil error from Manager.Run for valid empty wordlist, got: %v", runErr)
	}
}

func TestWordlistError_CancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	validPath := filepath.Join(tmpDir, "valid.txt")
	_ = os.WriteFile(validPath, []byte("admin\nuser\n"), 0644)
	reader := wordlist.FileReader{Path: validPath}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := recursion.NewDirectoryGenerator(
		ctx,
		reader,
		srv.URL,
		nil,
		1,
		"",
		nil,
		nil,
		false,
		false,
		false,
		false,
		false,
		nil,
		make(map[string]struct{}),
		nil,
		nil,
		nil,
		nil,
	)
	if err == nil {
		t.Fatalf("expected context.Canceled error, got nil")
	}
}
