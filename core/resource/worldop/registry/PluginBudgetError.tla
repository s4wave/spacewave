---- MODULE PluginBudgetError ----
\* bridgeOperation.callErr receives errors from a nested handler through RPC,
\* which loses Go error types. Outer and nested timers share a WriteBudget
\* deadline but may publish cancellation in either order. Expiry is a fact
\* about that budget, independent of which timer goroutine runs first.
\* ApplyWorldOp, ApplyWorldObjectOp and validation use this classifier.
\* One call and one nested error suffice; storage, retries and plugin loading
\* do not change this ordering and are outside this model.
EXTENDS Naturals
VARIABLES spent, nested, outer, reply, result
vars == <<spent, nested, outer, reply, result>>
Init == /\ spent = FALSE /\ nested = FALSE /\ outer = FALSE
        /\ reply = FALSE /\ result = "pending"
Deadline == /\ ~spent /\ spent' = TRUE
            /\ UNCHANGED <<nested, outer, reply, result>>
NestedTimer == /\ spent /\ ~nested /\ nested' = TRUE
               /\ UNCHANGED <<spent, outer, reply, result>>
OuterTimer == /\ spent /\ ~outer /\ outer' = TRUE
              /\ UNCHANGED <<spent, nested, reply, result>>
RpcError == /\ nested /\ ~reply /\ reply' = TRUE
            /\ UNCHANGED <<spent, nested, outer, result>>
CallErr == /\ reply /\ result = "pending"
           /\ result' = IF outer \/ spent THEN "timeout" ELSE "unavailable"
           /\ UNCHANGED <<spent, nested, outer, reply>>
Done == /\ result # "pending" /\ UNCHANGED vars
Next == Deadline \/ NestedTimer \/ OuterTimer \/ RpcError \/ CallErr \/ Done
Spec == Init /\ [][Next]_vars
TypeOK == /\ spent \in BOOLEAN /\ nested \in BOOLEAN /\ outer \in BOOLEAN
          /\ reply \in BOOLEAN /\ result \in {"pending", "timeout", "unavailable"}
SharedDeadlineIsTimeout == result # "unavailable"
====
