// Package embedprogress measures how much of the catalogue carries a current semantic
// embedding and how fast the rest is being covered, for the public /embeddings page
// (openspec/changes/embedding-progress-page).
//
// Two measurements with very different costs are combined at read time. Coverage — how
// many postings are eligible and how many are covered — is a scan over every open
// posting that took over a minute on production, so cmd/rollup-stats measures it and
// publishes a Snapshot to the cache; nothing on a request path ever counts it. The
// claimable depth of semantic_outbox is a 4.5ms index read, so cmd/server samples it
// every 15 seconds (Sampler) and keeps ten minutes of samples in memory (Ring), which
// is where the rate comes from. The queue is by construction the eligible postings
// that are not covered, so covered = eligible − queued is current to the last enqueue
// rather than to the last snapshot.
//
// It sits in the search block because the coverage predicate is stated against
// search.CurrentEmbedderModel(), which ai (where internal/ai/embed lives) cannot see.
package embedprogress
