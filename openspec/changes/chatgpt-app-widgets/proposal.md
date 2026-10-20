## Why

ChatGPT is freehire's main traffic channel, and 89% of the visitors it sends land on one
specific posting. Today the ChatGPT app's `search_jobs` and `get_job` answer in data the
model retells as prose: a numbered list with no logo, and a link the person has to find in
the text. The proposal for `chatgpt-app-mcp` named widgets out of scope and deferred them to
this change once real use showed what people do. What they do is ask for jobs and open one.

The MCP Apps extension (`text/html;profile=mcp-app` resources, `_meta.ui.resourceUri` on a
tool) is now the standard for this, and both ChatGPT and Claude render it. One widget serves
both clients; ChatGPT's own `openai/outputTemplate` key is only a compatibility alias.

## What Changes

- The anonymous MCP server (`/api/v1/mcp`) serves ONE UI resource: a self-contained HTML
  page embedded in the binary. It renders a `search_jobs` result as a list of job cards and
  a `get_job` result as one posting in full.
- `search_jobs` and `get_job` point at that resource through `_meta.ui.resourceUri`. Their
  inputs, outputs and annotations do not change, so a client without MCP Apps support sees
  exactly what it sees today.
- The resource URI carries a hash of the page's content, so a changed page is a new URI and
  no client renders a cached old one.
- The page loads images from `logo.freehire.me` only and makes no network request of its
  own; it opens links through the host's `ui/open-link`.

## Capabilities

### New Capabilities

- `chatgpt-app-widgets`: the UI resource, which tools render through it, what it shows, and
  the rules that keep it safe and in step with the wire shapes.

### Modified Capabilities

None. `chatgpt-app`'s tool contracts are unchanged; a widget is an addition beside them.

## Impact

- `internal/api/mcpapp`: a `widget/` directory (one HTML file), resource registration in
  `server.go`, `_meta` on two tools, tests.
- No change to `web/`, the release script, CI or the database.
- The ChatGPT app's listing must be resubmitted for review before the widget reaches people
  outside developer mode — the owner's action in the OpenAI portal.

## Out of scope

Widgets for `similar_jobs`, the company tools and the account server; saving a posting from
the widget; fullscreen or picture-in-picture modes. Each is the same mechanism and can follow.
