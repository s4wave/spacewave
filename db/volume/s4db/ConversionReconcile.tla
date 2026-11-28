---- MODULE ConversionReconcile ----
\* The reconciling conversion of a Volume file at path P: one idempotent step
\* that every open runs until P holds the new format. It never resumes a
\* partial copy and never deletes by file identity. It owns only the reserved
\* name T (P.converting), and it keeps the source inode under B
\* (P.pre-rewrite).
\*
\* The step, under the source's exclusive lock (the old writer lock and every
\* reader slot):
\*   clear T, build a fresh copy at T, link B to the source, sync the
\*   directory, rename T over P, sync the directory, and only then unlock.
\* Opening P in the new format syncs the directory before the first write,
\* so no caller serves an installed file whose name a crash could undo.
\*
\* Inodes: S is the source in an old format. N is the new copy, rebuilt in
\* place of any earlier one. Directory names are volatile until a sync makes
\* them durable. Commit contents are durable once acknowledged, since both
\* engines sync their own commits. A Crash is a power loss: every process
\* dies and the names revert to their durable copy. A Kill stops only the
\* converting process, whose locks the kernel then releases.
\*
\* One old-format process may run beside it (OldOpener). It opens P by name,
\* waits for the writer lock, and then commits to the inode it opened. With
\* OldRechecks it first sees that P names another file and refuses, as bbolt
\* does from v0.57.22. Without that check it writes the source after the
\* unlock, as the previous s4db engine and older bbolt do. That case is
\* testdata/ConversionReconcile-r1.cfg, and it keeps every commit reachable.
EXTENDS FiniteSets

CONSTANTS OldOpener, OldRechecks

Names == {"P", "B", "T"}
Inodes == {"S", "N"}
None == "none"

\* names is the directory as processes see it, and durable is what a crash
\* leaves. content holds each inode's acknowledged commits. pc is the
\* converter's step, and locked its hold on the source. built is the source's
\* content when the copy was made. unlinked holds each name the converter
\* removed. old, oldInode and oldAcked are the old-format process. newAcked
\* records a commit acknowledged through the new format.
VARIABLES names, durable, content, pc, locked, built, unlinked, old, oldInode,
  oldAcked, newAcked
vars == <<names, durable, content, pc, locked, built, unlinked, old, oldInode,
  oldAcked, newAcked>>
oldVars == <<old, oldInode, oldAcked>>

TypeOK ==
  /\ names \in [Names -> Inodes \cup {None}]
  /\ durable \in [Names -> Inodes \cup {None}]
  /\ content \in [Inodes -> SUBSET {"c0", "o", "n"}]
  /\ pc \in {"idle", "locked", "built", "linked", "linksynced", "renamed",
       "installed", "serving"}
  /\ locked \in BOOLEAN
  /\ unlinked \subseteq Names
  /\ old \in {"idle", "opened", "slot", "done", "refused"}

Init ==
  /\ names = [n \in Names |-> IF n = "P" THEN "S" ELSE None]
  /\ durable = names
  /\ content = [i \in Inodes |-> IF i = "S" THEN {"c0"} ELSE {}]
  /\ pc = "idle" /\ locked = FALSE /\ built = {} /\ unlinked = {}
  /\ old = "idle" /\ oldInode = None /\ oldAcked = FALSE /\ newAcked = FALSE

\* The steps the lock is held through before the converter releases it.
ReleaseAt == {"installed"}

\* Lock takes the source exclusively, refusing while an old process holds a
\* slot, and clears the reserved name.
Lock ==
  /\ pc = "idle" /\ names["P"] = "S" /\ old # "slot"
  /\ locked' = TRUE /\ pc' = "locked"
  /\ names' = [names EXCEPT !["T"] = None]
  /\ unlinked' = unlinked \cup (IF names["T"] # None THEN {"T"} ELSE {})
  /\ UNCHANGED <<durable, content, built, newAcked>> /\ UNCHANGED oldVars

\* Build writes a fresh, synced and verified copy at T.
Build ==
  /\ pc = "locked"
  /\ content' = [content EXCEPT !["N"] = content["S"]]
  /\ built' = content["S"]
  /\ names' = [names EXCEPT !["T"] = "N"]
  /\ pc' = "built"
  /\ UNCHANGED <<durable, locked, unlinked, newAcked>> /\ UNCHANGED oldVars

\* Link keeps the source under B. A B that already names the source is the
\* link of an attempt that crashed before its rename.
Link ==
  /\ pc = "built" /\ names["B"] \in {None, "S"}
  /\ names' = [names EXCEPT !["B"] = "S"]
  /\ pc' = "linked"
  /\ UNCHANGED <<durable, content, locked, built, unlinked, newAcked>>
  /\ UNCHANGED oldVars

SyncLink ==
  /\ pc = "linked" /\ durable' = names /\ pc' = "linksynced"
  /\ UNCHANGED <<names, content, locked, built, unlinked, newAcked>>
  /\ UNCHANGED oldVars

Rename ==
  /\ pc = "linksynced"
  /\ names' = [names EXCEPT !["P"] = "N", !["T"] = None]
  /\ pc' = "renamed"
  /\ UNCHANGED <<durable, content, locked, built, unlinked, newAcked>>
  /\ UNCHANGED oldVars

SyncRename ==
  /\ pc = "renamed" /\ durable' = names /\ pc' = "installed"
  /\ UNCHANGED <<names, content, locked, built, unlinked, newAcked>>
  /\ UNCHANGED oldVars

\* Release drops the source's lock once the steps in ReleaseAt are done.
Release ==
  /\ locked /\ pc \in ReleaseAt
  /\ locked' = FALSE
  /\ pc' = IF pc = "installed" THEN "idle" ELSE pc
  /\ UNCHANGED <<names, durable, content, built, unlinked, newAcked>>
  /\ UNCHANGED oldVars

\* Serve opens P in the new format, syncing the directory first.
Serve ==
  /\ pc = "idle" /\ names["P"] = "N"
  /\ durable' = names
  /\ pc' = "serving"
  /\ UNCHANGED <<names, content, locked, built, unlinked, newAcked>>
  /\ UNCHANGED oldVars

NewCommit ==
  /\ pc = "serving"
  /\ content' = [content EXCEPT !["N"] = @ \cup {"n"}]
  /\ newAcked' = TRUE
  /\ UNCHANGED <<names, durable, pc, locked, built, unlinked>>
  /\ UNCHANGED oldVars

\* An old-format process opens P, which it can read only while P is old.
OldOpen ==
  /\ OldOpener /\ old = "idle"
  /\ IF names["P"] = "S"
       THEN old' = "opened" /\ oldInode' = "S"
       ELSE old' = "refused" /\ UNCHANGED oldInode
  /\ UNCHANGED <<names, durable, content, pc, locked, built, unlinked,
       oldAcked, newAcked>>

\* It takes the writer lock and a slot once the converter has unlocked.
OldAdmit ==
  /\ old = "opened" /\ ~locked
  /\ old' = IF OldRechecks /\ names["P"] # oldInode THEN "refused" ELSE "slot"
  /\ UNCHANGED <<names, durable, content, pc, locked, built, unlinked,
       oldInode, oldAcked, newAcked>>

OldCommit ==
  /\ old = "slot"
  /\ content' = [content EXCEPT ![oldInode] = @ \cup {"o"}]
  /\ oldAcked' = TRUE /\ old' = "done"
  /\ UNCHANGED <<names, durable, pc, locked, built, unlinked, oldInode,
       newAcked>>

\* Kill stops the converting process; the kernel drops its locks.
Kill ==
  /\ pc \notin {"idle", "serving"}
  /\ pc' = "idle" /\ locked' = FALSE
  /\ UNCHANGED <<names, durable, content, built, unlinked, newAcked>>
  /\ UNCHANGED oldVars

\* Crash is a power loss: every process dies, and names revert.
Crash ==
  /\ names' = durable /\ pc' = "idle" /\ locked' = FALSE
  /\ old' = IF old \in {"opened", "slot"} THEN "idle" ELSE old
  /\ UNCHANGED <<durable, content, built, unlinked, oldInode, oldAcked,
       newAcked>>

Next == Lock \/ Build \/ Link \/ SyncLink \/ Rename \/ SyncRename \/ Release
  \/ Serve \/ NewCommit \/ OldOpen \/ OldAdmit \/ OldCommit \/ Kill \/ Crash
Spec == Init /\ [][Next]_vars

\* An acknowledged new-format commit is in the file P names.
NewAckKept == newAcked => names["P"] = "N" /\ "n" \in content["N"]
\* A commit to the source is inside the file P names, or the source is still
\* P.
CommitKept == oldAcked => "o" \in content[names["P"]] \/ names["P"] = "S"
\* An acknowledged old-format commit is on some named inode.
OldAckReachable == oldAcked => \E n \in Names :
  names[n] # None /\ "o" \in content[names[n]]
\* Once P is not the source, B keeps the source.
OriginalKept == names["P"] # "S" => names["B"] = "S"
\* The converter removes only its reserved name.
OnlyReservedRemoved == unlinked \subseteq {"T"}
\* Once P is not the source, the kept source holds exactly what the copy
\* started from. Before that, B and P name one live file, which the next
\* copy reads whole.
BackupIntact == names["P"] # "S" => content["S"] = built
====
