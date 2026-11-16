---- MODULE ServiceAdvertising ----
\* Two independently scheduled LookupRpcService resolvers race to publish a
\* value. The actual Resource root can serve the requested service; a generic
\* InvokerController wraps a mux with an unrelated service. The caller takes
\* the first value, so it must never receive an unimplemented candidate.
\* Prefix translation and opaque forwarding are covered by the Go namespace
\* tests; this model bounds one nonempty service request and two known muxes.
CONSTANT CheckService
VARIABLES resolved, advertised, chosen
vars == <<resolved, advertised, chosen>>
Init == /\ resolved = {} /\ advertised = {} /\ chosen = "none"
ResolveRight == /\ "right" \notin resolved
                /\ resolved' = resolved \cup {"right"}
                /\ advertised' = advertised \cup {"right"}
                /\ UNCHANGED chosen
ResolveWrong == /\ "wrong" \notin resolved
                /\ resolved' = resolved \cup {"wrong"}
                /\ advertised' = IF CheckService THEN advertised ELSE advertised \cup {"wrong"}
                /\ UNCHANGED chosen
Select(v) == /\ chosen = "none" /\ v \in advertised
             /\ chosen' = v /\ UNCHANGED <<resolved, advertised>>
Done == /\ chosen # "none" /\ resolved = {"right", "wrong"}
        /\ UNCHANGED vars
Next == ResolveRight \/ ResolveWrong \/ (\E v \in advertised: Select(v)) \/ Done
Spec == Init /\ [][Next]_vars
TypeOK == /\ resolved \subseteq {"right", "wrong"}
          /\ advertised \subseteq resolved
          /\ chosen \in {"none", "right", "wrong"}
ChosenServiceExists == chosen # "wrong"
====
