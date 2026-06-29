package httpclient

import (
	"net/http"
	"time"
)

// New returns an *http.Client tuned for high-concurrency scanning.
// MaxIdleConns and MaxIdleConnsPerHost are set high to avoid connection
// starvation across many workers. IdleConnTimeout evicts connections before
// the server can RST them on reuse.
func New(timeout int) *http.Client {
	tr := &http.Transport{
		MaxIdleConns:        1000,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  false,
	}

	return &http.Client{
		Transport: tr,
		Timeout:   time.Duration(timeout) * time.Second,
	}
}
