package app

import (
	"context"
	"net/http"

	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/httpclient"
)

// App holds the shared runtime state passed to the engine.
// The engine depends only on App and config.Config — never on CLI or YAML packages.
type App struct {
	Context    context.Context
	Config     config.Config
	HTTPClient *http.Client
}

// New creates an App from the given context and config.
// A nil context is replaced with context.Background().
func New(ctx context.Context, cfg config.Config) *App {
	if ctx == nil {
		ctx = context.Background()
	}

	client := httpclient.New(cfg.Timeout, cfg.ConnectTimeout)

	return &App{
		Context:    ctx,
		Config:     cfg,
		HTTPClient: client,
	}
}
