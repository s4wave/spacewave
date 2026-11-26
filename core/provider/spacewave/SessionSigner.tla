---- MODULE SessionSigner ----
EXTENDS Naturals
(* session-signer.go and account-session-client.go: the account's session
   signer slot. Each install binds one SessionSigner, with the next
   generation, to one Session's key, and reauthentication binds the Session's
   fresh key the same way. Installing for the Session that already holds a live
   signer keeps that signer. Every other install, a drop, a lock, a logout and
   a reauthentication revokes the signer the slot held. A holder retains a
   signer it resolved, such as the block sync owner for one request or the
   SharedObject host for its mount, and signs through it. A revoked signer
   refuses to sign, and its holder sees Done close and resolves the slot
   again. *)
CONSTANTS Sessions, Holders, MaxGen
None == "none"
Gens == 1..MaxGen
\* unlocked: whether each Session's key is in memory.
\* slotSid, slotGen: the Session the slot is bound to and its signer's
\*   generation; slotGen = 0 is the empty slot.
\* sstate, ssid: each generation's signer state and the Session whose key it
\*   holds, which the slot may label differently in slotSid.
\* next: the generation the next install takes.
\* held: the generation each holder retains, or 0.
\* stale: the last signature was made by a revoked signer.
VARIABLES unlocked, slotSid, slotGen, sstate, ssid, next, held, stale
vars == <<unlocked, slotSid, slotGen, sstate, ssid, next, held, stale>>

TypeOK ==
  /\ unlocked \in [Sessions -> BOOLEAN]
  /\ slotSid \in Sessions \cup {None} /\ slotGen \in 0..MaxGen
  /\ sstate \in [Gens -> {"unused", "live", "revoked"}]
  /\ ssid \in [Gens -> Sessions \cup {None}]
  /\ next \in 1..(MaxGen + 1)
  /\ held \in [Holders -> 0..MaxGen]
  /\ stale \in BOOLEAN

Init ==
  /\ unlocked = [s \in Sessions |-> FALSE]
  /\ slotSid = None /\ slotGen = 0
  /\ sstate = [g \in Gens |-> "unused"] /\ ssid = [g \in Gens |-> None]
  /\ next = 1 /\ held = [h \in Holders |-> 0] /\ stale = FALSE

\* Revoke the slot's signer, if it holds one.
RevokeSlot == IF slotGen = 0 THEN sstate
              ELSE [sstate EXCEPT ![slotGen] = "revoked"]

\* Bind a new live signer for Session k's key, labeled sid in the slot, and
\* revoke the one the slot held.
Bind(sid, k) ==
  /\ next <= MaxGen
  /\ sstate' = [RevokeSlot EXCEPT ![next] = "live"]
  /\ ssid' = [ssid EXCEPT ![next] = k]
  /\ slotSid' = sid /\ slotGen' = next /\ next' = next + 1

\* A Session unlocks its key.
Unlock(s) ==
  /\ ~unlocked[s]
  /\ unlocked' = [unlocked EXCEPT ![s] = TRUE]
  /\ UNCHANGED <<slotSid, slotGen, sstate, ssid, next, held, stale>>

\* maybeSetSessionClient and the repair path install an unlocked Session's
\* signer unless another Session holds the slot. The Session that already
\* holds a live signer keeps it.
Install(s) ==
  /\ unlocked[s]
  /\ slotSid \in {None, s}
  /\ ~(slotSid = s /\ slotGen # 0 /\ sstate[slotGen] = "live")
  /\ Bind(s, s)
  /\ UNCHANGED <<unlocked, held, stale>>

\* ReplaceSessionClient installs the signer for a Session's fresh key, which
\* reauthentication also writes as the Session's unlock key, so the key is in
\* memory and the Session's lock revokes the signer.
Reauthenticate(s) ==
  /\ unlocked' = [unlocked EXCEPT ![s] = TRUE]
  /\ Bind(s, s)
  /\ UNCHANGED <<held, stale>>

\* Lock, logout and the Session tracker's exit drop the Session's signer.
Lock(s) ==
  /\ unlocked' = [unlocked EXCEPT ![s] = FALSE]
  /\ IF slotSid = s
       THEN /\ sstate' = RevokeSlot /\ slotSid' = None /\ slotGen' = 0
       ELSE UNCHANGED <<sstate, slotSid, slotGen>>
  /\ UNCHANGED <<ssid, next, held, stale>>

\* A holder resolves the slot's live signer and retains it.
Acquire(h) ==
  /\ held[h] = 0 /\ slotGen # 0 /\ sstate[slotGen] = "live"
  /\ held' = [held EXCEPT ![h] = slotGen]
  /\ UNCHANGED <<unlocked, slotSid, slotGen, sstate, ssid, next, stale>>

\* A holder signs a request through its retained signer, which refuses once
\* revoked.
Sign(h) ==
  /\ held[h] # 0
  /\ sstate[held[h]] = "live"
  /\ stale' = (sstate[held[h]] # "live")
  /\ UNCHANGED <<unlocked, slotSid, slotGen, sstate, ssid, next, held>>

\* A holder sees its signer's Done close and releases it to resolve again.
Release(h) ==
  /\ held[h] # 0 /\ sstate[held[h]] = "revoked"
  /\ held' = [held EXCEPT ![h] = 0]
  /\ UNCHANGED <<unlocked, slotSid, slotGen, sstate, ssid, next, stale>>

Next ==
  \/ \E s \in Sessions : Unlock(s) \/ Install(s) \/ Lock(s) \/ Reauthenticate(s)
  \/ \E h \in Holders : Acquire(h) \/ Sign(h) \/ Release(h)

Spec == Init /\ [][Next]_vars

\* At most one signer is live, and it is the one the slot holds.
OneLiveSigner ==
  \A g \in Gens : sstate[g] = "live" => slotGen = g

\* A locked Session has no live signer.
LockedHasNoLiveSigner ==
  \A g \in Gens : sstate[g] = "live" => unlocked[ssid[g]]

\* No signature is made by a revoked signer.
NoStaleSignature == ~stale

\* Generations only grow: the slot's signer is the newest one bound.
SlotIsNewest == slotGen = 0 \/ slotGen = next - 1
====
