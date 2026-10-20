package ogimage

import (
	"bytes"
	"image/png"
	"slices"
	"testing"
)

func decodeCard(t *testing.T, data []byte) {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("response is not a valid PNG: %v", err)
	}
	if b := img.Bounds(); b.Dx() != width || b.Dy() != height {
		t.Errorf("size = %dx%d, want %dx%d", b.Dx(), b.Dy(), width, height)
	}
}

func TestRenderCatalogCard_ExactStats(t *testing.T) {
	data, err := RenderCatalogCard(
		"The open-source search engine for tech jobs",
		"Millions of openings indexed straight from company career boards.",
		Stats{OpenJobs: 5_771_399, UniqueOpenJobs: 3_371_563, Companies: 477_323, Sources: 320, TelegramChannels: 97, Exact: true},
	)
	if err != nil {
		t.Fatalf("RenderCatalogCard: %v", err)
	}
	decodeCard(t, data)
}

// The degraded snapshot leaves Companies at zero and Exact false; the card must
// render without needing to know that in advance, and must not show the zero.
func TestRenderCatalogCard_DegradedStats(t *testing.T) {
	data, err := RenderCatalogCard(
		"freehire's numbers, live",
		"Every figure comes straight from the public API.",
		Stats{OpenJobs: 3_150_000, Sources: 227, Exact: false},
	)
	if err != nil {
		t.Fatalf("RenderCatalogCard: %v", err)
	}
	decodeCard(t, data)
}

func TestRenderCatalogCard_WrapsALongHeading(t *testing.T) {
	data, err := RenderCatalogCard(
		"A heading long enough that it cannot possibly fit on a single line of this card no matter the font size chosen",
		"",
		Stats{OpenJobs: 1, Companies: 1, Sources: 1, Exact: true},
	)
	if err != nil {
		t.Fatalf("RenderCatalogCard: %v", err)
	}
	decodeCard(t, data)
}

// The strip quotes the de-duplicated count — the figure /jobs shows — never the raw
// one, and drops a figure the snapshot could not measure rather than print a zero.
func TestChips(t *testing.T) {
	cases := []struct {
		name  string
		stats Stats
		want  []string
	}{
		{
			name:  "exact quotes unique jobs and channels",
			stats: Stats{OpenJobs: 5_771_399, UniqueOpenJobs: 3_371_563, Companies: 477_323, Sources: 320, TelegramChannels: 97, Exact: true},
			want:  []string{"3,371,563 unique jobs", "477,323 companies", "320 sources", "97 Telegram channels"},
		},
		{
			name:  "exact drops unmeasured unique count and channels",
			stats: Stats{OpenJobs: 5_771_399, Companies: 477_323, Sources: 320, Exact: true},
			want:  []string{"477,323 companies", "320 sources"},
		},
		{
			name:  "degraded marks the estimate",
			stats: Stats{OpenJobs: 3_150_000, Sources: 227},
			want:  []string{"~3,150,000 postings tracked, estimated", "227 sources"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, c := range chips(tc.stats) {
				got = append(got, c.value+" "+c.label)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("chips = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGroupThousands(t *testing.T) {
	cases := map[int64]string{
		0:       "0",
		227:     "227",
		1000:    "1,000",
		294282:  "294,282",
		3300658: "3,300,658",
		-12345:  "-12,345",
	}
	for in, want := range cases {
		if got := groupThousands(in); got != want {
			t.Errorf("groupThousands(%d) = %q, want %q", in, got, want)
		}
	}
}
