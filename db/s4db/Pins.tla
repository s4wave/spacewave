---- MODULE Pins ----
\* Reader slots, as the design's db.go runs them: a process that frees chunks
\* makes them free only below every other process's slot pin, and a reading
\* process keeps its pin at or below every state its snapshots read.
\*
\* The writer commits; each commit frees the value it replaced (a value chunk
\* entry) at that commit, and the entry is pending until released. release
\* reads the slots (minPin), then frees the pending entries at or below the
\* lowest pin; another process acts between the two. The reader opens the
\* file: it locks a slot still holding a pin a past owner left, writes the
\* zero pin, loads the newest commit, and writes its local pin. Its snapshots
\* acquire the published state, count it, and check that it is still
\* published (acquire). Its tail publishes newer commits and retires the
\* replaced state. updatePin, which publish and release call one at a time
\* under pinMtx, computes the local pin (the published state and every
\* retired state still counted), then writes it.
\*
\* Callers found from source (master db/s4db at aa2254798, carried over):
\* NewTransaction(read) and Tx.Discard (Take, Count, Check, Done); load
\* (Zero, Load, PinLoaded); tailLoop and publish (Publish); updatePin
\* (PinCompute, PinWrite); the writer's commit and release (Commit,
\* ReadSlots, Release). The design's run and manifest chunks are freed at
\* manifest versions by the same rule with the pin's version field, so the
\* model's single pin stands for both fields. Not modeled: durability
\* (Durability.tla) and the table's partition (Free.tla).

EXTENDS Naturals, FiniteSets

MaxSeq == 3
Readers == {"r1", "r2"}
Stale == 9

VARIABLES
  aseq,      \* the writer's last commit
  pend,      \* commits whose freed entry is pending
  released,  \* commits whose freed entry the writer released
  seen,      \* the bound release computed: its state and the pin it read
  slot,      \* the reader process's slot pin
  bpc,       \* its open: "locked", "zeroed", "loaded", "open"
  cur,       \* its published state
  retired,   \* states it replaced
  refs,      \* how many snapshots count each state
  upc,       \* updatePin: "idle", or "write" once it computed upin
  upin,
  rpc,       \* each snapshot's step: "idle", "count", "check", "read"
  rs         \* the state each snapshot took
vars == <<aseq, pend, released, seen, slot, bpc, cur, retired, refs, upc, upin,
          rpc, rs>>

Seqs == 0..MaxSeq
Min(S) == CHOOSE x \in S : \A y \in S : x <= y

Init ==
  /\ aseq = 1 /\ pend = {1} /\ released = {} /\ seen = 0
  /\ slot = Stale /\ bpc = "locked"
  /\ cur = 0 /\ retired = {} /\ refs = [s \in Seqs |-> 0]
  /\ upc = "idle" /\ upin = 0
  /\ rpc = [r \in Readers |-> "idle"] /\ rs = [r \in Readers |-> 0]

\* The writer's commit frees the entry the last one wrote.
Commit ==
  /\ aseq < MaxSeq
  /\ seen' = 0
  /\ aseq' = aseq + 1
  /\ pend' = pend \cup {aseq + 1}
  /\ UNCHANGED <<released, slot, bpc, cur, retired, refs, upc, upin, rpc, rs>>

\* release bounds the release by the writer's published state and the slot
\* minPin reads, under the writer lock, so no commit runs until it releases.
ReadSlots ==
  /\ seen' = Min({slot, aseq})
  /\ UNCHANGED <<aseq, pend, released, slot, bpc, cur, retired, refs, upc, upin, rpc, rs>>

Release ==
  /\ \E e \in pend :
       /\ e <= seen
       /\ pend' = pend \ {e}
       /\ released' = released \cup {e}
  /\ UNCHANGED <<aseq, seen, slot, bpc, cur, retired, refs, upc, upin, rpc, rs>>

\* load: write the zero pin, then load the newest commit, then pin it.
Zero ==
  /\ bpc = "locked"
  /\ slot' = 0 /\ bpc' = "zeroed"
  /\ UNCHANGED <<aseq, pend, released, seen, cur, retired, refs, upc, upin, rpc, rs>>

Load ==
  /\ bpc = "zeroed"
  /\ cur' = aseq /\ bpc' = "loaded"
  /\ UNCHANGED <<aseq, pend, released, seen, slot, retired, refs, upc, upin, rpc, rs>>

PinLoaded ==
  /\ bpc = "loaded"
  /\ slot' = cur /\ bpc' = "open"
  /\ UNCHANGED <<aseq, pend, released, seen, cur, retired, refs, upc, upin, rpc, rs>>

\* The tail publishes the newest commit and retires the state it replaced.
Publish ==
  /\ bpc = "open" /\ cur < aseq
  /\ cur' = aseq
  /\ retired' = retired \cup {cur}
  /\ UNCHANGED <<aseq, pend, released, seen, slot, bpc, refs, upc, upin, rpc, rs>>

\* localPin: the published state and each retired state still counted; the
\* rest are forgotten.
LocalPin == Min({cur} \cup {s \in retired : refs[s] > 0})

PinCompute ==
  /\ bpc = "open" /\ upc = "idle"
  /\ upin' = LocalPin
  /\ retired' = {s \in retired : refs[s] > 0}
  /\ upc' = "write"
  /\ UNCHANGED <<aseq, pend, released, seen, slot, bpc, cur, refs, rpc, rs>>

PinWrite ==
  /\ upc = "write"
  /\ slot' = upin /\ upc' = "idle"
  /\ UNCHANGED <<aseq, pend, released, seen, bpc, cur, retired, refs, upin, rpc, rs>>

\* acquire: load the published state, count it, then check it is still
\* published, uncounting and retrying when it is not.
Take(r) ==
  /\ bpc = "open" /\ rpc[r] = "idle"
  /\ rs' = [rs EXCEPT ![r] = cur] /\ rpc' = [rpc EXCEPT ![r] = "count"]
  /\ UNCHANGED <<aseq, pend, released, seen, slot, bpc, cur, retired, refs, upc, upin>>

Count(r) ==
  /\ rpc[r] = "count"
  /\ refs' = [refs EXCEPT ![rs[r]] = @ + 1] /\ rpc' = [rpc EXCEPT ![r] = "check"]
  /\ UNCHANGED <<aseq, pend, released, seen, slot, bpc, cur, retired, upc, upin, rs>>

Check(r) ==
  /\ rpc[r] = "check"
  /\ IF cur = rs[r]
     THEN rpc' = [rpc EXCEPT ![r] = "read"] /\ UNCHANGED refs
     ELSE /\ refs' = [refs EXCEPT ![rs[r]] = @ - 1]
          /\ rpc' = [rpc EXCEPT ![r] = "idle"]
  /\ UNCHANGED <<aseq, pend, released, seen, slot, bpc, cur, retired, upc, upin, rs>>

\* release: the snapshot ends.
Done(r) ==
  /\ rpc[r] = "read"
  /\ refs' = [refs EXCEPT ![rs[r]] = @ - 1] /\ rpc' = [rpc EXCEPT ![r] = "idle"]
  /\ UNCHANGED <<aseq, pend, released, seen, slot, bpc, cur, retired, upc, upin, rs>>

Next ==
  \/ Commit \/ ReadSlots \/ Release \/ Zero \/ Load \/ PinLoaded \/ Publish
  \/ PinCompute \/ PinWrite
  \/ \E r \in Readers : Take(r) \/ Count(r) \/ Check(r) \/ Done(r)
  \/ UNCHANGED vars

\* Every step that moves a pending entry toward free runs eventually, and a
\* snapshot that reads eventually ends.
Fairness ==
  /\ WF_vars(ReadSlots) /\ WF_vars(Release) /\ WF_vars(Publish)
  /\ WF_vars(PinCompute) /\ WF_vars(PinWrite)
  /\ WF_vars(Zero) /\ WF_vars(Load) /\ WF_vars(PinLoaded)
  /\ \A r \in Readers : WF_vars(Done(r)) /\ WF_vars(Check(r)) /\ WF_vars(Count(r))
Spec == Init /\ [][Next]_vars /\ Fairness

\* A snapshot of state s reads the entries commits after s freed, so none of
\* them is released while it reads.
NoReleasedRead == \A r \in Readers : rpc[r] = "read" => \A e \in released : e <= rs[r]
\* The reader process's published state never falls below a released free.
LoadedAboveReleased == bpc \in {"loaded", "open"} => \A e \in released : e <= cur
\* Every pending entry is eventually released: the pending partition drains.
PendingDrains == \A e \in Seqs : e \in pend ~> e \in released
====
