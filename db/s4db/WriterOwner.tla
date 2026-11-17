---- MODULE WriterOwner ----
\* Native writable opens claim an exclusive lifetime byte before recovery.
\* Read-only handles claim disjoint reader slots; close/death releases all
\* locks. OpenWait blocks before recovery and cancels without opening a DB.
\* OS lock grant is atomic. Disk/recovery content and durations are omitted.
EXTENDS Naturals, FiniteSets
Processes == {"P1", "P2"}
VARIABLES owner, writers, readers, waiting
vars == <<owner, writers, readers, waiting>>
Init == /\ owner = "none" /\ writers = {} /\ readers = {} /\ waiting = {}
Claim(p) ==
 /\ owner = "none" /\ p \notin readers /\ p \notin writers
 /\ owner' = p /\ waiting' = waiting \ {p}
 /\ UNCHANGED <<writers, readers>>
Recover(p) ==
 /\ owner = p /\ p \notin writers
 /\ writers' = writers \cup {p}
 /\ UNCHANGED <<owner, readers, waiting>>
ReadOpen(p) ==
 /\ p # owner /\ p \notin readers /\ p \notin waiting
 /\ readers' = readers \cup {p}
 /\ UNCHANGED <<owner, writers, waiting>>
Wait(p) ==
 /\ p # owner /\ p \notin readers /\ p \notin waiting
 /\ waiting' = waiting \cup {p}
 /\ UNCHANGED <<owner, writers, readers>>
Cancel(p) ==
 /\ p \in waiting /\ waiting' = waiting \ {p}
 /\ UNCHANGED <<owner, writers, readers>>
CloseOrDie(p) ==
 /\ owner = p \/ p \in readers
 /\ owner' = IF owner = p THEN "none" ELSE owner
 /\ writers' = writers \ {p} /\ readers' = readers \ {p}
 /\ UNCHANGED waiting
Next == \E p \in Processes : Claim(p) \/ Recover(p) \/ ReadOpen(p) \/ Wait(p) \/ Cancel(p) \/ CloseOrDie(p)
Spec == Init /\ [][Next]_vars
OneWriter == Cardinality(writers) <= 1
OwnBeforeRecover == \A p \in writers : owner = p
DisjointHandles == writers \intersect readers = {}
====
