// Kill switch for the service worker this site used to install (vite-plugin-pwa,
// removed 2026-10-18). A browser that registered it keeps checking this URL for
// updates, so serving this file is the only way to reach it: it installs, clears
// every cache the old worker filled, and unregisters itself. Nothing registers it
// any more, so a browser that never had the old worker never runs this.
//
// It does not reload open tabs. The next ordinary navigation already goes to the
// network; forcing a reload would drop whatever someone was halfway through typing.
//
// Safe to delete once requests for /sw.js have died down in the access log.
/* global self, caches */
self.addEventListener('install', () => self.skipWaiting());

self.addEventListener('activate', (event) => {
  event.waitUntil(
    (async () => {
      const keys = await caches.keys();
      await Promise.all(keys.map((key) => caches.delete(key)));
      await self.registration.unregister();
    })(),
  );
});
