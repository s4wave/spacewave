------------------------- MODULE RequirementPlacement -------------------------
EXTENDS Naturals, FiniteSets, Sequences
\* claimRequirements observes slots before claimNode's transactional recheck.
\* ReserveInTx and ReleaseInTx change the same ledger as the placement edges.
\* UpdateFlowgraph changes the replica count; claimNode trims newest copies.
\* Lease expiry frees a slot, replacement moves its debit atomically, and the
\* placement watch stops a resumed holder. Labels and runtime launch are omitted.
CONSTANT Devices
VARIABLES copies, debit, reads, expired, count, running
vars == <<copies, debit, reads, expired, count, running>>
Holders == {copies[i] : i \in 1..Len(copies)}
Live == Holders \ expired
Init == /\ copies = <<>> /\ debit = {} /\ expired = {}
        /\ reads = [d \in Devices |-> FALSE] /\ count = 2 /\ running = {}
Read(d) == /\ ~reads[d] /\ d \notin Holders /\ Cardinality(Live) < count
           /\ reads' = [reads EXCEPT ![d] = TRUE]
           /\ UNCHANGED <<copies, debit, expired, count, running>>
Commit(d) == /\ reads[d] /\ d \notin Holders
             /\ Cardinality(Live) < count
             /\ Len(copies) <= count
             /\ \E stale \in (Holders \cap expired) \cup {"none"}:
                 /\ (stale = "none") <=> (Holders \cap expired = {})
                 /\ copies' = Append(SelectSeq(copies, LAMBDA x: x # stale), d)
                 /\ debit' = (debit \ {stale}) \cup {d}
             /\ reads' = [reads EXCEPT ![d] = FALSE]
             /\ UNCHANGED <<expired, count, running>>
Retry(d) == /\ reads[d] /\ reads' = [reads EXCEPT ![d] = FALSE]
            /\ UNCHANGED <<copies, debit, expired, count, running>>
Lower == /\ count = 2 /\ count' = 1
         /\ UNCHANGED <<copies, debit, reads, expired, running>>
Trim == /\ Len(copies) > count
        /\ copies' = SubSeq(copies, 1, count)
        /\ debit' = {copies[i] : i \in 1..count}
        /\ UNCHANGED <<reads, expired, count, running>>
Expire(d) == /\ d \in Holders /\ expired' = expired \cup {d}
             /\ UNCHANGED <<copies, debit, reads, count, running>>
Renew(d) == /\ d \in expired /\ expired' = expired \ {d}
            /\ UNCHANGED <<copies, debit, reads, count, running>>
Watch(d) == /\ running' = IF d \in Holders THEN running \cup {d} ELSE running \ {d}
            /\ UNCHANGED <<copies, debit, reads, expired, count>>
Next == (\E d \in Devices: Read(d) \/ Commit(d) \/ Retry(d) \/ Expire(d) \/ Renew(d) \/ Watch(d)) \/ Lower \/ Trim \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars /\ \A d \in Devices: WF_vars(Watch(d))
\* Count reductions may await trimming, but claims never add above that count.
CountBound == Len(copies) <= 2
OneCopyPerDevice == Cardinality(Holders) = Len(copies)
EdgeAndDebit == debit = Holders
\* A copy that stays unplaced eventually stays stopped, even if Renew runs.
StopsDeparted == \A d \in Devices:
    (<>[](d \notin Holders)) => (<>[](d \notin running))
\* Removing Commit's count recheck violates CountBound with three Devices:
\* Init, Read(a), Commit(a), Read(b), Read(c), Commit(b), Commit(c).
\* Keeping a departed copy in running instead of removing it in Watch violates
\* StopsDeparted. Lease renewal alone cannot restore a removed placement.
============================================================
