package location

import (
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/dict/vocab"
	"github.com/strelov1/freehire/internal/platform/stringset"
)

// regionAliases are the labels a structured source states for an area that spans
// several canonical regions, or sits inside one, without being a country or a
// macro-region name nameToRegion already resolves. A label covering several regions
// expands to all of them, so the posting stays reachable under each.
var regionAliases = map[string][]string{
	"emea":             {"eu", "uk", "mena", "africa"},
	"americas":         {"north_america", "latam"},
	"amer":             {"north_america", "latam"},
	"oceania":          {"apac"},
	"south asia":       {"apac"},
	"caribbean":        {"latam"},
	"dach":             {"eu"},
	"nordic":           {"eu"},
	"nordics":          {"eu"},
	"eea":              {"eu"},
	"eastern europe":   {"eu"},
	"southeast europe": {"eu"},
	"caucasus":         {"cis"},
}

// NormalizeRegions folds a structured region signal into vocab.RegionValues. A
// canonical value is kept; a country (code, alpha-3 or name) or a country-prefixed
// subdivision code of one to three letters or digits ("us-fl", "ru-mow") folds into
// its country's region; a macro-region name or a regionAliases label resolves to the
// region(s) it names. Anything else — a timezone band, an exclusion phrase, a
// placeholder — is dropped, the same never-guess contract Parse keeps. The result is
// deduplicated and sorted; nil when nothing resolves.
func NormalizeRegions(in []string) []string {
	set := make(map[string]struct{}, len(in))
	for _, raw := range in {
		for _, r := range normalizeRegion(raw) {
			set[r] = struct{}{}
		}
	}
	if len(set) == 0 {
		return nil
	}
	return stringset.Sorted(set)
}

func normalizeRegion(raw string) []string {
	v := strings.ToLower(strings.TrimSpace(raw))
	if slices.Contains(vocab.RegionValues, v) {
		return []string{v}
	}
	name := strings.ReplaceAll(v, "_", " ")
	if rs, ok := regionAliases[name]; ok {
		return rs
	}
	if r, ok := nameToRegion[name]; ok {
		return []string{r}
	}
	if r, ok := countryToRegion[NormalizeCountry(name)]; ok {
		return []string{r}
	}
	if country, ok := subdivisionCountry(v); ok {
		if r, ok := countryToRegion[country]; ok {
			return []string{r}
		}
	}
	return nil
}

func subdivisionCountry(v string) (string, bool) {
	if len(v) < 4 || len(v) > 6 || (v[2] != '-' && v[2] != '_') {
		return "", false
	}
	for i := 3; i < len(v); i++ {
		c := v[i]
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return "", false
		}
	}
	return v[:2], true
}
