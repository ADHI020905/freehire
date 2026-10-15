package plan

import (
	"testing"
	"time"
)

// enforcing is the config the shadow run used to end with, with a REAL daily ceiling pinned
// on the pro plan for every AI feature (2/3/10/10/3, the figures free used to carry). The
// tests built on it exercise the decide/store MECHANISM — idempotency, day rollover, the
// boundary refusal, the tailoring session's own bounds — which needs an allowance somebody
// can consume partway into. Free gets nothing at all now, so the only tier that can carry
// such an allowance is a paid one. Several tailoring-session tests depend on the EXACT tailor
// figure (2): they assert a specific session refused after exactly that many charges.
func enforcing() Config {
	return DefaultConfig().Enforcing().
		WithProCeiling(FeatureTailor, 2).
		WithProCeiling(FeatureFit, 3).
		WithProCeiling(FeatureAssistant, 10).
		WithProCeiling(FeatureDictation, 10).
		WithProCeiling(FeatureCoverLetter, 3)
}

// notEnforcing is DefaultConfig with every feature's refusal forced back off. The pro
// allowances are left as shipped — unlimited behind a fair-use guard — so the guard's own
// tests can stand on it.
func notEnforcing() Config {
	return DefaultConfig().with(func(_ Feature, fc *featureConfig) { fc.enforce = false })
}

// shadowCeiling is notEnforcing with FeatureFit's pro allowance pinned to a real ceiling of
// three, so a shadow decision has a ceiling to shadow past: an unlimited allowance never
// reaches one.
func shadowCeiling() Config {
	return notEnforcing().WithProCeiling(FeatureFit, 3)
}

func TestDecideWithinAllowance(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	d := enforcing().decide(TierPro, FeatureFit, 1, false, now)

	if !d.Allowed {
		t.Fatal("a subscriber one analysis into an allowance of three was refused")
	}
	if d.Charge != 1 {
		t.Errorf("Charge = %d, want 1", d.Charge)
	}
	if d.Used != 2 {
		t.Errorf("Used = %d, want 2 — the decision reports where the user stands after it, not before", d.Used)
	}
	if !d.ResetsAt.Equal(time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("ResetsAt = %v, want the next UTC midnight", d.ResetsAt)
	}
}

func TestDecideRefusesAtTheAllowance(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	d := enforcing().decide(TierPro, FeatureFit, 3, false, now)

	if d.Allowed {
		t.Fatal("a subscriber who has used all three analyses was allowed a fourth")
	}
	if d.Charge != 0 {
		t.Errorf("a refusal charged %d; nothing is taken from someone who was told no", d.Charge)
	}
	if d.FairUse {
		t.Error("an ordinary plan refusal was reported as a fair-use refusal; one is sold against, the other is not shown at all")
	}
	if d.Used != 3 {
		t.Errorf("Used = %d, want 3 unchanged", d.Used)
	}
}

func TestDecideChargesAnAlreadyPaidRefNothing(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	// Over the allowance AND already charged: a recompute of work already paid for must
	// go through, or a user would be punished for looking at their own result again.
	d := enforcing().decide(TierPro, FeatureFit, 99, true, now)

	if !d.Allowed {
		t.Fatal("a recompute of an already-charged reference was refused")
	}
	if d.Charge != 0 {
		t.Errorf("Charge = %d, want 0 — the reference was already paid for", d.Charge)
	}
	if d.Used != 99 {
		t.Errorf("Used = %d, want 99 unchanged", d.Used)
	}
}

func TestDecideShadowRecordsButNeverRefuses(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	d := shadowCeiling().decide(TierPro, FeatureFit, 3, false, now) // enforcement off

	if !d.Allowed {
		t.Fatal("shadow mode refused; the point of it is to count what a limit WOULD stop, without stopping anyone")
	}
	if !d.Shadowed {
		t.Error("the decision does not record that it would have refused; that flag is the whole measurement")
	}
	if d.Charge != 1 {
		t.Errorf("Charge = %d, want 1 — shadow mode still records consumption, or the counter describes nothing", d.Charge)
	}
	if d.Enforced {
		t.Error("a shadow decision reports itself enforced; the clients that pre-block read this flag, and one that reads true refuses what the server just allowed")
	}
}

// Enforcement travels with the decision because every surface that pre-blocks needs it. A
// client that can only see used-against-limit hides a button on a spent allowance the
// server would still honour — a wall nobody meant to build, and one that suppresses the
// very requests the shadow run is counting.
func TestDecisionCarriesWhetherTheCeilingIsLive(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	d := enforcing().decide(TierPro, FeatureFit, 0, false, now)

	if !d.Enforced {
		t.Error("an enforcing configuration produced a decision that says it is not enforced")
	}
}

// Standing.Refuses is the same rule, asked by whoever holds a standing rather than by the
// store — including the SPA, which receives the flag on the wire.
func TestStandingRefusesOnlyWhenTheCeilingIsLive(t *testing.T) {
	spent := Standing{Feature: FeatureFit, Used: 3, Limit: 3}

	if !spent.Exhausted() {
		t.Fatal("a standing at its limit does not report itself exhausted")
	}
	if spent.Refuses() {
		t.Error("a spent allowance refuses while enforcement is off; that is the shadow run refusing people")
	}
	spent.Enforced = true
	if !spent.Refuses() {
		t.Error("a spent allowance does not refuse with enforcement on")
	}
	unlimited := Standing{Feature: FeatureFit, Used: 500, Unlimited: true, Enforced: true}
	if unlimited.Refuses() {
		t.Error("an unlimited standing refuses; the fair-use guard belongs at the point of use, not here")
	}
}

func TestDecideProHasNoPlanCeiling(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	cfg := DefaultConfig().Enforcing()
	d := cfg.decide(TierPro, FeatureFit, 50, false, now)

	if !d.Allowed {
		t.Fatal("a pro user was refused well below the fair-use guard")
	}
	if !d.Unlimited {
		t.Error("a pro decision reports a countable limit; pro is the same product without the ceilings")
	}
}

func TestDecideFairUseGuardStopsAScript(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	cfg := DefaultConfig().Enforcing()
	guard := cfg.Allowance(TierPro, FeatureFit).Limit
	d := cfg.decide(TierPro, FeatureFit, guard, false, now)

	if d.Allowed {
		t.Fatal("a pro account past its fair-use guard was allowed; one subscription can drain the gateway")
	}
	if !d.FairUse {
		t.Error("the refusal is not marked as a fair-use one, so it would be shown to the user as a plan limit")
	}
}

func TestFairUseGuardHoldsEvenInShadow(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	cfg := notEnforcing() // enforcement off
	d := cfg.decide(TierPro, FeatureFit, cfg.Allowance(TierPro, FeatureFit).Limit, false, now)

	// Shadow mode exists so nobody is refused on an unread number. The guard is not that
	// kind of number: it sits twenty times above human behaviour, so anything reaching it
	// is automation, and it protects the gateway rather than selling a subscription.
	if d.Allowed {
		t.Fatal("the fair-use guard was disabled by shadow mode; shadow protects people from unproven ceilings, not the gateway from a script")
	}
	if !d.FairUse {
		t.Error("the shadow-mode guard refusal is not marked as a fair-use one")
	}
}

func TestDecideRefusesAnUnconfiguredFeature(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	d := enforcing().decide(TierPro, Feature("no-such-feature"), 0, false, now)

	if d.Allowed {
		t.Fatal("a feature nobody configured was allowed; forgetting to configure a surface must surface as refused, not as free")
	}
}

// Free gets no AI at all, and that is a rule about the TIER rather than an allowance that
// happens to be zero: it holds before any allowance is read, so nothing configured, set in
// the environment or already in the ledger can reopen one.
func TestDecideRefusesFreeWhateverElseIsTrue(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	cfg := enforcing()
	for _, f := range AllFeatures() {
		for _, charged := range []bool{false, true} {
			d := cfg.decide(TierFree, f, 0, charged, now)
			if d.Allowed {
				t.Errorf("%s (already charged: %v): free was allowed; no free tier may reach a model call", f, charged)
			}
			if d.Charge != 0 || d.Shadowed || d.FairUse {
				t.Errorf("%s: a free refusal reads charge=%d shadowed=%v fairUse=%v, want a plain refusal",
					f, d.Charge, d.Shadowed, d.FairUse)
			}
			// The fields the 402 body is rendered from are still there.
			if d.Tier != TierFree || d.Feature != f || d.ResetsAt.IsZero() {
				t.Errorf("%s: refusal lost the fields its body renders from: %+v", f, d)
			}
		}
	}
	// Shadow mode does not reopen it either: it protects people from an unread ceiling, and
	// free has no ceiling to read.
	if d := notEnforcing().decide(TierFree, FeatureFit, 0, false, now); d.Allowed {
		t.Error("free was allowed in shadow mode")
	}
	// An unknown tier is not paid either.
	if d := cfg.decide(Tier("staff"), FeatureFit, 0, false, now); d.Allowed {
		t.Error("an unknown tier was allowed; only a tier somebody bought may spend")
	}
}

func TestDecideAdmitsEveryPaidTier(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	cfg := DefaultConfig()
	for _, tier := range []Tier{TierPro, TierUltra} {
		for _, f := range AllFeatures() {
			if d := cfg.decide(tier, f, 0, false, now); !d.Allowed {
				t.Errorf("%s was refused %s on an untouched day", tier, f)
			}
		}
	}
}

// An entitlement is over at the instant it expires, and what is left is free — which is
// refused. The pairing is the point: TierOf alone is tested elsewhere, and decide alone
// above; a lapsed subscription is where the two meet.
func TestALapsedEntitlementIsRefused(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	tier := TierOf(now.Add(-time.Minute), time.Time{}, now)
	if tier != TierFree {
		t.Fatalf("an expired pro_until resolved to %q, want free", tier)
	}
	if d := DefaultConfig().decide(tier, FeatureFit, 0, false, now); d.Allowed {
		t.Fatal("an expired subscription was allowed a model call")
	}
}
