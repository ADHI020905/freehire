package location

import (
	"reflect"
	"testing"
)

// TestParseForeignSubdivisions covers the cmd/report-geo-gaps finding: a location
// line naming only a non-US/CA region placed the posting nowhere.
func TestParseForeignSubdivisions(t *testing.T) {
	tests := []struct {
		location string
		want     []string
	}{
		{"Свердловская область", []string{"ru"}},
		{"Краснодарский край", []string{"ru"}},
		{"Республика Татарстан (Татарстан)", []string{"ru"}},
		{"Чувашская Республика - Чувашия", []string{"ru"}},
		{"Ханты-Мансийский автономный округ - Югра", []string{"ru"}},
		{"Республика Саха (Якутия)", []string{"ru"}},
		{"Кемеровская область - Кузбасс", []string{"ru"}},
		{"Belfast, Northern Ireland", []string{"gb"}},
		{"The City, Central London", []string{"gb"}},
		{"Birmingham, West Midlands", []string{"gb"}},
		{"Cambridge, Cambridgeshire", []string{"gb"}},
		{"Canberra region, Australian Capital Territory", []string{"au"}},
		{"Brisbane, Queensland", []string{"au"}},
		{"Bonn, Nordrhein-Westfalen", []string{"de"}},
		{"Munich, Bavaria", []string{"de"}},
		{"Bangalore Urban", []string{"in"}},
		{"Pune, Maharashtra", []string{"in"}},
		// The state-code dash export, confirmed by a US city in the tail.
		{"TX-El Paso", []string{"us"}},
		{"FL-Orlando", []string{"us"}},
		{"OH-Columbus", []string{"us"}},
		// Withheld names and the contested territories place nothing.
		{"Punjab", nil},
		{"Victoria", nil},
		{"Berkshire", nil},
		{"Республика Крым", nil},
		// A hyphenated word whose lead happens to be a code is not a dash export.
		{"On-site", nil},
		{"Co-op", nil},
		{"Hi-Tech", nil},
	}
	for _, tt := range tests {
		t.Run(tt.location, func(t *testing.T) {
			got := Parse(tt.location).Countries
			if len(got) == 0 {
				got = nil
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse(%q).Countries = %v, want %v", tt.location, got, tt.want)
			}
		})
	}
}

// TestForeignSubdivisionCountriesArePlaceable keeps every entry reachable: a
// country outside countryToRegion would resolve to a country with no region.
func TestForeignSubdivisionCountriesArePlaceable(t *testing.T) {
	for name, country := range foreignSubdivisionNames {
		if _, ok := countryToRegion[country]; !ok {
			t.Errorf("foreignSubdivisionNames[%q] = %q, which has no region", name, country)
		}
	}
}
