package wordlist_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/wordlist"
)

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

// fakeReader streams lines as-is, letting Producer tests control exact input
// without touching the embedded file.
type fakeReader struct {
	lines []string
}

func (r fakeReader) Read(ctx context.Context, out chan<- string) error {
	for _, line := range r.lines {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- line:
		}
	}
	return nil
}

func collect(ch <-chan string) []string {
	var out []string
	for s := range ch {
		out = append(out, s)
	}
	return out
}

// ─────────────────────────────────────────────────────────────────────────────
// EmbeddedReader
// ─────────────────────────────────────────────────────────────────────────────

func TestEmbeddedReader_EmitsEntries(t *testing.T) {
	// Buffer matches DefaultWordBuffer so the goroutine is never blocked by a
	// slow test consumer regardless of wordlist size.
	out := make(chan string, wordlist.DefaultWordBuffer)
	r := wordlist.EmbeddedReader{}

	go func() {
		defer close(out)
		if err := r.Read(context.Background(), out); err != nil {
			t.Errorf("Read returned error: %v", err)
		}
	}()

	entries := collect(out)
	if len(entries) == 0 {
		t.Fatal("EmbeddedReader emitted no entries")
	}
}

func TestEmbeddedReader_NoBlankLines(t *testing.T) {
	out := make(chan string, wordlist.DefaultWordBuffer)
	errCh := make(chan error, 1)
	r := wordlist.EmbeddedReader{}
	go func() {
		defer close(out)
		errCh <- r.Read(context.Background(), out)
	}()

	for entry := range out {
		if strings.TrimSpace(entry) == "" {
			t.Errorf("blank line emitted: %q", entry)
		}
	}
	if err := <-errCh; err != nil {
		t.Errorf("Read returned error: %v", err)
	}
}

func TestEmbeddedReader_NoCommentLines(t *testing.T) {
	out := make(chan string, wordlist.DefaultWordBuffer)
	errCh := make(chan error, 1)
	r := wordlist.EmbeddedReader{}
	go func() {
		defer close(out)
		errCh <- r.Read(context.Background(), out)
	}()

	for entry := range out {
		if strings.HasPrefix(entry, "#") {
			t.Errorf("comment line emitted: %q", entry)
		}
	}
	if err := <-errCh; err != nil {
		t.Errorf("Read returned error: %v", err)
	}
}

func TestEmbeddedReader_WhitespaceTrimmed(t *testing.T) {
	out := make(chan string, wordlist.DefaultWordBuffer)
	errCh := make(chan error, 1)
	r := wordlist.EmbeddedReader{}
	go func() {
		defer close(out)
		errCh <- r.Read(context.Background(), out)
	}()

	for entry := range out {
		if entry != strings.TrimSpace(entry) {
			t.Errorf("untrimmed entry emitted: %q", entry)
		}
	}
	if err := <-errCh; err != nil {
		t.Errorf("Read returned error: %v", err)
	}
}

func TestEmbeddedReader_Cancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	out := make(chan string, wordlist.DefaultWordBuffer)
	r := wordlist.EmbeddedReader{}
	err := r.Read(ctx, out)

	if err == nil {
		t.Error("Read returned nil with cancelled context, want ctx.Err()")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Producer
// ─────────────────────────────────────────────────────────────────────────────

func TestProducer_URLJoining(t *testing.T) {
	tests := []struct {
		base string
		word string
		want string
	}{
		{"https://example.com", "admin", "https://example.com/admin"},
		{"https://example.com/", "admin", "https://example.com/admin"},
		{"https://example.com", "/admin", "https://example.com/admin"},
		{"https://example.com/", "/admin", "https://example.com/admin"},
		{"https://example.com", "api/v1", "https://example.com/api/v1"},
	}

	for _, tc := range tests {
		t.Run(tc.base+"+"+tc.word, func(t *testing.T) {
			t.Parallel()

			r := fakeReader{lines: []string{tc.word}}
			p := wordlist.Producer{BaseURL: tc.base, Reader: r}

			jobs := make(chan engine.Job, 1)
			if err := p.Produce(context.Background(), jobs); err != nil {
				t.Fatalf("Produce returned error: %v", err)
			}

			var got []string
			for j := range jobs {
				got = append(got, j.URL)
			}

			if len(got) != 1 || got[0] != tc.want {
				t.Errorf("URL = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestProducer_EmitsAllJobs(t *testing.T) {
	words := []string{"admin", "login", "api"}
	r := fakeReader{lines: words}
	p := wordlist.Producer{BaseURL: "https://example.com", Reader: r}

	jobs := make(chan engine.Job, len(words))
	if err := p.Produce(context.Background(), jobs); err != nil {
		t.Fatalf("Produce returned error: %v", err)
	}

	var got []string
	for j := range jobs {
		got = append(got, j.URL)
	}

	want := []string{
		"https://example.com/admin",
		"https://example.com/login",
		"https://example.com/api",
	}
	sort.Strings(got)
	sort.Strings(want)

	if len(got) != len(want) {
		t.Fatalf("got %d jobs, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("job[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestProducer_ClosesJobsWhenDone(t *testing.T) {
	r := fakeReader{lines: []string{"x"}}
	p := wordlist.Producer{BaseURL: "https://example.com", Reader: r}

	jobs := make(chan engine.Job, 1)
	if err := p.Produce(context.Background(), jobs); err != nil {
		t.Fatalf("Produce returned error: %v", err)
	}

	for range jobs {
	}
}

func TestProducer_Cancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	lines := make([]string, 10000)
	for i := range lines {
		lines[i] = "path"
	}

	r := fakeReader{lines: lines}
	p := wordlist.Producer{BaseURL: "https://example.com", Reader: r}

	jobs := make(chan engine.Job, wordlist.DefaultWordBuffer)
	err := p.Produce(ctx, jobs)
	if err == nil {
		t.Error("Produce returned nil with cancelled context, want ctx.Err()")
	}
}

func TestProducer_EmptyWordlist(t *testing.T) {
	r := fakeReader{lines: nil}
	p := wordlist.Producer{BaseURL: "https://example.com", Reader: r}

	jobs := make(chan engine.Job, 1)
	if err := p.Produce(context.Background(), jobs); err != nil {
		t.Fatalf("Produce returned error for empty wordlist: %v", err)
	}
	if _, ok := <-jobs; ok {
		t.Error("expected closed channel for empty wordlist, got a value")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// FileReader
// ─────────────────────────────────────────────────────────────────────────────

func TestFileReader_EmitsEntries(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "words.txt")
	content := `
# Comment line
  word1  
  
word2
# Another comment
word3
`
	if err := os.WriteFile(filePath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	r := wordlist.FileReader{Path: filePath}
	out := make(chan string, 10)

	go func() {
		defer close(out)
		if err := r.Read(context.Background(), out); err != nil {
			t.Errorf("FileReader.Read returned error: %v", err)
		}
	}()

	var got []string
	for entry := range out {
		got = append(got, entry)
	}

	want := []string{"word1", "word2", "word3"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FileReader.Read = %v, want %v", got, want)
	}
}

func TestFileReader_MissingFile(t *testing.T) {
	r := wordlist.FileReader{Path: "does_not_exist_xyz.txt"}
	out := make(chan string, 10)
	err := r.Read(context.Background(), out)
	if err == nil {
		t.Error("FileReader.Read expected error for missing file, got nil")
	}
}

func TestFileReader_Cancellation(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "words.txt")
	content := "w1\nw2\n"
	if err := os.WriteFile(filePath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	r := wordlist.FileReader{Path: filePath}
	out := make(chan string) // Unbuffered so it blocks on write and triggers select ctx.Done
	err := r.Read(ctx, out)
	if err == nil {
		t.Error("FileReader.Read expected context error, got nil")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Producer Edge Cases
// ─────────────────────────────────────────────────────────────────────────────

func TestProducer_InvalidBaseURL(t *testing.T) {
	r := fakeReader{lines: []string{"admin"}}
	tests := []struct {
		base string
		name string
	}{
		{"http://example.com?query=1", "query string"},
		{"http://example.com#frag", "fragment"},
		{"http://[::1", "invalid parsing"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := wordlist.Producer{BaseURL: tc.base, Reader: r}
			jobs := make(chan engine.Job, 10)
			err := p.Produce(context.Background(), jobs)
			if err == nil {
				t.Errorf("expected error for base URL %q, got nil", tc.base)
			}
		})
	}
}

func TestProducer_CancelDuringProduction(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// A reader that yields one word and then cancels the context to simulate
	// cancellation during stream production.
	r := customBlockReader{
		onRead: func() {
			cancel()
		},
		lines: []string{"a", "b", "c"},
	}

	p := wordlist.Producer{BaseURL: "http://example.com", Reader: r}
	jobs := make(chan engine.Job, 1)

	err := p.Produce(ctx, jobs)
	if err == nil {
		t.Error("expected context error, got nil")
	}
}

type customBlockReader struct {
	onRead func()
	lines  []string
}

func (r customBlockReader) Read(ctx context.Context, out chan<- string) error {
	for _, line := range r.lines {
		if r.onRead != nil {
			r.onRead()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- line:
		}
	}
	return nil
}
