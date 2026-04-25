---
title: Session Lifecycle
section: sessions
order: 2
summary: Landing vs login vs sessions routing, deleted-account overlay, and session removal.
---

## What This Is

A session in Spacewave is a local cryptographic identity tied to an account. It holds your session key, knows which provider manages your data, and determines what you see when you open the app. Understanding the session lifecycle helps you know what to expect as you sign in, use, and eventually close or remove sessions.

## How It Works

When you open Spacewave, the app decides where to send you based on your existing sessions:

- **No sessions on this device.** You see the landing page with options to sign in or create an account.
- **One session, auto-unlock.** The app mounts the session automatically and takes you to your dashboard or most recent space.
- **One session, PIN-locked.** You see the PIN unlock overlay. Enter your PIN to proceed.
- **Multiple sessions.** You see the session selector to choose which session to open.

After a session mounts, the app checks whether setup is complete. Cloud sessions may redirect to the plan selection page or setup wizard if onboarding is not finished. Local sessions go straight to the dashboard unless the setup banner is showing.

## Deleted Account Overlay

If a cloud account has been deleted on the server side, the session can no longer connect. Instead of the dashboard, you see a deleted-account overlay explaining that the account no longer exists. The only action available is to remove the session from your local session list.

## Re-Authentication Overlay

If your cloud session becomes unauthenticated (for example, after a credential change on another device), a re-authentication overlay appears. You can re-authenticate with your current password or backup key, or log out and remove the session.

## Session Removal

Sessions can be removed in several ways:

- **Log out** (cloud sessions): revokes the session on the server and removes it locally.
- **Delete account**: permanently deletes the account and all associated data, then removes the session.
- **Remove from selector**: for sessions with deleted accounts, the overlay lets you remove them from the list.

After removal, you are returned to the session selector or landing page.

## Why It Matters

The session lifecycle ensures you always land in the right place. If your session is locked, you unlock it. If your account was deleted, you are told immediately rather than seeing confusing errors. If setup is incomplete, you are guided through the remaining steps.

## Next Steps

- [Switch or Remove Sessions](/docs/users/sessions/switch-or-remove-sessions) for the session selector interface.
- [Session Security](/docs/users/sessions/session-security) for lock modes and PIN behavior.
