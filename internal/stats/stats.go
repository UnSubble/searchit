package stats

import (
	"math"
	"sync/atomic"
	"time"
)

// Collector accumulates runtime execution statistics.
// All operations are concurrency-safe and optimized using atomic operations to minimize overhead.
type Collector struct {
	requestsSent      int64
	responsesReceived int64
	requestsFiltered  int64
	requestsFailed    int64
	requestsSucceeded int64
	bytesReceived     int64
	activeWorkers     int64
	queuedJobs        int64
	discovered        int64
	startTime         int64 // Unix nano timestamp

	// Future metrics support
	retries                int64
	redirects              int64
	bodyInspected          int64
	totalLatencyNano       int64
	latencyCount           int64
	peakRequestsPerSecBits uint64

	// Fixed-size status code array covering status codes 0-999.
	// This avoids mutex locks and allocations during updates.
	statusCodes [1000]int64
}

// NewCollector instantiates a new statistics collector and sets the start time.
func NewCollector() *Collector {
	return &Collector{
		startTime: time.Now().UnixNano(),
	}
}

// RecordRequestSent increments the total requests sent counter.
func (c *Collector) RecordRequestSent() {
	atomic.AddInt64(&c.requestsSent, 1)
}

// RecordResponseReceived increments total responses, updates status code counters and byte counts.
func (c *Collector) RecordResponseReceived(statusCode int, bytes int64) {
	atomic.AddInt64(&c.responsesReceived, 1)
	atomic.AddInt64(&c.bytesReceived, bytes)
	if statusCode >= 0 && statusCode < 1000 {
		atomic.AddInt64(&c.statusCodes[statusCode], 1)
	}
}

// RecordRequestFiltered increments the filtered requests counter.
func (c *Collector) RecordRequestFiltered() {
	atomic.AddInt64(&c.requestsFiltered, 1)
}

// RecordRequestFailed increments the failed requests counter.
func (c *Collector) RecordRequestFailed() {
	atomic.AddInt64(&c.requestsFailed, 1)
}

// RecordRequestSucceeded increments the succeeded requests counter.
func (c *Collector) RecordRequestSucceeded() {
	atomic.AddInt64(&c.requestsSucceeded, 1)
}

// RecordDiscovered increments the discovered resources counter.
func (c *Collector) RecordDiscovered() {
	atomic.AddInt64(&c.discovered, 1)
}

// IncrementActiveWorkers increments the active worker count by 1.
func (c *Collector) IncrementActiveWorkers() {
	atomic.AddInt64(&c.activeWorkers, 1)
}

// DecrementActiveWorkers decrements the active worker count by 1.
func (c *Collector) DecrementActiveWorkers() {
	atomic.AddInt64(&c.activeWorkers, -1)
}

// SetActiveWorkers sets the active worker count directly.
func (c *Collector) SetActiveWorkers(workers int64) {
	atomic.StoreInt64(&c.activeWorkers, workers)
}

// SetQueuedJobs sets the number of queued jobs.
func (c *Collector) SetQueuedJobs(jobs int64) {
	atomic.StoreInt64(&c.queuedJobs, jobs)
}

// RecordRetry increments the retries counter.
func (c *Collector) RecordRetry() {
	atomic.AddInt64(&c.retries, 1)
}

// RecordRedirect increments the redirects counter.
func (c *Collector) RecordRedirect() {
	atomic.AddInt64(&c.redirects, 1)
}

// RecordBodyInspected increments the body-inspected responses counter.
func (c *Collector) RecordBodyInspected() {
	atomic.AddInt64(&c.bodyInspected, 1)
}

// RecordLatency adds a latency sample to the average calculations.
func (c *Collector) RecordLatency(d time.Duration) {
	atomic.AddInt64(&c.totalLatencyNano, d.Nanoseconds())
	atomic.AddInt64(&c.latencyCount, 1)
}

// Snapshot returns an immutable, consistent copy of current statistics.
func (c *Collector) Snapshot() Snapshot {
	sent := atomic.LoadInt64(&c.requestsSent)
	recv := atomic.LoadInt64(&c.responsesReceived)
	filt := atomic.LoadInt64(&c.requestsFiltered)
	fail := atomic.LoadInt64(&c.requestsFailed)
	succ := atomic.LoadInt64(&c.requestsSucceeded)
	bytes := atomic.LoadInt64(&c.bytesReceived)
	workers := atomic.LoadInt64(&c.activeWorkers)
	queued := atomic.LoadInt64(&c.queuedJobs)
	disc := atomic.LoadInt64(&c.discovered)
	startNano := atomic.LoadInt64(&c.startTime)

	retries := atomic.LoadInt64(&c.retries)
	redirects := atomic.LoadInt64(&c.redirects)
	inspected := atomic.LoadInt64(&c.bodyInspected)
	totalLat := atomic.LoadInt64(&c.totalLatencyNano)
	latCount := atomic.LoadInt64(&c.latencyCount)

	startTime := time.Unix(0, startNano)
	elapsed := time.Since(startTime)

	var reqPerSec float64
	if elapsed.Seconds() > 0 {
		reqPerSec = float64(sent) / elapsed.Seconds()
	}

	// Update and load Peak Requests Per Second atomically
	for {
		currentPeakBits := atomic.LoadUint64(&c.peakRequestsPerSecBits)
		currentPeak := math.Float64frombits(currentPeakBits)
		if reqPerSec <= currentPeak {
			break
		}
		if atomic.CompareAndSwapUint64(&c.peakRequestsPerSecBits, currentPeakBits, math.Float64bits(reqPerSec)) {
			break
		}
	}
	peakReqPerSec := math.Float64frombits(atomic.LoadUint64(&c.peakRequestsPerSecBits))

	var avgLat time.Duration
	if latCount > 0 {
		avgLat = time.Duration(totalLat / latCount)
	}

	statusCopy := make(map[int]int64)
	for i := 0; i < len(c.statusCodes); i++ {
		val := atomic.LoadInt64(&c.statusCodes[i])
		if val > 0 {
			statusCopy[i] = val
		}
	}

	return Snapshot{
		RequestsSent:          sent,
		ResponsesReceived:     recv,
		RequestsFiltered:      filt,
		RequestsFailed:        fail,
		RequestsSucceeded:     succ,
		BytesReceived:         bytes,
		ActiveWorkers:         workers,
		QueuedJobs:            queued,
		Discovered:            disc,
		StartTime:             startTime,
		StatusCodes:           statusCopy,
		Retries:               retries,
		Redirects:             redirects,
		BodyInspected:         inspected,
		AverageLatency:        avgLat,
		RequestsPerSecond:     reqPerSec,
		PeakRequestsPerSecond: peakReqPerSec,
	}
}
