package embedprogress

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/platform/cache"
)

func TestSnapshot_RoundTrip(t *testing.T) {
	ctx := context.Background()
	c := cache.NewMemory()
	want := Snapshot{Eligible: 10, Covered: 7, MeasuredAt: t0}

	if err := Store(ctx, c, want); err != nil {
		t.Fatalf("Store: %v", err)
	}
	got, ok := Load(ctx, c)
	if !ok || got != want {
		t.Errorf("Load = %+v (ok %v), want %+v", got, ok, want)
	}
}

func TestSnapshot_MissingIsNotZeros(t *testing.T) {
	if _, ok := Load(context.Background(), cache.NewMemory()); ok {
		t.Error("an empty cache read as a snapshot")
	}
	if _, ok := Load(context.Background(), nil); ok {
		t.Error("no cache read as a snapshot")
	}
}
