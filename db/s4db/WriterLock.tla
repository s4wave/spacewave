---- MODULE WriterLock ----
EXTENDS Naturals, Sequences, FiniteSets
\* One handle, one checkpoint worker and two writers. Queue insertion,
\* cancellation and grant are serialized by mtx. A grant keeps held set
\* until the selected caller releases or passes on its canceled grant.
\* File-lock and I/O completion are assumed: this model proves scheduling
\* progress, not a wall-clock bound on checkpoint work or another process.
CONSTANT PassCanceledGrant
Actors == {"checkpoint", "w1", "w2"}
VARIABLES owner, first, rest, pc, canceled
vars == <<owner, first, rest, pc, canceled>>
Queued == {first[i] : i \in 1..Len(first)} \cup {rest[i] : i \in 1..Len(rest)}
Remove(q, a) == SelectSeq(q, LAMBDA x: x # a)
Init == /\ owner = "none" /\ first = <<>> /\ rest = <<>>
        /\ pc = [a \in Actors |-> "idle"] /\ canceled = {}
Enter(a) ==
 /\ pc[a] = "idle"
 /\ canceled' = canceled \ {a}
 /\ IF owner = "none"
    THEN /\ owner' = a /\ pc' = [pc EXCEPT ![a] = "held"]
         /\ UNCHANGED <<first, rest>>
    ELSE /\ pc' = [pc EXCEPT ![a] = "waiting"]
         /\ owner' = owner
         /\ IF a = "checkpoint"
            THEN first' = Append(first, a) /\ rest' = rest
            ELSE rest' = Append(rest, a) /\ first' = first
Cancel(a) ==
 /\ pc[a] \in {"waiting", "granted"} /\ a \notin canceled
 /\ canceled' = canceled \cup {a}
 /\ UNCHANGED <<owner, first, rest, pc>>
LeaveQueue(a) ==
 /\ pc[a] = "waiting" /\ a \in canceled
 /\ first' = Remove(first, a) /\ rest' = Remove(rest, a)
 /\ pc' = [pc EXCEPT ![a] = "idle"]
 /\ UNCHANGED <<owner, canceled>>
Accept(a) ==
 /\ pc[a] = "granted"
 \* select can choose the grant even when cancellation is ready.
 /\ pc' = [pc EXCEPT ![a] = "held"]
 /\ UNCHANGED <<owner, first, rest, canceled>>
Release(a) ==
 /\ owner = a
 /\ pc[a] = "held" \/ (pc[a] = "granted" /\ a \in canceled /\ PassCanceledGrant)
 /\ LET q == IF Len(first) > 0 THEN first ELSE rest IN
    IF Len(q) = 0
    THEN /\ owner' = "none" /\ pc' = [pc EXCEPT ![a] = "idle"]
         /\ UNCHANGED <<first, rest>>
    ELSE /\ owner' = Head(q)
         /\ pc' = [pc EXCEPT ![a] = "idle", ![Head(q)] = "granted"]
         /\ IF Len(first) > 0
            THEN first' = Tail(first) /\ rest' = rest
            ELSE rest' = Tail(rest) /\ first' = first
 /\ UNCHANGED canceled
DropGrant(a) ==
 /\ ~PassCanceledGrant /\ pc[a] = "granted" /\ a \in canceled
 /\ pc' = [pc EXCEPT ![a] = "idle"]
 /\ UNCHANGED <<owner, first, rest, canceled>>
Next == \E a \in Actors: Enter(a) \/ Cancel(a) \/ LeaveQueue(a) \/ Accept(a) \/ Release(a) \/ DropGrant(a)
Safety ==
 /\ owner \in Actors \cup {"none"}
 /\ \A a \in Actors: (pc[a] \in {"held", "granted"}) <=> owner = a
 /\ \A a \in Actors: (pc[a] = "waiting") <=> a \in Queued
 /\ Len(first) <= 1
 /\ Len(first) + Len(rest) = Cardinality(Queued)
Progress == \A a \in Actors: (pc[a] = "waiting") ~> (pc[a] \in {"held", "idle"})
Spec == Init /\ [][Next]_vars /\ \A a \in Actors:
 WF_vars(Release(a)) /\ WF_vars(Accept(a)) /\ WF_vars(LeaveQueue(a))
====
