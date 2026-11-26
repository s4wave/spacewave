---------------------------- MODULE EvictRefused ----------------------------
\* Evict advances the durable epoch, checks one bounded batch, drops values,
\* then atomically deletes/moves queue entries. One entry models a metadata
\* slice: Go scans/probes 1,024 entries and commits at most 64 with retry on.
\* Store.writeFills reserves queue coverage before writing values while holding
\* fillMtx. Its queue commit may fail, its value write may fail, and a crash
\* may abandon the fill between either step. Evict takes fillMtx only around
\* its epoch advance: Start waits for an unfinished fill, and a fill may start
\* at any later point of the pass, reserving under the pass epoch, which the
\* pass neither visits nor deletes. A pass deletes only the older entry it
\* scanned. Retry off models the former value-before-queue
\* order without that exclusion. The ordering assumes committed metadata and
\* values share their Volume's ordered commit/recovery contract, not atomicity
\* across the two transactions or durability of an ordered commit by itself.
\* Block 3 starts absent with an upload that already observed it absent. A
\* cache fill restores it; dropped clear removes only the marker, even if the
\* fill restored the value meanwhile. Its preexisting entry may be absent or
\* at epoch zero. ClearDropped can interleave with the fill and pass. Normal
\* upload clear queues at the current epoch. Block 1 models a peer fill whose
\* placement lacks the remote copy. The caller always keeps block 4.
\* The Volume may drop the fill instead of storing it, as it drops a fill
\* whose refs it does not trust, leaving block 3 absent. With retry on, the
\* fill step then removes the entry it reserved, still under fillMtx, so the
\* epoch it reserved under is current; the removal may fail, leaving a surplus
\* entry for an absent value, which a later pass deletes. Retry off queues
\* only the stored fills. Outside the model: an ordinary put of block 3 that
\* lands after the drop, whose upload clear commits before the removal. The
\* removal then deletes the clear's entry and the resident block is never
\* evicted, a one-block cache leak that loses no data.
\* Producers: normal/ClearLocated upload clear and one cache/value-only fill
\* are modeled; ordinary puts, backfill, SetEnabled(false), epoch overflow,
\* upload I/O and provider scheduling are outside this bounded model. Current
\* marker/remote decisions stand for the existing probe semantics. Foreground
\* writes never take fillMtx; KV writer admission is not modeled. The pass
\* takes fillMtx before evictMtx and releases it after the epoch advance;
\* disable still takes only the latter. A map with
\* one entry per hash abstracts coverage, not cross-epoch multiplicity or byte
\* accounting; duplicate epochs and the marked-byte overlap remain unresolved.
EXTENDS Integers, FiniteSets
CONSTANT RetryRefused
Blocks == {1, 2, 3, 4}
VARIABLES queue, local, remote, marked, epoch, phase, current, seen, refused,
          restarted, fillPhase, fillOutcome
vars == <<queue, local, remote, marked, epoch, phase, current, seen, refused,
          restarted, fillPhase, fillOutcome>>

Init == /\ \E old \in {-1, 0} : queue = [b \in Blocks |-> IF b = 3 THEN old ELSE 0]
        /\ local = Blocks \ {3} /\ remote = {2, 3, 4} /\ marked = {3}
        /\ epoch = 0 /\ phase = "idle" /\ current = 0
        /\ seen = {} /\ refused = {} /\ restarted = FALSE /\ fillPhase = "waiting"
        /\ fillOutcome = "none"

FillActive == fillPhase \in {"reserve", "write", "queue", "unqueue"}
FillStart == /\ fillPhase = "waiting"
             /\ fillPhase' = IF RetryRefused THEN "reserve" ELSE "write"
             /\ UNCHANGED <<queue, local, remote, marked, epoch, phase, current,
                            seen, refused, restarted, fillOutcome>>
FillReserve == /\ fillPhase = "reserve"
               /\ queue' = [queue EXCEPT ![3] = epoch] /\ fillPhase' = "write"
               /\ UNCHANGED <<local, remote, marked, epoch, phase, current,
                              seen, refused, restarted, fillOutcome>>
\* The Volume stores the fill, or drops it and stores nothing.
FillWrite == /\ fillPhase = "write"
             /\ \/ /\ local' = local \cup {3} /\ fillOutcome' = "stored"
                   /\ fillPhase' = IF RetryRefused THEN "done" ELSE "queue"
                \/ /\ 3 \notin local /\ fillOutcome' = "dropped"
                   /\ fillPhase' = IF RetryRefused THEN "unqueue" ELSE "done"
                   /\ UNCHANGED local
             /\ UNCHANGED <<queue, remote, marked, epoch, phase, current,
                            seen, refused, restarted>>
\* The step removes a dropped fill's reserved entry.
FillUnqueue == /\ fillPhase = "unqueue"
               /\ queue' = [queue EXCEPT ![3] = -1]
               /\ fillPhase' = "done" /\ fillOutcome' = "unqueued"
               /\ UNCHANGED <<local, remote, marked, epoch, phase, current,
                              seen, refused, restarted>>
FillQueue == /\ fillPhase = "queue"
             /\ queue' = [queue EXCEPT ![3] = epoch] /\ fillPhase' = "done"
             /\ UNCHANGED <<local, remote, marked, epoch, phase, current,
                            seen, refused, restarted, fillOutcome>>
\* Failed queue/value writes and crash discard only the unfinished step.
FillFail == /\ FillActive /\ fillPhase' = "done"
            /\ UNCHANGED <<queue, local, remote, marked, epoch, phase, current,
                           seen, refused, restarted, fillOutcome>>

Start == /\ phase = "idle" /\ epoch < 2 /\ (~RetryRefused \/ ~FillActive)
         /\ epoch' = epoch + 1 /\ phase' = "scan" /\ seen' = {} /\ restarted' = FALSE
         /\ UNCHANGED <<queue, local, remote, marked, current, refused, fillPhase, fillOutcome>>
Eligible == {b \in Blocks : queue[b] >= 0 /\ queue[b] < epoch}
Select == /\ phase = "scan" /\ Eligible # {}
          /\ \E b \in Eligible :
               /\ \A c \in Eligible : b <= c
               /\ current' = b /\ phase' = "check"
          /\ UNCHANGED <<queue, local, remote, marked, epoch, seen, refused,
                         restarted, fillPhase, fillOutcome>>
Check == /\ phase = "check" /\ current \notin seen
         /\ seen' = seen \cup {current}
         /\ phase' = IF current \notin local THEN "cleanup"
                     ELSE IF current \in marked \/ current \notin remote
                          THEN "refuse"
                          ELSE IF current = 4 THEN "keep" ELSE "drop"
         /\ UNCHANGED <<queue, local, remote, marked, epoch, current, refused,
                        restarted, fillPhase, fillOutcome>>
Refuse == /\ phase = "refuse"
          /\ queue' = [queue EXCEPT ![current] = IF @ >= epoch THEN @
                                                  ELSE IF RetryRefused THEN epoch ELSE -1]
          /\ refused' = refused \cup {current} /\ phase' = "scan"
          /\ UNCHANGED <<local, remote, marked, epoch, current, seen, restarted, fillPhase, fillOutcome>>
Keep == /\ phase = "keep"
        /\ queue' = [queue EXCEPT ![current] = epoch] /\ phase' = "scan"
        /\ UNCHANGED <<local, remote, marked, epoch, current, seen, refused,
                       restarted, fillPhase, fillOutcome>>
Drop == /\ phase = "drop" /\ current \in remote /\ current \notin marked
        /\ local' = local \ {current} /\ phase' = "cleanup"
        /\ UNCHANGED <<queue, remote, marked, epoch, current, seen, refused,
                       restarted, fillPhase, fillOutcome>>
Cleanup == /\ phase = "cleanup"
           /\ queue' = [queue EXCEPT ![current] = IF @ >= epoch THEN @ ELSE -1]
           /\ phase' = "scan"
           /\ UNCHANGED <<local, remote, marked, epoch, current, seen, refused,
                          restarted, fillPhase, fillOutcome>>
Finish == /\ phase = "scan" /\ Eligible = {} /\ phase' = "idle"
          /\ UNCHANGED <<queue, local, remote, marked, epoch, current, seen,
                         refused, restarted, fillPhase, fillOutcome>>
Restart == /\ phase \in {"check", "refuse", "keep", "drop", "cleanup"}
           /\ ~restarted /\ phase' = "scan" /\ seen' = {} /\ restarted' = TRUE
           /\ UNCHANGED <<queue, local, remote, marked, epoch, current, refused, fillPhase, fillOutcome>>

\* Normal upload clear records queue coverage in its marker-removal transaction.
ClearCause == /\ phase = "idle" /\ epoch = 1 /\ (remote # Blocks \/ marked # {})
              /\ remote' = Blocks /\ marked' = {}
              /\ queue' = [b \in Blocks |-> IF b \in marked THEN epoch ELSE queue[b]]
              /\ UNCHANGED <<local, epoch, phase, current, seen, refused, restarted, fillPhase, fillOutcome>>
\* This clear used an absent read captured before the fill, not current local.
ClearDropped == /\ 3 \in marked /\ marked' = marked \ {3}
                /\ UNCHANGED <<queue, local, remote, epoch, phase, current, seen,
                               refused, restarted, fillPhase, fillOutcome>>
Done == /\ phase = "idle" /\ epoch = 2 /\ fillPhase = "done" /\ UNCHANGED vars
Next == Start \/ Select \/ Check \/ Refuse \/ Keep \/ Drop \/ Cleanup \/ Finish
        \/ Restart \/ ClearCause \/ ClearDropped \/ FillStart \/ FillReserve
        \/ FillWrite \/ FillUnqueue \/ FillQueue \/ FillFail \/ Done
Spec == Init /\ [][Next]_vars
        /\ WF_vars(Select) /\ WF_vars(Check) /\ WF_vars(Refuse)
        /\ WF_vars(Keep) /\ WF_vars(Drop) /\ WF_vars(Cleanup) /\ WF_vars(Finish)
        /\ WF_vars(FillStart) /\ WF_vars(FillReserve) /\ WF_vars(FillWrite)
        /\ WF_vars(FillQueue) /\ WF_vars(FillUnqueue)
NoMissingCopyDropped == Blocks \ remote \subseteq local
CallerKept == 4 \in local /\ queue[4] >= 0
RefusedRetained == \A b \in refused \cap local : queue[b] >= 0
NoStrandedValue == \A b \in local \ marked : queue[b] >= 0
\* Once the passes end, a dropped fill whose removal ran leaves no entry: a
\* reservation the last pass could not see is gone, and an older one that
\* pass deleted.
DropUnqueued ==
  (phase = "idle" /\ epoch = 2 /\ fillPhase = "done" /\ fillOutcome = "unqueued")
    => queue[3] < 0
PassCompletes == (phase # "idle") ~> (phase = "idle")
=============================================================================
