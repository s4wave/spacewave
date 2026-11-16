---------------------------- MODULE EvictRefused ----------------------------
\* Evict advances the durable epoch, checks one bounded batch, drops values,
\* then atomically deletes/moves queue entries. One entry models a metadata
\* slice, including restart after earlier slices committed. The Go scan and
\* probes still batch 1,024 entries; retry metadata commits at most 64 entries.
\* Fill is modeled by the initial queued values, including
\* a peer fill absent remotely and a marked block. Upload clear queues an
\* uploaded block at the current epoch; a dropped upload clears its marker
\* without queueing. This exception requires retaining marked refusals. Puts
\* are outside this model; remote restoration occurs between passes. It checks
\* refusal retention, not the existing
\* concurrent put versus eviction contract or backend existence accuracy.
\* The caller always keeps block 4, which moves forward under either policy.
\* Errors before commit preserve the queue. Restart forgets the cursor and
\* recovers the committed queue; a crash after a value drop leaves cleanup.
\* Disable's deliberate queue reset, upload I/O, KV durability and
\* provider timers/backoff are outside this bounded queue-transition model.
EXTENDS Integers, FiniteSets
CONSTANT RetryRefused
Blocks == {1, 2, 3, 4}
VARIABLES queue, local, remote, marked, epoch, phase, current, seen, refused, restarted
vars == <<queue, local, remote, marked, epoch, phase, current, seen, refused, restarted>>

Init == /\ queue = [b \in Blocks |-> 0]
        /\ local = Blocks /\ remote = {2, 3, 4} /\ marked = {3}
        /\ epoch = 0 /\ phase = "idle" /\ current = 0
        /\ seen = {} /\ refused = {} /\ restarted = FALSE

Start == /\ phase = "idle" /\ epoch < 2
         /\ epoch' = epoch + 1 /\ phase' = "scan" /\ seen' = {} /\ restarted' = FALSE
         /\ UNCHANGED <<queue, local, remote, marked, current, refused>>

Eligible == {b \in Blocks : queue[b] >= 0 /\ queue[b] < epoch}
Select == /\ phase = "scan" /\ Eligible # {}
          /\ \E b \in Eligible :
               /\ \A c \in Eligible : b <= c
               /\ current' = b /\ phase' = "check"
          /\ UNCHANGED <<queue, local, remote, marked, epoch, seen, refused, restarted>>

Check == /\ phase = "check"
         /\ current \notin seen
         /\ seen' = seen \cup {current}
         /\ phase' = IF current \notin local THEN "cleanup"
                     ELSE IF current \in marked \/ current \notin remote
                          THEN "refuse"
                          ELSE IF current = 4 THEN "keep" ELSE "drop"
         /\ UNCHANGED <<queue, local, remote, marked, epoch, current, refused, restarted>>

Refuse == /\ phase = "refuse"
          /\ queue' = [queue EXCEPT ![current] = IF RetryRefused THEN epoch ELSE -1]
          /\ refused' = refused \cup {current}
          /\ phase' = "scan"
          /\ UNCHANGED <<local, remote, marked, epoch, current, seen, restarted>>

Keep == /\ phase = "keep"
        /\ queue' = [queue EXCEPT ![current] = epoch] /\ phase' = "scan"
        /\ UNCHANGED <<local, remote, marked, epoch, current, seen, refused, restarted>>

Drop == /\ phase = "drop"
        /\ current \in remote /\ current \notin marked
        /\ local' = local \ {current} /\ phase' = "cleanup"
        /\ UNCHANGED <<queue, remote, marked, epoch, current, seen, refused, restarted>>

Cleanup == /\ phase = "cleanup"
           /\ queue' = [queue EXCEPT ![current] = -1]
           /\ phase' = "scan"
           /\ UNCHANGED <<local, remote, marked, epoch, current, seen, refused, restarted>>

Finish == /\ phase = "scan" /\ Eligible = {}
          /\ phase' = "idle"
          /\ UNCHANGED <<queue, local, remote, marked, epoch, current, seen, refused, restarted>>

\* A transient failure/restart forgets only work not committed. Restart after
\* Drop sees an absent local value and cleans up without another remote drop.
Restart == /\ phase \in {"check", "refuse", "keep", "drop", "cleanup"}
           /\ ~restarted
           /\ phase' = "scan" /\ seen' = {} /\ restarted' = TRUE
           /\ UNCHANGED <<queue, local, remote, marked, epoch, current, refused>>

\* The next pass rechecks the backend and marker; refusal alone changes neither.
ClearCause == /\ phase = "idle" /\ epoch = 1
              /\ remote # Blocks \/ marked # {}
              /\ remote' = Blocks /\ marked' = {}
              /\ queue' = [b \in Blocks |-> IF b \in marked THEN epoch ELSE queue[b]]
              /\ UNCHANGED <<local, epoch, phase, current, seen, refused, restarted>>

\* Upload observed an absent local block. Its unchanged pending entry may
\* clear without queueing eviction; this is not the uploaded clear above.
ClearDropped == /\ phase = "idle" /\ epoch = 1 /\ marked # {}
                /\ local' = local \ marked /\ marked' = {}
                /\ UNCHANGED <<queue, remote, epoch, phase, current, seen, refused, restarted>>

Done == /\ phase = "idle" /\ epoch = 2 /\ UNCHANGED vars
Next == Start \/ Select \/ Check \/ Refuse \/ Keep \/ Drop \/ Cleanup \/ Finish
        \/ Restart \/ ClearCause \/ ClearDropped \/ Done
\* One failure per pass bounds the retry abstraction. A fair pass must finish
\* even when its remote remains missing and its marker never clears.
Spec == Init /\ [][Next]_vars
        /\ WF_vars(Select) /\ WF_vars(Check) /\ WF_vars(Refuse)
        /\ WF_vars(Keep) /\ WF_vars(Drop) /\ WF_vars(Cleanup) /\ WF_vars(Finish)

\* Missing remote bytes survive every pass and every modeled restart.
NoMissingCopyDropped == Blocks \ remote \subseteq local
MarkedKept == marked \subseteq local
CallerKept == 4 \in local /\ queue[4] >= 0
RefusedRetained == \A b \in refused \cap local : queue[b] >= 0
PassCompletes == (phase # "idle") ~> (phase = "idle")
=============================================================================
