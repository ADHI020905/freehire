package main

import (
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/job/dictgap"
)

func TestWriteGeoGapReport(t *testing.T) {
	report := dictgap.GeoGapReport{
		Gaps: []dictgap.GeoGapCandidate{
			{Location: "свердловская область", Count: 400, LLMCountry: "ru", LLMShare: 0.97},
			{Location: "пермский край", Count: 160, LLMCountry: "ru", LLMShare: 1},
		},
		Conflicts: []dictgap.GeoGapCandidate{
			{Location: "tbilisi, georgia", Count: 40, LLMCountry: "us", LLMShare: 1, DictionaryCountries: []string{"ge"}},
		},
	}
	var b strings.Builder
	if err := writeGeoGapReport(&b, report, 1); err != nil {
		t.Fatal(err)
	}
	want := "# gaps\tcount\tshare\tllm\tlocation\n" +
		"400\t97%\tru\tсвердловская область\n" +
		"# conflicts\tcount\tshare\tllm\tdictionary\tlocation\n" +
		"40\t100%\tus\tge\ttbilisi, georgia\n"
	if b.String() != want {
		t.Errorf("report =\n%s\nwant\n%s", b.String(), want)
	}
}
