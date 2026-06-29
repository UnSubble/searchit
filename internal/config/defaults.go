package config

import "github.com/unsubble/searchit/internal/status"

// Default returns a Config populated with sane baseline values.
func Default() Config {
	exclude, err := status.Parse("404")
	if err != nil {
		panic("config: invalid default status expression: " + err.Error())
	}

	return Config{
		Threads: 64,
		Timeout: 10,
		Status: StatusConfig{
			Exclude: exclude,
		},
	}
}
