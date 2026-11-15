{{flutter_js}}
{{flutter_build_config}}

// CanvasKit is served from our own origin (copied into build/web/canvaskit by
// `flutter build web`), so the CSP does not need the gstatic CDN.
// No service worker is registered: the panel must never be served from a
// stale cache, and nothing is cached offline.
_flutter.loader.load({
  config: {
    canvasKitBaseUrl: "canvaskit/",
  },
});
