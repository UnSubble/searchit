package fingerprint

// Engine orchestrates the target fingerprinting lifecycle.
// It acts as the single entry point for analysis.
type Engine struct {
	cache *Cache
}

// NewEngine creates a new Engine using the provided Fingerprint Cache.
func NewEngine(cache *Cache) *Engine {
	return &Engine{cache: cache}
}

// Analyze runs the detector pipeline against the given request context.
// It resolves the target host's Fingerprint and dispatches it to all detectors.
func (e *Engine) Analyze(ctx *Context) {
	fp := e.cache.GetOrCreate(ctx.Host)

	// Static detector dispatch pipeline.
	// As new detectors are added, add their invocation here.
	detectHeaders(ctx, fp)
}
