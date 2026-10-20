package main

import (
	"context"
	"testing"
)

func TestApplicantStackProbe(t *testing.T) {
	p := applicantstackProber{}
	// The pager's total is the board's whole count, past the 100 rows a page carries.
	paged := `<html><body><table id="data-table"><tbody>
<tr><td><a href="https://acme.applicantstack.com/x/detail/a2aaaaaaaaaa">Cook</a></td></tr>
</tbody></table><div><div class="tablepager">Showing 1 - 100 of 282 | <a href="javascript:dopage(2)">2</a></div></div></body></html>`
	empty := `<html><body><table id="data-table"><thead><tr><th>Title</th></tr></thead><tbody>
<tr><td>Sorry, but there are no current job openings at this time.</td></tr></tbody></table></body></html>`
	marketing := `<html><body><h1>Job not found</h1></body></html>`
	getter := fakeGetter{
		"https://acme.applicantstack.com/x/openings":  paged,
		"https://empty.applicantstack.com/x/openings": empty,
		"https://gone.applicantstack.com/x/openings":  marketing,
	}
	if name, n, err := p.probe(context.Background(), getter, "Acme"); err != nil || name != "" || n != 282 {
		t.Errorf("live: got (%q,%d,%v), want (\"\",282,nil)", name, n, err)
	}
	for _, slug := range []string{"empty", "gone", "unreachable"} {
		if _, n, err := p.probe(context.Background(), getter, slug); err != nil || n != 0 {
			t.Errorf("%s: got n=%d err=%v, want 0,nil", slug, n, err)
		}
	}
}
