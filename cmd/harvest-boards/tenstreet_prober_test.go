package main

import (
	"context"
	"slices"
	"testing"
)

func TestTenstreetProbe(t *testing.T) {
	p := tenstreetProber{}
	listing := `<html><body>
<span id="carrier_detail_company_name">Covenant Transport</span>
<div onclick="window.location='https://pulse.tenstreet.com/share_app_show_active_jobs.php?active_job_id=1483&company_id=774&portal_user_id=1'">CDL-A Team Driver</div>
<div onclick="window.location='https://pulse.tenstreet.com/share_app_show_active_jobs.php?active_job_id=1483&company_id=774&portal_user_id=1'">CDL-A Team Driver</div>
<div onclick="window.location='https://pulse.tenstreet.com/share_app_show_active_jobs.php?active_job_id=123622&company_id=774&portal_user_id=1'">CDL-A Team Truck Driver</div>
</body></html>`
	getter := fakeGetter{
		"https://pulse.tenstreet.com/share_app_index.php?company_id=774": listing,
		// An unknown company id is answered 200 with an empty profile.
		"https://pulse.tenstreet.com/share_app_index.php?company_id=999": `<html><body><span id="carrier_detail_company_name"></span></body></html>`,
	}
	// The carded-twice posting counts once, and the carrier names itself.
	if name, n, err := p.probe(context.Background(), getter, "774"); err != nil || name != "Covenant Transport" || n != 2 {
		t.Errorf("live: got (%q,%d,%v), want (\"Covenant Transport\",2,nil)", name, n, err)
	}
	if name, n, err := p.probe(context.Background(), getter, "999"); err != nil || name != "" || n != 0 {
		t.Errorf("unknown: got (%q,%d,%v), want (\"\",0,nil)", name, n, err)
	}
	ids, err := p.postingIDs(context.Background(), getter, "774")
	if err != nil || !slices.Equal(ids, []string{"1483", "123622"}) {
		t.Errorf("postingIDs = %v, %v", ids, err)
	}
}
