------------------------------ MODULE ReadTurns ------------------------------
\* A store's snapshot reads take a turn for each call into SQLite and leave it
\* before anything else waits: the next call, a pooled connection, or the
\* caller's code between statements, such as a scan's callback, which may read
\* in a transaction of its own. So a reader waits for a turn holding none, and
\* every turn ends. Writers and the checkpointer take no turn. A reader's call
\* waits inside SQLite only for a writer, a checkpoint or recovery, which take
\* no turn, so they are left out; readers' shared locks do not block another
\* reader's call. The scan's callback reads once; HoldAcrossCallback is the
\* defect of keeping the row's turn through the callback.
EXTENDS Naturals, Sequences, FiniteSets
CONSTANTS Turns, HoldAcrossCallback
VARIABLES pos, held
vars == <<pos, held>>
\* Each program is its turns in order: "in" enters one, "out" leaves one.
Scan == IF HoldAcrossCallback
        THEN <<"in", "in", "out", "out", "in", "out">>
        ELSE <<"in", "out", "in", "out", "in", "out">>
Get == <<"in", "out", "in", "out">>
Prog == [scan |-> Scan, get |-> Get, get2 |-> Get]
Readers == DOMAIN Prog
Sum(f) == LET S[R \in SUBSET Readers] ==
               IF R = {} THEN 0 ELSE LET r == CHOOSE x \in R : TRUE IN f[r] + S[R \ {r}]
          IN S[Readers]
Init == pos = [r \in Readers |-> 1] /\ held = [r \in Readers |-> 0]
Step(r) == /\ pos[r] <= Len(Prog[r])
           /\ IF Prog[r][pos[r]] = "in"
              THEN Sum(held) < Turns /\ held' = [held EXCEPT ![r] = @ + 1]
              ELSE held' = [held EXCEPT ![r] = @ - 1]
           /\ pos' = [pos EXCEPT ![r] = @ + 1]
Done == \A r \in Readers : pos[r] > Len(Prog[r])
Next == (\E r \in Readers : Step(r)) \/ (Done /\ UNCHANGED vars)
Spec == Init /\ [][Next]_vars /\ WF_vars(\E r \in Readers : Step(r))
Bounded == Sum(held) <= Turns
Finishes == <>Done
=============================================================================
