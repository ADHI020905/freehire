package search

import (
	"fmt"
	"net/url"
	"reflect"
	"testing"
)

func TestIgnoredValues(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  map[string][]string
	}{
		{"clean request", "work_mode=remote&seniority=senior", nil},
		{"unknown value", "work_mode=foo", map[string][]string{"work_mode": {"foo"}}},
		{"valid and unknown mixed, comma-joined", "work_mode=remote,foo", map[string][]string{"work_mode": {"foo"}}},
		{"case and whitespace the filter could still match", "work_mode=Remote&seniority=%20senior", nil},
		{"exclude form reported under its own name", "seniority_exclude=guru", map[string][]string{"seniority_exclude": {"guru"}}},
		{"regions unspecified sentinel is valid", "regions=none", nil},
		{"retired category is translated, not ignored", "category=management", nil},
		{"boolean facet", "requires_clearance=flase&is_tech=true", map[string][]string{"requires_clearance": {"flase"}}},
		{"reality class", "reality=likely-evergreen&reality=ghost", map[string][]string{"reality": {"ghost"}}},
		{"open-ended facets are not checked", "skills=notaskill&countries=zz&cities=atlantis", nil},
		{"repeated value reported once", "work_mode=foo&work_mode=foo", map[string][]string{"work_mode": {"foo"}}},
		{"stray comma is not a value", "work_mode=remote,", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, err := url.ParseQuery(tc.query)
			if err != nil {
				t.Fatal(err)
			}
			if got := IgnoredValues(v); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("IgnoredValues(%q) = %v, want %v", tc.query, got, tc.want)
			}
		})
	}
}

func TestIgnoredValuesIsBounded(t *testing.T) {
	v := url.Values{}
	for i := range 3 * maxUnknownParamsReported {
		v.Add("work_mode", fmt.Sprintf("bad%02d", i))
	}
	for facet := range closedFacets {
		v.Add(facet, "zzz-not-a-value")
		v.Add(facet+"_exclude", "zzz-not-a-value")
	}
	got := IgnoredValues(v)
	if len(got) != maxUnknownParamsReported {
		t.Errorf("reported %d params, want %d", len(got), maxUnknownParamsReported)
	}
	for param, values := range got {
		if len(values) > maxUnknownParamsReported {
			t.Errorf("%s: reported %d values, want at most %d", param, len(values), maxUnknownParamsReported)
		}
	}
}

// Every closed facet must be a facet the filter reads and carry a non-empty vocabulary:
// an empty one would report every value, and a facet the filter does not read would name
// a value the request never filtered on.
func TestClosedFacetsAreRealFacetsWithAVocabulary(t *testing.T) {
	for facet, allowed := range closedFacets {
		if _, ok := StringFacets[facet]; !ok {
			t.Errorf("%s: not in StringFacets", facet)
		}
		if len(allowed) == 0 {
			t.Errorf("%s: empty vocabulary", facet)
		}
	}
}
