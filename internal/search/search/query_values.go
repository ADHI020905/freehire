package search

import (
	"net/url"
	"slices"
	"sort"
	"strings"

	"github.com/strelov1/freehire/internal/dict/vocab"
	"github.com/strelov1/freehire/internal/job/jobreality"
)

// boolValues is the vocabulary of the boolean facets. The true-or-absent ones read
// anything but "false" as true (see trueOrAbsentFragment), so a typo there is not
// dropped but MISREAD — `requires_clearance=flase` asks for exactly the postings the
// caller meant to exclude — which is the stronger reason to name it.
var boolValues = []string{"true", "false"}

// closedFacets maps each facet whose values come from a closed vocabulary to that
// vocabulary — the dictionary itself, never a copy, so a value added to vocab joins
// this check by existing. A value outside it matches no posting, so the filter narrows
// the answer to nothing and the empty page reads exactly like an empty market.
//
// The facets NOT listed have no finite list to check against, and writing one here would
// be a second dictionary drifting from the real one:
//   - skills, cities, company_slug: open by nature; an unknown value can be legitimate.
//   - countries: any ISO code is a valid ask, including one no posting carries yet.
//   - source: the adapter registry grows, and non-adapter sources (telegram) exist.
//   - collections: curated tags kept in data, not in code.
//   - salary_currency, posting_language: open code systems (ISO 4217, ISO 639).
//
// Every value is read through splitFacetValues, the same way filterFromValues reads it,
// so the report and the filter cannot disagree about what was asked.
var closedFacets = map[string][]string{
	"work_mode": vocab.WorkModeValues,
	// RegionUnspecified is a reserved value the filter maps to IS EMPTY.
	"regions":   append(slices.Clone(vocab.RegionValues), RegionUnspecified),
	"seniority": vocab.SeniorityValues,
	// The retired category is translated, not dropped (see retiredCategory).
	"category":        append(slices.Clone(vocab.CategoryValues), vocab.RetiredCategoryManagement),
	"employment_type": vocab.EmploymentTypeValues,
	"education_level": vocab.EducationLevelValues,
	"company_type":    vocab.CompanyTypeValues,
	"company_size":    vocab.CompanySizeValues,
	"salary_period":   vocab.SalaryPeriodValues,
	"relocation":      vocab.RelocationValues,
	"english_level":   vocab.EnglishLevelValues,
	"role_type":       vocab.RoleTypeValues,
	"ai_archetype":    vocab.AIArchetypeValues,
	"domains":         vocab.DomainValues,
	"reality": {
		jobreality.ClassFresh, jobreality.ClassStale, jobreality.ClassLikelyEvergreen,
	},
	"is_tech":               boolValues,
	RequiresClearanceParam:  boolValues,
	AIInterviewParam:        boolValues,
	AutoApplyAvailableParam: boolValues,
}

// IgnoredValues reports, per query param, the values of a closed-vocabulary facet (or its
// `_exclude` form) that fall outside the vocabulary. nil when there are none.
//
// It is the value-level counterpart of UnknownParams and follows the same rule: report,
// never refuse. A saved search or a shared link may carry a value the vocabulary has since
// retired, and a 400 would break it; staying silent is what made `work_mode=foo` answer
// `total: 0` indistinguishable from a market with no such postings. The filter is left
// alone — the value still reaches it — so the report describes the answer rather than
// changing it.
//
// A value is compared trimmed and case-insensitively, the same check the MCP app makes
// (mcpapp's known): a value the filter could match must never be named as ignored, and a
// near-miss in spelling is the caller's to fix, not ours to guess.
//
// Bounded like UnknownParams — every reported value is copied from a public, anonymous
// request into the response — to maxUnknownParamsReported params of at most
// maxUnknownParamsReported values each, chosen in sorted order so the report is stable.
func IgnoredValues(v url.Values) map[string][]string {
	var params []string
	out := map[string][]string{}
	for facet, allowed := range closedFacets {
		for _, param := range []string{facet, facet + "_exclude"} {
			var bad []string
			for _, val := range splitFacetValues(v[param]) {
				if !inVocabulary(allowed, val) && !slices.Contains(bad, val) {
					bad = append(bad, val)
				}
			}
			if len(bad) == 0 {
				continue
			}
			sort.Strings(bad)
			if len(bad) > maxUnknownParamsReported {
				bad = bad[:maxUnknownParamsReported]
			}
			out[param] = bad
			params = append(params, param)
		}
	}
	if len(params) == 0 {
		return nil
	}
	sort.Strings(params)
	for _, param := range params[min(len(params), maxUnknownParamsReported):] {
		delete(out, param)
	}
	return out
}

func inVocabulary(allowed []string, value string) bool {
	value = strings.TrimSpace(value)
	return slices.ContainsFunc(allowed, func(a string) bool { return strings.EqualFold(a, value) })
}
