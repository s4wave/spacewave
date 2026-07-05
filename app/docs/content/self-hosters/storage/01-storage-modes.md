---
title: Storage Modes
section: storage
order: 1
summary: Tie each storage mode to the provider, state root, and backup owner.
---

Storage mode is an ownership choice. The important question is which provider or
state root owns the blocks, sessions, and recovery path.

## Browser local storage

Browser local sessions use the local provider and browser storage. They are easy
to start and risky for irreplaceable data because the browser may clear storage
under pressure.

## Desktop state roots

Desktop can register a native directory state root. The current implemented kind
is a native directory opened by the desktop app. The root runtime can be watched,
autostarted, and selected by alias. Browser directory-grant roots and single-file
state roots are reserved in the protocol but not current stable user features.

## Daemon state paths

The CLI daemon resolves a state path from flags and environment, then listens on
`spacewave.sock` in that directory. The socket is runtime access. The state path
is data custody.

## Cloud storage

The Spacewave Cloud provider owns cloud-backed account resources, sessions, and
block storage. User-facing copy describes Cloud as encrypted sync, backup, shared
Spaces, and multi-device storage. Storage stats are provider-supported, so not
every provider must report byte counts.

## Backup scope

A backup key protects account or session authentication. It is not a full copy of
every block. Full data custody follows the provider and state root that store the
Space data.
