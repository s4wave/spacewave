---- MODULE DeadlistLedgerState ----
\* Record-state projection of DeadlistLedger, not its save-generation proof.
\* The chart compiler has a 500-state budget; the detailed model has millions.
\* Pending/due name durability and recount responsibility stay in that model.
VARIABLE status
vars == <<status>>
Init == status = "absent"
Name == status = "absent" /\ status' \in {"waiting", "forfeited"}
Resolve == status = "waiting" /\ status' = "pending"
Advance == (status = "pending" /\ status' = "due")
           \/ (status = "forfeited" /\ status' = "done")
Orphan == status \in {"waiting", "pending"} /\ status' = "orphaned"
Confirm == status \in {"due", "orphaned"} /\ status' = "done"
RestoreDeadlists == status \in {"waiting", "pending", "due", "orphaned", "forfeited"}
                    /\ status' \in {"absent", "forfeited", "pending"}
\* Done ends a run: the record is confirmed and has nothing left to do.
Done == status = "done" /\ UNCHANGED vars
Next == Name \/ Resolve \/ Advance \/ Orphan \/ Confirm \/ RestoreDeadlists
        \/ Done
Spec == Init /\ [][Next]_vars
TypeOK == status \in {"absent", "waiting", "pending", "due", "orphaned", "done", "forfeited"}
====
