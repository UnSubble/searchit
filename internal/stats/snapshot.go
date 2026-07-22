package stats

import "time"

// Snapshot is an immutable representation of the statistics engine's state at a point in time.
type Snapshot struct {
	RequestsSent      int64
	ResponsesReceived int64
	RequestsFiltered  int64
	RequestsFailed    int64
	RequestsSucceeded int64
	BytesReceived     int64
	ActiveWorkers     int64
	QueuedJobs        int64
	Discovered        int64
	InvalidWords      int64
	JobsProduced      int64
	StartTime         time.Time
	StatusCodes       map[int]int64

	// Future metrics support
	Retries               int64
	Redirects             int64
	BodyInspected         int64
	AverageLatency        time.Duration
	RequestsPerSecond     float64
	PeakRequestsPerSecond float64
}
