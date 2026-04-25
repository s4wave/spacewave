---
title: Recovery and Account Management
section: accounts
order: 2
summary: Password recovery, account-management links, and expiry behavior.
---

## Overview

If you lose access to your account, Spacewave provides a recovery flow to reset your password. You can also manage account links, linked devices, and authentication methods from your session settings.

## Password Recovery

When you forget your password, request a recovery link from the login page. Spacewave sends a time-limited token to your verified email address. The link takes the form `spacewave.app/#/recover?token=xyz`.

1. Open the recovery link from your email.
2. Spacewave verifies the token. If it has expired or is invalid, you will see an error with a prompt to request a new link.
3. Once verified, you see your username (read-only) and two password fields.
4. Enter a new password (minimum 8 characters) and confirm it. A strength indicator shows weak, fair, or strong.
5. Click "Reset password." Spacewave derives new encryption keys, which may take a moment.
6. On success, you are directed to sign in with your new password.

Recovery tokens are single-use and expire after a set period. If your token has expired, return to the login page and request a new one.

## Backup Key Recovery

If you downloaded a backup `.pem` file during setup, you can use it as an alternative recovery method. The backup key works independently of your password and can re-authenticate your account even if you have lost all other credentials. See [Backup and Lock Setup](/docs/users/sessions/backup-and-lock-setup) for details on creating a backup key.

## Account Links and SSO

Your account can be linked to external identity providers (Google, GitHub) through SSO. When an SSO identity is linked, signing in through that provider logs you into your existing account automatically. If your account key is additionally protected with a PIN, you will be prompted to enter it after SSO authentication.

You can manage linked auth methods from the [session settings](/docs/users/sessions/session-settings) dashboard, including adding new backup keys or passkeys.

## Account Deletion

You can delete your account from the Danger Zone section in session
settings. For cloud accounts, this starts a verified deletion process: you
confirm from an email link or 6-digit code, billing is finalized
immediately, the account becomes read-only, and you get a 24-hour undo
window before cloud deletion completes. For local accounts, this removes
all local data immediately and cannot be undone.

## Troubleshooting

- **"Token verification failed."** The recovery link may have expired. Request a new one from the login page.
- **"Missing recovery token."** The link is incomplete or was truncated. Check the original email and use the full link.
- **"Incorrect password or unrecognized key"** when using a backup key means the `.pem` file does not match any registered auth method on the account.

## Next Steps

- [Accounts and Sign-In](/docs/users/accounts/accounts-and-sign-in) for the full list of sign-in methods.
- [Session Security](/docs/users/sessions/session-security) to understand PIN locks and security levels.
