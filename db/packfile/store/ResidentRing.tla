--------------------------- MODULE ResidentRing ---------------------------
\* A PackReader mutates its resident spans under its own lock and then the
\* budget lock. residentBudget.reclaim chooses a reader under budget.mtx,
\* releases it, then calls evictOldestSpan/dropIndex under the reader lock.
\* One reader and one sweep bound the lock-order model; CLOCK victim ranking
\* and byte accounting are exercised by the shared-budget read run-throughs.
EXTENDS Naturals
VARIABLES readerLock, budgetLock, readPC, sweepPC
vars == <<readerLock, budgetLock, readPC, sweepPC>>
Init == /\ readerLock = "none" /\ budgetLock = "none"
        /\ readPC = "start" /\ sweepPC = "start"
ReaderTake == /\ readPC = "start" /\ readerLock = "none"
              /\ readerLock' = "reader" /\ readPC' = "budget"
              /\ UNCHANGED <<budgetLock, sweepPC>>
ReaderBudget == /\ readPC = "budget" /\ budgetLock = "none"
                /\ budgetLock' = "reader" /\ readPC' = "release"
                /\ UNCHANGED <<readerLock, sweepPC>>
ReaderRelease == /\ readPC = "release" /\ readerLock' = "none"
                 /\ budgetLock' = "none" /\ readPC' = "done"
                 /\ UNCHANGED sweepPC
SweepChoose == /\ sweepPC = "start" /\ budgetLock = "none"
               /\ budgetLock' = "sweep" /\ sweepPC' = "chosen"
               /\ UNCHANGED <<readerLock, readPC>>
SweepUnlock == /\ sweepPC = "chosen" /\ budgetLock' = "none"
               /\ sweepPC' = "reader"
               /\ UNCHANGED <<readerLock, readPC>>
SweepReader == /\ sweepPC = "reader" /\ readerLock = "none"
               /\ readerLock' = "sweep" /\ sweepPC' = "budget"
               /\ UNCHANGED <<budgetLock, readPC>>
SweepBudget == /\ sweepPC = "budget" /\ budgetLock = "none"
               /\ budgetLock' = "sweep" /\ sweepPC' = "release"
               /\ UNCHANGED <<readerLock, readPC>>
SweepRelease == /\ sweepPC = "release" /\ readerLock' = "none"
                /\ budgetLock' = "none" /\ sweepPC' = "done"
                /\ UNCHANGED readPC
\* Done ends a run: the reader and the sweep both released their locks.
Done == readPC = "done" /\ sweepPC = "done" /\ UNCHANGED vars
Next == ReaderTake \/ ReaderBudget \/ ReaderRelease \/ SweepChoose
        \/ SweepUnlock \/ SweepReader \/ SweepBudget \/ SweepRelease
        \/ Done
Spec == Init /\ [][Next]_vars
NoInversion == sweepPC = "reader" => budgetLock # "sweep"
=============================================================================
