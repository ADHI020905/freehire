package sources

import (
	"context"
	"testing"
)

// ttOrbioFeed is orbioworld.teamtailor.com/jobs.rss, captured 2026-10-18 and cut to the one
// item behind the report (descriptions truncated). The job page states "Remote status: Hybrid"
// only in a <dl> whose label and value are in the career site's own language ("Status
// Homeoffice/Vor Ort" on a German site) and omits it entirely for on-site; its ld+json
// carries no jobLocationType at all. The feed's <remoteStatus> is the same field as a closed,
// unlocalized enum — none/onsite/hybrid/temporary/fully.
const ttOrbioFeed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:tt="https://teamtailor.com/locations">
  <channel>
    <title>Orbio World</title>
    <description>Current job openings</description>
    <link>https://orbioworld.teamtailor.com/jobs</link>
    <item>
      <title>QA Team Lead</title>
      <description>&lt;p&gt;truncated&lt;/p&gt;</description>
      <pubDate>Fri, 25 Sep 2026 13:07:00 +0300</pubDate>
      <link>https://orbioworld.teamtailor.com/jobs/8454889-qa-team-lead</link>
      <remoteStatus>hybrid</remoteStatus>
      <guid>bee47895-c4b2-444d-a565-13059b311709</guid>
      <tt:locations>
        <tt:location>
          <tt:name>Kaunas</tt:name>
          <tt:address>K. Donelaičio g. 60</tt:address>
          <tt:zip>44248</tt:zip>
          <tt:city>Kaunas</tt:city>
          <tt:country>Lithuania</tt:country>
        </tt:location>
      </tt:locations>
      <tt:department>IT</tt:department>
      <tt:role/>
    </item>
  </channel>
</rss>`

// The reported posting (job_reports 2026-10): served as remote because its perks line ("one
// work-from-home day each week") matched the description heuristic, while Teamtailor itself
// files it as hybrid.
func TestTeamtailorWorkModeFromFeedRemoteStatus(t *testing.T) {
	jobURL := "https://orbioworld.teamtailor.com/jobs/8454889-qa-team-lead"
	fake := (&routedHTTP{}).
		route("jobs?page=1", ttListingHTML(jobURL)).
		route("jobs?page=2", ttListingHTML()).
		route("jobs.rss", ttOrbioFeed).
		route("/jobs/8454889", ttDetailHTML("QA Team Lead",
			"&lt;p&gt;enjoy one work-from-home day each week&lt;/p&gt;",
			"2026-09-25T13:07:00+03:00", "Kaunas", "LT", ""))

	jobs, err := NewTeamtailor(fake).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "Orbio World", Board: "orbioworld.teamtailor.com"},
		func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	if jobs[0].WorkMode != "hybrid" {
		t.Errorf("WorkMode = %q, want hybrid", jobs[0].WorkMode)
	}
	if jobs[0].Remote {
		t.Error("a hybrid posting must not be flagged remote")
	}
}

func TestTTRemoteStatusMode(t *testing.T) {
	for in, want := range map[string]string{
		"fully": "remote", "hybrid": "hybrid", "onsite": "onsite",
		// "none" is the default nobody set, and "temporary" is remote only for now — neither
		// states the arrangement the posting is hired into.
		"none": "", "temporary": "", "": "",
	} {
		if got := ttRemoteStatusMode(in); got != want {
			t.Errorf("ttRemoteStatusMode(%q) = %q, want %q", in, got, want)
		}
	}
}

// The feed is a facet source, not the listing: a site that serves none (a root-listing
// career site answers jobs.rss with a 404) still crawls, its postings carrying whatever the
// page itself states — here jobLocationType TELECOMMUTE, which Teamtailor emits for a
// "fully" remote posting (adaptiveteams.teamtailor.com/jobs/8552556, verified 2026-10-18).
func TestTeamtailorFeedFailureIsNotABoardFailure(t *testing.T) {
	jobURL := "https://adaptiveteams.teamtailor.com/jobs/8552556-social-media-community-manager"
	fake := (&routedHTTP{}).
		route("jobs?page=1", ttListingHTML(jobURL)).
		route("jobs?page=2", ttListingHTML()).
		route("/jobs/8552556", ttDetailHTML("Social Media &amp; Community Manager", "&lt;p&gt;x&lt;/p&gt;",
			"2026-10-15T13:04:00+00:00", "", "BR", "TELECOMMUTE"))

	jobs, err := NewTeamtailor(fake).Fetch(context.Background(),
		CompanyEntry{Company: "Adaptive Teams", Board: "adaptiveteams.teamtailor.com"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	if jobs[0].WorkMode != "remote" || !jobs[0].Remote {
		t.Errorf("WorkMode = %q, Remote = %v; want remote from jobLocationType TELECOMMUTE",
			jobs[0].WorkMode, jobs[0].Remote)
	}
}
