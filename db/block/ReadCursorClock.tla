------------------------ MODULE ReadCursorClock ------------------------
EXTENDS Naturals, FiniteSets, Sequences
CONSTANT Cap
VARIABLES queue, freq, recent, visits, evicted
vars == <<queue, freq, recent, visits, evicted>>
\* Two main-queue candidates compete while a newly stored keep entry cannot
\* be evicted. This is evictOne's slow case; probation promotion resets its
\* frequency and cannot require more passes. Pointer readers may touch either
\* candidate without taking the shard lock between every writer step.
Victims == {1, 2}
MaxFreq == 3
\* len(entries) includes the keep entry; the source adds one before multiplying.
Limit == (Cardinality(Victims) + 2) * (MaxFreq + 1)
Init == /\ queue = <<1, 2>>
        /\ freq = [v \in Victims |-> MaxFreq]
        /\ recent = [v \in Victims |-> FALSE]
        /\ visits = 0 /\ evicted = FALSE
Touch(v) == /\ ~evicted
            /\ recent' = [recent EXCEPT ![v] = TRUE]
            /\ UNCHANGED <<queue, freq, visits, evicted>>
Visit == /\ ~evicted /\ visits < Limit + 1
         /\ LET v == Head(queue)
                f == IF recent[v] /\ freq[v] = 0 THEN 1 ELSE freq[v]
            IN /\ visits' = visits + 1
               /\ evicted' = ((f = 0) \/ (Cap /\ visits + 1 = Limit))
               /\ freq' = [freq EXCEPT ![v] = IF f > 0 THEN f - 1 ELSE 0]
               /\ recent' = [recent EXCEPT ![v] = FALSE]
               /\ queue' = Append(Tail(queue), v)
Next == (\E v \in Victims : Touch(v)) \/ Visit \/ UNCHANGED vars
BoundedPasses == visits <= Limit
Terminates == <>evicted
Spec == Init /\ [][Next]_vars /\ WF_vars(Visit)
========================================================================
