package config

import (
	"time"

	"github.com/unsubble/searchit/internal/recursion"
	"github.com/unsubble/searchit/internal/status"
)

// Default returns a Config populated with sane baseline values.
func Default() Config {
	exclude, err := status.Parse("404")
	if err != nil {
		panic("config: invalid default status expression: " + err.Error())
	}

	return Config{
		Threads:         32,
		Timeout:         10 * time.Second,
		ConnectTimeout:  3 * time.Second,
		Recursive:       false,
		MaxDepth:        3,
		Strategy:        recursion.BFS,
		FollowRedirects: false,
		MaxRedirects:    10,
		RecurseOn:       status.MustParse("200,301,302,403"),
		Paths: PathConfig{
			NormalizePaths:  false,
			CollapseSlashes: false,
		},
		OutputFile:   "",
		OutputFormat: "text",
		Quiet:        false,
		Status: StatusConfig{
			Exclude: exclude,
		},
		Adaptive:     false,
		FuzzStrategy: "eager",
	}
}
