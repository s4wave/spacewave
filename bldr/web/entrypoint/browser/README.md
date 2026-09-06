# Web Browser

The browser target implements deploying an app to a web page.

The user navigates to the app, which loads the Go runtime in a WebWorker. The
root WebView cannot be closed (as per browser policy). A service worker is used
to intercept requests and forward them to the Go runtime for processing. The
root page can create and close more pages through the WebView API.

A controlled page attaches to the running runtime before checking for service-worker
updates. Service-worker activation only establishes control; offline downloads run
through separate events. A forced browser reload that bypasses an installed worker
gets one recovery reload. Caching a new release does not reload active pages.
An already cached release descriptor returns immediately while the service worker
checks for a newer generation in the background.
Missing boot-version metadata initializes in place, without deleting storage or
reloading a fresh browser. Explicit older versions retain their migration path.

Browser distributions publish an offline inventory containing the entrypoint,
split runtime modules, web packages, and immutable kvfile. Startup reads the
manifest DAG on demand through HTTP ranges. The service worker downloads complete
assets in the background and serves ranges from its cache. Remote Release World
plugin manifests are copied into local storage after the startup group is ready.
Transient range failures are retried on the next read instead of being cached.
The local provider accepts the browser distribution's same-origin signaling URL
(`/`), so its configuration can start instead of leaving Quickstart waiting.

The SharedWorker OPFS workaround currently selects a dedicated runtime worker.
Web Locks elect its owning tab; other tabs attach through the service worker.
Closing the owner requires another tab to start a replacement runtime. The bundled
Drive startup benchmark measures cold startup, a second tab with a live owner,
and restarts with warm storage separately.
