---- MODULE Conversion ----
\* As implemented in convertPrevious, resumeConverted, buildConverted and
\* installConverted. s4dbv1.OpenExclusive owns the old inode's writer lock until
\* after the rename. Reader.Fence then takes every reader slot before it drops
\* the writer lock, so an old-engine opener that opened the inode before the
\* rename fails its slot claim instead of writing the backup. The process holds
\* the fence until it exits; the model assumes such an opener reaches its slot
\* claim before then. Windows does not convert.
\* Resume compares the marker to the current source digest and re-verifies
\* every converted pair. OpenFile shares one process registry across both
\* formats; rejected local probes never close a descriptor of the locked source.
\* Tags 1 and 2 differ even with the SAME key count.
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

\* opener is an old-engine process holding the original inode: "none",
\* "waiting" between its open and its locks, "refused" or "admitted".
\* fenced is the fence's hold on every reader slot. backupWritten records an
\* old-engine write to the original inode after the rename.
VARIABLES pc, source, temp, marker, locked, pathNew, backup, opener, fenced,
  backupWritten
vars == <<pc, source, temp, marker, locked, pathNew, backup, opener, fenced,
  backupWritten>>
other == <<opener, fenced, backupWritten>>
Init ==
  /\ pc = "open" /\ source = 1 /\ temp = 0 /\ marker = FALSE
  /\ locked = FALSE /\ pathNew = FALSE /\ backup = FALSE
  /\ opener = "none" /\ fenced = FALSE /\ backupWritten = FALSE
Open ==
  /\ pc = "open" /\ ~pathNew /\ opener # "admitted" /\ locked' = TRUE
  /\ pc' = IF marker /\ temp = source THEN "link" ELSE "build"
  /\ UNCHANGED <<source, temp, marker, pathNew, backup>> /\ UNCHANGED other
Build ==
  /\ pc = "build" /\ locked
  /\ temp' = source /\ marker' = FALSE /\ pc' = "verify"
  /\ UNCHANGED <<source, locked, pathNew, backup>> /\ UNCHANGED other
Verify ==
  /\ pc = "verify" /\ temp = source
  /\ pc' = "marker"
  /\ UNCHANGED <<source, temp, marker, locked, pathNew, backup>> /\ UNCHANGED other
Marker ==
  /\ pc = "marker" /\ marker' = TRUE /\ pc' = "link"
  /\ UNCHANGED <<source, temp, locked, pathNew, backup>> /\ UNCHANGED other
\* Fence takes the slots, then drops the writer lock.
Fence ==
  /\ pc = "done" /\ locked
  /\ locked' = FALSE /\ fenced' = TRUE /\ pc' = "fenced"
  /\ UNCHANGED <<source, temp, marker, pathNew, backup, opener, backupWritten>>
\* An old-engine process opens the file by name, reaching the original inode
\* only before the rename.
OpenerOpen ==
  /\ opener = "none" /\ ~pathNew /\ opener' = "waiting"
  /\ UNCHANGED <<pc, source, temp, marker, locked, pathNew, backup, fenced,
       backupWritten>>
\* It takes the writer lock once free, then claims a slot.
OpenerAdmit ==
  /\ opener = "waiting" /\ ~locked
  /\ opener' = IF fenced THEN "refused" ELSE "admitted"
  /\ UNCHANGED <<pc, source, temp, marker, locked, pathNew, backup, fenced,
       backupWritten>>
\* An admitted process commits to the original inode.
OpenerCommit ==
  /\ opener = "admitted" /\ ~locked
  /\ IF pathNew THEN backupWritten' = TRUE /\ UNCHANGED source
     ELSE source' = 2 /\ UNCHANGED backupWritten
  /\ UNCHANGED <<pc, temp, marker, locked, pathNew, backup, opener, fenced>>
OldCommit ==
  /\ ~locked /\ ~pathNew /\ source = 1
  /\ source' = 2
  /\ UNCHANGED <<pc, temp, marker, locked, pathNew, backup>> /\ UNCHANGED other
Link ==
  /\ pc = "link" /\ backup' = TRUE /\ pc' = "rename"
  /\ UNCHANGED <<source, temp, marker, locked, pathNew>> /\ UNCHANGED other
Rename ==
  /\ pc = "rename" /\ temp # 0 /\ backup
  /\ pathNew' = TRUE /\ pc' = "done" /\ marker' = FALSE
  /\ UNCHANGED <<source, temp, locked, backup>> /\ UNCHANGED other
Crash ==
  /\ ~pathNew /\ pc # "open"
  /\ pc' = "open" /\ locked' = FALSE
  /\ UNCHANGED <<source, temp, marker, pathNew, backup>> /\ UNCHANGED other
\* A local format probe is refused before opening another source descriptor.
ProbeSource == UNCHANGED vars

Next == ProbeSource \/ Open \/ Build \/ Verify \/ Marker \/ Fence \/ OldCommit
  \/ OpenerOpen \/ OpenerAdmit \/ OpenerCommit \/ Link \/ Rename \/ Crash
  \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars

\* Installation keeps the original and every acknowledged source write.
OriginalRetained == pathNew => backup
InstalledWasBuilt == pathNew => temp # 0
\* Required acceptance: the installed file includes every source commit that
\* finished before rename. The source inode in the backup is not the active DB.
InstalledCurrent == pathNew => temp = source
\* The backup holds exactly the pairs the installed file started with.
BackupIntact == ~backupWritten
====
