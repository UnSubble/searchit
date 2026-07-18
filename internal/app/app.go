package app

import (
	"bytes"
	"context"
	"io"
	"net/http"

	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/fingerprint"
	"github.com/unsubble/searchit/internal/httpclient"
)

// App holds the shared runtime state passed to the engine.
// The engine depends only on App and config.Config — never on CLI or YAML packages.
type App struct {
	Context          context.Context
	Config           config.Config
	HTTPClient       *http.Client
	FingerprintCache *fingerprint.Cache
}

type fingerprintRoundTripper struct {
	underlying http.RoundTripper
	cache      *fingerprint.Cache
}

func (f *fingerprintRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := f.underlying.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	if resp == nil {
		return nil, nil
	}

	var bodyBytes []byte
	if resp.Body != nil {
		// Read up to 4096 bytes for fingerprinting (ignoring error, just grab what is available)
		limitReader := io.LimitReader(resp.Body, 4096)
		bodyBytes, _ = io.ReadAll(limitReader)
		// Restore body so other code can read it from the start
		resp.Body = &readCloser{
			Reader: io.MultiReader(bytes.NewReader(bodyBytes), resp.Body),
			Closer: resp.Body,
		}
	}

	// Analyze the response using the fingerprint engine
	ctx := &fingerprint.Context{
		Host:       req.URL.Host,
		Path:       req.URL.Path,
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		Body:       bodyBytes,
	}
	fingerprint.NewEngine(f.cache).Analyze(ctx)

	return resp, nil
}

type readCloser struct {
	io.Reader
	io.Closer
}

// WrapTransport wraps an http.RoundTripper with the fingerprintRoundTripper.
func WrapTransport(rt http.RoundTripper, cache *fingerprint.Cache) http.RoundTripper {
	if rt == nil {
		rt = http.DefaultTransport
	}
	return &fingerprintRoundTripper{
		underlying: rt,
		cache:      cache,
	}
}

// New creates an App from the given context and config.
// A nil context is replaced with context.Background().
func New(ctx context.Context, cfg config.Config) *App {
	if ctx == nil {
		ctx = context.Background()
	}

	client := httpclient.NewWithMaxRedirects(cfg.Timeout, cfg.ConnectTimeout, cfg.FollowRedirects, cfg.MaxRedirects, cfg.Proxy)

	var fpCache *fingerprint.Cache
	if cfg.Adaptive {
		fpCache = fingerprint.NewCache()
		client.Transport = WrapTransport(client.Transport, fpCache)
	}

	return &App{
		Context:          ctx,
		Config:           cfg,
		HTTPClient:       client,
		FingerprintCache: fpCache,
	}
}
