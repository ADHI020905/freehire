package embedprogress

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

func ringOf(depths []int64, step time.Duration) *Ring {
	r := NewRing()
	for i, d := range depths {
		r.Add(Sample{Depth: d, At: t0.Add(time.Duration(i) * step)})
	}
	return r
}

func TestRing_EvenDrain(t *testing.T) {
	// 1000 → 0 over 10 minutes, one sample a minute.
	depths := []int64{1000, 900, 800, 700, 600, 500, 400, 300, 200, 100, 0}
	rate, ok := ringOf(depths, time.Minute).DrainRatePerMinute()
	if !ok || rate != 100 {
		t.Errorf("rate = %v (ok %v), want 100 per minute", rate, ok)
	}
}

func TestRing_AnEnqueueDoesNotCancelTheRate(t *testing.T) {
	// −300 over 3 min, +5000 when the hourly enqueue lands, −300 over 3 min.
	depths := []int64{1000, 900, 800, 700, 5700, 5600, 5500, 5400}
	// Seven one-minute steps, but the jump step drained nothing we can see: 600 over 7.
	rate, ok := ringOf(depths, time.Minute).DrainRatePerMinute()
	if !ok {
		t.Fatal("no rate, want one")
	}
	if rate <= 0 {
		t.Errorf("rate = %v, want positive — an enqueue is not negative progress", rate)
	}
	if want := 600.0 / 7; rate != want {
		t.Errorf("rate = %v, want %v", rate, want)
	}
}

func TestRing_NoRateUnderFiveMinutes(t *testing.T) {
	r := ringOf([]int64{1000, 900, 800}, time.Minute) // spans 2 minutes
	if _, ok := r.DrainRatePerMinute(); ok {
		t.Error("got a rate from 2 minutes of samples, want none")
	}
	if s, ok := r.Latest(); !ok || s.Depth != 800 {
		t.Errorf("Latest = %+v (ok %v), want depth 800", s, ok)
	}
}

func TestRing_ForgetsSamplesOlderThanTheWindow(t *testing.T) {
	// 15 minutes of a fast drain, then 10 minutes of nothing: the rate is the recent one.
	var depths []int64
	d := int64(100000)
	for i := 0; i < 15; i++ {
		depths = append(depths, d)
		d -= 1000
	}
	for i := 0; i < 11; i++ {
		depths = append(depths, d)
	}
	rate, ok := ringOf(depths, time.Minute).DrainRatePerMinute()
	if !ok || rate != 0 {
		t.Errorf("rate = %v (ok %v), want 0 — the fast drain ended over 10 minutes ago", rate, ok)
	}
}

func TestRing_EmptyHasNoLatest(t *testing.T) {
	if _, ok := NewRing().Latest(); ok {
		t.Error("an empty ring reported a latest sample")
	}
}
