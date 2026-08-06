package httpclient

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"sync/atomic"
	"testing"
)

// trackingReadCloser wraps an io.Reader and counts Close() calls.
type trackingReadCloser struct {
	io.Reader
	closeCount int32
}

func (t *trackingReadCloser) Close() error {
	atomic.AddInt32(&t.closeCount, 1)
	return nil
}

func TestDrainAndClose_NilBody(t *testing.T) {
	drained := DrainAndClose(nil, 100)
	if drained != 0 {
		t.Errorf("expected 0 bytes for nil body, got %d", drained)
	}

	drainedResp := DrainAndCloseResponse(nil)
	if drainedResp != 0 {
		t.Errorf("expected 0 bytes for nil response, got %d", drainedResp)
	}

	respNilBody := &http.Response{Body: nil}
	drainedRespNilBody := DrainAndCloseResponse(respNilBody)
	if drainedRespNilBody != 0 {
		t.Errorf("expected 0 bytes for response with nil body, got %d", drainedRespNilBody)
	}
}

func TestDrainAndClose_SmallBody(t *testing.T) {
	data := []byte("hello world small body")
	tracker := &trackingReadCloser{Reader: bytes.NewReader(data)}

	drained := DrainAndClose(tracker, int64(len(data)))
	if drained != int64(len(data)) {
		t.Errorf("expected %d bytes drained, got %d", len(data), drained)
	}
	if atomic.LoadInt32(&tracker.closeCount) != 1 {
		t.Errorf("expected Close() to be called exactly once, got %d", tracker.closeCount)
	}
}

func TestDrainAndClose_ExactDefaultMaxDrainBytes(t *testing.T) {
	data := make([]byte, DefaultMaxDrainBytes)
	for i := range data {
		data[i] = 'A'
	}
	tracker := &trackingReadCloser{Reader: bytes.NewReader(data)}

	drained := DrainAndClose(tracker, int64(len(data)))
	if drained != int64(len(data)) {
		t.Errorf("expected %d bytes drained, got %d", len(data), drained)
	}
	if atomic.LoadInt32(&tracker.closeCount) != 1 {
		t.Errorf("expected Close() to be called exactly once, got %d", tracker.closeCount)
	}
}

func TestDrainAndClose_LargerThanDefaultMaxDrainBytes(t *testing.T) {
	totalSize := int64(DefaultMaxDrainBytes + 1024*1024) // 1MB over limit
	data := make([]byte, totalSize)
	tracker := &trackingReadCloser{Reader: bytes.NewReader(data)}

	drained := DrainAndClose(tracker, totalSize)
	if drained != DefaultMaxDrainBytes {
		t.Errorf("expected %d bytes drained for oversized body, got %d", DefaultMaxDrainBytes, drained)
	}
	if atomic.LoadInt32(&tracker.closeCount) != 1 {
		t.Errorf("expected Close() to be called exactly once, got %d", tracker.closeCount)
	}
}

func TestDrainAndClose_UnknownContentLength(t *testing.T) {
	// Unknown ContentLength (-1) with small body (< DefaultMaxDrainBytes)
	smallData := []byte("small chunked data")
	tracker1 := &trackingReadCloser{Reader: bytes.NewReader(smallData)}
	drained1 := DrainAndClose(tracker1, -1)
	if drained1 != int64(len(smallData)) {
		t.Errorf("expected %d bytes drained, got %d", len(smallData), drained1)
	}
	if atomic.LoadInt32(&tracker1.closeCount) != 1 {
		t.Errorf("expected Close() called once, got %d", tracker1.closeCount)
	}

	// Unknown ContentLength (-1) with huge body (> DefaultMaxDrainBytes)
	hugeData := make([]byte, DefaultMaxDrainBytes*2)
	tracker2 := &trackingReadCloser{Reader: bytes.NewReader(hugeData)}
	drained2 := DrainAndClose(tracker2, -1)
	if drained2 != DefaultMaxDrainBytes {
		t.Errorf("expected %d bytes drained for huge unknown body, got %d", DefaultMaxDrainBytes, drained2)
	}
	if atomic.LoadInt32(&tracker2.closeCount) != 1 {
		t.Errorf("expected Close() called once, got %d", tracker2.closeCount)
	}
}

func TestDrainAndClose_HTTPTrace_ConnectionReuse(t *testing.T) {
	tests := []struct {
		name          string
		bodySize      int
		chunked       bool
		expectedReuse bool
	}{
		{
			name:          "Small Response (512 B) - KeepAlive Reused",
			bodySize:      512,
			chunked:       false,
			expectedReuse: true,
		},
		{
			name:          "Medium 404 Response (4 KB) - KeepAlive Reused",
			bodySize:      4 * 1024,
			chunked:       false,
			expectedReuse: true,
		},
		{
			name:          "Boundary Response (64 KB) - KeepAlive Reused",
			bodySize:      DefaultMaxDrainBytes,
			chunked:       false,
			expectedReuse: true,
		},
		{
			name:          "Small Chunked Response (2 KB) - KeepAlive Reused",
			bodySize:      2 * 1024,
			chunked:       true,
			expectedReuse: true,
		},
		{
			name:          "Oversized Response (256 KB) - Not Reused (Capped Drain)",
			bodySize:      256 * 1024,
			chunked:       false,
			expectedReuse: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bodyData := make([]byte, tc.bodySize)
			for i := range bodyData {
				bodyData[i] = 'Z'
			}

			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				if tc.chunked {
					// Trigger chunked transfer encoding by flushing before writing full payload
					flusher, ok := w.(http.Flusher)
					if ok {
						w.WriteHeader(http.StatusOK)
						flusher.Flush()
					}
				}
				w.Write(bodyData)
			}))
			defer ts.Close()

			client := New(Options{
				MaxWorkers: 2,
			})

			var req1Reused, req2Reused bool

			// Request #1: should establish a new connection
			trace1 := &httptrace.ClientTrace{
				GotConn: func(info httptrace.GotConnInfo) {
					req1Reused = info.Reused
				},
			}
			req1, err := http.NewRequestWithContext(
				httptrace.WithClientTrace(context.Background(), trace1),
				http.MethodGet,
				ts.URL,
				nil,
			)
			if err != nil {
				t.Fatalf("failed to create req1: %v", err)
			}
			resp1, err := client.Do(req1)
			if err != nil {
				t.Fatalf("req1 failed: %v", err)
			}
			DrainAndCloseResponse(resp1)

			if req1Reused {
				t.Errorf("expected Request #1 Reused == false, got true")
			}

			// Request #2: should reuse connection if response was fully drained
			trace2 := &httptrace.ClientTrace{
				GotConn: func(info httptrace.GotConnInfo) {
					req2Reused = info.Reused
				},
			}
			req2, err := http.NewRequestWithContext(
				httptrace.WithClientTrace(context.Background(), trace2),
				http.MethodGet,
				ts.URL,
				nil,
			)
			if err != nil {
				t.Fatalf("failed to create req2: %v", err)
			}
			resp2, err := client.Do(req2)
			if err != nil {
				t.Fatalf("req2 failed: %v", err)
			}
			DrainAndCloseResponse(resp2)

			if req2Reused != tc.expectedReuse {
				t.Errorf("expected Request #2 Reused == %v, got %v", tc.expectedReuse, req2Reused)
			}
		})
	}
}

func TestDrainAndClose_RedirectResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/target", http.StatusFound)
			return
		}
		w.Write([]byte("final destination"))
	}))
	defer ts.Close()

	client := New(Options{
		FollowRedirects: false, // Stop at 302
	})

	var req1Reused, req2Reused bool

	trace1 := &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			req1Reused = info.Reused
		},
	}
	req1, _ := http.NewRequestWithContext(
		httptrace.WithClientTrace(context.Background(), trace1),
		http.MethodGet,
		ts.URL+"/redirect",
		nil,
	)
	resp1, err := client.Do(req1)
	if err != nil {
		t.Fatalf("req1 failed: %v", err)
	}
	DrainAndCloseResponse(resp1)

	if req1Reused {
		t.Errorf("expected Request #1 Reused == false, got true")
	}

	trace2 := &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			req2Reused = info.Reused
		},
	}
	req2, _ := http.NewRequestWithContext(
		httptrace.WithClientTrace(context.Background(), trace2),
		http.MethodGet,
		ts.URL+"/redirect",
		nil,
	)
	resp2, err := client.Do(req2)
	if err != nil {
		t.Fatalf("req2 failed: %v", err)
	}
	DrainAndCloseResponse(resp2)

	if !req2Reused {
		t.Errorf("expected Request #2 after 302 redirect to reuse connection, got Reused == false")
	}
}
