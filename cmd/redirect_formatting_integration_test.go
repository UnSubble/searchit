package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRedirectFormatting_ParityAcrossModes(t *testing.T) {
	// Status codes to test: 301, 302, 303, 307, 308
	redirectCodes := []int{301, 302, 303, 307, 308}

	for _, code := range redirectCodes {
		t.Run(fmt.Sprintf("Status_%d", code), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/", "":
					w.WriteHeader(http.StatusOK)
				case "/test":
					w.Header().Set("Location", "/dest/")
					w.WriteHeader(code)
				case "/dest/":
					w.WriteHeader(http.StatusOK)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer srv.Close()

			tmpDir := t.TempDir()
			wlPath := filepath.Join(tmpDir, "wordlist.txt")
			if err := os.WriteFile(wlPath, []byte("test\n"), 0644); err != nil {
				t.Fatalf("failed to write wordlist: %v", err)
			}

			// Expected output fragment for text mode
			expectedText := fmt.Sprintf("[%d] - ", code)
			expectedArrow := fmt.Sprintf("/test -> %s/dest/", srv.URL)

			modes := []struct {
				name string
				args []string
			}{
				{
					name: "Standard Scan",
					args: []string{"scan", "-u", srv.URL, "-w", wlPath, "-X", "GET", "--mc", fmt.Sprintf("200,%d", code)},
				},
				{
					name: "Recursive Scan",
					args: []string{"scan", "-u", srv.URL, "-w", wlPath, "-r", "--max-depth", "1", "--mc", fmt.Sprintf("200,%d", code)},
				},
				{
					name: "Adaptive Scan",
					args: []string{"scan", "-u", srv.URL, "-w", wlPath, "--adaptive", "--mc", fmt.Sprintf("200,%d", code)},
				},
				{
					name: "Fuzz",
					args: []string{"fuzz", "-u", srv.URL + "/FUZZ", "-w", wlPath, "-X", "GET", "--mc", fmt.Sprintf("200,%d", code)},
				},
			}

			for _, m := range modes {
				t.Run(m.name, func(t *testing.T) {
					outStr, err := runIntegrationCommand(m.args)
					if err != nil {
						t.Fatalf("mode %s failed: %v", m.name, err)
					}

					if !strings.Contains(outStr, expectedText) || !strings.Contains(outStr, expectedArrow) {
						t.Errorf("mode %s output missing redirect format.\nWant prefix: %q and arrow: %q\nGot:\n%s", m.name, expectedText, expectedArrow, outStr)
					}

					// Verify legacy format "[+] 30x -" is NOT present
					legacyPrefix := fmt.Sprintf("[+] %d -", code)
					if strings.Contains(outStr, legacyPrefix) {
						t.Errorf("mode %s still emitted legacy format %q!\nGot:\n%s", m.name, legacyPrefix, outStr)
					}
				})
			}
		})
	}
}

func TestRedirectFormatting_StructuredFormatsUnchanged(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			w.Header().Set("Location", "/destination")
			w.WriteHeader(http.StatusMovedPermanently)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "wordlist.txt")
	_ = os.WriteFile(wlPath, []byte("redirect\n"), 0644)

	jsonOutFile := filepath.Join(tmpDir, "out.json")

	// Test JSON output format
	_, err := runIntegrationCommand([]string{"scan", "-u", srv.URL, "-w", wlPath, "-o", jsonOutFile, "--format", "json", "--mc", "301"})
	if err != nil {
		t.Fatalf("json execution failed: %v", err)
	}

	jsonBytes, err := os.ReadFile(jsonOutFile)
	if err != nil {
		t.Fatalf("failed to read json output file: %v", err)
	}

	var jsonParsed []map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &jsonParsed); err != nil {
		t.Fatalf("invalid json output: %v, raw: %s", err, string(jsonBytes))
	}
	if len(jsonParsed) != 1 {
		t.Fatalf("expected 1 json result, got %d", len(jsonParsed))
	}
	if status, ok := jsonParsed[0]["status"].(float64); !ok || int(status) != 301 {
		t.Errorf("json status = %v; want 301", jsonParsed[0]["status"])
	}

	csvOutFile := filepath.Join(tmpDir, "out.csv")

	// Test CSV output format
	_, err = runIntegrationCommand([]string{"scan", "-u", srv.URL, "-w", wlPath, "-o", csvOutFile, "--format", "csv", "--mc", "301"})
	if err != nil {
		t.Fatalf("csv execution failed: %v", err)
	}

	csvBytes, err := os.ReadFile(csvOutFile)
	if err != nil {
		t.Fatalf("failed to read csv output file: %v", err)
	}

	csvStr := strings.ToLower(string(csvBytes))
	if !strings.Contains(csvStr, "url,status,length") || !strings.Contains(csvStr, "301") {
		t.Errorf("unexpected csv output: %s", csvStr)
	}
}
