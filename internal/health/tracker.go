package health

import (
	"sync"
	"time"
)

// BackendTracker records latency and errors for runtime backend health scoring.
type BackendTracker struct {
	mu         sync.RWMutex
	window     time.Duration
	samples    []sample
	maxSamples int
}

type sample struct {
	latencyMs float64
	failed    bool
	at        time.Time
}

func NewBackendTracker(window time.Duration, maxSamples int) *BackendTracker {
	if window <= 0 {
		window = 5 * time.Minute
	}
	if maxSamples <= 0 {
		maxSamples = 100
	}
	return &BackendTracker{window: window, maxSamples: maxSamples}
}

func (t *BackendTracker) Record(latencyMs float64, failed bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.samples = append(t.samples, sample{latencyMs: latencyMs, failed: failed, at: time.Now()})
	if len(t.samples) > t.maxSamples {
		t.samples = t.samples[len(t.samples)-t.maxSamples:]
	}
	t.pruneLocked(time.Now())
}

func (t *BackendTracker) Snapshot() (avgLatencyMs float64, errorRate float64) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	now := time.Now()
	t.pruneLocked(now)
	if len(t.samples) == 0 {
		return 0, 0
	}
	var totalLatency float64
	var failures int
	for _, s := range t.samples {
		totalLatency += s.latencyMs
		if s.failed {
			failures++
		}
	}
	return totalLatency / float64(len(t.samples)), float64(failures) / float64(len(t.samples))
}

func (t *BackendTracker) pruneLocked(now time.Time) {
	cutoff := now.Add(-t.window)
	i := 0
	for _, s := range t.samples {
		if s.at.After(cutoff) {
			t.samples[i] = s
			i++
		}
	}
	t.samples = t.samples[:i]
}
