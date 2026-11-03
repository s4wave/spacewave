---- MODULE Scheduling ----
\* Admission grants one slot, in priority order. Cancellation removes a waiter.
\* Pacing waits on actor writes and explicit pokes; unrelated writes do not pass.
EXTENDS Naturals, FiniteSets, Sequences
Callers == {1,2,3}
VARIABLES holder, waiting, granted, revision, own, poked, passed
vars == <<holder,waiting,granted,revision,own,poked,passed>>
Init == /\ holder = 0 /\ waiting = {} /\ granted = {}
        /\ revision = 0 /\ own = 0 /\ poked = FALSE /\ passed = FALSE
Enqueue(c) == /\ c \notin waiting /\ c # holder
              /\ waiting' = waiting \cup {c}
              /\ UNCHANGED <<holder,granted,revision,own,poked,passed>>
Grant == /\ holder = 0 /\ waiting # {}
         /\ holder' = CHOOSE c \in waiting: \A other \in waiting: c <= other
         /\ waiting' = waiting \ {holder'} /\ granted' = {holder'}
         /\ UNCHANGED <<revision,own,poked,passed>>
Cancel(c) == /\ c \in waiting /\ waiting' = waiting \ {c}
             /\ UNCHANGED <<holder,granted,revision,own,poked,passed>>
Release == /\ holder # 0 /\ holder' = 0 /\ granted' = {}
           /\ UNCHANGED <<waiting,revision,own,poked,passed>>
OwnWrite == /\ own = 0 /\ own' = 1
            /\ UNCHANGED <<holder,waiting,granted,revision,poked,passed>>
Poke == /\ ~poked /\ poked' = TRUE
        /\ UNCHANGED <<holder,waiting,granted,revision,own,passed>>
Pass == /\ ~passed /\ (own > revision \/ poked) /\ passed' = TRUE
        /\ UNCHANGED <<holder,waiting,granted,revision,own,poked>>
Next == (\E c \in Callers: Enqueue(c) \/ Cancel(c)) \/ Grant \/ Release \/ OwnWrite \/ Poke \/ Pass
Spec == Init /\ [][Next]_vars
OneSlot == Cardinality(granted) <= 1
OneOwner == holder = 0 <=> granted = {}
ExplicitWake == passed => (own > revision \/ poked)
=============================================================================
