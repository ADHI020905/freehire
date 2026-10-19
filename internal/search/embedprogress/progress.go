package embedprogress

import (
	"math"
	"time"
)

// Progress is the document the JSON endpoint and the stream serve. Every field is a
// pointer because every one of them can be unmeasured — no snapshot yet, a server that
// has not sampled, a rate not yet earned — and unmeasured must reach the wire as null:
// a 0 here would be read as a measurement.
type Progress struct {
	Eligible        *int64     `json:"eligible"`
	Covered         *int64     `json:"covered"`
	SnapshotCovered *int64     `json:"snapshot_covered"`
	Queued          *int64     `json:"queued"`
	RatePerMinute   *float64   `json:"rate_per_minute"`
	ETASeconds      *int64     `json:"eta_seconds"`
	SnapshotAt      *time.Time `json:"snapshot_at"`
	MeasuredAt      *time.Time `json:"measured_at"`
}

// Build combines what is known into one document. covered is eligible − queued clamped
// to [0, eligible]: the queue also carries closed postings waiting to have their state
// cleared, so it can exceed what is eligible, and the error then runs toward claiming
// less progress, not more. The snapshot's own covered count travels beside it so that a
// live figure a recent measurement contradicts is visible — which is what an empty
// queue with nothing enqueuing looks like.
func Build(snap *Snapshot, latest *Sample, ratePerMinute *float64) Progress {
	var p Progress
	if snap != nil {
		p.Eligible = &snap.Eligible
		p.SnapshotCovered = &snap.Covered
		p.SnapshotAt = &snap.MeasuredAt
	}
	if latest != nil {
		p.Queued = &latest.Depth
		p.MeasuredAt = &latest.At
	}
	if snap != nil && latest != nil {
		covered := min(max(snap.Eligible-latest.Depth, 0), snap.Eligible)
		p.Covered = &covered
	}
	if ratePerMinute != nil {
		p.RatePerMinute = ratePerMinute
		if latest != nil && *ratePerMinute > 0 {
			eta := int64(math.Round(float64(latest.Depth) / *ratePerMinute * 60))
			p.ETASeconds = &eta
		}
	}
	return p
}
