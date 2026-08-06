package stats

import "time"

// Snapshot is an immutable representation of the statistics engine's state at a point in time.
type Snapshot struct {
	RequestsSent          int64
	ResponsesReceived     int64
	RequestsFiltered      int64
	RequestsFailed        int64
	RequestsSucceeded     int64
	BytesReceived         int64
	ActiveWorkers         int64
	TotalWork             int64
	Tried                 int64
	Skipped               int64
	Completed             int64
	Progress              float64
	TotalCandidates       int64 // Alias to TotalWork
	Discovered            int64
	InvalidWords          int64
	JobsProduced          int64
	SearchSpaceProgress   int64 // Alias to Completed
	StartTime             time.Time
	StatusCodes           map[int]int64
	IsFinite              bool
	DirectoriesDiscovered int64
	DirectoriesQueued     int64

	// Future metrics support
	Retries                  int64
	Redirects                int64
	BodyInspected            int64
	AverageLatency           time.Duration
	RequestsPerSecond        float64 // Lifetime average Req/s
	CurrentRequestsPerSecond float64 // Sliding-window current Req/s (~1–5s window)
	PeakRequestsPerSecond    float64 // Highest observed CurrentRequestsPerSecond
}
