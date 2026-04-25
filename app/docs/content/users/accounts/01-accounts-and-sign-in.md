---
title: Accounts and Sign-In
section: accounts
order: 1
summary: Password, passkey, and backup-key login paths.
---

## Overview

Spacewave offers several ways to sign in or create an account. You can use a password, a passkey (biometric or hardware key), or a backup key. Each method opens the same kind of account; the choice is about which credential you want to use day-to-day.

## Sign-In Methods

**Password.** Enter your username and password on the login page. New usernames can create an account. Passwords must be at least 8 characters. A strength indicator shows whether your choice is weak, fair, or strong.

**Passkey.** Click "Continue with passkey" on the login page to start the passkey flow. You will be asked for your username first. If your account already has a passkey registered, your browser will prompt you to authenticate with it (fingerprint, face, or hardware key). If no passkey is registered, you will be directed back to the login page to sign in with another method and add a passkey from your account settings. If no account exists for that username, you can register a passkey and create your account in one step.

**Backup key.** If you have a `.pem` backup key file, you can use it to log in by providing it on the recovery page. This is useful if you have lost your password or other credentials.

## After Sign-In

Once authenticated, Spacewave mounts a local session for your account. If your account key is protected by a PIN, you will be prompted to enter it before the session opens. New accounts are directed to the [plan selection page](/docs/users/sessions/plans-and-storage-choices) to choose between Cloud and Local storage.

## Troubleshooting

- **"Username is already taken"** during account creation means someone else registered that name. Choose a different username.
- **Passkey prompt does not appear.** Your browser or device may not support WebAuthn. Use password sign-in instead, then add a passkey later from [session settings](/docs/users/sessions/session-settings).

## Next Steps

- [Recovery and Account Management](/docs/users/accounts/recovery-and-account-management) for password resets and account links.
- [Email Verification](/docs/users/accounts/email-verification) if you need to verify your email for cloud features.
