---- MODULE PackAheadOwnership ----
(* Two openers contend for one directory. Exit covers close, failed recovery,
   and process death. The lock guards recovery as well as writing. *)
EXTENDS FiniteSets, Naturals
CONSTANT Broken
VARIABLES owner, phase

Processes == {1, 2}
Active == {p \in Processes : phase[p] # "closed"}

Init == /\ owner = 0
        /\ phase = [p \in Processes |-> "closed"]

Open(p) == /\ phase[p] = "closed"
           /\ Broken \/ owner = 0
           /\ owner' = IF Broken THEN 0 ELSE p
           /\ phase' = [phase EXCEPT ![p] = "recovering"]

StartWriting(p) == /\ phase[p] = "recovering"
                   /\ phase' = [phase EXCEPT ![p] = "writing"]
                   /\ UNCHANGED owner

Exit(p) == /\ p \in Active
           /\ phase' = [phase EXCEPT ![p] = "closed"]
           /\ owner' = 0

Next == \E p \in Processes : Open(p) \/ StartWriting(p) \/ Exit(p)
Spec == Init /\ [][Next]_<<owner, phase>>
Exclusive == Cardinality(Active) <= 1
====
