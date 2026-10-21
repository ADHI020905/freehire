package dictgap

import (
	"reflect"
	"testing"
)

func TestGeoGapCandidates(t *testing.T) {
	rows := []LocationCountries{
		// A gap: the dictionary places nothing, the model names a country. Spellings
		// that differ only in case and surrounding space fold into one entry.
		{Location: "Свердловская область", LLMCountries: []string{"RU"}, Count: 300},
		{Location: "  свердловская область ", LLMCountries: []string{"ru"}, Count: 88},
		{Location: "Свердловская область", LLMCountries: []string{"KZ"}, Count: 12},
		// A conflict: both answer, and they disagree.
		{Location: "Tbilisi, Georgia", LLMCountries: []string{"US"}, Count: 40},
		// Agreement is not a finding.
		{Location: "Berlin, Germany", LLMCountries: []string{"DE"}, Count: 500},
		// A posting naming several countries agrees if ANY of them is the
		// dictionary's — the model listed more, it did not contradict.
		{Location: "Berlin, Germany", LLMCountries: []string{"AT", "DE"}, Count: 7},
		// A region-only answer is a placement, so it is neither a gap nor a conflict.
		{Location: "Europe", LLMCountries: []string{"UA"}, Count: 103},
		// No model opinion: nothing to compare.
		{Location: "Somewhere Unknown", LLMCountries: nil, Count: 50},
	}

	got := GeoGapCandidates(rows)

	wantGaps := []GeoGapCandidate{{
		Location:   "свердловская область",
		Count:      400,
		LLMCountry: "ru",
		LLMShare:   0.97,
	}}
	if !reflect.DeepEqual(got.Gaps, wantGaps) {
		t.Errorf("Gaps = %+v\nwant  %+v", got.Gaps, wantGaps)
	}

	wantConflicts := []GeoGapCandidate{{
		Location:            "tbilisi, georgia",
		Count:               40,
		LLMCountry:          "us",
		LLMShare:            1,
		DictionaryCountries: []string{"ge"},
	}}
	if !reflect.DeepEqual(got.Conflicts, wantConflicts) {
		t.Errorf("Conflicts = %+v\nwant       %+v", got.Conflicts, wantConflicts)
	}
}

func TestGeoGapCandidatesRanksByCountThenLocation(t *testing.T) {
	rows := []LocationCountries{
		{Location: "Краснодарский край", LLMCountries: []string{"RU"}, Count: 10},
		{Location: "Алтайский край", LLMCountries: []string{"RU"}, Count: 10},
		{Location: "Пермский край", LLMCountries: []string{"RU"}, Count: 30},
	}
	got := GeoGapCandidates(rows).Gaps
	var order []string
	for _, c := range got {
		order = append(order, c.Location)
	}
	want := []string{"пермский край", "алтайский край", "краснодарский край"}
	if !reflect.DeepEqual(order, want) {
		t.Errorf("order = %v, want %v", order, want)
	}
}
