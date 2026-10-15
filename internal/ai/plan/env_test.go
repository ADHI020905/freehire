package plan

import (
	"regexp"
	"testing"
	"time"
)

func TestConfigFromEnvDefaultsToTheShippedConfig(t *testing.T) {
	cfg := ConfigFromEnv()

	if cfg.Allowance(TierPro, FeatureFit) != DefaultConfig().Allowance(TierPro, FeatureFit) {
		t.Errorf("an unset environment changed the pro allowance")
	}
	for _, f := range AllFeatures() {
		if !cfg.Enforced(f) {
			t.Errorf("%q does not enforce with nothing set; every metered AI feature ships enforcing", f)
		}
	}
}

// enforcedFeatures is the pure function PLAN_ENFORCE's per-name behaviour actually lives
// in. It is tested directly rather than through ConfigFromEnv().Enforced(), because every
// feature now enforces by default — a naming test built on that combination could no
// longer tell "named" from "already on regardless".
func TestEnforcementIsNamedPerFeature(t *testing.T) {
	got := enforcedFeatures("match, dictation")

	if !got[FeatureFit] || !got[FeatureDictation] {
		t.Error("a named feature does not enforce")
	}
	if got[FeatureTailor] || got[FeatureAssistant] {
		t.Error("an unnamed feature enforces; the switch must turn on exactly what it names")
	}
}

func TestEnforceAllIsSpelledOut(t *testing.T) {
	got := enforcedFeatures("all")

	for _, f := range AllFeatures() {
		if !got[f] {
			t.Errorf("%q does not enforce under 'all'", f)
		}
	}
}

func TestAnUnknownFeatureNameIsIgnoredNotGuessed(t *testing.T) {
	// A typo must not silently enforce something else, and must not stop the features
	// that were spelled correctly from taking effect.
	got := enforcedFeatures("mach,tailor")

	if !got[FeatureTailor] {
		t.Error("a correctly named feature was lost because another name was misspelled")
	}
	if got[FeatureFit] {
		t.Error("a misspelled name was resolved to a feature by guessing")
	}
}

// PLAN_FREE_DAILY_<FEATURE> used to reopen a free allowance without a deploy. It is no longer
// read: a host that still carries one must not be able to hand free accounts a model call.
func TestAFreeAllowanceCannotBeSetFromTheEnvironment(t *testing.T) {
	t.Setenv("PLAN_FREE_DAILY_MATCH", "7")
	t.Setenv("PLAN_FREE_DAILY_COVER_LETTER", "5")
	cfg := ConfigFromEnv()

	for _, f := range AllFeatures() {
		if a := cfg.Allowance(TierFree, f); a.Limit != 0 || a.Unlimited {
			t.Errorf("free allowance for %s = %+v with PLAN_FREE_DAILY_* set, want nothing", f, a)
		}
	}
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	if d := cfg.decide(TierFree, FeatureFit, 0, false, now); d.Allowed {
		t.Error("free was allowed a fit analysis with PLAN_FREE_DAILY_MATCH set")
	}
}

func TestADashedFeatureIsOverriddenByItsUnderscoredName(t *testing.T) {
	// The two features whose ledger value carries a dash. A suffix taken verbatim from it
	// asks for PLAN_PRO_DAILY_AUTO-APPLY, a name systemd's EnvironmentFile= will not parse,
	// so the documented lever would be unsettable on the host it exists for — and this one
	// is a ceiling under a plan people have already bought.
	t.Setenv("PLAN_PRO_DAILY_AUTO_APPLY", "9")
	t.Setenv("PLAN_PRO_DAILY_COVER_LETTER", "5")
	t.Setenv("PLAN_ULTRA_DAILY_AUTO_APPLY", "250")
	cfg := ConfigFromEnv()

	if got := cfg.Allowance(TierPro, FeatureAutoApply).Limit; got != 9 {
		t.Errorf("pro daily allowance for auto-apply = %d, want 9", got)
	}
	if got := cfg.Allowance(TierUltra, FeatureAutoApply).Limit; got != 250 {
		t.Errorf("ultra daily allowance for auto-apply = %d, want 250", got)
	}
	if got := cfg.Allowance(TierPro, FeatureCoverLetter).Limit; got != 5 {
		t.Errorf("pro daily allowance for cover-letter = %d, want 5", got)
	}
}

func TestEveryFeatureHasASettableOverrideName(t *testing.T) {
	// The guard for the next feature named with a dash: an override an operator cannot set
	// is indistinguishable from one that was ignored.
	settable := regexp.MustCompile(`^[A-Z0-9_]+$`)
	for _, f := range AllFeatures() {
		if suffix := envSuffix(f); !settable.MatchString(suffix) {
			t.Errorf("%q overrides PLAN_*_DAILY_%s, which is not a legal environment "+
				"variable name", f, suffix)
		}
	}
}

func TestAnUnreadableOverrideKeepsTheDefault(t *testing.T) {
	// A typo in a number must not silently mean zero. Zero would refuse the feature to every
	// subscriber, which looks exactly like a deliberate decision.
	want := DefaultConfig().Allowance(TierPro, FeatureFit)
	t.Setenv("PLAN_PRO_DAILY_MATCH", "three")
	if got := ConfigFromEnv().Allowance(TierPro, FeatureFit); got != want {
		t.Errorf("an unparseable override left %+v, want the default %+v", got, want)
	}

	t.Setenv("PLAN_PRO_DAILY_MATCH", "-2")
	if got := ConfigFromEnv().Allowance(TierPro, FeatureFit); got != want {
		t.Errorf("a negative override left %+v, want the default %+v", got, want)
	}
}

func TestTurnCeilingOverride(t *testing.T) {
	t.Setenv("PLAN_TAILOR_TURNS_PER_SESSION", "25")
	if got := ConfigFromEnv().TailorTurnsPerSession; got != 25 {
		t.Errorf("turn ceiling = %d, want 25", got)
	}
}
