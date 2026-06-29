package engine

// Job is the unit of work dispatched to a worker.
// One Job produces at most one Result.
type Job struct {
	URL string
}
