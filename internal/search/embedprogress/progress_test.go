package embedprogress

import (
	"encoding/json"
	"testing"
	"time"
)

func TestBuild_EverythingMeasured(t *testing.T) {
	snap := &Snapshot{Eligible: 1000, Covered: 600, MeasuredAt: t0}
	latest := &Sample{Depth: 300, At: t0.Add(time.Hour)}
	rate := 50.0

	p := Build(snap, latest, &rate)

	if *p.Eligible != 1000 || *p.Covered != 700 || *p.SnapshotCovered != 600 || *p.Queued != 300 {
		t.Errorf("figures = %+v, want eligible 1000, covered 700 (1000-300), snapshot_covered 600, queued 300", p)
	}
	if *p.RatePerMinute != 50 || *p.ETASeconds != 360 {
		t.Errorf("rate/eta = %v/%v, want 50 and 360 (300 / 50 per minute)", *p.RatePerMinute, *p.ETASeconds)
	}
	if !p.SnapshotAt.Equal(t0) || !p.MeasuredAt.Equal(t0.Add(time.Hour)) {
		t.Errorf("times = %v/%v", p.SnapshotAt, p.MeasuredAt)
	}
}

func TestBuild_CoveredIsClamped(t *testing.T) {
	snap := &Snapshot{Eligible: 100, Covered: 90, MeasuredAt: t0}
	// More queued than eligible — closed postings waiting to be cleared ride in the queue too.
	p := Build(snap, &Sample{Depth: 250, At: t0}, nil)
	if *p.Covered != 0 {
		t.Errorf("covered = %d, want 0, never negative", *p.Covered)
	}
}

func TestBuild_NoETAWithoutAPositiveRate(t *testing.T) {
	zero := 0.0
	p := Build(nil, &Sample{Depth: 10, At: t0}, &zero)
	if p.ETASeconds != nil {
		t.Errorf("eta = %v, want none at rate 0", *p.ETASeconds)
	}
}

// Every unmeasured figure is null on the wire, never 0 — a zero reaching a page reads
// as a measurement.
func TestBuild_UnmeasuredIsNull(t *testing.T) {
	raw, err := json.Marshal(Build(nil, nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"eligible", "covered", "snapshot_covered", "queued", "rate_per_minute", "eta_seconds", "snapshot_at", "measured_at"} {
		v, present := m[k]
		if !present || v != nil {
			t.Errorf("%s = %v (present %v), want null", k, v, present)
		}
	}
}

func TestBuild_QueueWithoutSnapshot(t *testing.T) {
	p := Build(nil, &Sample{Depth: 42, At: t0}, nil)
	if p.Queued == nil || *p.Queued != 42 {
		t.Errorf("queued = %v, want 42 even with no snapshot", p.Queued)
	}
	if p.Eligible != nil || p.Covered != nil {
		t.Error("eligible/covered set without a snapshot")
	}
}
