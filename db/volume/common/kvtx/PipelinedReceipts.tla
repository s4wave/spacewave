------------------------- MODULE PipelinedReceipts -------------------------
(* The Volume's publication writer with pipelined commits. Groups commit in
   order, each without waiting for its durability; a later group may commit
   on top of an earlier one that is committed but not yet durable. Each
   group's durability is learned beside the next group's apply, in any order,
   and can fail. A receipt resolves in group order: as durable only once its
   group and every earlier group are durable, and as failed once any of them
   failed.

   A crash keeps the groups whose commits are durable and replays nothing
   past the first that is not, since the journal's replay stops at the first
   missing record and a local flush covers a prefix. So a receipt resolved as
   durable must name a group in the longest durable prefix.

   PipelinedReceipts.mutants.json lists the guards each invariant depends
   on. *)
EXTENDS Naturals

CONSTANTS N

VARIABLES
    committed, \* the groups committed, 1..committed
    durable,   \* the groups whose durability the store confirmed
    lost,      \* the groups whose durability failed
    resolved,  \* the groups resolved, 1..resolved, in order
    ok,        \* the groups resolved as durable
    crashed    \* the process crashed

vars == <<committed, durable, lost, resolved, ok, crashed>>

Groups == 1..N

\* The longest prefix of groups whose commits are durable.
Prefix == CHOOSE k \in 0..N : (\A j \in 1..k : j \in durable) /\ (k = N \/ (k + 1) \notin durable)

Init ==
    /\ committed = 0
    /\ durable = {}
    /\ lost = {}
    /\ resolved = 0
    /\ ok = {}
    /\ crashed = FALSE

\* The next group commits, on top of earlier ones durable or not.
Commit ==
    /\ ~crashed
    /\ committed < N
    /\ committed' = committed + 1
    /\ UNCHANGED <<durable, lost, resolved, ok, crashed>>

\* A committed group's durability is confirmed, in any order.
Durable(g) ==
    /\ ~crashed
    /\ g <= committed
    /\ g \notin durable \cup lost
    /\ durable' = durable \cup {g}
    /\ UNCHANGED <<committed, lost, resolved, ok, crashed>>

\* Or it fails, such as on a failed local flush.
Fail(g) ==
    /\ ~crashed
    /\ g <= committed
    /\ g \notin durable \cup lost
    /\ lost' = lost \cup {g}
    /\ UNCHANGED <<committed, durable, resolved, ok, crashed>>

\* The next group resolves as durable once it and every earlier group are.
ResolveOk ==
    /\ ~crashed
    /\ resolved < committed
    /\ \A j \in 1..(resolved + 1) : j \in durable
    /\ resolved' = resolved + 1
    /\ ok' = ok \cup {resolved + 1}
    /\ UNCHANGED <<committed, durable, lost, crashed>>

\* Or as failed once it or an earlier group failed.
ResolveFailed ==
    /\ ~crashed
    /\ resolved < committed
    /\ \E j \in 1..(resolved + 1) : j \in lost
    /\ resolved' = resolved + 1
    /\ UNCHANGED <<committed, durable, lost, ok, crashed>>

Crash ==
    /\ ~crashed
    /\ crashed' = TRUE
    /\ UNCHANGED <<committed, durable, lost, resolved, ok>>

\* A crashed process, or one that resolved every group, does nothing more.
Done ==
    /\ crashed \/ resolved = N
    /\ UNCHANGED vars

Next ==
    \/ Commit \/ ResolveOk \/ ResolveFailed \/ Crash \/ Done
    \/ \E g \in Groups : Durable(g) \/ Fail(g)

Spec == Init /\ [][Next]_vars

FairSpec ==
    /\ Spec
    /\ WF_vars(Commit) /\ WF_vars(ResolveOk) /\ WF_vars(ResolveFailed)
    /\ \A g \in Groups : WF_vars(Durable(g) \/ Fail(g))

\* Every receipt resolved as durable survives a crash.
ResolvedSurvives == \A g \in ok : g <= Prefix

\* No receipt resolves as durable when its group or an earlier one failed.
NoDurableAfterFailure == \A g \in ok : \A j \in 1..g : j \notin lost

\* Receipts resolve in group order.
InOrder == ok \subseteq 1..resolved

\* Every committed group resolves, unless the process crashed.
AllResolve == (committed = N) ~> (resolved = N \/ crashed)

TypeOK ==
    /\ committed \in 0..N /\ resolved \in 0..committed
    /\ durable \subseteq Groups /\ lost \subseteq Groups /\ ok \subseteq Groups
    /\ crashed \in BOOLEAN
=============================================================================
