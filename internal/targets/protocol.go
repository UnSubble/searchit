package targets

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"

	"github.com/unsubble/searchit/internal/httpclient"
)

// HasExplicitScheme returns true if the raw URL string begins with an explicit scheme (http://, https://, etc.).
func HasExplicitScheme(rawURL string) bool {
	trimmed := strings.TrimSpace(rawURL)
	lower := strings.ToLower(trimmed)

	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return true
	}

	if idx := strings.Index(lower, "://"); idx != -1 {
		scheme := lower[:idx]
		for i, r := range scheme {
			if i == 0 {
				if !(r >= 'a' && r <= 'z') {
					return false
				}
			} else {
				if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '+' || r == '-' || r == '.') {
					return false
				}
			}
		}
		return true
	}

	return false
}

// ShouldFallbackToHTTP inspects an error resulting from an HTTPS probe request.
// Returns true ONLY if HTTPS is genuinely unavailable (e.g. connection refused, EOF during TLS handshake,
// server speaks plain HTTP). Returns false for certificate errors (invalid, expired, hostname mismatch),
// user context cancellation, or explicit HTTPS requests.
func ShouldFallbackToHTTP(err error) bool {
	if err == nil {
		return false
	}

	// 1. Check for x509 / TLS certificate errors -> DO NOT FALLBACK
	var unknownAuth x509.UnknownAuthorityError
	var hostnameErr x509.HostnameError
	var certInvalid x509.CertificateInvalidError
	if errors.As(err, &unknownAuth) || errors.As(err, &hostnameErr) || errors.As(err, &certInvalid) {
		return false
	}

	errStr := strings.ToLower(err.Error())
	if strings.Contains(errStr, "x509:") ||
		strings.Contains(errStr, "certificate signed by unknown authority") ||
		strings.Contains(errStr, "certificate is valid for") ||
		strings.Contains(errStr, "certificate has expired") ||
		strings.Contains(errStr, "certificate expired") ||
		strings.Contains(errStr, "cert") {
		return false
	}

	// 2. Check for context cancellation or timeout -> DO NOT FALLBACK
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// 3. Fallback conditions (HTTPS genuinely unavailable)
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}

	if strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "first record does not look like a tls handshake") ||
		strings.Contains(errStr, "server gave http response") ||
		strings.Contains(errStr, "tls: oversized record") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "eof") ||
		strings.Contains(errStr, "no route to host") ||
		strings.Contains(errStr, "network is unreachable") {
		return true
	}

	// Check net.OpError / syscall errors
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		var sysErr syscall.Errno
		if errors.As(opErr.Err, &sysErr) {
			switch sysErr {
			case syscall.ECONNREFUSED, syscall.ECONNRESET, syscall.ENETUNREACH, syscall.EHOSTUNREACH:
				return true
			}
		}
	}

	return false
}

// AutoDetectTarget checks if rawTarget has an explicit scheme. If not, it attempts HTTPS first
// and falls back to HTTP if HTTPS is genuinely unavailable.
func AutoDetectTarget(ctx context.Context, client *http.Client, rawTarget string, out io.Writer) (string, error) {
	rawTarget = strings.TrimSpace(rawTarget)
	if rawTarget == "" {
		return "", fmt.Errorf("at least one target is required")
	}

	if HasExplicitScheme(rawTarget) {
		return rawTarget, nil
	}

	if client == nil {
		client = &http.Client{
			Timeout: 5 * time.Second,
		}
	}

	httpsURL := "https://" + rawTarget
	httpURL := "http://" + rawTarget

	if out != nil {
		fmt.Fprintf(out, "[*] Target %s has no protocol specified, attempting HTTPS...\n", rawTarget)
	}

	// Validate URL syntax
	if _, err := url.Parse(httpsURL); err != nil {
		return "", fmt.Errorf("invalid target URL %q: %w", rawTarget, err)
	}

	// Attempt HTTPS probe
	probeReq, err := http.NewRequestWithContext(ctx, http.MethodGet, httpsURL, nil)
	if err != nil {
		return "", fmt.Errorf("invalid target URL %q: %w", rawTarget, err)
	}

	resp, err := client.Do(probeReq)
	if err == nil {
		httpclient.DrainAndCloseResponse(resp)
		if out != nil {
			fmt.Fprintln(out, "[*] Using HTTPS.")
		}
		return httpsURL, nil
	}

	if ShouldFallbackToHTTP(err) {
		if out != nil {
			fmt.Fprintln(out, "[*] HTTPS unavailable, falling back to HTTP.")
		}
		return httpURL, nil
	}

	if out != nil {
		fmt.Fprintln(out, "[*] Using HTTPS.")
	}
	return httpsURL, nil
}

// AutoDetectTargets resolves target protocol auto-detection across a list of Target objects.
func AutoDetectTargets(ctx context.Context, client *http.Client, targetList []Target, out io.Writer) ([]Target, error) {
	resolved := make([]Target, len(targetList))
	copy(resolved, targetList)

	for i := range resolved {
		u, err := AutoDetectTarget(ctx, client, resolved[i].URL, out)
		if err != nil {
			return nil, err
		}
		resolved[i].URL = u
	}

	return resolved, nil
}
