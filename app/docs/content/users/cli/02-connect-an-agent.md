---
title: Connect an Agent
section: cli
order: 2
summary: Let a coding agent such as Claude Code or Codex use your account through the spacewave command.
---

A coding agent can read and change your Spaces through the `spacewave`
command. The Connect an agent button in the bottom bar gives the agent what it
needs in one paste, and you approve it once.

## Connect

1. Open the Space, Drive or file you want the agent to work on.
2. Click the robot button in the bottom right corner. Spacewave copies a
   prompt to your clipboard and opens the Connect an agent panel.
3. Paste the prompt into your agent. It reads
   [spacewave.app/llms.txt](https://spacewave.app/llms.txt), installs the
   `spacewave` command if needed, and connects.
4. The agent shows you six emoji. When the panel shows the same six with the
   agent's name, click **Yes, they match**.

The agent now works in the Space you had open. In the desktop app the agent
connects through the app's command-line socket, so there is nothing to
approve.

The prompt carries a pairing code that lasts ten minutes and works once. If it
expires, click **Copy new prompt** in the panel.

## What the agent can reach

A connected agent has a session on your account, like another device you have
linked. It can open every Space in the account, not only the one you had open.
For a local account, the agent syncs from this device, so keep Spacewave open
while the agent works.

## Remove an agent

Open the panel or your account settings and expand **Sessions**. Each agent
appears under the name it gave, such as "Claude Code on build-box". Click
remove next to it to end its access.
