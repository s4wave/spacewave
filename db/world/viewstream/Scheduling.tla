---- MODULE Scheduling ----
\* Admission grants slots in priority order, a lower caller first, and its
\* slot count changes while callers hold and wait. Raising it grants waiters at
\* once; lowering it retires held slots as they return. Cancellation removes a
\* waiter. Pacing waits on actor writes and explicit pokes; unrelated writes do
\* not pass.
EXTENDS Naturals, FiniteSets
Callers == {1,2,3}
MaxSlots == 2
VARIABLES holders, waiting, slots, revision, own, poked, passed
vars == <<holders,waiting,slots,revision,own,poked,passed>>
pace == <<revision,own,poked,passed>>
\* Lowest(S, k) is the k lowest callers of S, the next ones admitted.
Lowest(S, k) == CHOOSE L \in SUBSET S:
                  /\ Cardinality(L) = k
                  /\ \A l \in L, o \in S \ L: l < o
Min(a, b) == IF a < b THEN a ELSE b
Init == /\ holders = {} /\ waiting = {} /\ slots \in 1..MaxSlots
        /\ revision = 0 /\ own = 0 /\ poked = FALSE /\ passed = FALSE
\* Acquire takes a free slot or waits in line.
Enqueue(c) == /\ c \notin waiting \cup holders
              /\ IF Cardinality(holders) < slots
                 THEN holders' = holders \cup {c} /\ waiting' = waiting
                 ELSE waiting' = waiting \cup {c} /\ holders' = holders
              /\ UNCHANGED <<slots>> /\ UNCHANGED pace
Cancel(c) == /\ c \in waiting /\ waiting' = waiting \ {c}
             /\ UNCHANGED <<holders,slots>> /\ UNCHANGED pace
\* A returned slot passes to the first waiter unless it is past the count.
Release(c) == /\ c \in holders
              /\ LET next == Lowest(waiting, Min(1, Cardinality(waiting)))
                     pass == IF Cardinality(holders) <= slots THEN next ELSE {}
                 IN /\ holders' = (holders \ {c}) \cup pass
                    /\ waiting' = waiting \ pass
              /\ UNCHANGED <<slots>> /\ UNCHANGED pace
\* SetSlots grants the slots it adds to the first waiters at once.
Resize(n) == /\ n # slots /\ slots' = n
             /\ LET free == IF n > Cardinality(holders) THEN n - Cardinality(holders) ELSE 0
                    next == Lowest(waiting, Min(free, Cardinality(waiting)))
                IN /\ holders' = holders \cup next
                   /\ waiting' = waiting \ next
             /\ UNCHANGED pace
OwnWrite == /\ own = 0 /\ own' = 1
            /\ UNCHANGED <<holders,waiting,slots,revision,poked,passed>>
Poke == /\ ~poked /\ poked' = TRUE
        /\ UNCHANGED <<holders,waiting,slots,revision,own,passed>>
Pass == /\ ~passed /\ (own > revision \/ poked) /\ passed' = TRUE
        /\ UNCHANGED <<holders,waiting,slots,revision,own,poked>>
Next == \/ \E c \in Callers: Enqueue(c) \/ Cancel(c) \/ Release(c)
        \/ \E n \in 1..MaxSlots: Resize(n)
        \/ OwnWrite \/ Poke \/ Pass
Spec == Init /\ [][Next]_vars
TypeOK == holders \subseteq Callers /\ waiting \subseteq Callers
          /\ holders \cap waiting = {} /\ slots \in 1..MaxSlots
\* No caller waits while a slot is free.
NoIdleSlot == waiting # {} => Cardinality(holders) >= slots
\* Never more holders than the most slots ever set.
Bounded == Cardinality(holders) <= MaxSlots
ExplicitWake == passed => (own > revision \/ poked)
\* A grant never takes the holders past the count, and goes to callers ahead
\* of every one left waiting.
GrantWithinSlots == [][\A c \in holders' \ holders:
                        /\ Cardinality(holders') <= slots'
                        /\ \A w \in waiting': c < w]_vars
=============================================================================
