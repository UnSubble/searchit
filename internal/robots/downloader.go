package robots

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// Download fetches the robots.txt file for the given target URL.
// It resolves robots.txt at the host root (e.g., http://example.com/robots.txt).
func Download(ctx context.Context, client *http.Client, targetURL string) (io.ReadCloser, string, error) {
	u, err := url.Parse(targetURL)
	if err != nil {
		return nil, "", fmt.Errorf("invalid target URL: %w", err)
	}

	// Resolve robots.txt at the root of the host
	robotsURL := fmt.Sprintf("%s://%s/robots.txt", u.Scheme, u.Host)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, robotsURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to execute request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return resp.Body, robotsURL, nil
}
