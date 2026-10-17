//go:build integration

package db

import (
	"context"
	"testing"
	"time"
)

func TestGetTalentNetworkVisibility_MissingExtractStampIsNotListed(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	id := seedAccount(t, pool, "listed-null@example.test")
	if _, err := pool.Exec(ctx, `
		UPDATE users SET
			talent_network_visibility = 'anonymous',
			talent_handle = 'backend-ab12',
			resume_uploaded_at = $2
		WHERE id = $1`, id, time.Now().UTC()); err != nil {
		t.Fatalf("seed member: %v", err)
	}

	row, err := q.GetTalentNetworkVisibility(ctx, id)
	if err != nil {
		t.Fatalf("GetTalentNetworkVisibility: %v", err)
	}
	if row.Listed {
		t.Error("listed = true for a member whose extract stamp is missing")
	}

	if _, err := pool.Exec(ctx, `
		UPDATE users SET resume_structured_uploaded_at = resume_uploaded_at WHERE id = $1`, id); err != nil {
		t.Fatalf("stamp extract: %v", err)
	}
	row, err = q.GetTalentNetworkVisibility(ctx, id)
	if err != nil {
		t.Fatalf("GetTalentNetworkVisibility after stamp: %v", err)
	}
	if !row.Listed {
		t.Error("listed = false once the extract stamp matches the CV")
	}
}
