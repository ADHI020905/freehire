package search

import (
	"encoding/json"
	"fmt"

	"github.com/meilisearch/meilisearch-go"
)

// buildFederatedSearch turns a q with several OR alternatives into ONE federated
// multi-search over the jobs index. Each alternative carries the caller's filter,
// sort and vector; federation pages, deduplicates and ranks the merged list. The
// per-query limit/offset are cleared because Meilisearch rejects them inside a
// federation, which is where paging lives.
func buildFederatedSearch(p SearchParams, alts []Alternative) *meilisearch.MultiSearchRequest {
	queries := make([]*meilisearch.SearchRequest, 0, len(alts))
	for _, a := range alts {
		q := p
		if a.Fields != nil {
			q.QFields = a.Fields
		}
		req := buildSearchRequest(q)
		req.IndexUID = facetIndexUID
		req.Query = a.Text
		req.Limit, req.Offset = 0, 0
		queries = append(queries, req)
	}
	return &meilisearch.MultiSearchRequest{
		Federation: &meilisearch.MultiSearchFederation{Limit: int64(p.Limit), Offset: int64(p.Offset)},
		Queries:    queries,
	}
}

// buildFederatedFacets is the facet-count counterpart of buildFederatedSearch:
// the same alternatives under one filter, with the facets requested from the
// federation (per-query facets are a 400 there) and merged per document.
// MaxValuesPerFacet must match the index setting: federation's own default is
// 100 and it truncates silently, the failure maxValuesPerFacet exists to avoid.
// Limit 1, not 0: the SDK omits a zero limit, which federation reads as 20 hits.
func buildFederatedFacets(alts []Alternative, filter any, facets []string) *meilisearch.MultiSearchRequest {
	queries := make([]*meilisearch.SearchRequest, 0, len(alts))
	for _, a := range alts {
		queries = append(queries, &meilisearch.SearchRequest{
			IndexUID:             facetIndexUID,
			Query:                a.Text,
			Filter:               filter,
			AttributesToSearchOn: a.Fields,
		})
	}
	fed := &meilisearch.MultiSearchFederation{Limit: 1}
	if len(facets) > 0 {
		fed.FacetsByIndex = map[string][]string{facetIndexUID: facets}
		fed.MergeFacets = &meilisearch.MultiSearchFederationMergeFacets{MaxValuesPerFacet: maxValuesPerFacet}
	}
	return &meilisearch.MultiSearchRequest{Federation: fed, Queries: queries}
}

// federatedAsSearchResponse reshapes a merged federated response into the
// single-index response buildFacetResult already decodes, so the facet paths
// share one decoder whichever way the query ran.
func federatedAsSearchResponse(resp *meilisearch.MultiSearchResponse) (*meilisearch.SearchResponse, error) {
	out := &meilisearch.SearchResponse{Hits: resp.Hits, EstimatedTotalHits: resp.EstimatedTotalHits}
	if len(resp.FacetDistribution) > 0 {
		raw, err := json.Marshal(resp.FacetDistribution)
		if err != nil {
			return nil, fmt.Errorf("search: re-encode merged facets: %w", err)
		}
		out.FacetDistribution = raw
	}
	if len(resp.FacetStats) > 0 {
		raw, err := json.Marshal(resp.FacetStats)
		if err != nil {
			return nil, fmt.Errorf("search: re-encode merged facet stats: %w", err)
		}
		out.FacetStats = raw
	}
	return out, nil
}
