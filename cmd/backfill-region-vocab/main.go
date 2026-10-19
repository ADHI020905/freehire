// Command backfill-region-vocab folds the off-vocabulary values stored in jobs.regions
// into vocab.RegionValues (freehire#3266).
//
// The regions facet is meant to hold nine values; measured 2026-10-18 the live index
// carried 167 — `us` (18,232), `ru`, `russia`, `us-fl`, `emea`, `gmt+1 to gmt+5`, … —
// because jobderive.Derive took a structured region signal as authoritative without
// checking it against the vocabulary, and backfill-derive feeds a row's STORED regions
// back in as that signal, so a value once written was carried forward forever. Derive now
// folds the signal through location.NormalizeRegions; this pass applies the same fold to
// the rows already stored.
//
// Each candidate is re-derived exactly as backfill-derive would re-derive it — the
// stored countries and regions passed back in as the structured signal — so this pass
// writes nothing backfill-derive would not, only sooner and for a few thousand rows
// rather than the whole table.
//
// Candidates come from Meilisearch: the regions facet distribution names the
// off-vocabulary values, and a search filtered on them names the rows. That covers the
// open, searchable catalogue — the part a visitor or an API client can see. A closed row
// stays as it is until the next backfill-derive pass, which now folds it the same way.
//
// Idempotent (SetJobGeography is IS DISTINCT FROM-guarded): a re-run writes nothing for a
// row already folded, so stopping the pass mid-way costs nothing to resume.
//
// Needs a full `make reindex` afterward: countries/regions are not part of content_hash,
// so a folded row's facet only reaches Meilisearch on a full rebuild — the same gap
// cmd/backfill-clearance documents.
//
// Needs DATABASE_URL, MEILI_URL and MEILI_MASTER_KEY.
package main

import (
	"context"
	"log"
	"slices"
	"time"

	"github.com/strelov1/freehire/internal/dict/vocab"
	"github.com/strelov1/freehire/internal/job/jobderive"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
	"github.com/strelov1/freehire/internal/search/search"
)

// pageSize is how many hits one search request returns.
const pageSize = 1000

// readBatch is how many rows one database round trip fetches. Descriptions are TOASTed,
// so this bounds peak memory more than it bounds query time.
const readBatch = 500

// pauseBetweenBatches lets the host breathe, the same value the sibling geography
// passes use: this pass is never urgent, and it competes with ingest and reindex.
const pauseBetweenBatches = 100 * time.Millisecond

func main() { worker.Main(run) }

func run() int {
	ctx, cfg, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	// Unset means unbounded; a value that fails to parse is an error, not a silent
	// fallback — the same EnvInt64 contract every one-off pass's cap goes through.
	maxPerRun, err := worker.EnvInt64("BACKFILL_REGION_VOCAB_MAX", 0)
	if err != nil {
		log.Printf("backfill-region-vocab: %v", err)
		return 1
	}

	sc := search.NewClient(cfg.MeiliURL, cfg.MeiliKey)

	ids, err := candidateIDs(ctx, sc)
	if err != nil {
		log.Printf("backfill-region-vocab: gather candidates: %v", err)
		return 1
	}
	if len(ids) == 0 {
		log.Print("backfill-region-vocab: no candidates")
		return 0
	}
	if maxPerRun > 0 && int64(len(ids)) > maxPerRun {
		log.Printf("backfill-region-vocab: capping this run at %d of %d candidates", maxPerRun, len(ids))
		ids = ids[:maxPerRun]
	}
	log.Printf("backfill-region-vocab: %d candidates", len(ids))

	q := db.New(pool)
	var read, folded, written int64
	lastLog := time.Now()
	for start := 0; start < len(ids); start += readBatch {
		end := min(start+readBatch, len(ids))
		rows, err := q.JobsForGeographyRecheckByIDs(ctx, ids[start:end])
		if err != nil {
			log.Printf("backfill-region-vocab: read %d..%d after %d written: %v", start, end, written, err)
			return 1
		}
		for _, r := range rows {
			read++
			// The index can lag Postgres by a search-drain cycle, so the row actually
			// read decides, not the candidate query.
			if !offVocabulary(r.Regions) {
				continue
			}
			countries, regions := refold(r)
			folded++
			n, err := q.SetJobGeography(ctx, db.SetJobGeographyParams{ID: r.ID, Countries: countries, Regions: regions})
			if err != nil {
				log.Printf("backfill-region-vocab: write id=%d after %d written: %v", r.ID, written, err)
				return 1
			}
			written += n
		}
		if time.Since(lastLog) >= time.Minute {
			log.Printf("backfill-region-vocab: progress read=%d folded=%d written=%d of %d", read, folded, written, len(ids))
			lastLog = time.Now()
		}
		select {
		case <-ctx.Done():
			log.Printf("backfill-region-vocab: cancelled after %d written, resume by re-running", written)
			return 1
		case <-time.After(pauseBetweenBatches):
		}
	}
	log.Printf("backfill-region-vocab: done, read=%d folded=%d written=%d (follow with a reindex)", read, folded, written)
	return 0
}

// offVocabulary reports whether any stored region falls outside vocab.RegionValues.
func offVocabulary(regions []string) bool {
	for _, r := range regions {
		if !slices.Contains(vocab.RegionValues, r) {
			return true
		}
	}
	return false
}

// refold re-derives a row's geography the way backfill-derive does — its stored
// countries and regions passed back in as the structured signal — so the result is
// exactly what that pass would now write. jobderive is pure.
func refold(r db.JobsForGeographyRecheckByIDsRow) (countries, regions []string) {
	d := jobderive.Derive(jobderive.Input{
		Title:       r.Title,
		Location:    r.Location,
		Description: r.Description,
		Countries:   r.Countries,
		Regions:     r.Regions,
	})
	return d.Countries, d.Regions
}

// candidateIDs names the indexed jobs carrying at least one off-vocabulary region: the
// facet distribution lists the values, and one OR group over them selects the rows.
func candidateIDs(ctx context.Context, sc *search.Client) ([]int64, error) {
	dist, err := sc.FacetCounts(ctx, search.FacetParams{Facets: []string{"regions"}})
	if err != nil {
		return nil, err
	}
	var group []string
	for v := range dist.Facets["regions"] {
		if !slices.Contains(vocab.RegionValues, v) {
			group = append(group, search.Eq("regions", v))
		}
	}
	if len(group) == 0 {
		return nil, nil
	}
	log.Printf("backfill-region-vocab: %d off-vocabulary region values in the index", len(group))

	var ids []int64
	filter := search.Filter(group)
	for offset := 0; ; offset += pageSize {
		res, err := sc.Search(ctx, search.SearchParams{Filter: filter, Limit: pageSize, Offset: offset})
		if err != nil {
			return nil, err
		}
		for _, hit := range res.Hits {
			ids = append(ids, hit.ID)
		}
		if len(res.Hits) < pageSize {
			break
		}
	}
	return ids, nil
}
