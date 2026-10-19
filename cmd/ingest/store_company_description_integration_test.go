//go:build integration

// Integration test for dbStore.RecordCompanyDescription and CompanyDescriptionDue — the ingest-time company-info
// write path a sources.CompanyDescriber adapter (Greenhouse today) feeds. Run with:
// go test -tags=integration ./cmd/ingest/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestFillCompanyDescription_NewCompanyIsInsertedAsNonReference(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE companies RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	store := newDBStore(pool, 1, nil, nil, defaultSeenPolicy())

	if err := store.RecordCompanyDescription(ctx, "coinbase", "Coinbase", "Ready to be pushed beyond what you think you're capable of?"); err != nil {
		t.Fatalf("RecordCompanyDescription: %v", err)
	}

	var isReference bool
	var tagline pgtype.Text
	var companyInfo []byte
	if err := pool.QueryRow(ctx,
		`SELECT is_reference, tagline, company_info FROM companies WHERE slug = 'coinbase'`).Scan(&isReference, &tagline, &companyInfo); err != nil {
		t.Fatalf("select: %v", err)
	}
	if isReference {
		t.Error("is_reference = true, want false for an ingest-discovered company")
	}
	if tagline.Valid && tagline.String != "" {
		t.Errorf("tagline = %q, want unset — this source writes only company_info.summary, never tagline", tagline.String)
	}
	var info map[string]string
	if err := json.Unmarshal(companyInfo, &info); err != nil {
		t.Fatalf("company_info not valid JSON: %v", err)
	}
	if info["summary"] == "" {
		t.Error("company_info.summary missing")
	}
}

func TestFillCompanyDescription_NeverOverwritesAnotherSourcesTagline(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE companies RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO companies (slug, name, tagline) VALUES ('acme', 'Acme', 'Tagline from YC')`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	store := newDBStore(pool, 1, nil, nil, defaultSeenPolicy())

	if err := store.RecordCompanyDescription(ctx, "acme", "Acme", "From the Greenhouse board page"); err != nil {
		t.Fatalf("RecordCompanyDescription: %v", err)
	}

	var tagline string
	if err := pool.QueryRow(ctx, `SELECT tagline FROM companies WHERE slug = 'acme'`).Scan(&tagline); err != nil {
		t.Fatalf("select: %v", err)
	}
	if tagline != "Tagline from YC" {
		t.Errorf("tagline = %q, want the stored one preserved", tagline)
	}
}

// TestCompanyDescriptionDue_SkipsFilledAndRecentlyChecked pins the gate that stops the
// crawl re-asking a platform for a company's description on every run (measured on prod
// 2026-10-19: half of Workable's ~63k requests a day). A company is due only while it has
// no summary AND no answered fetch since the bound; a blank answer stamps the attempt.
func TestCompanyDescriptionDue_SkipsFilledAndRecentlyChecked(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE companies RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	store := newDBStore(pool, 1, nil, nil, defaultSeenPolicy())
	window := time.Now().Add(-7 * 24 * time.Hour)

	due := func(slug string, since time.Time) bool {
		t.Helper()
		d, err := store.CompanyDescriptionDue(ctx, slug, since)
		if err != nil {
			t.Fatalf("CompanyDescriptionDue(%s): %v", slug, err)
		}
		return d
	}

	if !due("never-seen", window) {
		t.Error("a slug with no row is not due, want due")
	}

	if err := store.RecordCompanyDescription(ctx, "blank", "Blank", ""); err != nil {
		t.Fatalf("record blank: %v", err)
	}
	if due("blank", window) {
		t.Error("a blank answer recorded just now is still due, want not due inside the window")
	}
	if !due("blank", time.Now().Add(time.Minute)) {
		t.Error("a blank answer older than the bound is not due, want due again")
	}
	var info []byte
	var infoAt pgtype.Timestamptz
	if err := pool.QueryRow(ctx, `SELECT company_info, company_info_at FROM companies WHERE slug = 'blank'`).Scan(&info, &infoAt); err != nil {
		t.Fatalf("select blank: %v", err)
	}
	if string(info) != "{}" || infoAt.Valid {
		t.Errorf("blank answer wrote company_info=%s company_info_at=%v, want neither touched", info, infoAt)
	}

	if err := store.RecordCompanyDescription(ctx, "filled", "Filled", "About us"); err != nil {
		t.Fatalf("record filled: %v", err)
	}
	if due("filled", time.Now().Add(time.Minute)) {
		t.Error("a company holding a summary is due, want never due (the fill could not change it)")
	}

	// A summary another source stored counts the same: the ingest fill keeps it.
	if _, err := pool.Exec(ctx,
		`INSERT INTO companies (slug, name, company_info) VALUES ('other', 'Other', '{"summary":"From elsewhere"}')`); err != nil {
		t.Fatalf("seed other: %v", err)
	}
	if due("other", window) {
		t.Error("a company with a summary from another source is due, want not due")
	}
}
