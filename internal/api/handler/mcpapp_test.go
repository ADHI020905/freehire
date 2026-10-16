package handler

import (
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/api/mcpapp"
	"github.com/strelov1/freehire/internal/job/ghost"
	"github.com/strelov1/freehire/internal/job/jobreality"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

func mcpappUnder(s searcher, cs companySearcher, store mcpappStore) *mcpappHandlers {
	return newMCPAppHandlers(s, cs, store, "https://freehire.me")
}

func TestTheAppSearchRunsTheSameQueryTheWebsiteRuns(t *testing.T) {
	// The claim the whole design rests on: a question asked through ChatGPT reaches the same
	// search core as the same question asked through /jobs/search, so the two cannot be
	// answered differently.
	fake := &fakeSearcher{}

	_, err := mcpappUnder(fake, nil, fakeOJCPStore{}).SearchJobs(context.Background(), mcpapp.SearchInput{
		Query: "go engineer", Countries: []string{"DE"}, Limit: 20, Offset: 40,
	})
	if err != nil {
		t.Fatal(err)
	}

	if fake.got.Query != "go engineer" {
		t.Errorf("query = %q, want it passed through", fake.got.Query)
	}
	if fake.got.Limit != 20 || fake.got.Offset != 40 {
		t.Errorf("page = %d/%d, want 20/40", fake.got.Limit, fake.got.Offset)
	}
	if fake.got.Filter == nil {
		t.Error("no filter was built; the facets the caller asked for reached nothing")
	}
}

func TestTheAppSearchNamesTheFiltersItCouldNotHonour(t *testing.T) {
	// The answer is wider than asked. Saying so is what stops a model reading a narrow
	// result as the catalogue's whole content.
	got, err := mcpappUnder(&fakeSearcher{}, nil, fakeOJCPStore{}).
		SearchJobs(context.Background(), mcpapp.SearchInput{Query: "ai", Category: []string{"ai"}})
	if err != nil {
		t.Fatal(err)
	}

	if len(got.IgnoredParams) == 0 {
		t.Fatal("nothing reported; the caller cannot tell the filter was dropped")
	}
	if got.IgnoredParams[0] != "category=ai" {
		t.Errorf("ignored = %v, want category=ai", got.IgnoredParams)
	}
}

func TestAPostingTheWebsiteWouldNotShowIsUnreachable(t *testing.T) {
	// GetJobBySlug carries NO predicate — it is the read a private job's own creator uses,
	// and the detail page relies on it to serve a closed posting. Neither is right here: a
	// model enumerates, caches and republishes what it is handed.
	//
	// Every refusal answers NOT FOUND rather than forbidden. Whether a private posting
	// exists under some slug is itself not an anonymous caller's business.
	cases := []struct {
		name string
		job  db.Job
	}{
		{"private", db.Job{PublicSlug: "x", IsPrivate: true}},
		{"closed", db.Job{PublicSlug: "x", ClosedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}}},
		{"duplicate", db.Job{PublicSlug: "x", DuplicateOf: pgtype.Int8{Int64: 9, Valid: true}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := mcpappUnder(&fakeSearcher{}, nil, fakeOJCPStore{job: tc.job}).
				JobDetail(context.Background(), "x")

			var notFound mcpapp.NotFoundError
			if !errors.As(err, &notFound) {
				t.Fatalf("err = %v, want a not-found", err)
			}
		})
	}
}

func TestAnAbsentPostingAndARefusedOneAreIndistinguishable(t *testing.T) {
	absent := fakeOJCPStore{jobErr: pgx.ErrNoRows}
	refused := fakeOJCPStore{job: db.Job{PublicSlug: "x", IsPrivate: true}}

	_, absentErr := mcpappUnder(&fakeSearcher{}, nil, absent).JobDetail(context.Background(), "x")
	_, refusedErr := mcpappUnder(&fakeSearcher{}, nil, refused).JobDetail(context.Background(), "x")

	if absentErr.Error() != refusedErr.Error() {
		t.Errorf("a refused posting says %q while an absent one says %q; the difference is a disclosure",
			refusedErr, absentErr)
	}
}

func TestAnUnknownEmployerIsNotFoundRatherThanAnEmptyOne(t *testing.T) {
	_, err := mcpappUnder(&fakeSearcher{}, nil, fakeOJCPStore{compErr: pgx.ErrNoRows}).
		CompanyDetail(context.Background(), "nope")

	var notFound mcpapp.NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("err = %v, want a not-found rather than a blank employer", err)
	}
}

func TestAnUnconfiguredSearchBackendRefusesRatherThanAnsweringNothing(t *testing.T) {
	// A deployment without Meilisearch must not report an empty catalogue, which is what a
	// nil backend answering zero hits would look like.
	if _, err := mcpappUnder(nil, nil, fakeOJCPStore{}).SearchJobs(context.Background(), mcpapp.SearchInput{}); err == nil {
		t.Error("an unconfigured search answered successfully")
	}
	if _, err := mcpappUnder(&fakeSearcher{}, nil, fakeOJCPStore{}).SearchCompanies(context.Background(), mcpapp.CompanySearchInput{}); err == nil {
		t.Error("an unconfigured company search answered successfully")
	}
}

func TestTheCompanySearchProjectsEachHit(t *testing.T) {
	cs := &fakeCompanySearcherForMCP{res: search.CompanyResult{
		Hits:  []search.CompanyDocument{{Slug: "acme", Name: "Acme", JobCount: 12}},
		Total: 1,
	}}

	got, err := mcpappUnder(&fakeSearcher{}, cs, fakeOJCPStore{}).
		SearchCompanies(context.Background(), mcpapp.CompanySearchInput{Query: "acme"})
	if err != nil {
		t.Fatal(err)
	}

	if len(got.Companies) != 1 {
		t.Fatalf("companies = %v, want one", got.Companies)
	}
	if got.Companies[0].URL != "https://freehire.me/companies/acme" {
		t.Errorf("url = %q, want the employer's page", got.Companies[0].URL)
	}
	if got.Companies[0].OpenJobs != 12 {
		t.Errorf("open_jobs = %d, want 12", got.Companies[0].OpenJobs)
	}
}

type fakeCompanySearcherForMCP struct {
	res search.CompanyResult
	err error
	got search.CompanySearchParams
}

func (f *fakeCompanySearcherForMCP) SearchCompanies(_ context.Context, p search.CompanySearchParams) (search.CompanyResult, error) {
	f.got = p
	return f.res, f.err
}

func TestTheMountedRouteSpeaksMCPOverHTTP(t *testing.T) {
	// The Fiber adaptor mount, which no unit test on the handler methods can reach. A server
	// that works in memory and 404s or hangs behind the adaptor is the whole feature broken,
	// and it is exactly the shape of failure the OJCP manifest hit in production.
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.All("/mcp", adaptor.HTTPHandler(mcpapp.Handler(mcpappUnder(&fakeSearcher{}, nil, fakeOJCPStore{}))))

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`
	req := httptest.NewRequestWithContext(t.Context(), fiber.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	answer, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	// Our own name, not the OJCP server's. The two are mounted side by side and a caller
	// must be able to tell which one answered.
	if !strings.Contains(string(answer), `"name":"freehire"`) {
		t.Errorf("body = %s, want this server to name itself", answer)
	}
	if strings.Contains(string(answer), "freehire-ojcp") {
		t.Errorf("body = %s, want the OJCP server not to have answered", answer)
	}
}

func TestTheAppJobDetailCarriesTheLiveLivenessSignal(t *testing.T) {
	// Ghost and Reality are NOT intrinsic to a jobview — every surface attaches them
	// explicitly, and the OJCP surface once published nothing for every posting because a
	// projection read a field nothing had set. This drives the seam: the handler must call
	// the attacher, and what it leaves behind must reach the answer. What counts as a verdict
	// is internal/job/ghost's own business and has its own tests.
	store := fakeOJCPStore{job: db.Job{ID: 7, PublicSlug: "go-dev-x", Title: "Go Dev", Company: "Acme"}}
	h := mcpappUnder(&fakeSearcher{}, nil, store)
	attached := false
	h.attachReality = func(_ context.Context, _ db.Job, view *jobview.Job) {
		attached = true
		view.Reality = &jobview.Reality{Class: jobreality.ClassStale}
		view.Ghost = &jobview.Ghost{
			Level:         ghost.LevelLikely,
			Criteria:      []string{ghost.CriterionATSAbsent},
			CriteriaTotal: ghost.CriteriaTotal,
		}
	}

	got, err := h.JobDetail(context.Background(), "go-dev-x")
	if err != nil {
		t.Fatalf("JobDetail: %v", err)
	}
	if !attached {
		t.Fatal("the handler never asked for the liveness signal")
	}
	if got.Reality != jobreality.ClassStale || got.GhostLevel != ghost.LevelLikely {
		t.Errorf("reality = %q, ghost_level = %q; the attached verdict did not reach the answer", got.Reality, got.GhostLevel)
	}
}

func TestTheAppProductionWiringAttachesTheLivenessSignal(t *testing.T) {
	// The seam above is worth nothing unless production fills it, and that is the one path a
	// fake store cannot exercise.
	if h := mcpappUnder(&fakeSearcher{}, nil, &db.Queries{}); h.attachReality == nil {
		t.Error("attachReality is nil for a production store; get_job would never state liveness")
	}
	if h := mcpappUnder(&fakeSearcher{}, nil, fakeOJCPStore{}); h.attachReality != nil {
		t.Error("attachReality is set for a store that cannot answer the lookups")
	}
}

func TestTheAppOpenCheckAnswersEverySlugInTheOrderAsked(t *testing.T) {
	// closed is the answer this tool exists for, so a closed posting is reported as closed —
	// unlike get_job, which refuses it. Private and duplicate postings stay indistinguishable
	// from absent ones, exactly as get_job keeps them.
	closedAt := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	seenAt := time.Date(2026, 10, 15, 6, 0, 0, 0, time.UTC)
	store := fakeOJCPStore{liveness: []db.ListJobLivenessBySlugsRow{
		{PublicSlug: "closed-one", ClosedAt: pgtype.Timestamptz{Time: closedAt, Valid: true}},
		{PublicSlug: "open-one", LastSeenAt: pgtype.Timestamptz{Time: seenAt, Valid: true}},
		{PublicSlug: "private-one", IsPrivate: true},
		{PublicSlug: "dup-one", DuplicateOf: pgtype.Int8{Int64: 9, Valid: true}},
	}}

	got, err := mcpappUnder(&fakeSearcher{}, nil, store).CheckJobsOpen(context.Background(),
		[]string{"open-one", "closed-one", "private-one", "dup-one", "never-existed"})
	if err != nil {
		t.Fatalf("CheckJobsOpen: %v", err)
	}

	want := []mcpapp.JobOpenState{
		{Slug: "open-one", Status: mcpapp.StatusOpen, LastSeenAt: "2026-10-15T06:00:00Z"},
		{Slug: "closed-one", Status: mcpapp.StatusClosed, ClosedAt: "2026-10-01T09:00:00Z"},
		{Slug: "private-one", Status: mcpapp.StatusNotFound},
		{Slug: "dup-one", Status: mcpapp.StatusNotFound},
		{Slug: "never-existed", Status: mcpapp.StatusNotFound},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d states, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("state %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestTheAppOpenCheckFailsRatherThanCallingEverythingMissing(t *testing.T) {
	// A store that cannot answer must not read as "none of your postings exist" — that is a
	// confident, wrong answer a person acts on.
	store := fakeOJCPStore{livenessErr: errors.New("db down")}
	if _, err := mcpappUnder(&fakeSearcher{}, nil, store).CheckJobsOpen(context.Background(), []string{"x"}); err == nil {
		t.Error("a failed lookup answered as a result")
	}
}
