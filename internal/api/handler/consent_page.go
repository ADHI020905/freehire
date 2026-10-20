package handler

import (
	"fmt"
	"html"
	"strings"
)

// consentScreen is what one server-rendered approval screen says. Every field but
// Caution is plain text and is escaped by renderConsentScreen; Hidden rides back to Action
// as hidden form fields. The OAuth authorize flow (ChatGPT, Claude, a CLI on a
// loopback port) and the browser-extension connect share it, so the two screens
// a person approves an agent on cannot drift apart in look or in what they warn.
type consentScreen struct {
	Title    string // the heading, e.g. "Connect ChatGPT to freehire"
	SignedIn string // the account's email; empty omits the line
	Allows   []string
	Caution  string // trusted HTML (it may <strong> a host) — the caller escapes what it interpolates
	Action   string // the POST target
	Hidden   [][2]string
	Decline  string // the Cancel button's decision value, which each POST handler names its own way
	Origin   string // the web app, for the privacy and terms links
}

// freehireMark is the logo from web/static/favicon.svg, inlined so the page needs
// no second request and its colour can follow the page's own theme.
const freehireMark = `<svg class="mark" viewBox="0 0 512 512" aria-hidden="true"><path fill-rule="evenodd" clip-rule="evenodd" d="M256 56C366.457 56 456 145.543 456 256C456 366.457 366.457 456 256 456C145.543 456 56 366.457 56 256C56 145.543 145.543 56 256 56ZM256 166L346 256L256 346L166 256L256 166Z"/></svg>`

// consentCSS carries the design system's light and dark tokens by value
// (design-system/dist/tokens-*.css): this page is served by the API, outside the
// SPA build that would otherwise hand them over.
const consentCSS = `
:root{--bg:oklch(.997 .003 130);--fg:oklch(.13 0 0);--card:oklch(1 0 0);--muted:oklch(.43 0 0);--border:oklch(.92 0 0);--brand:#5b6f00;--brand-fg:#fff;--tile:oklch(.96 0 0);color-scheme:light dark}
@media (prefers-color-scheme:dark){:root{--bg:oklch(.16 .006 110);--fg:oklch(.985 0 0);--card:oklch(.2 0 0);--muted:oklch(.72 0 0);--border:oklch(1 0 0/10%);--brand:oklch(.72 .095 121);--brand-fg:oklch(.16 .006 110);--tile:oklch(1 0 0/6%)}}
*{box-sizing:border-box}
body{margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;padding:1.5rem;background:var(--bg);color:var(--fg);font:15px/1.5 ui-sans-serif,system-ui,-apple-system,"Segoe UI",Roboto,"Helvetica Neue",Arial,sans-serif}
.card{width:100%;max-width:28rem;background:var(--card);border:1px solid var(--border);border-radius:.875rem;overflow:hidden}
.brand{display:flex;align-items:center;justify-content:center;gap:.5rem;padding:1rem;border-bottom:1px solid var(--border);font-weight:600;letter-spacing:-.01em}
.mark{width:1.375rem;height:1.375rem;fill:currentColor}
.body{padding:1.75rem 2rem 1.5rem}
.lock{width:2.75rem;height:2.75rem;margin:0 auto 1rem;display:flex;align-items:center;justify-content:center;border-radius:.625rem;background:var(--tile)}
h1{margin:0;text-align:center;font-size:1.375rem;line-height:1.3;letter-spacing:-.02em}
.who{margin:.375rem 0 0;text-align:center;color:var(--muted);font-size:.875rem}
.section{margin:1.5rem 0 0;padding:1.25rem 0;border-top:1px solid var(--border);border-bottom:1px solid var(--border)}
.label{margin:0 0 .75rem;font-size:.75rem;font-weight:600;letter-spacing:.06em;text-transform:uppercase;color:var(--muted)}
ul{list-style:none;margin:0;padding:0;display:grid;gap:.625rem}
li{display:flex;gap:.625rem;align-items:flex-start}
li svg{flex:none;width:1.125rem;height:1.125rem;margin-top:.15rem}
.ok{stroke:var(--brand)}
.caution{margin-top:1rem;color:var(--muted);font-size:.875rem}
.caution svg{stroke:var(--muted)}
form{display:grid;grid-template-columns:1fr 1fr;gap:.75rem;margin:1.5rem 0 0}
button{font:inherit;font-weight:500;height:2.75rem;border-radius:.625rem;cursor:pointer;border:1px solid var(--border);background:transparent;color:var(--fg)}
button:hover{background:var(--tile)}
button.primary{background:var(--brand);border-color:var(--brand);color:var(--brand-fg)}
button.primary:hover{filter:brightness(1.08)}
button:focus-visible{outline:2px solid var(--brand);outline-offset:2px}
.foot{margin:1.25rem 0 0;text-align:center;color:var(--muted);font-size:.8125rem}
.foot a{color:inherit}
`

const (
	lockIcon    = `<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="4" y="11" width="16" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/><circle cx="12" cy="16" r="1"/></svg>`
	checkIcon   = `<svg class="ok" viewBox="0 0 24 24" fill="none" stroke-width="2.25" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M20 6 9 17l-5-5"/></svg>`
	cautionIcon = `<svg viewBox="0 0 24 24" fill="none" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 3 4 6v6c0 5 3.5 8 8 9 4.5-1 8-4 8-9V6z"/><path d="M12 8v4M12 16h.01"/></svg>`
)

func renderConsentScreen(s consentScreen) string {
	e := html.EscapeString
	var b strings.Builder
	fmt.Fprintf(&b, `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="robots" content="noindex"><title>%s</title><link rel="icon" href="/favicon.svg"><style>%s</style></head>
<body>
<main class="card">
  <div class="brand">%s<span>freehire</span></div>
  <div class="body">
    <div class="lock">%s</div>
    <h1>%s</h1>
`, e(s.Title), consentCSS, freehireMark, lockIcon, e(s.Title))
	if s.SignedIn != "" {
		fmt.Fprintf(&b, "    <p class=\"who\">Signed in as %s</p>\n", e(s.SignedIn))
	}
	b.WriteString("    <div class=\"section\">\n      <p class=\"label\">This allows</p>\n      <ul>\n")
	for _, a := range s.Allows {
		fmt.Fprintf(&b, "        <li>%s<span>%s</span></li>\n", checkIcon, e(a))
	}
	b.WriteString("      </ul>\n")
	fmt.Fprintf(&b, "      <ul class=\"caution\"><li>%s<span>%s</span></li></ul>\n    </div>\n", cautionIcon, s.Caution)
	fmt.Fprintf(&b, "    <form method=\"post\" action=\"%s\">\n", e(s.Action))
	for _, h := range s.Hidden {
		fmt.Fprintf(&b, "      <input type=\"hidden\" name=\"%s\" value=\"%s\">\n", e(h[0]), e(h[1]))
	}
	fmt.Fprintf(&b, `      <button type="submit" name="decision" value="%s">Cancel</button>
      <button type="submit" name="decision" value="allow" class="primary">Connect</button>
    </form>
`, e(s.Decline))
	origin := e(strings.TrimSuffix(s.Origin, "/"))
	fmt.Fprintf(&b, `    <p class="foot">You can disconnect at any time. See the freehire <a href="%s/privacy">privacy policy</a> and <a href="%s/terms">terms</a>.</p>
  </div>
</main>
</body>
</html>`, origin, origin)
	return b.String()
}
