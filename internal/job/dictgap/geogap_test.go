package dictgap

import (
	"reflect"
	"testing"
)

func TestGeoGapCandidates(t *testing.T) {
	rows := []LocationCountries{
		// A gap: the dictionary places nothing, the model names a country. The places
		// are invented so that no dictionary growth ever resolves them. Spellings
		// that differ only in case and surrounding space fold into one entry.
		{Location: "Тридесятая область", LLMCountries: []string{"RU"}, Count: 300},
		{Location: "  тридесятая область ", LLMCountries: []string{"ru"}, Count: 88},
		{Location: "Тридесятая область", LLMCountries: []string{"KZ"}, Count: 12},
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
		Location:   "тридесятая область",
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
		{Location: "Тридевятый край", LLMCountries: []string{"RU"}, Count: 10},
		{Location: "Заморский край", LLMCountries: []string{"RU"}, Count: 10},
		{Location: "Лукоморский край", LLMCountries: []string{"RU"}, Count: 30},
	}
	got := GeoGapCandidates(rows).Gaps
	var order []string
	for _, c := range got {
		order = append(order, c.Location)
	}
	want := []string{"лукоморский край", "заморский край", "тридевятый край"}
	if !reflect.DeepEqual(order, want) {
		t.Errorf("order = %v, want %v", order, want)
	}
}
