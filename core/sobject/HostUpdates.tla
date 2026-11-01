---- MODULE HostUpdates ----
EXTENDS Naturals, FiniteSets
(* host.go UpdateSOState: provider lock acquisition, clone/mutate, write,
   deferred Release. Two concurrent update callers append distinct operations.
   Callers: control-driver proposal/finalization, control-request submission
   and World checkpoint publication use this same update boundary.
   ImportPeerSnapshot/MergePeerBranch have additional validation outside this
   model. Crypto, merge algorithms and provider receipts are abstracted;
   provider publication is atomic and failure leaves the live state unchanged.
   Each caller runs once; cancellation is an unsuccessful operation. *)
CONSTANT HoldThroughWrite
VARIABLES live, owner, phase, draft, committed
vars == <<live, owner, phase, draft, committed>>
Actors == {1, 2}
Init == /\ live = {} /\ owner = 0
        /\ phase = [a \in Actors |-> "ready"]
        /\ draft = [a \in Actors |-> {}] /\ committed = {}
Lock(a) == /\ phase[a] = "ready" /\ owner = 0
           /\ owner' = a /\ phase' = [phase EXCEPT ![a] = "locked"]
           /\ UNCHANGED <<live, draft, committed>>
CloneMutate(a) ==
  /\ phase[a] = "locked" /\ owner = a
  /\ draft' = [draft EXCEPT ![a] = live \cup {a}]
  /\ phase' = [phase EXCEPT ![a] = "write"]
  /\ owner' = IF HoldThroughWrite THEN a ELSE 0
  /\ UNCHANGED <<live, committed>>
WriteSOState(a) ==
  /\ phase[a] = "write"
  /\ live' = draft[a] /\ committed' = committed \cup {a}
  /\ phase' = [phase EXCEPT ![a] = "release"]
  /\ UNCHANGED <<owner, draft>>
Fail(a) == /\ phase[a] \in {"ready", "locked", "write"}
           /\ phase' = [phase EXCEPT ![a] = "release"]
           /\ UNCHANGED <<live, owner, draft, committed>>
Release(a) ==
  /\ phase[a] = "release"
  /\ owner' = IF owner = a THEN 0 ELSE owner
  /\ phase' = [phase EXCEPT ![a] = "done"]
  /\ UNCHANGED <<live, draft, committed>>
Step(a) == Lock(a) \/ CloneMutate(a) \/ WriteSOState(a) \/ Fail(a) \/ Release(a)
Next == (\E a \in Actors: Step(a)) \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars /\ \A a \in Actors: WF_vars(Step(a))
KeepsSuccessfulUpdates == committed \subseteq live
OnlyWrittenOperations == live \subseteq committed
WritersSerialized == Cardinality({a \in Actors: phase[a] \in {"locked", "write"}}) <= 1
Completes == <> (\A a \in Actors: phase[a] = "done")
====
