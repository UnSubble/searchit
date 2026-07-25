package stats

import (
	"math"
	"sync/atomic"
	"time"
)

// windowSlots is the number of ring-buffer slots used for the sliding-window rate.
// Each slot spans windowSlotDuration. Total window = windowSlots * windowSlotDuration.
const (
	windowSlots        = 5
	windowSlotDuration = time.Second
)

// rateSlot is a pair of (nanosecond timestamp, cumulative request count) stored
// as two separate int64 atomics so each field can be updated independently.
type rateSlot struct {
	timestampNano int64
	requestsSent  int64
}

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
	invalidWords      int64
	jobsProduced      int64
	startTime         int64 // Unix nano timestamp

	// Future metrics support
	retries                int64
	redirects              int64
	bodyInspected          int64
	totalLatencyNano       int64
	latencyCount           int64
	peakRequestsPerSecBits uint64

	// Sliding-window ring buffer for Current Req/s tracking.
	// windowSlots slots, each covering windowSlotDuration. No lock required:
	// each slot is written by at most one goroutine at a time (slot index
	// advances monotonically by wall-clock time, not concurrently).
	window [windowSlots]rateSlot

	// Fixed-size status code array covering status codes 0-999.
	// This avoids mutex locks and allocations during updates.
	statusCodes [1000]int64
}

// NewCollector instantiates a new statistics collector and sets the start time.
func NewCollector() *Collector {
	now := time.Now()
	nowNano := now.UnixNano()
	c := &Collector{
		startTime: nowNano,
	}
	// Seed the slot immediately before the current one with the start time
	// so that the very first Snapshot() can compute a current rate rather
	// than returning 0 (which would happen if no prior slot existed).
	seedSlot := int((nowNano/int64(windowSlotDuration) - 1 + windowSlots) % windowSlots)
	atomic.StoreInt64(&c.window[seedSlot].timestampNano, nowNano-int64(windowSlotDuration))
	atomic.StoreInt64(&c.window[seedSlot].requestsSent, 0)
	return c
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

// RecordInvalidWord increments the invalid words counter.
func (c *Collector) RecordInvalidWord() {
	atomic.AddInt64(&c.invalidWords, 1)
}

// RecordJobProduced increments the produced jobs counter.
func (c *Collector) RecordJobProduced() {
	atomic.AddInt64(&c.jobsProduced, 1)
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

// DecrementQueuedJobs decrements the number of queued jobs by 1.
func (c *Collector) DecrementQueuedJobs() {
	atomic.AddInt64(&c.queuedJobs, -1)
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
	invalid := atomic.LoadInt64(&c.invalidWords)
	jobs := atomic.LoadInt64(&c.jobsProduced)
	startNano := atomic.LoadInt64(&c.startTime)

	retries := atomic.LoadInt64(&c.retries)
	redirects := atomic.LoadInt64(&c.redirects)
	inspected := atomic.LoadInt64(&c.bodyInspected)
	totalLat := atomic.LoadInt64(&c.totalLatencyNano)
	latCount := atomic.LoadInt64(&c.latencyCount)

	startTime := time.Unix(0, startNano)
	elapsed := time.Since(startTime)
	nowNano := time.Now().UnixNano()

	// Lifetime average Req/s.
	var avgReqPerSec float64
	if elapsed.Seconds() > 0 {
		avgReqPerSec = float64(sent) / elapsed.Seconds()
	}

	// Sliding-window Current Req/s.
	// Write the current sample into the ring buffer slot for this moment.
	slotIndex := int((nowNano / int64(windowSlotDuration)) % windowSlots)
	atomic.StoreInt64(&c.window[slotIndex].timestampNano, nowNano)
	atomic.StoreInt64(&c.window[slotIndex].requestsSent, sent)

	// Scan older slots to find the oldest reading within the full window duration.
	windowNano := int64(windowSlots) * int64(windowSlotDuration)
	oldestNano := nowNano - windowNano
	var bestOldNano, bestOldSent int64 = nowNano, sent
	for i := 0; i < windowSlots; i++ {
		if i == slotIndex {
			continue
		}
		slotNano := atomic.LoadInt64(&c.window[i].timestampNano)
		if slotNano > oldestNano && slotNano < bestOldNano {
			bestOldNano = slotNano
			bestOldSent = atomic.LoadInt64(&c.window[i].requestsSent)
		}
	}
	var currentReqPerSec float64
	if windowDelta := float64(nowNano-bestOldNano) / float64(time.Second); windowDelta > 0 {
		currentReqPerSec = float64(sent-bestOldSent) / windowDelta
		if currentReqPerSec < 0 {
			currentReqPerSec = 0
		}
	}

	// Update Peak using the current (windowed) rate, not the lifetime average.
	for {
		currentPeakBits := atomic.LoadUint64(&c.peakRequestsPerSecBits)
		currentPeak := math.Float64frombits(currentPeakBits)
		if currentReqPerSec <= currentPeak {
			break
		}
		if atomic.CompareAndSwapUint64(&c.peakRequestsPerSecBits, currentPeakBits, math.Float64bits(currentReqPerSec)) {
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
		RequestsSent:             sent,
		ResponsesReceived:        recv,
		RequestsFiltered:         filt,
		RequestsFailed:           fail,
		RequestsSucceeded:        succ,
		BytesReceived:            bytes,
		ActiveWorkers:            workers,
		QueuedJobs:               queued,
		Discovered:               disc,
		InvalidWords:             invalid,
		JobsProduced:             jobs,
		StartTime:                startTime,
		StatusCodes:              statusCopy,
		Retries:                  retries,
		Redirects:                redirects,
		BodyInspected:            inspected,
		AverageLatency:           avgLat,
		RequestsPerSecond:        avgReqPerSec,
		CurrentRequestsPerSecond: currentReqPerSec,
		PeakRequestsPerSecond:    peakReqPerSec,
	}
}
