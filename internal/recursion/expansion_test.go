package recursion_test

import (
	"context"
	"testing"

	"github.com/unsubble/searchit/internal/recursion"
)

func TestDirectoryGenerator_BFS_ExtensionExpansion(t *testing.T) {
	reader := sliceReader("foo", "bar", "baz")
	visited := make(map[string]struct{})

	gen, err := recursion.NewDirectoryGenerator(
		context.Background(),
		reader,
		"http://example.com",
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
		[]string{"", "php"},
		visited,
		nil,
		nil,
		nil,
		nil,
		recursion.BFS,
	)
	if err != nil {
		t.Fatalf("failed to create DirectoryGenerator: %v", err)
	}

	var urls []string
	for {
		job, ok := gen.Next()
		if !ok {
			break
		}
		urls = append(urls, job.URL)
	}

	expected := []string{
		"http://example.com/foo",
		"http://example.com/bar",
		"http://example.com/baz",
		"http://example.com/foo.php",
		"http://example.com/bar.php",
		"http://example.com/baz.php",
	}

	if len(urls) != len(expected) {
		t.Fatalf("expected %d URLs, got %d: %v", len(expected), len(urls), urls)
	}
	for i, exp := range expected {
		if urls[i] != exp {
			t.Errorf("URL[%d] = %q, want %q", i, urls[i], exp)
		}
	}
}

func TestDirectoryGenerator_DFS_ExtensionExpansion(t *testing.T) {
	reader := sliceReader("foo", "bar", "baz")
	visited := make(map[string]struct{})

	gen, err := recursion.NewDirectoryGenerator(
		context.Background(),
		reader,
		"http://example.com",
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
		[]string{"", "php"},
		visited,
		nil,
		nil,
		nil,
		nil,
		recursion.DFS,
	)
	if err != nil {
		t.Fatalf("failed to create DirectoryGenerator: %v", err)
	}

	var urls []string
	for {
		job, ok := gen.Next()
		if !ok {
			break
		}
		urls = append(urls, job.URL)
	}

	expected := []string{
		"http://example.com/foo",
		"http://example.com/foo.php",
		"http://example.com/bar",
		"http://example.com/bar.php",
		"http://example.com/baz",
		"http://example.com/baz.php",
	}

	if len(urls) != len(expected) {
		t.Fatalf("expected %d URLs, got %d: %v", len(expected), len(urls), urls)
	}
	for i, exp := range expected {
		if urls[i] != exp {
			t.Errorf("URL[%d] = %q, want %q", i, urls[i], exp)
		}
	}
}

func TestDirectoryGenerator_MultipleExtensions(t *testing.T) {
	reader := sliceReader("page")
	visited := make(map[string]struct{})

	gen, err := recursion.NewDirectoryGenerator(
		context.Background(),
		reader,
		"http://example.com",
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
		[]string{"", "php", "txt"},
		visited,
		nil,
		nil,
		nil,
		nil,
		recursion.BFS,
	)
	if err != nil {
		t.Fatalf("failed to create DirectoryGenerator: %v", err)
	}

	var urls []string
	for {
		job, ok := gen.Next()
		if !ok {
			break
		}
		urls = append(urls, job.URL)
	}

	expected := []string{
		"http://example.com/page",
		"http://example.com/page.php",
		"http://example.com/page.txt",
	}

	if len(urls) != len(expected) {
		t.Fatalf("expected %d URLs, got %d: %v", len(expected), len(urls), urls)
	}
	for i, exp := range expected {
		if urls[i] != exp {
			t.Errorf("URL[%d] = %q, want %q", i, urls[i], exp)
		}
	}
}

func TestScanDryRun_StrategyExtensionOrder(t *testing.T) {
	reader := sliceReader("a", "b")

	// BFS dry run
	reqsBFS, totalBFS, err := recursion.GenerateScanDryRunRequests(
		context.Background(),
		[]string{"http://example.com"},
		reader,
		false,
		false,
		[]string{"", "php"},
		10,
		recursion.BFS,
	)
	if err != nil {
		t.Fatalf("GenerateScanDryRunRequests BFS failed: %v", err)
	}
	if totalBFS != 4 {
		t.Fatalf("expected totalBFS 4, got %d", totalBFS)
	}
	expectedBFS := []string{
		"http://example.com/a",
		"http://example.com/b",
		"http://example.com/a.php",
		"http://example.com/b.php",
	}
	for i, exp := range expectedBFS {
		if reqsBFS[i].URL != exp {
			t.Errorf("BFS req[%d] = %q, want %q", i, reqsBFS[i].URL, exp)
		}
	}

	// DFS dry run
	reqsDFS, totalDFS, err := recursion.GenerateScanDryRunRequests(
		context.Background(),
		[]string{"http://example.com"},
		reader,
		false,
		false,
		[]string{"", "php"},
		10,
		recursion.DFS,
	)
	if err != nil {
		t.Fatalf("GenerateScanDryRunRequests DFS failed: %v", err)
	}
	if totalDFS != 4 {
		t.Fatalf("expected totalDFS 4, got %d", totalDFS)
	}
	expectedDFS := []string{
		"http://example.com/a",
		"http://example.com/a.php",
		"http://example.com/b",
		"http://example.com/b.php",
	}
	for i, exp := range expectedDFS {
		if reqsDFS[i].URL != exp {
			t.Errorf("DFS req[%d] = %q, want %q", i, reqsDFS[i].URL, exp)
		}
	}
}
