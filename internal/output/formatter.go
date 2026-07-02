package output

import "github.com/unsubble/searchit/internal/engine"

// Formatter abstracts output presentation to decouple CLI presentation from the scanning engine.
type Formatter interface {
	Print(engine.Result) error
	Close() error
}
