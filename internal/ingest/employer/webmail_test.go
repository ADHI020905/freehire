package employer

import "testing"

func TestIsAcademicDomain(t *testing.T) {
	cases := map[string]bool{
		"upenn.edu":              true,
		"pennmedicine.upenn.edu": true,
		"ox.ac.uk":               true,
		"unimelb.edu.au":         true,
		"tsinghua.edu.cn":        true,
		"u-tokyo.ac.jp":          true,
		"acme.com":               false,
		"edu.example.com":        false, // "edu" as a leading label is just a host name
		"school.ac":              false, // .ac is Ascension Island's TLD, not an academic zone
		"educative.io":           false,
		"":                       false,
	}
	for domain, want := range cases {
		if got := isAcademicDomain(domain); got != want {
			t.Errorf("isAcademicDomain(%q) = %v, want %v", domain, got, want)
		}
	}
}
