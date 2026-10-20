package sources

import (
	"context"
	"encoding/xml"
	"errors"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

// taleobeFake serves one body for the feed request and reports final as the URL the request
// ended on — the org's pod feed, as the live dispatcher redirects to, unless a test declares
// another landing page.
type taleobeFake struct {
	body   string
	final  string
	gotURL string
}

func (f *taleobeFake) GetXMLResolved(_ context.Context, url string, v any) (string, error) {
	f.gotURL = url
	return f.final, xml.Unmarshal([]byte(f.body), v)
}

func taleobeFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/taleobe/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestTaleoBEProvider(t *testing.T) {
	if got := NewTaleoBE(nil).Provider(); got != "taleobe" {
		t.Errorf("Provider() = %q, want taleobe", got)
	}
}

// The request goes through the dispatcher, never to a pod: the org's pod is not part of the
// board, and the dispatcher is what finds it.
func TestTaleoBEFeedURL(t *testing.T) {
	got, err := TaleoBEFeedURL("AMERINC:52")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if u.Host != "tbe.taleo.net" || u.Path != "/dispatcher/servlet/DispatcherServlet" {
		t.Errorf("feed URL %q is not the dispatcher", got)
	}
	q := u.Query()
	if q.Get("org") != "AMERINC" || q.Get("act") != "redirectOrg" ||
		q.Get("redirectUrl") != "https://tbe.taleo.net/ats/servlet/Rss?cws=52&org=AMERINC" {
		t.Errorf("feed URL %q has query %v", got, q)
	}

	for _, bad := range []string{"", "AMERINC", "AMERINC:", ":52", "AMERINC:x", "amerinc:52", "phh02/AMERINC:52"} {
		if _, err := TaleoBEFeedURL(bad); err == nil {
			t.Errorf("TaleoBEFeedURL(%q) accepted a malformed board", bad)
		}
	}
}

// The fixture is a live feed recorded 2026-10-20, byte for byte.
func TestTaleoBEFetch(t *testing.T) {
	fake := &taleobeFake{
		body:  taleobeFixture(t, "amerinc_52.xml"),
		final: "https://phh.tbe.taleo.net/phh02/ats/servlet/Rss?org=AMERINC&cws=52",
	}
	jobs, err := NewTaleoBE(fake).Fetch(context.Background(), CompanyEntry{
		Provider: "taleobe", Board: "AMERINC:52", Company: "AmeriHealth Caritas",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 11 {
		t.Fatalf("len(jobs) = %d, want 11", len(jobs))
	}

	j := jobs[0]
	if j.ExternalID != "45049" ||
		j.URL != "https://phh.tbe.taleo.net/phh02/ats/careers/requisition.jsp?org=AMERINC&cws=52&rid=45049" ||
		j.Title != "Network Contractor - Provider Network Management (PNM)" ||
		j.Company != "AmeriHealth Caritas" ||
		j.Location != "Baton Rouge, LA" || j.Remote {
		t.Errorf("first job = %+v", j)
	}
	if !reflect.DeepEqual(j.Countries, []string{"us"}) {
		t.Errorf("Countries = %v, want [us]", j.Countries)
	}
	if want := time.Date(2026, 10, 20, 13, 12, 18, 0, time.UTC); j.PostedAt == nil || !j.PostedAt.Equal(want) {
		t.Errorf("PostedAt = %v, want %v", j.PostedAt, want)
	}
	// The body is the taleo:html-description (markup), not the flattened plain-text
	// <description> beside it.
	if len(j.Description) < 500 || !strings.Contains(j.Description, "<p") {
		t.Errorf("description = %.200q, want the full HTML body", j.Description)
	}

	// "Remote, United States" is the career site's own place label for a remote posting.
	remote := jobs[1]
	if remote.Location != "Remote, United States" || !remote.Remote {
		t.Errorf("remote job location %q remote %v", remote.Location, remote.Remote)
	}
}

// A posting with no structured address keeps the career site's own place label and states no
// country — "Nairobi, Kenya" is free text, left to the pipeline's location dictionary.
func TestTaleoBEFetchWithoutStructuredAddress(t *testing.T) {
	fake := &taleobeFake{body: taleobeFixture(t, "ifpri_50.xml")}
	jobs, err := NewTaleoBE(fake).Fetch(context.Background(), CompanyEntry{
		Provider: "taleobe", Board: "IFPRI:50", Company: "IFPRI",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("len(jobs) = %d, want 3", len(jobs))
	}
	if jobs[0].ExternalID != "3841" || jobs[0].Location != "Nairobi, Kenya" || jobs[0].Countries != nil {
		t.Errorf("first job = %+v", jobs[0])
	}
}

// A career site with no open postings publishes a feed with an empty channel.
func TestTaleoBEFetchEmptyBoard(t *testing.T) {
	fake := &taleobeFake{body: taleobeFixture(t, "sigsource_37.xml")}
	jobs, err := NewTaleoBE(fake).Fetch(context.Background(), CompanyEntry{Provider: "taleobe", Board: "SIGSOURCE:37"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Errorf("len(jobs) = %d, want 0", len(jobs))
	}
}

// Each of the platform's three "no such board" answers is a redirect to an HTML page, and is
// reported as ErrBoardGone rather than as the XML syntax error it causes.
func TestTaleoBEBoardGone(t *testing.T) {
	cases := map[string]string{
		"unknown org":         "https://phf.tbe.taleo.net/dispatcher/login.jsp?loginErrorMsg=x",
		"deactivated org":     "https://tbe.taleo.net/errorpages/page_not_found_404.html.en",
		"unknown career site": "https://phf.tbe.taleo.net/phf02/ats/rssErrMsg.jsp?org=ADELPHI&orgCode=ADELPHI",
	}
	for name, final := range cases {
		t.Run(name, func(t *testing.T) {
			fake := &taleobeFake{body: "<!DOCTYPE html><html><head><link rel=x></head></html>", final: final}
			_, err := NewTaleoBE(fake).Fetch(context.Background(), CompanyEntry{Provider: "taleobe", Board: "ADELPHI:1"})
			if !errors.Is(err, ErrBoardGone) {
				t.Errorf("err = %v, want ErrBoardGone", err)
			}
		})
	}

	// A request that got no response at all is a transport failure, not a verdict.
	fake := &taleobeFake{body: "<html", final: ""}
	_, err := NewTaleoBE(fake).Fetch(context.Background(), CompanyEntry{Provider: "taleobe", Board: "ADELPHI:43"})
	if err == nil || errors.Is(err, ErrBoardGone) {
		t.Errorf("err = %v, want a plain failure", err)
	}
}

func TestTaleoBEReqIDFromLink(t *testing.T) {
	if got := taleobeReqID("https://phf.tbe.taleo.net/phf02/ats/careers/requisition.jsp?org=ADELPHI&cws=43&rid=3413"); got != "3413" {
		t.Errorf("taleobeReqID = %q, want 3413", got)
	}
	if got := taleobeReqID("https://phf.tbe.taleo.net/phf02/ats/careers/v2/searchResults?org=ADELPHI&cws=43"); got != "" {
		t.Errorf("taleobeReqID = %q, want empty", got)
	}
}
