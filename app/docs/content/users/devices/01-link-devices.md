---
title: Link Devices
section: devices
order: 1
summary: Connect another browser or the desktop app to the same account and Spaces.
---

Linking a device signs another browser or desktop app in to an account you
already have. Afterwards both see the same Spaces, the containers that hold your
work. Each linked client gets its own session, which is that client's sign-in
to the account. You can remove a session later.

Before any access changes, both clients show a row of emoji. You confirm that
the emoji match.

## Link with a code

1. On the client that has the account, choose **Link My Device** from setup or
   settings.
2. Choose **Generate code for another device**.
3. Open Spacewave in the other browser or in the desktop app. From Home, choose
   **Link My Device** and enter the 8-character code. You can also open the
   pairing link instead.
4. Compare the emoji on both clients and confirm that they match.

If the emoji differ, reject the connection and start again. Codes expire. The
client that made the code can make a new one.

## When both clients already have accounts

If the client entering the code already has an account, you can choose one of
four outcomes:

- sign in to the first account on the second client;
- sign in to the second account on the first client;
- merge the first account into the second;
- merge the second account into the first.

Both clients show which accounts and machines are involved before you confirm.

Signing in keeps the two accounts separate. Merging moves the Spaces and
sessions of one account into the other. The account you merge into keeps its
identity, settings, and storage, whether local or Cloud. Space IDs and sharing
with other people stay the same.

A permission problem, or an account that has no room for more sessions, stops
the merge. Spacewave tells you what needs attention. The data of the account
being merged stays available, so you can fix the problem and try again. Other
sessions of the account pick up the change when they reconnect. After a merge
between local accounts, reconnect those sessions before you merge the
destination account again.

## Link with a QR code

1. On the first client, choose **Show QR code**.
2. On the other client, scan the code or open its pairing link.
3. Send the other client's answer back to the first client.
4. Compare and confirm the emoji on both clients.

This method connects the two clients directly. It works for local and Cloud
accounts. A Cloud account still contacts Spacewave Cloud to approve the new
session.

## After connecting

**Account connected** means the new session can use the account. For a local
account, Spacewave then copies each Space's files to the new client in the
background. Keep a client that has the data online until copying finishes. The
copy status shows progress and any interruption. Copying resumes when the
source is reachable again. After the copy finishes, the new client can read
that data on its own.

From then on, changes and new Spaces sync through the account. Removing a
session stops its future access and syncing. It does not erase data that the
client already has.

You can link two browsers without installing anything. The desktop download
page has builds for macOS, Windows, and Linux, with instructions for each.

Linking a device to your account is different from adding a managed device to a
Space. A managed device is a machine or agent that Spacewave operates for you.
Use the **Add Device** flow in a Space for that.
