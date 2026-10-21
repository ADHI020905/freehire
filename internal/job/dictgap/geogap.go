package dictgap

import (
	"slices"
	"sort"
	"strings"

	"github.com/strelov1/freehire/internal/dict/location"
)

// LocationCountries is one group of jobs sharing a raw location string AND the same
// list of countries LLM enrichment recorded for them, with how many jobs it covers.
// The caller may pass the same location more than once (one row per distinct
// country list, per chunk); GeoGapCandidates folds them together.
type LocationCountries struct {
	Location     string
	LLMCountries []string
	Count        int
}

// GeoGapCandidate is one location string where internal/dict/location and LLM
// enrichment answer the country question differently.
//
// LLMCountry is the country the model named most often for this location, and
// LLMShare the fraction of the location's jobs that named it — the curator's
// confidence signal, since the model is also asked about postings whose location is
// only "Remote" and answers those with a guess. DictionaryCountries is what
// location.Parse resolves today; empty on a gap.
type GeoGapCandidate struct {
	Location            string
	Count               int
	LLMCountry          string
	LLMShare            float64
	DictionaryCountries []string
}

// GeoGapReport separates the two findings because they call for opposite fixes:
// a gap is a name the dictionary is missing, a conflict is a name it may be
// resolving wrongly (the `georgia` state-versus-country shape).
type GeoGapReport struct {
	// Gaps are locations location.Parse places nowhere — no country AND no region —
	// while the model named a country. A region-only answer ("Europe") is a
	// placement, not a gap.
	Gaps []GeoGapCandidate
	// Conflicts are locations where location.Parse names countries and most of the
	// location's jobs name none of them.
	Conflicts []GeoGapCandidate
}

// GeoGapCandidates recomputes location.Parse for every distinct location — the
// dictionary as it stands today, not whichever version derived the stored
// jobs.countries — and compares it with the countries LLM enrichment recorded.
// Locations are folded case- and space-insensitively, the same normalization Parse
// applies. Both lists are ranked by job count, then by location for a stable order.
func GeoGapCandidates(rows []LocationCountries) GeoGapReport {
	type tally struct {
		total     int
		byCountry map[string]int
		// rows are kept whole so each job group can be judged against the
		// dictionary's answer, which is only known once the location is folded.
		rows []LocationCountries
	}
	byLocation := make(map[string]*tally)
	for _, r := range rows {
		if len(r.LLMCountries) == 0 || r.Count <= 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(r.Location))
		if key == "" {
			continue
		}
		t, ok := byLocation[key]
		if !ok {
			t = &tally{byCountry: make(map[string]int)}
			byLocation[key] = t
		}
		t.total += r.Count
		for _, c := range normalizeCountryCodes(r.LLMCountries) {
			t.byCountry[c] += r.Count
		}
		t.rows = append(t.rows, r)
	}

	var report GeoGapReport
	for key, t := range byLocation {
		top, topCount := mostNamed(t.byCountry)
		candidate := GeoGapCandidate{
			Location:   key,
			Count:      t.total,
			LLMCountry: top,
			LLMShare:   float64(topCount) / float64(t.total),
		}
		geo := location.Parse(key)
		if len(geo.Countries) == 0 {
			if len(geo.Regions) == 0 {
				report.Gaps = append(report.Gaps, candidate)
			}
			continue
		}
		agree, disagree := 0, 0
		for _, r := range t.rows {
			if namesAny(normalizeCountryCodes(r.LLMCountries), geo.Countries) {
				agree += r.Count
			} else {
				disagree += r.Count
			}
		}
		if disagree > agree {
			candidate.DictionaryCountries = geo.Countries
			report.Conflicts = append(report.Conflicts, candidate)
		}
	}
	sortGeoCandidates(report.Gaps)
	sortGeoCandidates(report.Conflicts)
	return report
}

// normalizeCountryCodes lowercases and trims the model's codes and drops blanks and
// repeats. It deliberately does NOT route them through location.NormalizeCountry:
// that returns "" for a country outside the dictionary's curated set, and a country
// the dictionary cannot place at all is exactly what this report must surface.
func normalizeCountryCodes(codes []string) []string {
	out := make([]string, 0, len(codes))
	for _, c := range codes {
		c = strings.ToLower(strings.TrimSpace(c))
		if c != "" && !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}

// mostNamed returns the country named by the most jobs, breaking a tie by code so
// the report is deterministic.
func mostNamed(byCountry map[string]int) (string, int) {
	best, bestCount := "", 0
	for c, n := range byCountry {
		if n > bestCount || (n == bestCount && c < best) {
			best, bestCount = c, n
		}
	}
	return best, bestCount
}

func namesAny(codes, want []string) bool {
	for _, c := range codes {
		if slices.Contains(want, c) {
			return true
		}
	}
	return false
}

func sortGeoCandidates(c []GeoGapCandidate) {
	sort.Slice(c, func(i, j int) bool {
		if c[i].Count != c[j].Count {
			return c[i].Count > c[j].Count
		}
		return c[i].Location < c[j].Location
	})
}
