package main

import (
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

func TestRefold(t *testing.T) {
	tests := []struct {
		name          string
		row           db.JobsForGeographyRecheckByIDsRow
		wantCountries []string
		wantRegions   []string
	}{
		{
			name:        "a country stored as a region folds into its region",
			row:         db.JobsForGeographyRecheckByIDsRow{Title: "Engineer", Location: "Remote", Regions: []string{"us"}},
			wantRegions: []string{"north_america"},
		},
		{
			name:          "stored countries are kept as the signal",
			row:           db.JobsForGeographyRecheckByIDsRow{Title: "Engineer", Countries: []string{"ru"}, Regions: []string{"russia"}},
			wantCountries: []string{"ru"},
			wantRegions:   []string{"cis"},
		},
		{
			name:        "a multi-region label expands",
			row:         db.JobsForGeographyRecheckByIDsRow{Title: "Engineer", Location: "Remote", Regions: []string{"emea"}},
			wantRegions: []string{"africa", "eu", "mena", "uk"},
		},
		{
			name:          "an unresolvable label leaves the dictionary's reading",
			row:           db.JobsForGeographyRecheckByIDsRow{Title: "Engineer", Location: "Berlin", Regions: []string{"gmt+1 to gmt+5"}},
			wantCountries: []string{"de"},
			wantRegions:   []string{"eu"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			countries, regions := refold(tt.row)
			if !slices.Equal(countries, tt.wantCountries) {
				t.Errorf("countries = %v, want %v", countries, tt.wantCountries)
			}
			if !slices.Equal(regions, tt.wantRegions) {
				t.Errorf("regions = %v, want %v", regions, tt.wantRegions)
			}
			if offVocabulary(regions) {
				t.Errorf("regions %v still off-vocabulary", regions)
			}
		})
	}
}
