---
title: Teams and Space Ownership
section: ownership
order: 1
summary: Understand account ownership, organization ownership, sharing roles, and recovery limits.
---

Space ownership is separate from account login. A Space resource can be owned by
a personal account or by an organization. Sharing adds participants to a Space;
ownership transfer changes which principal owns the resource.

## Accounts and organizations

Personal ownership targets an account ID. Organization ownership targets an
organization ID. Organization members have roles such as `org:owner` and
`org:member`. Organization owners can manage members, invites, billing
attachment, and organization-owned Spaces.

## Space sharing

Spaces use participant roles and invites. A session can create invites, list
participants, remove participants, revoke invites, and accept targeted
invitations. Username-targeted invitations are addressed to an account identity
and still require acceptance or owner approval as configured.

## Transfer ownership

The transfer dialog can move a Space between a personal account and an
organization by calling the cloud resource transfer API with owner type `account`
or `organization`. Transfer changes the owning principal; it does not by itself
copy local browser data between sessions.

## Recovery boundary

Organization owners can repair or reinitialize the organization root shared
object when health checks report recoverable or closed states. Reinitialize is a
destructive in-place rewrite of that shared object. Non-owners cannot perform
that recovery action. General non-owner recovery is not exposed by the current
UI.
