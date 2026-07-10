package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/app"
	"github.com/unsubble/searchit/internal/config"
)

func TestNew_UsesBackgroundContextWhenNil(t *testing.T) {
	var nilCtx context.Context
	a := app.New(nilCtx, config.Default())
	if a.Context == nil {
		t.Fatal("Context is nil, want context.Background()")
	}
}

func TestNew_PreservesProvidedContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a := app.New(ctx, config.Default())
	if a.Context != ctx {
		t.Error("Context was replaced; want the provided context")
	}
}

func TestNew_CreatesHTTPClient(t *testing.T) {
	a := app.New(context.Background(), config.Default())
	if a.HTTPClient == nil {
		t.Fatal("HTTPClient is nil")
	}
}

func TestNew_ConfiguresHTTPClientTimeout(t *testing.T) {
	cfg := config.Default()
	cfg.Timeout = 30

	a := app.New(context.Background(), cfg)

	if want := 30 * time.Second; a.HTTPClient.Timeout != want {
		t.Errorf("HTTPClient.Timeout = %v, want %v", a.HTTPClient.Timeout, want)
	}
}
