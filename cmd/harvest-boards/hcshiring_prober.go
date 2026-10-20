package main

import (
	"context"
	"fmt"
	"regexp"
)

// hcshiringProber validates an HCS Hiring board (the <tenant> of <tenant>.hcshiring.com) by
// asking the tenant's public listing how many postings it has. It mirrors the URL shape of
// internal/ingest/sources/hcshiring.go, which is unexported, and costs one request per
// candidate: meta.totalJobs counts the whole board on the first page, while the adapter
// fallback would walk every ten-row page and hydrate every posting's body.
//
// It reports no employer name, so the seed's own supplies it. The platform does publish one
// (/api/jobSearches' companyName), but a tenant is as often one community as the operator
// running it: over 30 sampled boards it disagreed with the seed's employer on a third
// ("Calveria Senior Living" is served as "AgeWell Senior Living", "Country Lane - Beggs" as
// "Diakonos Group"), so wiring it in would reject live boards on the name gate.
type hcshiringProber struct{}

// hcsTenantPattern is one DNS label: the only board shape the adapter addresses. Anything else
// would build a URL on some other host.
var hcsTenantPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

func (hcshiringProber) probe(ctx context.Context, c httpClient, tenant string) (string, int, error) {
	if !hcsTenantPattern.MatchString(tenant) {
		return "", 0, nil
	}
	var page struct {
		Meta struct {
			TotalJobs int `json:"totalJobs"`
		} `json:"meta"`
	}
	// An unknown tenant answers 404 ("The account does not exist.") and the client surfaces it
	// as an error; for harvest that is simply "not a live board" — skip, do not propagate.
	if err := c.GetJSON(ctx, fmt.Sprintf("https://%s.hcshiring.com/api/jobs?page=1", tenant), &page); err != nil {
		return "", 0, nil
	}
	return "", page.Meta.TotalJobs, nil
}
