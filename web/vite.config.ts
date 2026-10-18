import { defineConfig } from 'vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { sentrySvelteKit } from '@sentry/sveltekit/vite';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
  // Strip Sentry's tracing/debug code at build time. We run errors-only
  // (tracesSampleRate:0, no replay), but Sentry's tracing/web-vitals paths sit
  // behind runtime `if (__SENTRY_TRACING__)` checks that ship regardless. Defining
  // these Sentry build flags to `false` turns those into dead `if (false)` branches
  // Rollup drops — ~22KB gzip off the client entry chunk, no behaviour change.
  define: {
    __SENTRY_DEBUG__: false,
    __SENTRY_TRACING__: false,
  },
  build: {
    // Keep flag-icons' ~500 flag SVGs as external files instead of inlining them
    // as data-URIs into the global CSS. They're each under Vite's 4KB inline
    // threshold, so the default would embed the whole sheet (~100KB gzip) into the
    // always-loaded stylesheet even though most pages show no flags. Externalizing
    // lets the browser fetch only the handful of flags a page actually renders.
    assetsInlineLimit: (filePath) => (filePath.includes('flag-icons/flags/') ? false : undefined),
  },
  // SvelteKit owns routing/SSR; it provides the $lib alias, so the manual alias
  // is gone. sentrySvelteKit() must precede sveltekit(); tailwindcss() too.
  //
  // Source-map upload is inert without SENTRY_AUTH_TOKEN — the build still
  // succeeds, only readable minified stack traces in Sentry are skipped. When
  // enabled in ops, org/project/token come from the environment (freehire-ops),
  // never from code.
  //
  // That is the ABSENT-token case, and for a long time it was the only one written
  // down while production was in the other one. A token Sentry REJECTS behaves the
  // same from out here — warning printed, build exits 0 — so **this build's exit
  // status is not evidence that anything was uploaded**, and no option set on the
  // plugin below changes that. Measured 2026-09-16: uploads stopped landing on
  // 2026-09-14 and every deploy in between was green.
  //
  // What asks instead is scripts/sentry-credential-check.mjs, run before this build
  // by release.sh (which lives in the private freehire-ops repository). Its header is
  // the canonical explanation — of the mechanism, of that date, and of the
  // release-`fileCount` measurement that looked like evidence and was not. Read it
  // there rather than trusting a second copy here to stay true; a second copy is
  // exactly what went stale last time.
  plugins: [
    sentrySvelteKit({
      // v11 flattened these onto the plugin options directly; the nested
      // `sourceMapsUploadOptions` wrapper from v10 no longer exists.
      org: process.env.SENTRY_ORG,
      project: process.env.SENTRY_PROJECT,
      authToken: process.env.SENTRY_AUTH_TOKEN,
    }),
    tailwindcss(),
    sveltekit(),
  ],
  server: {
    port: 5173,
    strictPort: false,
    // Bind to all interfaces (not just localhost) so the dev server is
    // reachable from other devices (e.g. a phone) on the same LAN.
    host: true,
    // Proxy the API so the browser (and server-side `load` via event.fetch) only
    // ever talks to this origin. That makes dev match the same-origin production
    // deployment, so the SameSite=Lax auth cookie is sent and no CORS is needed.
    // Target overridable via VITE_API_URL. /health is proxied too, mirroring the
    // prod nginx config (design D2) so dev and prod route identically.
    proxy: {
      '/api': {
        target: process.env.VITE_API_URL ?? 'http://localhost:8080',
        changeOrigin: true,
        // `/api/v1/tools/ws` (the browser-tool wire) upgrades to a WebSocket;
        // without this the dev proxy would answer the handshake with the SPA.
        ws: true,
      },
      '/health': {
        target: process.env.VITE_API_URL ?? 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
});
