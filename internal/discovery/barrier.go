package discovery

// BarrierType defines when/how fuzzer execution must synchronize to process strategies.
type BarrierType string

const (
	BarrierBootstrap BarrierType = "bootstrap" // Before starting the main crawl loop
	BarrierDepth     BarrierType = "depth"     // Level-by-level (depth N completed)
	BarrierTarget    BarrierType = "target"    // Multi-target scan transition
)

// Barrier describes a synchronization point in the traversal pipeline.
type Barrier struct {
	Type BarrierType
}
