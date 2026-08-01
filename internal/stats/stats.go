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
	bucketSec     int64
	timestampNano int64
	requestsSent  int64
}

// Collector accumulates runtime execution statistics.
// All operations are concurrency-safe and optimized using atomic operations to minimize overhead.
type Collector struct {
	requestsSent          int64
	responsesReceived     int64
	requestsFiltered      int64
	requestsFailed        int64
	requestsSucceeded     int64
	bytesReceived         int64
	activeWorkers         int64
	totalWork             int64
	tried                 int64
	skipped               int64
	totalCandidates       int64
	discovered            int64
	invalidWords          int64
	jobsProduced          int64
	isFinite              int64 // 1 if finite, 0 if infinite/open-ended
	directoriesDiscovered int64
	directoriesQueued     int64
	startTime             int64 // Unix nano timestamp

	// Future metrics support
	retries                int64
	redirects              int64
	bodyInspected          int64
	totalLatencyNano       int64
	latencyCount           int64
	peakRequestsPerSecBits uint64

	// Sliding-window ring buffer for Current Req/s tracking.
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
	startSec := nowNano / int64(time.Second)
	priorSec := startSec - 1
	priorSlot := int((priorSec%windowSlots + windowSlots) % windowSlots)
	atomic.StoreInt64(&c.window[priorSlot].timestampNano, nowNano-int64(time.Second))
	atomic.StoreInt64(&c.window[priorSlot].requestsSent, 0)
	atomic.StoreInt64(&c.window[priorSlot].bucketSec, priorSec)
	return c
}

// Sample records a discrete throughput sample if not already recorded for the current second.
// Decoupled from rendering: does not affect Snapshot() calls.
func (c *Collector) Sample() {
	c.sampleAt(time.Now().UnixNano())
}

func (c *Collector) sampleAt(nowNano int64) {
	bucketSec := nowNano / int64(time.Second)
	slotIndex := int(bucketSec % windowSlots)
	sent := atomic.LoadInt64(&c.requestsSent)

	atomic.StoreInt64(&c.window[slotIndex].timestampNano, nowNano)
	atomic.StoreInt64(&c.window[slotIndex].requestsSent, sent)
	atomic.StoreInt64(&c.window[slotIndex].bucketSec, bucketSec)

	// Update peak throughput metric during sampling
	startNano := atomic.LoadInt64(&c.startTime)
	elapsedSec := float64(nowNano-startNano) / float64(time.Second)
	if elapsedSec > 0 {
		currentRate := float64(sent) / elapsedSec
		var priorSec int64 = -1
		var priorNano, priorSent int64
		for i := 0; i < windowSlots; i++ {
			if i == slotIndex {
				continue
			}
			bSec := atomic.LoadInt64(&c.window[i].bucketSec)
			if bSec > 0 && bSec < bucketSec && bSec >= bucketSec-windowSlots {
				if bSec > priorSec {
					priorSec = bSec
					priorNano = atomic.LoadInt64(&c.window[i].timestampNano)
					priorSent = atomic.LoadInt64(&c.window[i].requestsSent)
				}
			}
		}
		if priorSec > 0 {
			dSec := float64(nowNano-priorNano) / float64(time.Second)
			dSent := sent - priorSent
			if dSec >= 0.2 && dSent >= 0 {
				currentRate = float64(dSent) / dSec
			}
		}
		for {
			currentPeakBits := atomic.LoadUint64(&c.peakRequestsPerSecBits)
			currentPeak := math.Float64frombits(currentPeakBits)
			if currentRate <= currentPeak {
				break
			}
			if atomic.CompareAndSwapUint64(&c.peakRequestsPerSecBits, currentPeakBits, math.Float64bits(currentRate)) {
				break
			}
		}
	}
}

// RecordRequestSent increments the total requests sent counter.
func (c *Collector) RecordRequestSent() {
	atomic.AddInt64(&c.requestsSent, 1)
	c.sampleAt(time.Now().UnixNano())
}

// SetTotalWork sets the fixed total theoretical work for the scan.
func (c *Collector) SetTotalWork(n int64) {
	atomic.StoreInt64(&c.totalWork, n)
	atomic.StoreInt64(&c.totalCandidates, n)
}

// RecordTried increments the tried work counter (HTTP request actually dispatched).
func (c *Collector) RecordTried() {
	atomic.AddInt64(&c.tried, 1)
}

// RecordSkipped increments the skipped work counter (pruned theoretical work).
func (c *Collector) RecordSkipped(n int64) {
	atomic.AddInt64(&c.skipped, n)
}

// RecordResponseReceived increments total responses, updates status code counters and byte counts.
func (c *Collector) RecordResponseReceived(statusCode int, bytes int64) {
	atomic.AddInt64(&c.responsesReceived, 1)
	if bytes > 0 {
		atomic.AddInt64(&c.bytesReceived, bytes)
	}
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

// RecordWildcardFiltered adjusts counters when a response is identified as a wildcard.
func (c *Collector) RecordWildcardFiltered() {
	atomic.AddInt64(&c.discovered, -1)
	atomic.AddInt64(&c.requestsFiltered, 1)
}

// RecordInvalidWord increments the invalid words counter.
func (c *Collector) RecordInvalidWord() {
	atomic.AddInt64(&c.invalidWords, 1)
}

// RecordJobProduced increments the total generated jobs counter.
func (c *Collector) RecordJobProduced() {
	atomic.AddInt64(&c.jobsProduced, 1)
}

// RecordSearchSpaceProgress maps to RecordSkipped for backwards compatibility.
func (c *Collector) RecordSearchSpaceProgress(n int64) {
	if n > 0 {
		c.RecordSkipped(n)
	}
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

// SetTotalCandidates sets the theoretical or dynamic search space total.
func (c *Collector) SetTotalCandidates(candidates int64) {
	c.SetTotalWork(candidates)
}

// SetIsFinite sets whether the scan search space is finite (true) or open-ended (false).
func (c *Collector) SetIsFinite(finite bool) {
	var val int64
	if finite {
		val = 1
	}
	atomic.StoreInt64(&c.isFinite, val)
}

// RecordDirectoryDiscovered increments the discovered directories count.
func (c *Collector) RecordDirectoryDiscovered() {
	atomic.AddInt64(&c.directoriesDiscovered, 1)
}

// RecordDirectoryQueued increments the queued directories count.
func (c *Collector) RecordDirectoryQueued() {
	atomic.AddInt64(&c.directoriesQueued, 1)
}

// SetDirectories sets discovered and queued directory counts directly.
func (c *Collector) SetDirectories(discovered, queued int64) {
	atomic.StoreInt64(&c.directoriesDiscovered, discovered)
	atomic.StoreInt64(&c.directoriesQueued, queued)
}

// AddTotalCandidates is deprecated under Total Work model and is a no-op to prevent mutating TotalWork after initialization.
func (c *Collector) AddTotalCandidates(candidates int64) {
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
	totWork := atomic.LoadInt64(&c.totalWork)
	tried := atomic.LoadInt64(&c.tried)
	skipped := atomic.LoadInt64(&c.skipped)
	completed := tried + skipped
	disc := atomic.LoadInt64(&c.discovered)
	invalid := atomic.LoadInt64(&c.invalidWords)
	jobs := atomic.LoadInt64(&c.jobsProduced)
	startNano := atomic.LoadInt64(&c.startTime)

	var prog float64
	if totWork > 0 {
		prog = float64(completed) / float64(totWork) * 100.0
		if prog > 100.0 {
			prog = 100.0
		}
	}

	retries := atomic.LoadInt64(&c.retries)
	redirects := atomic.LoadInt64(&c.redirects)
	inspected := atomic.LoadInt64(&c.bodyInspected)
	totalLat := atomic.LoadInt64(&c.totalLatencyNano)
	latCount := atomic.LoadInt64(&c.latencyCount)

	startTime := time.Unix(0, startNano)
	elapsed := time.Since(startTime)

	// Lifetime average Req/s.
	var avgReqPerSec float64
	if elapsed.Seconds() > 0 {
		avgReqPerSec = float64(sent) / elapsed.Seconds()
	}

	// Sliding-window Current Req/s (pure read-only from discrete bucket samples).
	var currentReqPerSec float64
	var newestSampleSec int64 = -1
	var newestSampleNano, newestSampleSent int64
	var priorSampleSec int64 = -1
	var priorSampleNano, priorSampleSent int64

	for i := 0; i < windowSlots; i++ {
		bSec := atomic.LoadInt64(&c.window[i].bucketSec)
		if bSec > 0 {
			if bSec > newestSampleSec {
				priorSampleSec = newestSampleSec
				priorSampleNano = newestSampleNano
				priorSampleSent = newestSampleSent

				newestSampleSec = bSec
				newestSampleNano = atomic.LoadInt64(&c.window[i].timestampNano)
				newestSampleSent = atomic.LoadInt64(&c.window[i].requestsSent)
			} else if bSec > priorSampleSec {
				priorSampleSec = bSec
				priorSampleNano = atomic.LoadInt64(&c.window[i].timestampNano)
				priorSampleSent = atomic.LoadInt64(&c.window[i].requestsSent)
			}
		}
	}

	if newestSampleSec >= 0 {
		var baseNano, baseSent int64
		if priorSampleSec >= 0 {
			baseNano = priorSampleNano
			baseSent = priorSampleSent
		} else {
			baseNano = startNano
			baseSent = 0
		}
		deltaSec := float64(newestSampleNano-baseNano) / float64(time.Second)
		deltaSent := newestSampleSent - baseSent
		if deltaSec > 0 && deltaSent >= 0 {
			currentReqPerSec = float64(deltaSent) / deltaSec
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

	isFinite := atomic.LoadInt64(&c.isFinite) != 0
	dirsDisc := atomic.LoadInt64(&c.directoriesDiscovered)
	dirsQueued := atomic.LoadInt64(&c.directoriesQueued)

	return Snapshot{
		RequestsSent:             sent,
		ResponsesReceived:        recv,
		RequestsFiltered:         filt,
		RequestsFailed:           fail,
		RequestsSucceeded:        succ,
		BytesReceived:            bytes,
		ActiveWorkers:            workers,
		TotalWork:                totWork,
		Tried:                    tried,
		Skipped:                  skipped,
		Completed:                completed,
		Progress:                 prog,
		TotalCandidates:          totWork,
		Discovered:               disc,
		InvalidWords:             invalid,
		JobsProduced:             jobs,
		SearchSpaceProgress:      completed,
		StartTime:                startTime,
		StatusCodes:              statusCopy,
		IsFinite:                 isFinite,
		DirectoriesDiscovered:    dirsDisc,
		DirectoriesQueued:        dirsQueued,
		Retries:                  retries,
		Redirects:                redirects,
		BodyInspected:            inspected,
		AverageLatency:           avgLat,
		RequestsPerSecond:        avgReqPerSec,
		CurrentRequestsPerSecond: currentReqPerSec,
		PeakRequestsPerSecond:    peakReqPerSec,
	}
}
