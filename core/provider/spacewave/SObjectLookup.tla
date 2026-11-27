---- MODULE SObjectLookup ----
EXTENDS Naturals
(* sobject-lookup.go: a descriptor reply is admitted only under the signer,
   connection, access and metadata generations that started it. A reply that
   a newer generation superseded repeats the request under the current one, so
   only a reply from the current generation reaches the caller. Admitted
   descriptors live apart from the catalog and every invalidation clears them.
   The old action dispatcher alone selects legacy catalog refresh. Typed retry
   deadlines keep demand pending; RefCount owns the wait and cancellation,
   represented by stuttering before Receive. *)
VARIABLES generation, pending, started, reply, cache, unsupported
vars == <<generation, pending, started, reply, cache, unsupported>>
Outcomes == {"found", "absent", "refused", "unreachable"}
Init == /\ generation = 0 /\ pending = FALSE /\ started = 0
        /\ reply = "none" /\ cache = FALSE /\ unsupported = FALSE
Start == /\ ~pending /\ reply = "none"
         /\ pending' = TRUE /\ started' = generation
         /\ UNCHANGED <<generation, reply, cache, unsupported>>
Invalidate == /\ generation < 2 /\ generation' = generation + 1
              /\ cache' = FALSE /\ unsupported' = FALSE
              /\ UNCHANGED <<pending, started, reply>>
Receive(outcome) ==
  /\ pending /\ outcome \in Outcomes
  /\ IF started = generation
     THEN /\ pending' = FALSE /\ reply' = outcome
          /\ cache' = (outcome = "found")
          /\ unsupported' = (outcome = "absent")
          /\ UNCHANGED <<generation, started>>
     ELSE /\ started' = generation
          /\ UNCHANGED <<generation, pending, reply, cache, unsupported>>
Next == Start \/ Invalidate \/ (\E outcome \in Outcomes: Receive(outcome))
Fresh == (cache \/ unsupported) => started = generation
Legacy == unsupported => reply = "absent"
Private == cache => reply = "found"
Spec == Init /\ [][Next]_vars
====
