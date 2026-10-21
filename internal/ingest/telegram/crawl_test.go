package telegram

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"
)

type fakeFetcher struct {
	posts map[string][]Post
	errs  map[string]error
	calls []string
}

func (f *fakeFetcher) Fetch(_ context.Context, channel string) ([]Post, error) {
	f.calls = append(f.calls, channel)
	if err := f.errs[channel]; err != nil {
		return nil, err
	}
	return f.posts[channel], nil
}

type storedPost struct {
	channel string
	post    Post
	done    bool
}

type fakePostStore struct {
	stored   []storedPost
	existing map[string]bool // "channel/msg_id" already stored
	err      error
	requeued []string // "channel/msg_id" handed back to the extraction queue
}

func (s *fakePostStore) Requeue(_ context.Context, channel string, msgID int64) (int64, error) {
	s.requeued = append(s.requeued, channel+"/"+strconv.FormatInt(msgID, 10))
	return 1, nil
}

// fakeJudge answers every question the same way and records what it was asked.
type fakeJudge struct {
	say   bool
	err   error
	asked []string
}

func (j *fakeJudge) IsVacancy(_ context.Context, text string) (bool, error) {
	j.asked = append(j.asked, text)
	return j.say, j.err
}

func (s *fakePostStore) Insert(_ context.Context, channel string, p Post, done bool) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	k := channel + "/" + strconv.FormatInt(p.MsgID, 10)
	if s.existing[k] {
		return false, nil
	}
	s.stored = append(s.stored, storedPost{channel: channel, post: p, done: done})
	return true, nil
}

func vacancyPost(id int64) Post {
	return Post{MsgID: id, PostedAt: time.Now(), Text: "Вакансия: Go разработчик, зарплата 300к"}
}

func memePost(id int64) Post {
	return Post{MsgID: id, PostedAt: time.Now(), Text: "Всем хороших выходных 🎉"}
}

func TestCrawlStoresVacanciesAndFiltersNoise(t *testing.T) {
	f := &fakeFetcher{posts: map[string][]Post{
		"jobs": {vacancyPost(1), memePost(2)},
	}}
	store := &fakePostStore{}
	r := CrawlRunner{Fetcher: f, Store: store}

	stats, err := r.Run(context.Background(), []ChannelEntry{{Channel: "jobs", Kind: KindBoard}})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if stats.Stored != 1 || stats.Filtered != 1 || stats.Failed != 0 {
		t.Errorf("stats = %+v, want Stored=1 Filtered=1 Failed=0", stats)
	}
	if len(store.stored) != 2 {
		t.Fatalf("stored = %d rows, want 2 (vacancy pending + meme done)", len(store.stored))
	}
	for _, sp := range store.stored {
		wantDone := sp.post.MsgID == 2
		if sp.done != wantDone {
			t.Errorf("post %d stored done=%v, want %v", sp.post.MsgID, sp.done, wantDone)
		}
	}
}

type fakeLinkMatcher struct{ match bool }

func (m fakeLinkMatcher) Matches(_ []Link) bool { return m.match }

func TestCrawlKeepsLinkOutDigestDespiteNoisyText(t *testing.T) {
	// A digest whose teaser text alone reads as noise, but which links out to a vacancy
	// page, must be queued (not filtered) so the extractor can follow the link.
	digest := memePost(3)
	digest.Links = []Link{{Text: "Product manager", URL: "https://u.habr.com/PnBO7"}}
	f := &fakeFetcher{posts: map[string][]Post{"jobs": {digest}}}
	store := &fakePostStore{}
	r := CrawlRunner{Fetcher: f, Store: store, Links: fakeLinkMatcher{match: true}}

	stats, err := r.Run(context.Background(), []ChannelEntry{{Channel: "jobs", Kind: KindBoard}})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if stats.Stored != 1 || stats.Filtered != 0 {
		t.Errorf("stats = %+v, want Stored=1 Filtered=0 (link-out digest kept)", stats)
	}
	if len(store.stored) != 1 || store.stored[0].done {
		t.Errorf("post stored done=%v, want pending (false)", store.stored)
	}
}

func TestCrawlOneFailingChannelDoesNotAbortTheRun(t *testing.T) {
	f := &fakeFetcher{
		posts: map[string][]Post{"good": {vacancyPost(1)}},
		errs:  map[string]error{"bad": errors.New("status 429")},
	}
	store := &fakePostStore{}
	r := CrawlRunner{Fetcher: f, Store: store}

	stats, err := r.Run(context.Background(), []ChannelEntry{
		{Channel: "bad", Kind: KindBoard},
		{Channel: "good", Kind: KindBoard},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if stats.Failed != 1 || stats.Stored != 1 {
		t.Errorf("stats = %+v, want Failed=1 Stored=1", stats)
	}
	if len(f.calls) != 2 {
		t.Errorf("fetch calls = %v, want both channels", f.calls)
	}
}

func TestCrawlSkipsAlreadyStoredPosts(t *testing.T) {
	f := &fakeFetcher{posts: map[string][]Post{"jobs": {vacancyPost(7)}}}
	store := &fakePostStore{existing: map[string]bool{"jobs/7": true}}
	r := CrawlRunner{Fetcher: f, Store: store}

	stats, err := r.Run(context.Background(), []ChannelEntry{{Channel: "jobs", Kind: KindBoard}})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if stats.Stored != 0 || stats.Filtered != 0 || stats.Failed != 0 {
		t.Errorf("stats = %+v, want all zero (post already stored)", stats)
	}
}

// A post the markers miss is not lost when the judge recognises it: it is stored, then
// handed back to the extraction queue. Measured 2026-10-21 over 150 declined posts, 90 were
// real vacancies the markers had no word for ("RUST DEVELOPER | REMOTE", a Farsi posting).
func TestCrawlRescuesAVacancyTheMarkersMiss(t *testing.T) {
	f := &fakeFetcher{posts: map[string][]Post{"jobs": {memePost(2)}}}
	store := &fakePostStore{}
	judge := &fakeJudge{say: true}
	r := CrawlRunner{Fetcher: f, Store: store, Judge: judge}

	stats, err := r.Run(context.Background(), []ChannelEntry{{Channel: "jobs", Kind: KindBoard}})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(store.requeued) != 1 || store.requeued[0] != "jobs/2" {
		t.Fatalf("requeued = %v, want [jobs/2]", store.requeued)
	}
	if stats.Stored != 1 || stats.Filtered != 0 || stats.Rescued != 1 {
		t.Errorf("stats = %+v, want Stored=1 Filtered=0 Rescued=1", stats)
	}
}

// The judge costs a request, so it is asked only about a post that is both NEW and declined
// by the markers. A channel's page is re-fetched on every crawl; asking about the posts it
// already stored would repeat the same questions every run.
func TestCrawlAsksTheJudgeOnlyAboutNewDeclinedPosts(t *testing.T) {
	f := &fakeFetcher{posts: map[string][]Post{"jobs": {vacancyPost(1), memePost(2), memePost(3)}}}
	store := &fakePostStore{existing: map[string]bool{"jobs/3": true}}
	judge := &fakeJudge{say: false}
	r := CrawlRunner{Fetcher: f, Store: store, Judge: judge}

	stats, err := r.Run(context.Background(), []ChannelEntry{{Channel: "jobs", Kind: KindBoard}})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(judge.asked) != 1 || judge.asked[0] != memePost(2).Text {
		t.Fatalf("judge asked %d times (%v), want once, about post 2", len(judge.asked), judge.asked)
	}
	if len(store.requeued) != 0 || stats.Filtered != 1 || stats.Rescued != 0 {
		t.Errorf("requeued=%v stats=%+v, want nothing requeued, Filtered=1", store.requeued, stats)
	}
}

// A judge that cannot answer must not cost a vacancy. The extractor is the real classifier
// and may still find nothing; a post left declined is never looked at again.
func TestCrawlAdmitsWhenTheJudgeFails(t *testing.T) {
	f := &fakeFetcher{posts: map[string][]Post{"jobs": {memePost(2)}}}
	store := &fakePostStore{}
	r := CrawlRunner{Fetcher: f, Store: store, Judge: &fakeJudge{err: errors.New("jev: 503")}}

	if _, err := r.Run(context.Background(), []ChannelEntry{{Channel: "jobs", Kind: KindBoard}}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(store.requeued) != 1 {
		t.Fatalf("requeued = %v, want the post handed to the extractor", store.requeued)
	}
}
