package atsapply

import (
	"testing"
)

// The two platforms this package can drive, and the values it drives them by.
//
// Every number and string here was read off a live posting, which is the point: a submit
// click cannot be withdrawn, so what triggers one is measured rather than inferred. See
// layout.go's own comment for the two occasions this package inferred instead.
func TestLayoutFor_KnowsTheTwoProvidersWithAFillPath(t *testing.T) {
	gh, ok := layoutFor("greenhouse")
	if !ok {
		t.Fatal("greenhouse has no layout")
	}
	want := formLayout{formSelector: "application-form", submitSelector: "#submit_app", addressBy: byID}
	if gh != want {
		t.Errorf("greenhouse layout = %+v, want %+v", gh, want)
	}

	// Measured on jobs.lever.co/coderio/6ce0e52b-e7bc-462f-ac9e-1d31c8c0e037/apply,
	// 2026-09-09: the form carries the same id Greenhouse's does, the button a person
	// clicks is #btn-submit, and the inputs carry no id at all — only name.
	lv, ok := layoutFor("lever")
	if !ok {
		t.Fatal("lever has no layout")
	}
	wantLever := formLayout{formSelector: "application-form", submitSelector: "#btn-submit", addressBy: byName, applyPath: "/apply"}
	if lv != wantLever {
		t.Errorf("lever layout = %+v, want %+v", lv, wantLever)
	}
}

// A platform nobody measured is never driven. Its attempts park for the reason that is true
// of them — no fill path exists — rather than reaching a browser on a page this package
// knows nothing about.
func TestLayoutFor_RefusesAProviderWithNoFillPath(t *testing.T) {
	for _, provider := range []string{"ashby", "workable", "recruitee", ""} {
		if _, ok := layoutFor(provider); ok {
			t.Errorf("layoutFor(%q) returned a layout; no fill path exists for it", provider)
		}
	}
}

// The registry and fillProviders answer one question in two files, and must not drift: a
// platform Submit will try to fill, with no layout to fill it by, would reach a submit
// click with selectors matching nothing.
//
// This passes trivially while Greenhouse is the only entry in fillProviders. It starts
// carrying weight when a platform is added THERE — adding one to layouts alone creates no
// risk, since the containment runs in one direction only.
func TestLayoutFor_CoversEveryFillProvider(t *testing.T) {
	for provider := range fillProviders {
		if _, ok := layoutFor(provider); !ok {
			t.Errorf("%q is in fillProviders but has no layout", provider)
		}
	}
}

// The selector must address a field the way its platform names it — and a name carrying
// brackets (Lever's `urls[LinkedIn]`, and every employer question as
// `cards[<uuid>][field0]`) must survive quoted intact, since brackets are selector syntax
// when unquoted.
func TestFieldSelector_AddressesAFieldTheWayItsPlatformNamesIt(t *testing.T) {
	if got := fieldSelector("first_name", byID); got != "#first_name" {
		t.Errorf("byID selector = %q, want #first_name", got)
	}
	if got := fieldSelector("name", byName); got != `[name="name"]` {
		t.Errorf("byName selector = %q, want [name=\"name\"]", got)
	}
	if got := fieldSelector("urls[LinkedIn]", byName); got != `[name="urls[LinkedIn]"]` {
		t.Errorf("byName selector for a bracketed name = %q, want it quoted intact", got)
	}
}

// Lever keeps its layout without being a fill provider, and that is not an oversight. The
// layout's applyPath is what tells ANY executor which page the form is on, and the cloud
// agent that now submits Lever reads it through agentTargetURL. The submit selector and
// addressing beside it are dormant rather than dead — they are what a Lever fill path would
// need again if the captcha ever stopped deciding these attempts.
func TestLeverKeepsItsLayoutAfterMovingToTheCloudAgent(t *testing.T) {
	layout, ok := layoutFor("lever")
	if !ok {
		t.Fatal("lever lost its layout, so nothing knows its form is at /apply")
	}
	if layout.applyPath != "/apply" {
		t.Errorf("lever applyPath = %q, want /apply", layout.applyPath)
	}
}

// Lever renders the posting and the application form on two different pages; Greenhouse
// renders them on one.
//
// The worker navigates to the posting's own URL (jobs.url), which for Greenhouse IS the
// form. On Lever that page carries the description and no form at all — the form lives at
// the same URL plus /apply. A live attempt on queue entry 6 parked as
// unrecognized_form_layout for exactly this reason: the page loaded fine and simply had no
// application form on it (freehire, 2026-09-10).
func TestApplyURL_SendsEachPlatformToThePageItsFormIsOn(t *testing.T) {
	gh, _ := layoutFor("greenhouse")
	const ghPosting = "https://job-boards.greenhouse.io/garnerhealth/jobs/6181651004"
	if got := gh.applyURL(ghPosting); got != ghPosting {
		t.Errorf("greenhouse apply url = %q, want the posting URL unchanged", got)
	}

	lv, _ := layoutFor("lever")
	const lvPosting = "https://jobs.lever.co/jobgether/08a82436-bd70-48bc-8304-1355c12a74ae"
	if got := lv.applyURL(lvPosting); got != lvPosting+"/apply" {
		t.Errorf("lever apply url = %q, want the posting URL plus /apply", got)
	}
}

// A posting URL that already ends in the form's path must not gain a second one.
func TestApplyURL_DoesNotDoubleTheSuffix(t *testing.T) {
	lv, _ := layoutFor("lever")
	const alreadyApply = "https://jobs.lever.co/jobgether/08a82436/apply"
	if got := lv.applyURL(alreadyApply); got != alreadyApply {
		t.Errorf("apply url = %q, want it unchanged when the path is already there", got)
	}
	if got := lv.applyURL(alreadyApply + "/"); got != alreadyApply+"/" {
		t.Errorf("apply url = %q, want a trailing slash to count as the same path", got)
	}
}
