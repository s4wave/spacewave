---
title: Teams and Ownership
section: operations
order: 1
summary: Org ownership boundaries and deletion constraints.
---

## What This Is

Organizations in Spacewave let multiple users share ownership of spaces under a single billing account. From an operations perspective, an organization defines a trust and access boundary: who can see which spaces, who pays for cloud storage, and who can invite or remove members.

As a self-hoster, you may use organizations to manage a small team, a household, or a group of collaborators. Understanding the ownership model helps you plan access control and deletion boundaries before you have data you cannot easily move.

## How It Works

Each organization has an owner and one or more members. The owner manages settings, billing, and membership. Members can access the organization's shared spaces. Spaces can be assigned to an organization, which means the organization's billing account covers their cloud storage costs (if cloud is used).

The organization dashboard shows members, roles, and attached spaces. The owner can invite, remove members, and adjust settings.

Deletion is constrained: you cannot delete an organization that still has spaces. All spaces must be removed or transferred first. When deleting, the owner must type the organization name to confirm. Non-owner members can leave at any time without affecting shared data.

## Why It Matters

Organizations define the blast radius of administrative actions. If you are running a self-hosted setup for a team, the organization boundary determines who can delete spaces, who pays for storage, and what happens when someone leaves. Getting this right early avoids messy data ownership disputes later.

For single-user self-hosting, organizations are optional. Your personal session already owns your spaces directly. Organizations become important when you add collaborators or want to separate billing for different projects.

## Next Steps

For information on transferring space ownership between sessions and organizations, see [Merge and Transfer Sessions](/docs/users/devices/merge-and-transfer-sessions). To understand the upgrade and update model for self-hosted deployments, see [Upgrades and Operations](/docs/self-hosters/operations/upgrades-and-operations).
