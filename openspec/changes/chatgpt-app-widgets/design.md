## Context

The ChatGPT app server (`internal/api/mcpapp`) is stateless and read-only. Its tools return
`structuredContent` built from `wire.go` types. MCP Apps lets a tool name a `ui://` resource;
the host loads that HTML into a sandboxed iframe, hands it the tool's input and result over a
`postMessage` JSON-RPC bridge, and grows the frame to the size the page reports.

## Goals / Non-Goals

**Goals:** cards instead of prose for `search_jobs` and `get_job`, in ChatGPT and Claude, with
no change to the tools' contracts and no new deployment step.

**Non-Goals:** a component library inside the widget, any request from the widget to our
API, any write action, any widget on the account or OJCP servers.

## Decisions

### One hand-written HTML file, embedded with `go:embed`

Not a Svelte build from `web/`. The page renders two read-only views and holds no state; the
design system would add a build that the Go binary then depends on — a change to
`release.sh` (in `freehire-ops`) and CI, and a way for a stale bundle to ship with every
check green. Living beside `wire.go` is also what lets a Go test hold the page to the wire
shape (below). The cost is a few CSS variables copied by hand; the host's own theme
variables take precedence over them anyway.

If the widget ever grows forms, sign-in or its own requests, that is the point to move it to
a real frontend build.

### One resource for both tools

The page tells the two shapes apart by what the result carries (`jobs` array vs a single
posting's `description`). One handshake, one stylesheet, one card header shared by the list
and the detail view.

### A content hash in the URI

`ui://freehire/jobs-<first 12 hex of sha256(page)>.html`, computed once at package init.
OpenAI's guidance is to version a resource identifier whenever its HTML changes; doing it
from the bytes means nobody has to remember.

### Untrusted data, plain text only

`structuredContent` comes from postings we crawled. The page writes it with `textContent`
and never with `innerHTML`. A URL is used only if it parses as `https:`. `get_job`'s
description is already plain text (`htmltext.ToText` in the projector), so nothing has to be
sanitised in the page.

### Links through the host

Buttons call `ui/open-link`; when the host does not advertise link opening, they fall back to
an `<a target="_blank" rel="noopener">`. The freehire link keeps its `utm_source` tag; the
employer link is the projector's already-untagged `official_job_url`.

### CSP

`resourceDomains: ["https://logo.freehire.me"]`, nothing else. No `connectDomains`: the
page makes no request. A logo that fails to load falls back to a monogram, as on the site.

## Risks / Trade-offs

- **Hosts differ in what they implement.** The page feature-detects (`hostCapabilities`) and
  degrades to plain links; it never blocks rendering on a capability.
- **The model still narrates.** The text content beside the widget is unchanged, so the model
  may repeat the list in prose. Acceptable for this change; tightening the instructions is a
  follow-up once we see real transcripts.
- **Review.** OpenAI re-reviews the app after this lands. The tools' annotations and
  contracts are untouched, which is what review checks most.
