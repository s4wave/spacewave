---
title: Session Settings
section: sessions
order: 6
summary: Dashboard walkthrough, adding and removing auth methods, passkey enrollment, logout, and session revocation.
---

## Overview

The session settings dashboard is your central place to view session identity, manage authentication methods, change lock modes, and handle account-level actions. Open it by clicking your account identifier in the bottom bar.

## Dashboard Layout

The dashboard shows the following sections from top to bottom:

**Header bar.** Displays a shortened peer ID, and buttons for Change Account (goes to the session selector), Lock (locks the session or returns to the selector), Logout (cloud sessions only), and Close.

**Status and Provider.** Two stat cards showing whether the session is active or locked, and which provider manages it (Local or Spacewave Cloud).

**Identifiers.** Your Session ID, Peer ID, and Account ID with copy buttons.

**Crypto Identity.** Key type and public key PEM export, plus a count of spaces and total storage used.

**Session Lock.** View and change the lock mode between auto-unlock and PIN lock. See [Session Security](/docs/users/sessions/session-security) for details.

**Linked Devices.** Lists devices linked to this account.

## Auth Methods (Cloud Sessions)

Cloud sessions show an Auth Methods section listing all registered authentication methods. Each entry shows the method type (Password, Backup Key, Passkey) and a truncated peer ID.

**Adding an auth method.** Click "Add auth method" and choose from:

- **Backup key (.pem)**: generates a key file for offline recovery. You confirm your identity, and the `.pem` file downloads automatically.
- **Passkey**: starts a WebAuthn registration ceremony. Your browser prompts for biometrics or a hardware key. After registration, you see whether the passkey supports zero-knowledge encryption (PRF) or uses server-assisted protection.

**Removing an auth method.** Click "Remove" next to any method. You must confirm your identity before removal. You cannot remove your last auth method.

**Changing your password.** Click "Change" next to the password entry to open the password change dialog.

When the security level is Enhanced or Maximum, adding or removing methods uses a multi-step wizard that collects credentials from multiple auth methods.

## Security Level (Cloud Sessions)

Shown when you have more than one auth method. Choose between Standard, Enhanced, and Maximum. Changing the level requires re-authentication. See [Session Security](/docs/users/sessions/session-security).

## Billing (Cloud Sessions)

Cloud sessions display a billing summary with current plan status. Links to the billing page for managing your subscription and viewing usage.

## Upgrade to Cloud (Local Sessions)

Local sessions show an "Upgrade to Cloud" button that takes you to the [plan selection page](/docs/users/sessions/plans-and-storage-choices).

## Transfer Sessions

When you have more than one session on the device, a "Transfer Sessions" button appears. This lets you merge spaces from one session into another.

## Danger Zone

**Log Out** (cloud sessions): revokes your cloud session and removes it from this device. Your account and data on the server are not affected.

**Delete Account / Delete Local Data**: for cloud accounts this starts a
verified deletion process. You confirm from an email link or 6-digit code,
billing is finalized immediately, the account becomes read-only, and you get
a 24-hour undo window before cloud deletion completes. For local accounts
this immediately removes all local data and cannot be undone.

## Troubleshooting

- **Cannot remove an auth method.** You must keep at least one auth method on the account.
- **"Incorrect password or unrecognized key"** when adding a backup key means the credential you provided does not match. Double-check your password or use a different auth method.
- **Passkey registration cancelled.** If you dismiss the browser prompt, registration returns to the idle state. Click "Start registration" to try again.

## Next Steps

- [Accounts and Sign-In](/docs/users/accounts/accounts-and-sign-in) for an overview of all sign-in methods.
- [Recovery and Account Management](/docs/users/accounts/recovery-and-account-management) for password recovery.
