---
title: Install and Update
section: settings
order: 2
summary: Browser, desktop, and native install paths plus plugin hot-swap behavior.
---

## Overview

Spacewave runs as a web app in modern browsers and as a native desktop app for macOS, Windows, and Linux. The web app requires no installation. The desktop app provides local filesystem storage and tighter OS integration.

## Browser (Web App)

Open Spacewave in any modern browser. No install needed. A service worker makes it work offline after the first visit. Updates happen automatically in the background and activate on your next page load.

You can install the web app as a PWA from your browser's menu. This gives it its own window but still uses browser storage.

## Desktop App

The desktop app stores data on your filesystem and runs outside the browser. Use the download page to get the current macOS, Windows, or Linux build for your device.

Desktop builds are distributed as release artifacts. Plugin updates can load independently of the main application. Binary updates require installing or launching the newer build.

## Plugin Updates

Plugins are loaded and updated independently of the main application. When a plugin update is available, it activates without interrupting your work.

## Verify

After an update, check the version displayed in the app. If you are using the browser, a hard refresh ensures you are running the latest service worker. For the desktop app, the version is shown in settings.

## Troubleshooting

If the web app appears stuck on an old version, try a hard refresh (Ctrl+Shift+R or Cmd+Shift+R) to bypass the service worker cache. If that does not work, clear site data for the Spacewave domain and reload.

If a desktop download fails, check your network connection and confirm outbound HTTPS is allowed.

## Next Steps

For details on how your data is stored, see [How Spacewave Stores Data](/docs/users/settings/how-spacewave-stores-data). For information on settings and what persists across reloads, see [Settings and Persistence](/docs/users/settings/settings-and-persistence).
