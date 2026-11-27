---- MODULE SessionCredential ----
EXTENDS Naturals
(* Session startup and UnlockSession install a generation. LockSession and
   tracker logout revoke it before returning. watchSessionTransport observes
   the generation, starts transport, repairs a rejected registration and joins
   cleanup before observing its successor. Key use is a synchronous mutex
   section; network setup and registration are separate, cancellable steps.
   Account signer holders and direct peers retain the same revocable key.
   PIN/storage failures leave the Session locked and install no generation.
   Direct-policy changes affect the composition, not the credential fence.
   Pairing and account-transition consumers obey the same key fence; their
   application state is outside this model. *)
CONSTANT MaxGen
VARIABLES phase, gen, live, worker, transport, keyUse, stale, request, requestLive, staleSend
vars == <<phase, gen, live, worker, transport, keyUse, stale, request, requestLive, staleSend>>

Init ==
  /\ phase = "locked" /\ gen = 0 /\ live = {}
  /\ worker = 0 /\ transport = "stopped" /\ keyUse = 0 /\ stale = FALSE /\ request = 0 /\ requestLive = FALSE /\ staleSend = FALSE

Unlock ==
  /\ phase = "locked" /\ gen < MaxGen
  /\ phase' = "unlocking"
  /\ UNCHANGED <<gen, live, worker, transport, keyUse, stale, request, requestLive, staleSend>>
Install ==
  /\ phase = "unlocking"
  /\ gen' = gen + 1 /\ live' = {gen + 1} /\ phase' = "unlocked"
  /\ UNCHANGED <<worker, transport, keyUse, stale, request, requestLive, staleSend>>
Start ==
  /\ phase = "unlocked" /\ worker = 0
  /\ worker' = gen /\ transport' = "starting"
  /\ UNCHANGED <<phase, gen, live, keyUse, stale, request, requestLive, staleSend>>
Setup ==
  /\ transport = "starting"
  /\ transport' \in {"running", "rejected", "failed"}
  /\ UNCHANGED <<phase, gen, live, worker, keyUse, stale, request, requestLive, staleSend>>
Repair ==
  /\ transport = "rejected" /\ worker \in live
  /\ transport' = "registering"
  /\ UNCHANGED <<phase, gen, live, worker, keyUse, stale, request, requestLive, staleSend>>
RepairResponse ==
  /\ transport = "registering"
  /\ transport' = "starting"
  /\ UNCHANGED <<phase, gen, live, worker, keyUse, stale, request, requestLive, staleSend>>
BeginKeyUse ==
  /\ worker \in live /\ keyUse = 0
  /\ keyUse' = worker
  /\ UNCHANGED <<phase, gen, live, worker, transport, stale, request, requestLive, staleSend>>
EndKeyUse ==
  /\ keyUse # 0 /\ stale' = (keyUse \notin live) /\ keyUse' = 0
  /\ UNCHANGED <<phase, gen, live, worker, transport, request, requestLive, staleSend>>
Revoke ==
  /\ phase \in {"unlocked", "unlocking", "locked"} /\ keyUse = 0
  /\ phase' \in {"locking", "loggingout"} /\ live' = {} /\ requestLive' = FALSE
  /\ UNCHANGED <<gen, worker, transport, keyUse, stale, request, staleSend>>
Return ==
  /\ phase \in {"locking", "loggingout"}
  /\ phase' = IF phase = "locking" THEN "locked" ELSE "loggedout"
  /\ UNCHANGED <<gen, live, worker, transport, keyUse, stale, request, requestLive, staleSend>>
Cleanup ==
  /\ worker # 0 /\ worker \notin live /\ keyUse = 0
  /\ worker' = 0 /\ transport' = "stopped"
  /\ UNCHANGED <<phase, gen, live, keyUse, stale, request, requestLive, staleSend>>

PrepareRequest ==
  /\ request = 0 /\ worker \in live
  /\ request' = worker /\ requestLive' = TRUE
  /\ UNCHANGED <<phase, gen, live, worker, transport, keyUse, stale, staleSend>>
DispatchRequest ==
  /\ request # 0 /\ requestLive
  /\ staleSend' = (request \notin live) /\ request' = 0
  /\ UNCHANGED <<phase, gen, live, worker, transport, keyUse, stale, requestLive>>
CloseRequest ==
  /\ request # 0 /\ request' = 0 /\ requestLive' = FALSE
  /\ UNCHANGED <<phase, gen, live, worker, transport, keyUse, stale, staleSend>>

Next == Unlock \/ Install \/ Start \/ Setup \/ Repair \/ RepairResponse
        \/ BeginKeyUse \/ EndKeyUse \/ Revoke \/ Return \/ Cleanup
        \/ PrepareRequest \/ DispatchRequest \/ CloseRequest
Spec == Init /\ [][Next]_vars
TypeOK ==
  /\ phase \in {"locked", "unlocking", "unlocked", "locking", "loggingout", "loggedout"}
  /\ gen \in 0..MaxGen /\ live \subseteq 1..MaxGen
  /\ worker \in 0..MaxGen /\ keyUse \in 0..MaxGen /\ stale \in BOOLEAN
  /\ request \in 0..MaxGen /\ requestLive \in BOOLEAN /\ staleSend \in BOOLEAN
  /\ transport \in {"stopped", "starting", "running", "rejected", "registering", "failed"}
LockedHasNoKey == phase \in {"locked", "loggedout"} => live = {}
OnlyCurrentKey == live \subseteq {gen}
NoStaleKeyUse == ~stale
NoStaleRequest == ~staleSend
(* Returning requires no transport state change, so readiness cannot block it. *)
ReturnIndependent == phase \in {"locking", "loggingout"} => ENABLED Return
====
