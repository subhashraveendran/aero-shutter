package importer

import (
	"sync"
	"time"
)

// speedometer measures transfer speed over a rolling time window using
// cumulative byte counters.
type speedometer struct {
	mu      sync.Mutex
	window  time.Duration
	samples []speedSample
}

type speedSample struct {
	t     time.Time
	total int64
}

func newSpeedometer(window time.Duration) *speedometer {
	return &speedometer{window: window}
}

// add records the cumulative number of bytes transferred for the current
// file. Totals may reset between files; resets are detected and bridged.
func (s *speedometer) add(cumulative int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if n := len(s.samples); n > 0 && cumulative < s.samples[n-1].total {
		// New file started: rebase so deltas stay monotonic.
		base := s.samples[n-1].total
		s.samples = append(s.samples, speedSample{t: now, total: base + cumulative})
	} else {
		s.samples = append(s.samples, speedSample{t: now, total: cumulative})
	}
	s.trim(now)
}

func (s *speedometer) trim(now time.Time) {
	cutoff := now.Add(-s.window)
	i := 0
	for i < len(s.samples)-1 && s.samples[i].t.Before(cutoff) {
		i++
	}
	s.samples = s.samples[i:]
}

// rate returns the rolling average speed in bytes per second.
func (s *speedometer) rate() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.samples) < 2 {
		return 0
	}
	first, last := s.samples[0], s.samples[len(s.samples)-1]
	dt := last.t.Sub(first.t).Seconds()
	if dt <= 0 {
		return 0
	}
	return float64(last.total-first.total) / dt
}
