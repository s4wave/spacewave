---
title: Switch or Remove Sessions
section: sessions
order: 1
summary: Multi-session picker, card meanings, and cloud badges.
---

## Overview

Spacewave supports multiple sessions on the same device. Each session represents a separate account or provider. The session selector lets you switch between them, add new accounts, or return to one you used before.

## The Session Selector

Navigate to the session selector from the landing page or by clicking "Change Account" in the account menu. The selector shows a card for each session on this device.

Each session card displays:

- **Display name** or a fallback like "Session 1" if no name is set.
- **Provider label and username**, showing which provider manages the session and the associated account identifier.
- **Cloud badge.** Sessions backed by Spacewave Cloud show a "CLOUD" badge. Local sessions have no badge.

Click a session card to open that session. If the session is PIN-locked, you will be prompted to enter your PIN before it opens.

## Adding an Account

Click **Add account** below the session list to go to the login page. After signing in or creating a new account, the new session appears in the selector.

## Removing a Session

To remove a session from this device, open the session and go to the account menu. In the Danger Zone section of session settings, you can log out (cloud sessions) or delete the local data. Logging out revokes the cloud session and removes it from the selector. See [Session Settings](/docs/users/sessions/session-settings) for details.

## Empty State

If no sessions exist on this device, the selector redirects you to the landing page where you can sign in or create an account.

## Troubleshooting

- **Session stuck loading.** If a session fails to mount, an error message appears with a Retry button. Try again, or remove the session and re-add your account.
- **Missing session after browser clear.** Browser data clearing removes local sessions. Use your credentials to sign in again, or restore from a backup key if you had one.

## Next Steps

- [Session Lifecycle](/docs/users/sessions/session-lifecycle) to understand how sessions are routed and managed.
- [Backup and Lock Setup](/docs/users/sessions/backup-and-lock-setup) to protect your sessions.
