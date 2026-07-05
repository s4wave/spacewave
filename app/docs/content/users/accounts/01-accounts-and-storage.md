---
title: Accounts and Storage
section: accounts
order: 1
summary: Choose local storage, Spacewave Cloud, or an existing desktop state root.
---

Spacewave stores work through a mounted session. A session belongs to a Provider
Account, which is the provider-owned account record behind local or cloud
resources.

## Local sessions

A local session is created by the `local` provider. It does not need a username
or password. It is the fastest way to try Spacewave, create a Drive, or work
offline on one device.

Local browser storage is not a durable backup. The browser may clear stored data
when disk space is low. Use the desktop app for a native state directory, or
move important work to Spacewave Cloud.

## Spacewave Cloud accounts

A Spacewave Cloud account uses the `spacewave` provider. It supports password
login, PEM backup-key login, browser handoff for desktop and CLI clients,
passkeys, SSO, recovery email, billing, and linked local sessions.

Cloud mode is the user-facing choice for cloud sync, backup, encrypted storage,
shared Spaces, and access from more than one device. Spacewave cannot read
end-to-end encrypted customer content.

## Existing desktop state roots

The desktop app can add an existing `.spacewave` state directory as a state root.
This is a desktop-only path. Browser directory-grant roots and single-file
`.s4wave` state files are reserved in the wire model but are not current
user-facing storage modes.

## Which choice to use

Use local storage for disposable experiments and private one-device work. Use
desktop local storage when you want the state directory under your control. Use
Cloud when you want backup, sync, billing-backed storage, and linked devices.
