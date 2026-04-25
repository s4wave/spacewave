---
title: Session Security
section: sessions
order: 5
summary: Auth method types, lock modes, PIN recovery, and security levels.
---

## What This Is

Session security in Spacewave has two layers: how your session key is protected on this device (lock mode), and how your cloud account verifies your identity for sensitive changes (security level). Together they determine how much protection your account has against unauthorized access.

## Lock Modes

Your session key can be stored in one of two modes:

**Auto-unlock.** The session key is stored on disk in the clear. The app opens without prompting for a PIN. This is convenient but means anyone with access to your device can open the session.

**PIN lock.** The session key is encrypted with a PIN you choose (at least 4 characters). Every time you open the app or return to the session, you must enter the PIN to decrypt the key. This protects your session if someone else uses your device.

You can change the lock mode at any time from the Session Lock section in [session settings](/docs/users/sessions/session-settings). Changing lock mode only requires your current session -- no account re-authentication is needed.

## PIN Recovery

If you forget your PIN, click "Forgot PIN?" on the unlock screen. You can then re-authenticate using your account password or a backup key file. This resets the session by generating a new session key, so you will be directed to the session selector to re-enter. Your account data and spaces are not affected.

## Security Levels

Cloud accounts with multiple auth methods can set a security level that controls how many methods are required for sensitive account changes (like adding or removing auth methods or changing the security level itself).

- **Standard.** Any single auth method can authorize changes. This is the default.
- **Enhanced.** A specific number of methods (more than one but fewer than all) must confirm each change.
- **Maximum.** All registered auth methods are required.

The security level selector only appears when you have more than one auth method registered. If you have only one, the settings page suggests adding a backup key to unlock higher security levels.

Changing the security level requires re-authentication with your current credentials.

## Why It Matters

Lock modes protect you at the device level. If you share a computer or use a browser that others can access, PIN lock prevents casual access to your session. Security levels protect you at the account level. With multiple auth methods and an elevated security level, an attacker who compromises one credential cannot modify your account without the others.

For most users, auto-unlock with a single auth method (Standard security) is sufficient. If you store sensitive data or share devices, consider PIN lock and adding a backup key to enable Enhanced or Maximum security.

## Next Steps

- [Backup and Lock Setup](/docs/users/sessions/backup-and-lock-setup) to configure lock mode during initial setup.
- [Session Settings](/docs/users/sessions/session-settings) to manage auth methods and security levels.
