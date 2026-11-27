--------------------------- MODULE BackupHold ---------------------------
EXTENDS FiniteSets, Naturals, TLC
\* Engine.PinBackup and CompleteBackup are serialized by backupMtx for two
\* independent backup IDs. Only the ordered named-root swap takes rootMtx;
\* metadata commits and the explicit durability fence run after releasing it.
\* Store.SetRoots atomically orders its hold, and a following full commit
\* makes every earlier ordered write durable, matching RootOps's contract.
Backups == {1, 2}
Old(b) == b
New(b) == b + 2
Roots == 1..4
VARIABLES held, diskHeld, pins, published, diskPins, diskPublished, active, phase, rootLocked
vars == <<held, diskHeld, pins, published, diskPins, diskPublished, active, phase, rootLocked>>
Rows(p, i) == UNION {p[b] \cup i[b]: b \in Backups}
Init == /\ held = {1, 2} /\ diskHeld = held
        /\ pins = [b \in Backups |-> {}] /\ published = [b \in Backups |-> {Old(b)}]
        /\ diskPins = pins /\ diskPublished = published
        /\ active = 0 /\ phase = "idle" /\ rootLocked = FALSE
Start(b) == /\ phase = "idle" /\ active' = b /\ phase' = "swap" /\ rootLocked' = TRUE
            /\ UNCHANGED <<held, diskHeld, pins, published, diskPins, diskPublished>>
Append == /\ phase = "swap" /\ held' = held \cup {New(active)}
          /\ phase' = "fence" /\ rootLocked' = FALSE
          /\ UNCHANGED <<diskHeld, pins, published, diskPins, diskPublished, active>>
RecordPin == /\ phase = "fence" /\ pins' = [pins EXCEPT ![active] = {New(active)}]
             /\ diskPins' = pins' /\ diskPublished' = published /\ diskHeld' = held
             /\ phase' = "copy" /\ UNCHANGED <<held, published, active, rootLocked>>
\* A complete pass publishes its remote index before CompleteBackup's local row.
Complete == /\ phase = "copy" /\ published' = [published EXCEPT ![active] = {New(active)}]
            /\ pins' = [pins EXCEPT ![active] = {}]
            /\ diskPins' = pins' /\ diskPublished' = published' /\ diskHeld' = held
            /\ phase' = "compact" /\ UNCHANGED <<held, active, rootLocked>>
Compact == /\ phase = "compact" /\ held' = Rows(pins, published)
           /\ phase' = "idle" /\ active' = 0
           /\ UNCHANGED <<diskHeld, pins, published, diskPins, diskPublished, rootLocked>>
\* A failed metadata commit leaves the old manifest and all extra holds intact.
Abort == /\ phase # "idle" /\ phase' = "idle" /\ active' = 0 /\ rootLocked' = FALSE
         /\ UNCHANGED <<held, diskHeld, pins, published, diskPins, diskPublished>>
Crash == /\ held' = diskHeld /\ pins' = diskPins /\ published' = diskPublished
         /\ phase' = "idle" /\ active' = 0 /\ rootLocked' = FALSE
         /\ UNCHANGED <<diskHeld, diskPins, diskPublished>>
Next == (\E b \in Backups: Start(b)) \/ Append \/ RecordPin \/ Complete \/ Compact \/ Abort \/ Crash
TypeOK == /\ held \subseteq Roots /\ diskHeld \subseteq Roots
          /\ pins \in [Backups -> SUBSET Roots] /\ published \in [Backups -> SUBSET Roots]
          /\ diskPins \in [Backups -> SUBSET Roots] /\ diskPublished \in [Backups -> SUBSET Roots]
          /\ active \in {0, 1, 2} /\ phase \in {"idle", "swap", "fence", "copy", "compact"}
          /\ rootLocked \in BOOLEAN
RunningAndPublishedHeld == Rows(pins, published) \subseteq held
CrashKeepsEveryRow == Rows(diskPins, diskPublished) \subseteq diskHeld
FenceOutsideRootLock == phase = "fence" => ~rootLocked
Spec == Init /\ [][Next]_vars
=============================================================================
