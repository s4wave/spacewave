---- MODULE Memtable ----
\* state.derive from tail and writer copies one published memtable. BTree.Copy
\* changes its source COW epoch before shallow-copying its fields. Only copy
\* operations serialize; Get, Scan and read iterators never read that epoch.
\* Node immutability and snapshot pins are covered by Pins/Publication.
EXTENDS Naturals, FiniteSets
Actors == {1, 2}
VARIABLES owner, epoch, pc, observed
vars == <<owner, epoch, pc, observed>>
Init == /\ owner = 0 /\ epoch = 0
  /\ pc = [a \in Actors |-> "idle"] /\ observed = [a \in Actors |-> 0]
Begin(a) ==
  /\ pc[a] = "idle" /\ owner = 0
  /\ owner' = a /\ epoch' = a /\ pc' = [pc EXCEPT ![a] = "copying"]
  /\ UNCHANGED observed
Finish(a) ==
  /\ pc[a] = "copying"
  /\ observed' = [observed EXCEPT ![a] = epoch]
  /\ pc' = [pc EXCEPT ![a] = "done"] /\ owner' = 0
  /\ UNCHANGED epoch
Next == (\E a \in Actors : Begin(a) \/ Finish(a)) \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars
CopiesObserveOwnEpoch == \A a \in Actors : pc[a] = "done" => observed[a] = a
====
