------------------------- MODULE RequirementPlacement -------------------------
EXTENDS Naturals, FiniteSets
\* Reconciler.claimNode rechecks placement and debits capacity in one World
\* transaction. UpdateFlowgraph removes the edge and credits in one transaction.
\* WorldRuntimeAdmission.ReserveInTx shares the ledger with Forge reservations.
\* Runtime launch, plugin loading and labels are outside this atomicity model.
CONSTANT Devices
VARIABLES owner, debit, reads
vars == <<owner, debit, reads>>
Init == /\ owner = "none" /\ debit = [d \in Devices |-> 0]
        /\ reads = [d \in Devices |-> "idle"]
Read(d) == /\ reads[d] = "idle" /\ owner = "none"
           /\ reads' = [reads EXCEPT ![d] = "unplaced"]
           /\ UNCHANGED <<owner, debit>>
Commit(d) == /\ reads[d] = "unplaced" /\ owner = "none"
             /\ owner' = d /\ debit' = [debit EXCEPT ![d] = 1]
             /\ reads' = [reads EXCEPT ![d] = "idle"]
Retry(d) == /\ reads[d] = "unplaced" /\ owner # "none"
            /\ reads' = [reads EXCEPT ![d] = "idle"]
            /\ UNCHANGED <<owner, debit>>
Remove == /\ owner # "none"
          /\ debit' = [debit EXCEPT ![owner] = 0] /\ owner' = "none"
          /\ UNCHANGED reads
Next == (\E d \in Devices: Read(d) \/ Commit(d) \/ Retry(d)) \/ Remove \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars
OneGrant == Cardinality({d \in Devices: debit[d] = 1}) <= 1
EdgeAndDebit == \A d \in Devices: (debit[d] = 1) <=> (owner = d)
=============================================================================
