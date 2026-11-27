---------------------- MODULE SpaceBackupSchedule ----------------------
EXTENDS Naturals, TLC
\* One actor executes passes. A manual requester may join an automatic pass;
\* only that explicit request survives turning the automatic switch off.
CONSTANT MaxTime
VARIABLES automatic, phase, origin, checkpoint, attempted, runCheckpoint, due, now, startedAt, startedDue
vars == <<automatic, phase, origin, checkpoint, attempted, runCheckpoint, due, now, startedAt, startedDue>>
Init == /\ automatic = FALSE /\ phase = "idle" /\ origin = "manual"
        /\ checkpoint = 0 /\ attempted = 0 /\ due = 0 /\ now = 0
        /\ startedAt = 0 /\ startedDue = 0 /\ runCheckpoint = 0
Enable == /\ ~automatic /\ automatic' = TRUE
          /\ UNCHANGED <<phase, origin, checkpoint, attempted, runCheckpoint, due, now, startedAt, startedDue>>
Disable == /\ automatic /\ automatic' = FALSE
           /\ phase' = IF phase = "copying" /\ origin = "automatic" THEN "idle" ELSE phase
           /\ UNCHANGED <<origin, checkpoint, attempted, runCheckpoint, due, now, startedAt, startedDue>>
Checkpoint == /\ checkpoint < 2 /\ checkpoint' = checkpoint + 1
              /\ UNCHANGED <<automatic, phase, origin, attempted, runCheckpoint, due, now, startedAt, startedDue>>
Tick == /\ now < MaxTime /\ now' = now + 1
        /\ UNCHANGED <<automatic, phase, origin, checkpoint, attempted, runCheckpoint, due, startedAt, startedDue>>
BeginAuto == /\ automatic /\ phase \in {"idle", "limit"} /\ now >= due
             /\ (checkpoint # attempted \/ phase = "limit")
             /\ phase' = "copying" /\ origin' = "automatic"
             /\ runCheckpoint' = checkpoint
             /\ startedAt' = now /\ startedDue' = due
             /\ UNCHANGED <<automatic, checkpoint, attempted, due, now>>
Manual == /\ phase' = "copying" /\ origin' = "manual"
          /\ runCheckpoint' = IF phase = "copying" THEN runCheckpoint ELSE checkpoint
          /\ startedAt' = now /\ startedDue' = due
          /\ UNCHANGED <<automatic, checkpoint, attempted, due, now>>
Finish(outcome) == /\ phase = "copying" /\ phase' = outcome
                   /\ attempted' = runCheckpoint /\ due' = now + 2
                   /\ UNCHANGED <<automatic, origin, checkpoint, runCheckpoint, now, startedAt, startedDue>>
\* A restart keeps the persisted quota/sign-in outcome and upload-limit deadline.
Restart == /\ phase # "copying" /\ UNCHANGED vars
Next == Enable \/ Disable \/ Checkpoint \/ Tick \/ BeginAuto \/ Manual
        \/ (\E outcome \in {"idle", "limit", "quota", "sign-in"}: Finish(outcome)) \/ Restart
TypeOK == /\ automatic \in BOOLEAN /\ phase \in {"idle", "copying", "limit", "quota", "sign-in"}
          /\ origin \in {"manual", "automatic"} /\ checkpoint \in 0..2 /\ attempted \in 0..2
          /\ runCheckpoint \in 0..2
          /\ now \in 0..MaxTime /\ due \in 0..(MaxTime + 2)
          /\ startedAt \in 0..MaxTime /\ startedDue \in 0..(MaxTime + 2)
OffStopsAutomatic == (phase = "copying" /\ origin = "automatic") => automatic
MinimumInterval == (phase = "copying" /\ origin = "automatic") => startedAt >= startedDue
Spec == Init /\ [][Next]_vars
=============================================================================
