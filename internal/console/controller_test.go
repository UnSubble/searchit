package console_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/console"
)

func TestController_Keys(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []console.Command
	}{
		{
			name:     "progress command",
			input:    "p",
			expected: []console.Command{console.CommandProgress},
		},
		{
			name:     "stats command",
			input:    "s",
			expected: []console.Command{console.CommandStats},
		},
		{
			name:     "stop command",
			input:    "q",
			expected: []console.Command{console.CommandStop},
		},
		{
			name:     "unknown keys ignored",
			input:    "x y z p q a s",
			expected: []console.Command{console.CommandProgress, console.CommandStop, console.CommandStats},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pr, pw := io.Pipe()
			c := console.NewController(pr)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			go c.Start(ctx)

			go func() {
				_, _ = pw.Write([]byte(tc.input))
				pw.Close()
			}()

			var got []console.Command
			timeout := time.After(500 * time.Millisecond)
			ch := c.Commands()

		loop:
			for {
				select {
				case cmd, ok := <-ch:
					if !ok {
						break loop
					}
					got = append(got, cmd)
				case <-timeout:
					t.Fatal("timeout waiting for commands")
				}
			}

			if len(got) != len(tc.expected) {
				t.Fatalf("expected %d commands, got %d", len(tc.expected), len(got))
			}
			for i, expectedCmd := range tc.expected {
				if got[i] != expectedCmd {
					t.Errorf("at index %d: expected command %v, got %v", i, expectedCmd, got[i])
				}
			}
		})
	}
}

func TestController_ContextCancellation(t *testing.T) {
	pr, _ := io.Pipe()
	c := console.NewController(pr)

	ctx, cancel := context.WithCancel(context.Background())
	go c.Start(ctx)

	cancel()

	select {
	case <-c.Done():
		// Pass
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for controller shutdown on context cancellation")
	}
}
