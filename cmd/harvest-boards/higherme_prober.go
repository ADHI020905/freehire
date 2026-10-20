package main

import (
	"context"
	"net/url"
)

// highermeProber probes a HigherMe brand (board = the 13-hex brand id from
// app.higherme.com/careers/<brand>) with one request: the brand-filtered listing at page size 1,
// whose meta.total is the brand's whole posting count. An unknown brand answers an empty listing,
// not an error, so a zero total is simply not live — and therefore every error is returned, never
// read as absence. The API's firewall blocks an IP that probes faster than ~0.2 req/s, and the
// first prober swallowed that as "0 postings": a 2026-10-20 run reported 29 of 511 brands live
// with probe-failures=0, while the brands it dropped were live from any other address.
//
// The brand resource does carry a name, but not one the corroboration gate can stand on: a brand
// is the CONCEPT ("La Quinta Inn & Suites", "Gil's") while a seed names the operator that runs
// it ("Shular Companies", "Guillory Oil Co."), and the two disagreed on most of the 40 apply links
// sampled 2026-10-20. The prober therefore reports no name and the seed's company labels the
// board — the same call hiringthingProber makes for the same reason.
type highermeProber struct{}

// highermeProbeURL is the brand's listing at page size 1, encoded the way the ingest adapter
// encodes its own listing request.
func highermeProbeURL(brand string) string {
	q := url.Values{
		"page":              {"1"},
		"limit":             {"1"},
		"filters[brand.id]": {brand},
	}
	return "https://api.higherme.com/classic/jobs?" + q.Encode()
}

func (highermeProber) probe(ctx context.Context, c httpClient, brand string) (string, int, error) {
	var resp struct {
		Meta struct {
			Total int `json:"total"`
		} `json:"meta"`
	}
	if err := c.GetJSON(ctx, highermeProbeURL(brand), &resp); err != nil {
		return "", 0, err
	}
	return "", resp.Meta.Total, nil
}
