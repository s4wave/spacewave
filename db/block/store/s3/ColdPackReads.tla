----------------------------- MODULE ColdPackReads -----------------------------
EXTENDS Naturals, FiniteSets

(***************************************************************************
 Two scopes each name one pack at ordinal zero, with one indexed extent.
 Scope A holds Canonical and scope B holds Extended. Both wire keys have the
 same body digest, so verifying payload bytes cannot prove wire membership.
 Current records are offered only after an exact canonical index hit and body
 verification. Recovery keeps only current records for the mounted scope,
 with no verified bodies. A legacy row may come from an extended-key read;
 its key and scope below are provenance, not fields its decoder can prove.

 Byte and CRC validation, local log locking, and append errors are abstracted
 to a durable set and an unsynced queue lost on crash. ReadIndexed and
 ReadPromoted model successful body reads and stats that need a body. A stat
 cached by a verified promotion has the same evidence as an existence hit
 from that promotion. ReadMiss models failures without publishing proof.
 Existence checks only resident evidence, never starting or joining I/O.
 A matching whole-object checksum also lets a resident exact index answer
 existence. Observed404 fences cached proof; unobserved deletion is outside
 the contract. A recorded answer says what held when it was made.
***************************************************************************)
CONSTANTS Canonical, Extended, SharedDigest, ScopeA, ScopeB,
          PromoteCap, QueueCap, ReadableScopes, Checksum
VARIABLES ready, enabled, scope, present, gone, tail, durable, verified,
          queue, answer
vars == <<ready, enabled, scope, present, gone, tail, durable, verified,
          queue, answer>>

Keys == {Canonical, Extended}
Scopes == {ScopeA, ScopeB}
DigestOf == [k \in Keys |-> SharedDigest]
IndexKey(s) == IF s = ScopeA THEN Canonical ELSE Extended
Bound == Checksum /\ scope \in ReadableScopes

Record(s, k, current) == [scope |-> s, key |-> k, current |-> current]
Observation(k, ok) == [scope |-> scope, key |-> k, yes |-> ok, wasGone |-> gone]

Init == /\ ready = FALSE /\ enabled = TRUE /\ scope = ScopeA
        /\ present = Scopes /\ gone = FALSE /\ tail = {}
        /\ durable \in {{}, {Record(ScopeB, Extended, FALSE)}}
        /\ verified = {} /\ queue = {}
        /\ answer = Observation(Canonical, FALSE)

Qualified(r, s) == r.current /\ r.scope = s

Recover(s) == /\ ~ready /\ ready' = TRUE /\ scope' = s
              /\ durable' = {r \in durable : Qualified(r, s)}
              /\ UNCHANGED <<enabled, present, gone, tail, verified, queue, answer>>

Toggle == /\ enabled' = ~enabled
          /\ UNCHANGED <<ready, scope, present, gone, tail, durable, verified, queue, answer>>

Delete == /\ ready /\ scope \in present /\ present' = present \ {scope}
          /\ UNCHANGED <<ready, enabled, scope, gone, tail, durable, verified, queue, answer>>

Observed404 == /\ ready /\ enabled /\ scope \notin present /\ ~gone /\ gone' = TRUE
               /\ UNCHANGED <<ready, enabled, scope, present, tail, durable, verified, queue, answer>>

Fill == /\ ready /\ enabled /\ scope \in present /\ ~gone /\ tail' = {IndexKey(scope)}
        /\ UNCHANGED <<ready, enabled, scope, present, gone, durable, verified, queue, answer>>

Evict == /\ tail # {} /\ tail' = {}
         /\ UNCHANGED <<ready, enabled, scope, present, gone, durable, verified, queue, answer>>

Hints(k, records) == IF k = Canonical
                     THEN {r \in records : DigestOf[r.key] = DigestOf[k]}
                     ELSE {}

CanRead == ready /\ enabled /\ scope \in present /\ ~gone /\ scope \in ReadableScopes

ReadIndexed(k) == /\ CanRead /\ k = IndexKey(scope)
                  /\ queue' = IF k = Canonical /\ Hints(k, durable) = {} /\ Cardinality(queue) < QueueCap
                               THEN queue \cup {Record(scope, k, TRUE)} ELSE queue
                  /\ answer' = Observation(k, TRUE)
                  /\ UNCHANGED <<ready, enabled, scope, present, gone, tail, durable, verified>>

(* A hint's ordinal is interpreted in the mounted scope, not its old scope. *)
ReadPromoted(k, r) == /\ CanRead /\ r \in Hints(k, durable)
                      /\ DigestOf[IndexKey(scope)] = DigestOf[k]
                      /\ verified' = verified \cup {r}
                      /\ answer' = Observation(k, TRUE)
                      /\ UNCHANGED <<ready, enabled, scope, present, gone, tail, durable, queue>>

ReadMiss(k) == /\ ready /\ enabled /\ answer' = Observation(k, FALSE)
               /\ UNCHANGED <<ready, enabled, scope, present, gone, tail, durable, verified, queue>>

SyncLog(r) == /\ r \in queue /\ Cardinality(durable) < PromoteCap
              /\ durable' = durable \cup {r} /\ verified' = verified \cup {r}
              /\ queue' = queue \ {r}
              /\ UNCHANGED <<ready, enabled, scope, present, gone, tail, answer>>

Crash == /\ ready' = FALSE /\ gone' = FALSE /\ tail' = {}
         /\ verified' = {} /\ queue' = {}
         /\ UNCHANGED <<enabled, scope, present, durable, answer>>

Exists(k) == /\ answer' = Observation(k, ready /\ enabled /\ ~gone /\
                                      (Hints(k, verified) # {} \/ (Bound /\ k \in tail)))
             /\ UNCHANGED <<ready, enabled, scope, present, gone, tail, durable, verified, queue>>

Next == Toggle \/ Delete \/ Observed404 \/ Fill \/ Evict \/ Crash
        \/ (\E s \in Scopes : Recover(s))
        \/ (\E r \in queue : SyncLog(r))
        \/ (\E k \in Keys : ReadIndexed(k) \/ ReadMiss(k) \/ Exists(k)
                          \/ (\E r \in durable : ReadPromoted(k, r)))

Spec == Init /\ [][Next]_vars

NoFalseYes == answer.yes => answer.key = IndexKey(answer.scope)
NoUnreadableYes == answer.yes /\ answer.key = IndexKey(answer.scope) => answer.scope \in ReadableScopes
NoGoneYes == answer.yes => ~answer.wasGone
VerifiedIsDurable == verified \subseteq durable
LogAndQueueBounded == Cardinality(durable) <= PromoteCap /\ Cardinality(queue) <= QueueCap
=============================================================================
