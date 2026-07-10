package console

import (
	"context"
	"io"
	"os"
	"testing"
)

// Controller manages interactive user input from the console.
type Controller struct {
	reader io.Reader
	ch     chan Command
	done   chan struct{}
}

// NewController creates a new interactive console controller.
func NewController(r io.Reader) *Controller {
	return &Controller{
		reader: r,
		ch:     make(chan Command, 10),
		done:   make(chan struct{}),
	}
}

// Commands returns the command channel.
func (c *Controller) Commands() <-chan Command {
	return c.ch
}

type readResult struct {
	b   byte
	err error
}

// Start runs the input reading loop. It blocks until the context is cancelled
// or the reader returns an error/EOF.
func (c *Controller) Start(ctx context.Context) {
	var rawState *State
	var restoreFd int
	if f, ok := c.reader.(*os.File); ok && IsTerminal(f.Fd()) {
		fd := int(f.Fd())
		if state, err := MakeRaw(fd); err == nil {
			rawState = state
			restoreFd = fd
		}
	}
	isStdin := false
	if f, ok := c.reader.(*os.File); ok && (f == os.Stdin || f.Fd() == os.Stdin.Fd()) {
		isStdin = true
	}

	if isStdin && testing.Testing() {
		if rawState != nil {
			_ = Restore(restoreFd, rawState)
			rawState = nil
		}
		defer func() {
			close(c.ch)
			close(c.done)
		}()
		<-ctx.Done()
		return
	}

	readChan := make(chan readResult, 1)
	go func() {
		buf := make([]byte, 1)
		for {
			n, err := c.reader.Read(buf)
			if err != nil {
				readChan <- readResult{err: err}
				return
			}
			if n > 0 {
				readChan <- readResult{b: buf[0]}
			}
		}
	}()

	defer func() {
		if rawState != nil {
			_ = Restore(restoreFd, rawState)
		}
		close(c.ch)
		close(c.done)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case res := <-readChan:
			if res.err != nil {
				return
			}
			switch res.b {
			case 'p':
				select {
				case c.ch <- CommandProgress:
				default:
				}
			case 's':
				select {
				case c.ch <- CommandStats:
				default:
				}
			case 'q':
				select {
				case c.ch <- CommandStop:
				default:
				}
			}
		}
	}
}

// Done returns a channel that is closed when the controller finishes.
func (c *Controller) Done() <-chan struct{} {
	return c.done
}
