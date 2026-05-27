---
title: Teams and Space Ownership
section: ownership
order: 1
summary: Keep account, organization, and Space ownership decisions separate from object routing.
---

Space ownership answers who controls the workspace. Object routing answers what opens first when the Space is selected.

Do not use `SpaceSettings.index_path` or plugin configuration as an ownership model. Those settings shape the app surface. They do not replace account, organization, or provider-level decisions.

## Operational Questions

Before hosting for a team, answer:

- Which Provider Account owns the session?
- Which Spaces are personal and which are shared?
- Who can recover the account if one device is lost?
- Where are backup keys stored?
- Which daemon or storage path is authoritative?

## Object Access

Objects inside a Space inherit the Space's operational boundary. A Drive object, Git object, or Canvas object can have different viewers, but it is still part of the same Space.

For developer-facing object behavior, read [ObjectTypes, Viewers, and Wizards](/docs/developers/objects/objecttypes-viewers-and-wizards).
