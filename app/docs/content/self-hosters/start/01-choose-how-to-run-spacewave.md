---
title: Choose How to Run Spacewave
section: start
order: 1
summary: Pick browser, desktop, local daemon, or cloud-backed operation by durability and access needs.
---

Start with the data you need to protect.

Use browser-only local storage for trials and disposable Spaces. It is the fastest path, but it has the weakest durability story.

Use the desktop runtime or a local daemon when you need a stable process, CLI access, local web listeners, and project-local state directories.

Use cloud-backed account and storage paths when account recovery, device moves, or managed persistence matter.

## Decision Matrix

| Need | Start With |
| --- | --- |
| Try Spacewave quickly | Browser or local session |
| Keep files on one trusted machine | Desktop runtime with backup setup |
| Script against Spaces | Local daemon plus CLI |
| Move between devices | Cloud-backed account and storage |
| Operate for a team | Explicit ownership and recovery policy |

## First Operational Check

Run:

```bash
spacewave status
```

If it cannot connect, start the desktop app or run `spacewave serve` with the state path you intend to operate. Avoid mixing multiple state directories until you can explain which one owns the Space you care about.
