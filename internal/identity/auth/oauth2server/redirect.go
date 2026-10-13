package oauth2server

import (
	"fmt"
	"net/url"
	"strings"
)

// RedirectURIAllowed reports whether candidate may be used for this authorize
// request, given the client's registered redirect_uris. Matching is exact string
// equality, with one exception: a loopback redirect (127.0.0.1, [::1], or
// localhost) matches on scheme+host+path alone, ignoring the port — native MCP
// clients bind an ephemeral local port per request and cannot register it in
// advance (RFC 8252 §7.3).
func RedirectURIAllowed(registered []string, candidate string) bool {
	for _, r := range registered {
		if r == candidate {
			return true
		}
		if loopbackMatch(r, candidate) {
			return true
		}
	}
	return false
}

func loopbackMatch(registered, candidate string) bool {
	a, errA := url.Parse(registered)
	b, errB := url.Parse(candidate)
	if errA != nil || errB != nil {
		return false
	}
	if a.Scheme != b.Scheme || a.Path != b.Path || a.RawQuery != "" || b.RawQuery != "" {
		return false
	}
	return isLoopbackHost(a.Hostname()) && a.Hostname() == b.Hostname()
}

func isLoopbackHost(host string) bool {
	return host == "127.0.0.1" || host == "::1" || host == "localhost"
}

// maxRedirectURILength bounds one registered redirect_uri. Real callbacks are a
// few dozen characters; the bound exists so an unauthenticated registration
// cannot park arbitrary payloads in oauth_clients.
const maxRedirectURILength = 2048

// blockedRedirectSchemes are schemes a browser would interpret itself rather than
// hand to an app, so a 302 carrying the code there never reaches a legitimate
// client — it only serves somebody hoping to run script or read local files.
var blockedRedirectSchemes = map[string]bool{
	"javascript": true, "data": true, "vbscript": true, "file": true, "blob": true, "about": true,
}

// ValidateRedirectURI decides whether a redirect_uri may be REGISTERED (RFC 7591).
// Three shapes are admitted, the three MCP clients actually use (RFC 8252):
//
//   - https on any host — claude.ai, chatgpt.com and every hosted client;
//   - http on a loopback host only — Claude Code, Claude Desktop, VS Code bind a
//     local port, and plain http anywhere else would put the code on the wire;
//   - a private-use app scheme (cursor://…) — the OS hands it to the installed app.
//
// It cannot judge whether the host belongs to whoever names the client; that is
// the consent screen's job, which therefore names the host (see oauthConsentPage).
// A fragment is refused because RFC 6749 §3.1.2 forbids one in a redirection endpoint.
func ValidateRedirectURI(raw string) error {
	if len(raw) > maxRedirectURILength {
		return fmt.Errorf("redirect_uri exceeds %d characters", maxRedirectURILength)
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return fmt.Errorf("redirect_uri %q is not an absolute URI", raw)
	}
	// Checked on the raw string: url.Parse reports an empty fragment ("…/cb#") as none.
	if strings.Contains(raw, "#") {
		return fmt.Errorf("redirect_uri %q must not carry a fragment", raw)
	}
	switch scheme := strings.ToLower(u.Scheme); {
	case scheme == "https":
		if u.Hostname() == "" {
			return fmt.Errorf("redirect_uri %q has no host", raw)
		}
	case scheme == "http":
		if !isLoopbackHost(u.Hostname()) {
			return fmt.Errorf("redirect_uri %q: plain http is allowed only on a loopback host", raw)
		}
	case blockedRedirectSchemes[scheme]:
		return fmt.Errorf("redirect_uri scheme %q is not allowed", u.Scheme)
	}
	return nil
}
