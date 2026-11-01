---- MODULE Conversion ----
\* As implemented in convertPrevious, resumeConverted, buildConverted and
\* installConverted. s4dbv1.Open owns the old inode's writer lock during the
\* stream/build/verify/marker steps; src.Close releases it BEFORE install.
\* An old-engine process can then reopen that inode and commit. A restarted
\* converter accepts a marker using convertedHolds (key count only), without
\* comparing its digest with the current source. Tags 1 and 2 are different
\* contents with the SAME key count. No corruption assumption is needed.
\* NewVolume calls conversion at open; CopyPreviousFormat streams/verifies
\* without installing and is outside this replacement lifecycle. A second
\* converter and same-process POSIX lock semantics are excluded, so failures
\* here already apply in the smaller one-converter, one-old-writer case.
\* Build is durable on return (Build's final sync); each crash abandons the
\* current control flow. Before the directory sync a marker may disappear.
\* Atomic rename chooses the old or new name on a crash; the original inode
\* survives under the backup when installed. Detailed fsync/rename durability
\* and torn values are not claimed by this bounded process-crash model.
EXTENDS Naturals

VARIABLES pc, source, temp, marker, locked, pathNew, backup
vars == <<pc, source, temp, marker, locked, pathNew, backup>>
Init ==
  /\ pc = "open" /\ source = 1 /\ temp = 0 /\ marker = FALSE
  /\ locked = FALSE /\ pathNew = FALSE /\ backup = FALSE
Open ==
  /\ pc = "open" /\ ~pathNew /\ locked' = TRUE
  /\ pc' = IF marker /\ temp # 0 THEN "close" ELSE "build"
  /\ UNCHANGED <<source, temp, marker, pathNew, backup>>
Build ==
  /\ pc = "build" /\ locked
  /\ temp' = source /\ marker' = FALSE /\ pc' = "verify"
  /\ UNCHANGED <<source, locked, pathNew, backup>>
Verify ==
  /\ pc = "verify" /\ temp = source
  /\ pc' = "marker"
  /\ UNCHANGED <<source, temp, marker, locked, pathNew, backup>>
Marker ==
  /\ pc = "marker" /\ marker' = TRUE /\ pc' = "close"
  /\ UNCHANGED <<source, temp, locked, pathNew, backup>>
CloseSource ==
  /\ pc = "close" /\ locked
  /\ locked' = FALSE /\ pc' = "link"
  /\ UNCHANGED <<source, temp, marker, pathNew, backup>>
OldCommit ==
  /\ ~locked /\ ~pathNew /\ source = 1
  /\ source' = 2
  /\ UNCHANGED <<pc, temp, marker, locked, pathNew, backup>>
Link ==
  /\ pc = "link" /\ backup' = TRUE /\ pc' = "rename"
  /\ UNCHANGED <<source, temp, marker, locked, pathNew>>
Rename ==
  /\ pc = "rename" /\ temp # 0 /\ backup
  /\ pathNew' = TRUE /\ pc' = "done" /\ marker' = FALSE
  /\ UNCHANGED <<source, temp, locked, backup>>
Crash ==
  /\ ~pathNew /\ pc # "open"
  /\ pc' = "open" /\ locked' = FALSE
  /\ UNCHANGED <<source, temp, marker, pathNew, backup>>
Next == Open \/ Build \/ Verify \/ Marker \/ CloseSource \/ OldCommit
  \/ Link \/ Rename \/ Crash \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars

\* These weaker guarantees actually hold, even when the installed copy is old.
OriginalRetained == pathNew => backup
InstalledWasBuilt == pathNew => temp # 0
\* Required acceptance: the installed file includes every source commit that
\* finished before rename. The source inode in the backup is not the active DB.
InstalledCurrent == pathNew => temp = source
====
