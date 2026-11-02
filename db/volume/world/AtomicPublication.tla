---- MODULE AtomicPublication ----
EXTENDS Naturals
(* worldTransaction stages one bounded KV tree under one World object root.
   World Commit may defer durability; engine.Sync fences that root and all its
   block bytes before the nested KV Commit returns. Validation rejection and
   Discard occur before root publication. A crash can expose the old or new
   committed root, never a metadata/bytes mixture. A failed fence can leave an
   unacknowledged committed root; it must never produce a success receipt.
   One publication, one metadata head and its bytes, and old/new generations
   cover ordering, discard, rejection, deferred durability and crash boundaries.
   WaitForFence=FALSE is the missing-Sync mutant. *)
CONSTANT WaitForFence
VARIABLES phase, liveHead, liveBytes, savedHead, savedBytes
vars == <<phase, liveHead, liveBytes, savedHead, savedBytes>>
Init == /\ phase = "idle" /\ liveHead = 0 /\ liveBytes = 0
        /\ savedHead = 0 /\ savedBytes = 0
Stage == /\ phase = "idle" /\ phase' = "staged"
         /\ UNCHANGED <<liveHead, liveBytes, savedHead, savedBytes>>
Reject == /\ phase = "staged" /\ phase' \in {"rejected", "discarded"}
          /\ UNCHANGED <<liveHead, liveBytes, savedHead, savedBytes>>
CommitWorld == /\ phase = "staged" /\ phase' = "committed"
               /\ liveHead' = 1 /\ liveBytes' = 1
               /\ savedHead' \in {0, 1} /\ savedBytes' = savedHead'
Fence == /\ phase = "committed" /\ phase' = "fenced"
         /\ savedHead' = 1 /\ savedBytes' = 1
         /\ UNCHANGED <<liveHead, liveBytes>>
Reply == /\ (phase = "fenced" \/ (~WaitForFence /\ phase = "committed"))
         /\ phase' = "acknowledged"
         /\ UNCHANGED <<liveHead, liveBytes, savedHead, savedBytes>>
FailFence == /\ phase = "committed" /\ phase' = "failed"
             /\ UNCHANGED <<liveHead, liveBytes, savedHead, savedBytes>>
Crash == /\ phase # "crashed" /\ phase' = "crashed"
         /\ liveHead' = savedHead /\ liveBytes' = savedBytes
         /\ UNCHANGED <<savedHead, savedBytes>>
Next == Stage \/ Reject \/ CommitWorld \/ Fence \/ Reply \/ FailFence \/ Crash \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars
AtomicRoot == liveHead = liveBytes /\ savedHead = savedBytes
SuccessIsDurable == phase = "acknowledged" => savedHead = 1
RejectedIsUnchanged == phase \in {"rejected", "discarded"} => savedHead = 0
CrashReadsDurable == phase = "crashed" => liveHead = savedHead
====
