---- MODULE LegacySettle ----
\* The legacy lane's settled write in sostate.go newLock: a checkpoint, or any
\* write that builds on the durable state, waits for the receipts pending
\* when it starts with no writer held, then takes the provider writer and
\* waits there for the admissions that arrived in between. World admissions
\* take the provider writer, so whatever the checkpoint waits for under it
\* holds the engine writer of every World transaction behind it.
\* Abstraction: an admission is one step under the provider writer; receipts
\* settle in admission order; q holds the pending receipts, TRUE for one that
\* was pending when the checkpoint began. Bounds: two pending receipts.
\* LegacySettle.mutants.json lists the guards each property depends on.
EXTENDS Naturals, Sequences
CONSTANTS PreWait
VARIABLES q, mtx, k
vars == <<q, mtx, k>>
Init == q = <<>> /\ mtx = "none" /\ k = "idle"
\* A World transaction admits a publication under the provider writer.
Admit == /\ mtx = "none" /\ Len(q) < 2
         /\ q' = Append(q, FALSE)
         /\ UNCHANGED <<mtx, k>>
Settle == /\ Len(q) > 0 /\ q' = Tail(q)
          /\ UNCHANGED <<mtx, k>>
Old == {i \in 1..Len(q) : q[i]}
Begin == /\ k = "idle"
         /\ q' = [i \in 1..Len(q) |-> TRUE]
         /\ k' = IF PreWait THEN "pre" ELSE "lock"
         /\ UNCHANGED mtx
\* waitPendingNow: wait for the receipts pending at Begin, no writer held.
Pre == /\ k = "pre" /\ Old = {}
       /\ k' = "lock" /\ UNCHANGED <<q, mtx>>
Lock == /\ k = "lock" /\ mtx = "none"
        /\ mtx' = "K" /\ k' = "settle" /\ UNCHANGED q
\* waitSettled under the provider writer.
Settled == /\ k = "settle" /\ q = <<>>
           /\ k' = "write" /\ UNCHANGED <<q, mtx>>
Write == /\ k = "write"
         /\ mtx' = "none" /\ k' = "idle" /\ UNCHANGED q
Next == Admit \/ Settle \/ Begin \/ Pre \/ Lock \/ Settled \/ Write
Spec == Init /\ [][Next]_vars
FairSpec == Spec /\ WF_vars(Settle) /\ WF_vars(Pre) /\ SF_vars(Lock) /\ WF_vars(Settled) /\ WF_vars(Write)
\* The provider writer never waits on a receipt pending when the write began.
NoOldWaitUnderWriter == k = "settle" => Old = {}
\* The write builds on the durable state.
BuildsOnDurable == k = "write" => q = <<>>
Bounded == Len(q) <= 2
\* A checkpoint that begins completes even while admissions continue.
Finishes == (k = "pre") ~> (k = "write")
====
