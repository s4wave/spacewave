---
title: Chat
section: features
order: 5
summary: Chat channels, message list, and channel switching.
draft: true
---

## Overview

A Chat Channel is a live messaging interface inside a space. Messages stream in real time, and the full conversation history is stored locally and synced across your linked devices. Chat channels are encrypted end-to-end, just like everything else in Spacewave.

## Prerequisites

- Create a space with the Chat quickstart, or create a Chat Channel object from the [object browser](/docs/users/spaces/object-browser).

## Steps

### Sending messages

Type your message in the input area at the bottom and press Enter to send. Messages appear in the message list immediately.

### Reading the conversation

The message list shows the full conversation history, with the most recent messages at the bottom. When new messages arrive from linked devices, they appear automatically without refreshing.

### Multiple channels

A single space can contain multiple Chat Channel objects. Create additional channels from the object browser. Each channel has its own independent message history. Switch between channels by opening different objects in the space.

### Real-time updates

The chat viewer uses a streaming connection to the backend. New messages from any linked device appear as they arrive. There is no refresh button or polling; updates are pushed to you instantly.

## Verify

Send a message and confirm it appears in the list. On a second linked device with the same space, the message should appear after sync completes.

## Troubleshooting

- **"Loading messages..." stays visible.** The backend connection is still initializing. Wait a moment. If it persists, check that the space is fully loaded.
- **Messages not appearing on other devices.** Both devices must be online and syncing. Messages sync through the same encrypted channels as all other space data.

## Next Steps

- [Canvas](/docs/users/features/canvas) for visual spatial layouts
- [Understanding spaces](/docs/users/spaces/understanding-spaces)
