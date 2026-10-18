package skilltag

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/dict/vocab"
)

// TestDictionaryInvariants guards the two properties the engine relies on:
// every canonical is a stable slug (lowercase, no spaces), and the vocabulary is
// at least the launch floor so an accidental truncation is caught.
func TestDictionaryInvariants(t *testing.T) {
	for alias, c := range wordAliases {
		assertSlug(t, "wordAliases["+alias+"]", c)
	}
	for _, p := range phraseAliases {
		assertSlug(t, "phraseAliases "+p.alias, p.canonical)
	}
	if got := len(wordAliases) + len(phraseAliases); got < 200 {
		t.Errorf("vocabulary size = %d, want >= 200 (launch floor)", got)
	}
	// Ambiguous English words must never be bare word aliases (they resolve only
	// via an unambiguous alias or a phrase).
	for _, w := range []string{"go", "c", "r"} {
		if _, ok := wordAliases[w]; ok {
			t.Errorf("ambiguous word %q must not be a wordAliases key", w)
		}
	}

	// Acronym canonicals must be valid slugs AND already reachable via an existing
	// alias — an acronym is another route to a known canonical, never a new facet value.
	existing := map[string]bool{}
	for _, c := range wordAliases {
		existing[c] = true
	}
	for _, p := range phraseAliases {
		existing[p.canonical] = true
	}
	for tier, acr := range map[string]map[string]string{"sharedAcronyms": sharedAcronyms, "resumeAcronyms": resumeAcronyms} {
		for surface, c := range acr {
			assertSlug(t, tier+"["+surface+"]", c)
			if !existing[c] {
				t.Errorf("%s[%q] → %q is not an existing canonical (would create a new facet value)", tier, surface, c)
			}
		}
	}

	// categoryScopedAcronyms shares the same invariant as the two tiers above (its
	// canonical must be a valid slug already reachable via an existing alias), plus
	// its own: every allow-listed category must be a real vocab.CategoryValues
	// member, so a typo'd category string can't silently make the tier permanently
	// dead (it would never match any job's resolved category).
	for surface, ca := range categoryScopedAcronyms {
		assertSlug(t, "categoryScopedAcronyms["+surface+"]", ca.canonical)
		if !existing[ca.canonical] {
			t.Errorf("categoryScopedAcronyms[%q] → %q is not an existing canonical (would create a new facet value)", surface, ca.canonical)
		}
		for category := range ca.allowedCategories {
			if !slices.Contains(vocab.CategoryValues, category) {
				t.Errorf("categoryScopedAcronyms[%q] allows category %q, not a member of vocab.CategoryValues", surface, category)
			}
		}
	}
}

func assertSlug(t *testing.T, what, s string) {
	t.Helper()
	if s == "" || s != trimLower(s) {
		t.Errorf("%s: canonical %q is not a lowercase no-space slug", what, s)
	}
}

// nonCorroboratingPhrases names canonicals by hand, so it can drift away from the
// phrase list it describes: a renamed canonical would silently start corroborating
// again, and nothing else would fail. This pins the two together.
func TestNonCorroboratingPhrasesAllExist(t *testing.T) {
	declared := make(map[string]bool, len(phraseAliases))
	for _, p := range phraseAliases {
		declared[p.canonical] = true
	}
	for c := range nonCorroboratingPhrases {
		if !declared[c] {
			t.Errorf("nonCorroboratingPhrases names %q, which no phrase alias emits", c)
		}
	}
}

func TestParse_SalesAndSupportVocab(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "sales",
			in:   "Account executive responsible for business development, pipeline management, cold outreach, sales enablement, and lead generation",
			want: []string{
				"account-executive",
				"business-development",
				"cold-outreach",
				"lead-generation",
				"pipeline-management",
				"sales-enablement",
			},
		},
		{
			name: "support",
			in:   "Help desk specialist responsible for ticket resolution and service desk operations",
			want: []string{
				"help-desk",
				"service-desk",
				"ticket-resolution",
			},
		},
		{
			name: "customer success renewal",
			in:   "Customer success manager owning renewal management for the book of business",
			want: []string{
				"renewal-management",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Parse(c.in); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("Parse(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

// TestNoNearDuplicateCanonicals guards the one failure a growing dictionary makes
// silently: the same facet value declared twice under different separators. Nothing
// errors when it happens — a posting simply comes back carrying BOTH spellings, the
// facet splits its count between them, and a filter on either shows half the jobs.
//
// It is easy to do because the tiers are independent: a batch can add a phrase whose
// canonical already exists as a word alias written the other way. That is exactly how
// "hugging-face" landed beside "huggingface". Folding the separators out catches that
// shape across every tier that declares a canonical.
//
// It does NOT catch the other shape, where one canonical is a QUALIFIED form of
// another ("bedrock" beside the pre-existing "aws-bedrock"), because a substring rule
// cannot tell that apart from the many legitimate pairs the vocabulary already carries
// — sap/sap-hana, azure/azure-synapse, aws/aws-glue. Those need a witness test on the
// term itself, which is why the mined batches carry one per entry.
func TestNoNearDuplicateCanonicals(t *testing.T) {
	fold := strings.NewReplacer("-", "", "_", "", ".", "")
	seen := map[string]string{}
	check := func(where, canonical string) {
		key := fold.Replace(canonical)
		if prev, ok := seen[key]; ok && prev != canonical {
			t.Errorf("%s: canonical %q duplicates %q — they differ only by separators, so a posting matching both splits the facet", where, canonical, prev)
			return
		}
		seen[key] = canonical
	}
	for alias, c := range wordAliases {
		check("wordAliases["+alias+"]", c)
	}
	for _, p := range phraseAliases {
		check("phraseAliases "+p.alias, p.canonical)
	}
	for tier, acr := range map[string]map[string]string{"sharedAcronyms": sharedAcronyms, "resumeAcronyms": resumeAcronyms} {
		for surface, c := range acr {
			check(tier+"["+surface+"]", c)
		}
	}
	for surface, ca := range categoryScopedAcronyms {
		check("categoryScopedAcronyms["+surface+"]", ca.canonical)
	}
}
