package handler

import (
	"context"
	"errors"
	"log"

	"github.com/strelov1/freehire/internal/search/search"
)

// queryEmbedder is the query-side embedding call. *search.QueryEmbedder satisfies it.
type queryEmbedder interface {
	EmbedQuery(ctx context.Context, text string) ([]float32, error)
}

// semanticSearch is the opt-in hybrid (keyword + description vector) search both agent
// surfaces offer: OJCP's `semantic: true` and /agent/jobs/search's `semantic=true`. One
// value, held by both handlers, so the two cannot disagree about when a request was
// answered by meaning and when it fell back to keywords.
type semanticSearch struct {
	// embedder turns the query into a vector. Nil means no embedding backend is
	// configured, and every request that asks is answered keyword-only and told so.
	embedder queryEmbedder
	// threshold is the ranking-score floor of a hybrid query (see
	// search.SearchParams.ScoreThreshold).
	threshold float64
}

// semanticOutcome is what became of a request's semantic flag.
type semanticOutcome int

const (
	// semanticNotAsked: the flag was absent or false, and the request ran as it always did.
	semanticNotAsked semanticOutcome = iota
	// semanticDropped: the flag was sent but cannot be honoured — no text to embed, or the
	// match sort, which ranks by its own vector and wins in search.buildSearchRequest. Each
	// surface reports the flag the way it reports any dropped param.
	semanticDropped
	// semanticApplied: the hybrid answered.
	semanticApplied
	// semanticUnavailable: the request was answered by keywords alone.
	semanticUnavailable
)

// wire renders the outcome in a surface's own vocabulary: applied or unavailable, and
// nothing when the flag was not asked or was dropped.
func (o semanticOutcome) wire(applied, unavailable string) string {
	switch o {
	case semanticApplied:
		return applied
	case semanticUnavailable:
		return unavailable
	default:
		return ""
	}
}

// search answers params, as a hybrid when asked and possible. It is the one place the
// three agent surfaces decide that, so they cannot disagree.
func (s semanticSearch) search(ctx context.Context, idx searcher, params search.SearchParams, asked bool) (search.SearchResult, semanticOutcome, error) {
	if !asked {
		res, err := idx.Search(ctx, params)
		return res, semanticNotAsked, err
	}
	if params.Query == "" || params.Vector != nil {
		res, err := idx.Search(ctx, params)
		return res, semanticDropped, err
	}
	res, applied, err := s.run(ctx, idx, params)
	if applied {
		return res, semanticApplied, err
	}
	return res, semanticUnavailable, err
}

// run answers params as a hybrid query and reports whether the hybrid is what answered.
//
// Every way the hybrid can fail ends in the keyword search, never an error: the backend
// absent, the embed failing or outlasting its timeout, or the engine refusing the hybrid
// request — which is what a binary querying `desc` before the live index declares it
// gets (see internal/search/search/AGENTS.md). Only a failure the keyword search would
// share — the engine unreachable, or a filter it refuses — reaches the caller.
func (s semanticSearch) run(ctx context.Context, idx searcher, params search.SearchParams) (search.SearchResult, bool, error) {
	if s.embedder == nil {
		res, err := idx.Search(ctx, params)
		return res, false, err
	}
	vec, err := s.embedder.EmbedQuery(ctx, params.Query)
	if err != nil {
		if ctx.Err() != nil {
			return search.SearchResult{}, false, ctx.Err()
		}
		log.Printf("semantic search fell back to keyword: %v", err)
		res, err := idx.Search(ctx, params)
		return res, false, err
	}

	hybrid := params
	hybrid.SemanticVector = vec
	hybrid.ScoreThreshold = s.threshold
	res, err := idx.Search(ctx, hybrid)
	if errors.Is(err, search.ErrBadQuery) {
		log.Printf("hybrid query refused, fell back to keyword: %v", err)
		res, err := idx.Search(ctx, params)
		return res, false, err
	}
	return res, err == nil, err
}
