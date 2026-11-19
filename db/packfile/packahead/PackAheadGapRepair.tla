---- MODULE PackAheadGapRepair ----
\* Recover an open pack whose previously synced data suffered corruption.
\* Each batch is one index. Bad framing is independent of bad values. Live
\* recovery knows all original boundaries. Startup knows only the first and
\* can advance through verified original headers or durable skip records.
\* The model keeps only the crash cuts after live verification. A crash before
\* the repair sync can still lose the framing that existed only in memory.
EXTENDS Naturals, FiniteSets

CONSTANTS Batches, SyncSkips
VARIABLES headers, values, phase, skips, reopened

vars == <<headers, values, phase, skips, reopened>>

Starts == 1..Batches
Kept == headers \cap values

\* No skip is needed past the last kept batch. A header whose values failed
\* verification can already carry the scanner to its successor.
Gaps == {i \in Starts \ headers : \E j \in Kept : i < j}
Reachable == {i \in Kept : 1..(i-1) \subseteq (headers \cup skips)}

Init ==
  /\ headers \in SUBSET Starts
  /\ values \in SUBSET Starts
  /\ phase = "verified"
  /\ skips = {}
  /\ reopened = {}

Repair ==
  /\ phase = "verified"
  /\ phase' = "repaired"
  /\ skips' = IF SyncSkips THEN Gaps ELSE {}
  /\ UNCHANGED <<headers, values, reopened>>

Index ==
  /\ phase = "repaired"
  /\ phase' = "indexed"
  /\ UNCHANGED <<headers, values, skips, reopened>>

Rename ==
  /\ phase = "indexed"
  /\ phase' = "renamed"
  /\ UNCHANGED <<headers, values, skips, reopened>>

Crash ==
  /\ phase \in {"repaired", "indexed", "renamed"}
  \* After the rename, the disk can hold either name until the directory sync.
  /\ reopened' \in IF phase = "renamed" THEN {Reachable, Kept} ELSE {Reachable}
  /\ phase' = "reopened"
  /\ UNCHANGED <<headers, values, skips>>

Next == Repair \/ Index \/ Rename \/ Crash
Spec == Init /\ [][Next]_vars

\* Startup and the recovered index must hold the same batches.
SameBatches == phase = "reopened" => reopened = Kept
====
