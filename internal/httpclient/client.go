package httpclient

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

// New returns an *http.Client tuned for high-concurrency scanning.
// MaxIdleConns and MaxIdleConnsPerHost are set high to avoid connection
// starvation across many workers. IdleConnTimeout evicts connections before
// the server can RST them on reuse.
func New(timeout time.Duration, connectTimeout time.Duration, followRedirects bool, proxyURL string) *http.Client {
	return NewWithMaxRedirects(timeout, connectTimeout, followRedirects, 10, proxyURL)
}

func NewWithMaxRedirects(timeout time.Duration, connectTimeout time.Duration, followRedirects bool, maxRedirects int, proxyURL string) *http.Client {
	tr := &http.Transport{
		MaxIdleConns:        1000,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  false,
		DialContext: (&net.Dialer{
			Timeout:   connectTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}

	if proxyURL != "" {
		pURL, err := url.Parse(proxyURL)
		if err != nil {
			panic(fmt.Sprintf("invalid proxy URL %q: %v", proxyURL, err))
		}
		tr.Proxy = http.ProxyURL(pURL)
	} else {
		tr.Proxy = http.ProxyFromEnvironment
	}

	var checkRedirect func(req *http.Request, via []*http.Request) error
	if !followRedirects {
		checkRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	} else {
		checkRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("maximum redirect limit exceeded")
			}
			// Detect loops
			for _, prev := range via {
				if prev.URL.String() == req.URL.String() {
					return fmt.Errorf("redirect loop detected")
				}
			}
			return nil
		}
	}

	return &http.Client{
		Transport:     tr,
		Timeout:       timeout,
		CheckRedirect: checkRedirect,
	}
}
