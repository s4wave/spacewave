------------------------------ MODULE ChangeFeed ------------------------------
EXTENDS Integers, FiniteSets, Sequences
(* Publication identities abstract root pairs, each touching its own key.
   Two subscribers may start before the worker catches up, pause delivery,
   reconnect after ring eviction, or cancel. Missing summaries include restart
   and imports. Atomic slot loads either match the requested publication or
   yield Unknown. No step acquires the publisher's lock from the feed. *)
CONSTANTS Capacity, Limit, Subscribers
VARIABLES published, ring, missing, base, joined, after, pending, unknown, seen
vars == <<published, ring, missing, base, joined, after, pending, unknown, seen>>
Init == /\ published = 0 /\ ring = {} /\ missing = {} /\ base = 0
        /\ joined = {} /\ after = [s \in Subscribers |-> 0]
        /\ pending = [s \in Subscribers |-> {}]
        /\ unknown = [s \in Subscribers |-> FALSE]
        /\ seen = [s \in Subscribers |-> 0]
Range(a,b) == {n \in 1..Limit : n > a /\ n <= b}
(* Unconfirmed proposals can name keys absent from the actual publication.
   Their payload is unobservable while Unknown; the proof guard prevents it
   from being certified. A successful exact admission removes that uncertainty
   in the implementation; retaining it forever here overapproximates delay. *)
Payload(a,b) == (Range(a,b) \ missing) \cup {n + Limit : n \in Range(a,b) \cap missing}
Covered(a,b) == Range(a,b) \subseteq ring /\ Range(a,b) \cap missing = {}
Publish == /\ published < Limit
           /\ published' = published + 1
           /\ ring' = {n \in ring \cup {published'} : n > published' - Capacity}
           /\ missing' \in {missing, missing \cup {published'}}
           /\ UNCHANGED <<base, joined, after, pending, unknown, seen>>
Max(a,b) == IF a > b THEN a ELSE b
Join(s) == /\ s \notin joined
           /\ \E cursor \in after[s]..published :
                /\ joined' = joined \cup {s}
                /\ pending' = [pending EXCEPT ![s] = Payload(cursor, base)]
                /\ unknown' = [unknown EXCEPT ![s] = ~Covered(cursor,base)]
                /\ seen' = [seen EXCEPT ![s] = Max(cursor,base)]
                /\ after' = [after EXCEPT ![s] = cursor]
           /\ UNCHANGED <<published, ring, missing, base>>
Feed == /\ base < published
        /\ pending' = [s \in Subscribers |-> IF s \in joined
                           THEN pending[s] \cup Payload(base,published) ELSE pending[s]]
        /\ unknown' = [s \in Subscribers |-> IF s \in joined
                           THEN unknown[s] \/ ~Covered(base,published) ELSE unknown[s]]
        /\ seen' = [s \in Subscribers |-> IF s \in joined THEN published ELSE seen[s]]
        /\ base' = published
        /\ UNCHANGED <<published, ring, missing, joined, after>>
Deliver(s) == /\ s \in joined /\ seen[s] > after[s]
              /\ (unknown[s] \/ Range(after[s],seen[s]) \subseteq pending[s])
              /\ after' = [after EXCEPT ![s] = seen[s]]
              /\ pending' = [pending EXCEPT ![s] = {}]
              /\ unknown' = [unknown EXCEPT ![s] = FALSE]
              /\ UNCHANGED <<published, ring, missing, base, joined, seen>>
Leave(s) == /\ s \in joined /\ joined' = joined \ {s}
            /\ pending' = [pending EXCEPT ![s] = {}]
            /\ UNCHANGED <<published, ring, missing, base, after, unknown, seen>>
Next == Publish \/ Feed \/ (\E s \in Subscribers : Join(s) \/ Deliver(s) \/ Leave(s))
Spec == Init /\ [][Next]_vars
NoMissedChange == \A s \in joined : unknown[s] \/ Range(after[s],seen[s]) \subseteq pending[s]
NoInventedChange == \A s \in joined : unknown[s] \/ pending[s] \subseteq 1..published
Ordered == \A s \in joined : after[s] <= seen[s] /\ seen[s] <= Max(base,after[s])
=============================================================================
