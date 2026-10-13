package ojcp

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestJobLocationNamesAnUnambiguousPlace(t *testing.T) {
	row := openPostingRow()
	row.Countries = []string{"DE"}
	row.Cities = []string{"Berlin"}
	j := viewOf(row)

	posting := jobPostingFrom(j, testOrigin)

	assertPlaces(t, posting, Address{AddressLocality: "Berlin", AddressCountry: "DE"})
	// One place stays ONE object on the wire, as it was before lists existed: a consumer
	// written against the singular shape keeps reading every single-place posting.
	if raw := jobLocationJSON(t, posting); !strings.HasPrefix(raw, "{") {
		t.Errorf("jobLocation = %s, want a single object for one place", raw)
	}
	if err := validateAgainstSchema(t, schemaJobPosting, posting); err != nil {
		t.Fatalf("posting with a location rejected: %v", err)
	}
}

func TestJobLocationListsEveryCountryOfAMultiCountryPosting(t *testing.T) {
	// The schema forbids reducing a multi-location role to one of its locations, and
	// omitting the field is reserved for a posting whose location is unknown. A posting open
	// in three countries is therefore three Places.
	row := openPostingRow()
	row.Countries = []string{"DE", "ES", "PL"}
	j := viewOf(row)

	posting := jobPostingFrom(j, testOrigin)

	assertPlaces(t, posting,
		Address{AddressCountry: "DE"},
		Address{AddressCountry: "ES"},
		Address{AddressCountry: "PL"},
	)
	if raw := jobLocationJSON(t, posting); !strings.HasPrefix(raw, "[") {
		t.Errorf("jobLocation = %s, want an array for several places", raw)
	}
	if err := validateAgainstSchema(t, schemaJobPosting, posting); err != nil {
		t.Fatalf("multi-country posting rejected: %v", err)
	}
}

func TestJobLocationPairsEveryCityWithTheOnlyCountry(t *testing.T) {
	// With one country every city lies in it, so each city is a complete Place.
	row := openPostingRow()
	row.Countries = []string{"DE"}
	row.Cities = []string{"Berlin", "Munich"}
	j := viewOf(row)

	posting := jobPostingFrom(j, testOrigin)

	assertPlaces(t, posting,
		Address{AddressLocality: "Berlin", AddressCountry: "DE"},
		Address{AddressLocality: "Munich", AddressCountry: "DE"},
	)
}

func TestJobLocationStatesOnlyCountriesWhereCitiesCannotBePaired(t *testing.T) {
	// Cities and countries are separate facets. With several countries nothing says which
	// city lies in which, and a Place pairing Warsaw with DE would state a workplace that
	// does not exist. The countries are still facts, so they are stated alone.
	row := openPostingRow()
	row.Countries = []string{"DE", "PL"}
	row.Cities = []string{"Berlin", "Warsaw"}
	j := viewOf(row)

	posting := jobPostingFrom(j, testOrigin)

	assertPlaces(t, posting,
		Address{AddressCountry: "DE"},
		Address{AddressCountry: "PL"},
	)
}

func TestJobLocationOfARemotePostingNamesCountriesNotCities(t *testing.T) {
	// For a remote role a Place is where the employee may live and work. A city on a remote
	// posting is usually the employer's office, and stating it would read as a requirement
	// to live there.
	row := openPostingRow()
	row.WorkMode = "remote"
	row.Countries = []string{"CA", "US"}
	row.Cities = []string{"New York"}
	j := viewOf(row)

	posting := jobPostingFrom(j, testOrigin)

	assertPlaces(t, posting,
		Address{AddressCountry: "CA"},
		Address{AddressCountry: "US"},
	)
}

func TestJobLocationNeverBorrowsOurMacroRegionForAddressRegion(t *testing.T) {
	// Our Regions are macro-regions — "europe", "global" — while schema.org's addressRegion
	// is a state or province. They are different kinds of thing, and an agent reading
	// addressRegion="europe" would file the posting under a province by that name.
	row := openPostingRow()
	row.Countries = []string{"DE"}
	row.Regions = []string{"europe"}
	j := viewOf(row)

	posting := jobPostingFrom(j, testOrigin)

	assertPlaces(t, posting, Address{AddressCountry: "DE"})
}

func TestJobLocationIsAbsentWhenNoPlaceIsKnown(t *testing.T) {
	// An empty Place block says "we know where this is" and then says nothing. Absence is
	// the honest rendering.
	posting := jobPostingFrom(openPosting(), testOrigin)

	if posting.JobLocation != nil {
		t.Errorf("jobLocation = %+v, want nil with no geography at all", posting.JobLocation)
	}
}

func TestJobPostingCarriesTheDescription(t *testing.T) {
	// get_job_detail exists to hand an agent the posting's own text; without this field it
	// would return everything about a job except what the job is.
	j := openPosting()
	j.Description = "We are looking for a Go engineer to work on our ingest pipeline."

	posting := jobPostingFrom(j, testOrigin)

	if posting.Description != j.Description {
		t.Errorf("description = %q, want the posting's own text", posting.Description)
	}
}

// assertPlaces fails unless the posting's jobLocation is exactly want, in order.
func assertPlaces(t *testing.T, posting JobPosting, want ...Address) {
	t.Helper()
	got := make([]Address, len(posting.JobLocation))
	for i, place := range posting.JobLocation {
		got[i] = place.Address
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("jobLocation addresses = %+v, want %+v", got, want)
	}
}

// jobLocationJSON returns the jobLocation member exactly as the wire carries it.
func jobLocationJSON(t *testing.T, posting JobPosting) string {
	t.Helper()
	body, err := json.Marshal(posting)
	if err != nil {
		t.Fatalf("marshal posting: %v", err)
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(body, &members); err != nil {
		t.Fatalf("unmarshal posting: %v", err)
	}
	return string(members["jobLocation"])
}
