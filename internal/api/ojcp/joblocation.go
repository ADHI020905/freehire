package ojcp

import (
	"encoding/json"
	"strings"

	"github.com/strelov1/freehire/internal/job/jobview"
)

// Place is OJCP's location block — one schema.org Place, singular. That shape is the whole
// difficulty of this projection: a posting open across several countries does not have one
// place, and the standard offers nowhere else to say so.
type Place struct {
	Type    string  `json:"@type,omitempty"`
	Address Address `json:"address"`
}

// Address is schema.org's postal address, as OJCP narrows it.
type Address struct {
	AddressLocality string `json:"addressLocality,omitempty"`
	// AddressRegion is a STATE OR PROVINCE. It is deliberately never filled from our own
	// Regions facet, which holds macro-regions ("europe", "global") — a different kind of
	// thing entirely. An agent reading addressRegion="europe" would file the posting under
	// a province by that name.
	AddressRegion  string `json:"addressRegion,omitempty"`
	AddressCountry string `json:"addressCountry,omitempty"`
}

// Places is jobLocation: the places a posting names. schema.org and the OJCP schema accept
// one Place or an array of them, and one place is written as a bare object so a consumer
// built on the singular shape keeps reading every posting that has exactly one.
type Places []Place

// MarshalJSON writes a single Place as an object and anything longer as an array.
func (p Places) MarshalJSON() ([]byte, error) {
	if len(p) == 1 {
		return json.Marshal(p[0])
	}
	return json.Marshal([]Place(p))
}

// jobLocationFor states every place the posting names, as precisely as the facets allow.
//
// Countries and cities are separate facets, so a city can be paired with a country only
// when there is one country for it to lie in. With several countries the cities are left
// out and each country is its own Place: Warsaw paired with DE would be a workplace that
// does not exist, while the countries alone are still facts. A remote posting states
// countries only, because there a Place is where the employee may live and work, and a
// city on it is usually the employer's office.
func jobLocationFor(j jobview.Job) Places {
	countries := make([]string, len(j.Countries))
	for i, country := range j.Countries {
		// The country facet is served lowercased (jobview.normalizeSet), while
		// addressCountry is the ISO 3166-1 alpha-2 code, canonically UPPERCASE. An agent
		// comparing against "DE" or feeding the value to a country library misses every
		// posting otherwise.
		countries[i] = strings.ToUpper(country)
	}
	// With no city to pair, or none that can be paired, each country is its own Place.
	if j.WorkMode == "remote" || len(countries) > 1 || len(j.Cities) == 0 {
		var places Places
		for _, country := range countries {
			places = append(places, place(Address{AddressCountry: country}))
		}
		return places
	}
	country := ""
	if len(countries) == 1 {
		country = countries[0]
	}
	places := make(Places, len(j.Cities))
	for i, city := range j.Cities {
		places[i] = place(Address{AddressLocality: city, AddressCountry: country})
	}
	return places
}

func place(address Address) Place {
	return Place{Type: "Place", Address: address}
}
