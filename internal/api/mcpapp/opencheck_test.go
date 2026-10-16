package mcpapp

import (
	"context"
	"fmt"
	"testing"
)

func TestOpenCheckAnswersEverySlugItCheckedAndNamesTheRest(t *testing.T) {
	// A shortlist longer than the cap is not refused and not silently cut: the slugs beyond it
	// are named, so the model can say "checked 50, 10 left" instead of implying it saw all 60.
	// The same doctrine as ignored_params.
	slugs := make([]string, maxOpenChecks+10)
	for i := range slugs {
		slugs[i] = fmt.Sprintf("job-%d", i)
	}
	var asked []string
	r := readerThatFinds(t)
	inner := r.checkJobsOpen
	r.checkJobsOpen = func(ctx context.Context, s []string) ([]JobOpenState, error) {
		asked = s
		return inner(ctx, s)
	}

	var got JobsOpenResult
	callJSON(t, connect(t, r), toolCheckJobsOpen, map[string]any{"slugs": slugs}, &got)

	if len(asked) != maxOpenChecks {
		t.Errorf("the reader was asked about %d slugs, want the cap of %d", len(asked), maxOpenChecks)
	}
	if len(got.Jobs) != maxOpenChecks || len(got.Unchecked) != 10 {
		t.Errorf("answered %d and left %d unchecked, want %d and 10", len(got.Jobs), len(got.Unchecked), maxOpenChecks)
	}
	if got.Unchecked[0] != fmt.Sprintf("job-%d", maxOpenChecks) {
		t.Errorf("unchecked starts at %q, want the first slug past the cap", got.Unchecked[0])
	}
}

func TestOpenCheckAsksOnceForARepeatedSlug(t *testing.T) {
	var asked []string
	r := readerThatFinds(t)
	inner := r.checkJobsOpen
	r.checkJobsOpen = func(ctx context.Context, s []string) ([]JobOpenState, error) {
		asked = s
		return inner(ctx, s)
	}

	var got JobsOpenResult
	callJSON(t, connect(t, r), toolCheckJobsOpen, map[string]any{"slugs": []string{"a", "b", "a"}}, &got)

	if len(asked) != 2 || len(got.Jobs) != 2 {
		t.Errorf("asked %v and answered %d, want each slug once", asked, len(got.Jobs))
	}
}
