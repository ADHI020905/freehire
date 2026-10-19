package location

import (
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/dict/vocab"
)

func TestNormalizeRegions(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"canonical kept", []string{"eu", "north_america"}, []string{"eu", "north_america"}},
		{"country code folds to its region", []string{"us"}, []string{"north_america"}},
		{"country name folds to its region", []string{"Switzerland"}, []string{"eu"}},
		{"alpha-3 folds to its region", []string{"usa"}, []string{"north_america"}},
		{"russia is cis", []string{"ru", "russia"}, []string{"cis"}},
		{"subdivision folds through its country", []string{"us-fl", "us_tx", "ru-mow", "au-qld", "fr-75"}, []string{"apac", "cis", "eu", "north_america"}},
		{"a phrase is not a subdivision", []string{"no-remote", "it-support", "in-office", "in_office"}, nil},
		{"region name with underscore", []string{"south_america", "latin_america", "central_asia"}, []string{"cis", "latam"}},
		{"multi-region label expands", []string{"emea"}, []string{"africa", "eu", "mena", "uk"}},
		{"americas expands", []string{"americas"}, []string{"latam", "north_america"}},
		{"sub-continental labels fold", []string{"dach", "nordic", "eea", "oceania", "caribbean"}, []string{"apac", "eu", "latam"}},
		{"unresolvable values are dropped", []string{"gmt+1 to gmt+5", "cyrillic_region_placeholder", "outside_us", "other"}, nil},
		{"mixed keeps only the resolvable", []string{"eu", "outside_russia", "jp"}, []string{"apac", "eu"}},
		{"empty", nil, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := NormalizeRegions(c.in)
			if !slices.Equal(got, c.want) {
				t.Fatalf("NormalizeRegions(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// Every alias must expand only into the region vocabulary, or the normaliser would
// itself become a source of the off-vocabulary values it exists to remove.
func TestRegionAliasesStayInVocabulary(t *testing.T) {
	for alias, regions := range regionAliases {
		for _, r := range regions {
			if !slices.Contains(vocab.RegionValues, r) {
				t.Errorf("regionAliases[%q] names %q, not in vocab.RegionValues", alias, r)
			}
		}
	}
}
