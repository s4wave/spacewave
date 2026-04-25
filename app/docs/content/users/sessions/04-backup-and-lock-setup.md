---
title: Backup and Lock Setup
section: sessions
order: 4
summary: Setup checklist, banner behavior, dismiss cooldown, and finishing setup later.
---

## Overview

After choosing a plan, Spacewave offers two optional security steps: downloading a backup key and setting a PIN lock. These protect your account and session data. You can complete them during initial setup or return later.

## The Setup Wizard

The setup wizard appears after plan selection. It presents two collapsible cards:

**Download a backup key.** Enter a recovery password, then click "Download backup .pem" to save a key file to your computer. This gives you a second way to recover your account if you lose your primary password. Once downloaded, the card shows a green checkmark.

**Set a PIN lock.** Choose between two modes:

- **Auto-unlock**: your session key is stored on disk and the app opens without a PIN. This is the default.
- **PIN lock**: your session key is encrypted with a PIN. You must enter the PIN each time you open the app.

If you choose PIN lock, enter and confirm your PIN (at least 4 characters), then click "Set lock mode."

A **Continue to app** button is always available at the bottom, so you can skip either step and proceed immediately.

## The Setup Banner

For local sessions, a persistent banner appears at the top of the screen if any setup step is incomplete. The banner reads "Finish setting up your account" and links directly to whichever step is still needed.

You can dismiss the banner by clicking the X. It stays hidden for 7 days, then reappears if setup is still incomplete. Once all steps are done, the banner disappears permanently.

The banner does not appear on cloud sessions or while you are already on a setup page.

## Browser Storage Warning

Local sessions running in a browser show a warning card during setup explaining that browsers may clear stored data if disk space runs low. The card offers two alternatives: download the desktop app for persistent storage, or upgrade to Cloud for automatic backup.

## Finishing Setup Later

If you skip setup during onboarding, you can return to it at any time. The setup banner (for local sessions) links back to the wizard. You can also download a backup key or change your lock mode from the [session settings](/docs/users/sessions/session-settings) dashboard at any time.

## Next Steps

- [Session Security](/docs/users/sessions/session-security) for a deeper look at lock modes and security levels.
- [Session Settings](/docs/users/sessions/session-settings) to change these settings later.
