---------------------- MODULE ReplicaCopyAuthority ----------------------
EXTENDS Naturals, Integers

\* startSOSync owns one runAccountReplicaCopy for each mounted object until its
\* context ends. runAccountReplicaCopy watches snapshots and publishes replicaHead
\* changes. copyAccountWorldHead attempts the graph, then waits for latest or
\* a DEX session start after missing blocks. For one immutable World and one
\* already-open session, a later accepted participant grant is the only event
\* that can make the source readable. The old root-only filter is TrackConfig
\* FALSE. New heads/session starts are omitted: either independently wakes the
\* copier and would conceal the grant-only stall. DEX role revocation itself
\* is checked by the real-provider requester-role run-through, not this model.
CONSTANT TrackConfig
VARIABLES grant, revision, seen, announced, attempted, complete, stopped
vars == <<grant, revision, seen, announced, attempted, complete, stopped>>

Init == /\ grant = FALSE /\ revision = 0 /\ seen = -1
        /\ announced = 0 /\ attempted = 0
        /\ complete = FALSE /\ stopped = FALSE

\* The remote source accepts the grant before replication observes its config.
Grant == /\ ~grant /\ ~stopped /\ grant' = TRUE
         /\ UNCHANGED <<revision, seen, announced, attempted, complete, stopped>>
ReceiveConfig == /\ grant /\ revision = 0 /\ ~stopped /\ revision' = 1
                 /\ UNCHANGED <<grant, seen, announced, attempted, complete, stopped>>

\* An initial graph always announces. Further same-root snapshots announce only
\* when the filter includes the changed accepted config-chain hash.
Watch == /\ seen # revision /\ ~stopped
         /\ announced' = IF seen = -1 \/ TrackConfig THEN announced + 1 ELSE announced
         /\ seen' = revision
         /\ UNCHANGED <<grant, revision, attempted, complete, stopped>>

\* A denied walk consumes this announcement and parks on latest/session start.
\* The grant is read at the request boundary, independently of local snapshots.
Copy == /\ attempted < announced /\ ~complete /\ ~stopped
        /\ attempted' = announced /\ complete' = grant
        /\ UNCHANGED <<grant, revision, seen, announced, stopped>>

Cancel == /\ ~stopped /\ stopped' = TRUE
          /\ UNCHANGED <<grant, revision, seen, announced, attempted, complete>>
Next == Grant \/ ReceiveConfig \/ Watch \/ Copy \/ Cancel \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars /\ WF_vars(ReceiveConfig) /\ WF_vars(Watch) /\ WF_vars(Copy)

TypeOK == /\ grant \in BOOLEAN /\ revision \in 0..1 /\ seen \in {-1, 0, 1}
          /\ announced \in 0..2 /\ attempted \in 0..2 /\ attempted <= announced
          /\ complete \in BOOLEAN /\ stopped \in BOOLEAN
NoUnauthorizedCompletion == complete => grant
GrantedMakesProgress == [](grant => <>(complete \/ stopped))
=============================================================================
