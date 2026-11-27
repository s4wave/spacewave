---- MODULE SObjectLookup ----
EXTENDS Naturals
(* sobject-lookup.go: pending descriptor replies are admitted only under the
   signer, connection, access and metadata generations that started them.
   The old action dispatcher alone selects legacy catalog refresh. Typed retry
   deadlines keep demand pending; RefCount owns the wait and cancellation,
   represented by stuttering before Receive. *)
VARIABLES generation, pending, started, reply, cache, unsupported
vars == <<generation, pending, started, reply, cache, unsupported>>
Init == /\ generation = 0 /\ pending = FALSE /\ started = 0
        /\ reply = "none" /\ cache = FALSE /\ unsupported = FALSE
Start == /\ ~pending /\ reply = "none"
         /\ pending' = TRUE /\ started' = generation
         /\ UNCHANGED <<generation, reply, cache, unsupported>>
Invalidate == /\ generation < 2 /\ generation' = generation + 1
              /\ cache' = FALSE /\ unsupported' = FALSE
              /\ UNCHANGED <<pending, started, reply>>
Receive(outcome) ==
  /\ pending /\ outcome \in {"found", "absent", "refused", "unreachable"}
  /\ pending' = FALSE /\ reply' = outcome
  /\ cache' = (outcome = "found" /\ started = generation)
  /\ unsupported' = (outcome = "absent" /\ started = generation)
  /\ UNCHANGED <<generation, started>>
Next == Start \/ Invalidate \/ (\E outcome \in {"found", "absent", "refused", "unreachable"}: Receive(outcome))
Fresh == (cache \/ unsupported) => started = generation
Legacy == unsupported => reply = "absent"
Private == cache => reply = "found"
Spec == Init /\ [][Next]_vars
====
