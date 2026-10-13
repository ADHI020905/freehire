package handler

import (
	"strings"
	"testing"
)

func TestRedirectDestination(t *testing.T) {
	cases := map[string]string{
		"https://attacker.example.com/cb":                    "attacker.example.com",
		"https://claude.ai/api/mcp/auth_callback":            "claude.ai",
		"http://127.0.0.1:51739/callback":                    "an app on this computer (127.0.0.1)",
		"cursor://anysphere.cursor-retrieval/oauth/callback": "the cursor:// app on this computer",
	}
	for uri, want := range cases {
		if got := redirectDestination(uri); got != want {
			t.Errorf("redirectDestination(%q) = %q, want %q", uri, got, want)
		}
	}
}

// A client may call itself anything, so the page must name the redirect host
// beside the self-chosen name — that host is what a phishing registration
// cannot fake.
func TestOAuthConsentPage_NamesRedirectHostAndFlagsUnverifiedName(t *testing.T) {
	page := oauthConsentPage("Claude", authorizeParams{
		ClientID:    "cid",
		RedirectURI: "https://attacker.example.com/cb",
	})
	if !strings.Contains(page, "<strong>attacker.example.com</strong>") {
		t.Errorf("consent page does not name the redirect host:\n%s", page)
	}
	if !strings.Contains(page, "not verified") {
		t.Errorf("consent page does not say the client name is unverified:\n%s", page)
	}
}
