---
title: Plans and Storage Choices
section: sessions
order: 3
summary: Local, cloud, linked-local, linked-cloud, and dormant session types with decision tree.
---

## What You Need to Decide

After creating an account, you choose where your data lives. This decision affects sync, backup, and what features are available.

## Options

**Cloud** -- Your data is encrypted on your device and synced to Spacewave Cloud. This gives you cloud backup, always-on sync across all your devices, shared spaces with collaborators, and 100 GB of cloud storage with 1M writes and 10M cloud reads per month. Cloud costs a monthly subscription.

**Local** -- Your data stays on your device. This is free forever, open-source, and requires no cloud account. You get the full local-first app, the plugin SDK, developer tools, and peer-to-peer sync between devices on the same network.

Both plans include end-to-end encryption, peer-to-peer sync, the full plugin SDK, and developer tools. The difference is whether a cloud relay backs up and syncs your data when your devices are not online at the same time.

## Linked Sessions

Cloud subscribers can also create a linked local session. This is a local session on the same device that shares the cloud account's identity. It stores data locally while the cloud session handles sync. The plan page detects whether a linked local session already exists and routes you accordingly.

If a linked cloud session exists on another device, the plan page redirects you to that session's plan page so you do not create duplicate subscriptions.

## Which Should I Choose?

- **Choose Cloud** if you want your data available on any device, need to share spaces with other people, or want automatic off-device backup.
- **Choose Local** if you want full control over where data is stored, do not need cross-device sync beyond local network, or want to avoid any subscription cost.
- You can switch later. Local sessions can upgrade to Cloud at any time.
  Cloud subscribers can cancel at the end of the billing period and keep
  full access until then. Immediate account deletion is a separate verified
  flow that finalizes billing, calculates any refund or outstanding
  balance (minus the fixed $0.30 processing fee), and gives you a
  24-hour undo window.

## Overages

If you exceed the Cloud baseline, overage pricing is low: storage is billed per GB-month, writes per million, and reads per million. You can monitor usage from your session settings. Limits reset monthly.

## Troubleshooting

- **Checkout window did not open.** Your browser may have blocked the popup. Look for a blocked-popup indicator in the address bar and allow it, or click the retry button that appears.
- **Checkout not completing.** If you completed payment but the app still shows "Waiting," the status check will catch up within a few seconds. If it does not, click retry.

## Next Steps

- [Backup and Lock Setup](/docs/users/sessions/backup-and-lock-setup) to secure your session after choosing a plan.
- [Session Settings](/docs/users/sessions/session-settings) to manage billing and upgrade later.
