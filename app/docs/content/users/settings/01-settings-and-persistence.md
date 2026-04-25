---
title: Settings and Persistence
section: settings
order: 1
summary: Session-scoped vs app-scoped preferences, reload persistence, and local-first behavior.
---

## What This Is

Spacewave has two layers of settings: session-scoped preferences that are tied to your identity and sync across devices, and app-scoped preferences that stay on the current browser tab or window. Understanding which is which helps you know what to expect when you reload the page, open a new tab, or switch devices.

## How It Works

**Session-scoped settings** are stored in your session's data and sync with your linked devices. These include your spaces, account configuration, and session identity. When you link a new device, these settings come along automatically.

**App-scoped settings** are UI state in the browser: which sidebar panel is open, current view mode, collapsed sections. These persist across page reloads but do not sync to other devices.

Some app-scoped settings use localStorage and share between tabs. Others are tab-local and reset when the tab closes. On reload, session-scoped settings always restore. localStorage-backed settings survive. Tab-local settings reset to defaults.

## Why It Matters

If you set up a view preference and it disappears after closing the tab, it is a tab-local setting. This is expected behavior, not a bug. If you need a preference to persist, check whether it is stored in the persistent layer (localStorage-backed) or the tab-local layer.

Settings that affect your data (space configuration, account details, encryption) are always session-scoped and always persist. Settings that affect the visual presentation of the UI are app-scoped and may or may not persist depending on how they are stored.

## Next Steps

To learn how data is stored on your device, see [How Spacewave Stores Data](/docs/users/settings/how-spacewave-stores-data). For install and update information, see [Install and Update](/docs/users/settings/install-and-update).
