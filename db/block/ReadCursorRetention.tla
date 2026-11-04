---------------------- MODULE ReadCursorRetention ----------------------
EXTENDS Naturals, FiniteSets
\* One old page, two evaluation holds, one old snapshot and a changed HEAD.
\* Callers: ordinary Cursor and retained ReadCursor reads, view/cache holds,
\* snapshot Discard, provider local-copy eviction, volume logical collection,
\* and process restart. Tree/hash/epoch correctness stays in ReadCursor and
\* ReadCursorRoots; shared-page count arithmetic stays in gc/SharedCounts.
\* Observer and hold registration are separate actions, as the callback and
\* hold-table locks in observeDecodedBlockHit can interleave. A read returns
\* only after both. A concurrently released hold intentionally retains nothing.
\* Local eviction has a separate keep/remote decision and deletion: a hold
\* acquired after that decision cannot cancel an already selected tombstone.
\* This is the existing eviction semantics, not a promise of local residency
\* for a view first evaluated during an eviction. Remote durability is required.
\* Collect abstracts the existing rooted-reachability contract: HEAD or an open
\* snapshot pins the old page. Holds preserve local copies, not volume roots.
\* No collector algorithm, upload proof, crash journal or generation arithmetic
\* is replaced here; those are inputs checked by their owning specs/tests.
CONSTANTS NotifyPointer, HoldPointer, ProtectPins, RequireRemote, CacheReadsOn
VARIABLES head, pin, stored, local, remote, cache, active, held, completed,
          phase, actor, kind, observed, registered, recent, pending,
          badRead, badCollect, badLocal, crashed
vars == <<head, pin, stored, local, remote, cache, active, held, completed,
          phase, actor, kind, observed, registered, recent, pending,
          badRead, badCollect, badLocal, crashed>>
Holders == {0, 1}
Init == /\ head = 0 /\ pin = TRUE /\ stored = TRUE /\ local = TRUE
        /\ remote \in BOOLEAN /\ cache = TRUE
        /\ active = Holders /\ held = {} /\ completed = {}
        /\ phase = 0 /\ actor = 0 /\ kind = "pointer"
        /\ observed = FALSE /\ registered = FALSE /\ recent = FALSE
        /\ pending = FALSE /\ badRead = FALSE /\ badCollect = FALSE
        /\ badLocal = FALSE /\ crashed = FALSE
Start(h, k) ==
    /\ phase = 0 /\ pin /\ h \in active \ completed /\ stored
    /\ (IF k = "disk" THEN local \/ remote ELSE cache)
    /\ phase' = 1 /\ actor' = h /\ kind' = k
    /\ observed' = FALSE /\ registered' = FALSE
    /\ UNCHANGED <<head, pin, stored, local, remote, cache, active, held,
                   completed, recent, pending, badRead, badCollect, badLocal, crashed>>
Observe ==
    /\ phase = 1 /\ phase' = 2
    /\ observed' = (kind # "pointer" \/ NotifyPointer)
    /\ recent' = (recent \/ kind = "disk" \/ (CacheReadsOn /\ observed'))
    /\ UNCHANGED <<head, pin, stored, local, remote, cache, active, held,
                   completed, actor, kind, registered, pending,
                   badRead, badCollect, badLocal, crashed>>
Register ==
    /\ phase = 2 /\ phase' = 3
    /\ registered' = (kind # "pointer" \/ HoldPointer)
    /\ held' = IF CacheReadsOn /\ registered' /\ actor \in active
               THEN held \cup {actor} ELSE held
    /\ UNCHANGED <<head, pin, stored, local, remote, cache, active, completed,
                   actor, kind, observed, recent, pending,
                   badRead, badCollect, badLocal, crashed>>
Return ==
    /\ phase = 3 /\ phase' = 0 /\ completed' = completed \cup {actor}
    /\ badRead' = (badRead \/ (CacheReadsOn /\ actor \in active /\
                                      (~observed \/ actor \notin held)))
    /\ UNCHANGED <<head, pin, stored, local, remote, cache, active, held,
                   actor, kind, observed, registered, recent, pending,
                   badCollect, badLocal, crashed>>
Cancel == /\ phase # 0 /\ phase' = 0 /\ completed' = completed \cup {actor}
          /\ UNCHANGED <<head, pin, stored, local, remote, cache, active, held,
                actor, kind, observed, registered, recent, pending,
                badRead, badCollect, badLocal, crashed>>
Release(h) ==
    /\ h \in active /\ active' = active \ {h} /\ held' = held \ {h}
    /\ UNCHANGED <<head, pin, stored, local, remote, cache, completed, phase,
                   actor, kind, observed, registered, recent, pending,
                   badRead, badCollect, badLocal, crashed>>
Publish == /\ head = 0 /\ head' = 1
           /\ UNCHANGED <<pin, stored, local, remote, cache, active, held,
                 completed, phase, actor, kind, observed, registered, recent,
                 pending, badRead, badCollect, badLocal, crashed>>
CloseSnapshot == /\ pin /\ phase = 0 /\ pin' = FALSE
                 /\ UNCHANGED <<head, stored, local, remote, cache, active, held,
                     completed, phase, actor, kind, observed, registered, recent,
                     pending, badRead, badCollect, badLocal, crashed>>
Collect ==
    /\ stored /\ head = 1 /\ (~ProtectPins \/ ~pin)
    /\ stored' = FALSE /\ cache' = FALSE /\ badCollect' = (badCollect \/ pin)
    /\ UNCHANGED <<head, pin, local, remote, active, held, completed, phase,
                   actor, kind, observed, registered, recent, pending,
                   badRead, badLocal, crashed>>
SelectEviction ==
    /\ local /\ ~pending /\ ~recent /\ held = {}
    /\ (~RequireRemote \/ remote) /\ pending' = TRUE
    /\ UNCHANGED <<head, pin, stored, local, remote, cache, active, held,
                   completed, phase, actor, kind, observed, registered, recent,
                   badRead, badCollect, badLocal, crashed>>
DeleteLocal ==
    /\ pending /\ local /\ local' = FALSE /\ pending' = FALSE
    /\ badLocal' = (badLocal \/ (stored /\ ~remote))
    /\ UNCHANGED <<head, pin, stored, remote, cache, active, held, completed,
                   phase, actor, kind, observed, registered, recent,
                   badRead, badCollect, crashed>>
ExpireRecent == /\ recent /\ recent' = FALSE
                /\ UNCHANGED <<head, pin, stored, local, remote, cache, active,
                     held, completed, phase, actor, kind, observed, registered,
                     pending, badRead, badCollect, badLocal, crashed>>
EvictDecoded == /\ cache /\ cache' = FALSE
                /\ UNCHANGED <<head, pin, stored, local, remote, active, held,
                     completed, phase, actor, kind, observed, registered, recent,
                     pending, badRead, badCollect, badLocal, crashed>>
Restart ==
    /\ ~crashed /\ crashed' = TRUE /\ cache' = FALSE /\ pin' = FALSE
    /\ active' = {} /\ held' = {} /\ phase' = 0
    /\ UNCHANGED <<head, stored, local, remote, completed, actor, kind,
                   observed, registered, recent, pending,
                   badRead, badCollect, badLocal>>
Next == (\E h \in Holders : (\E k \in {"disk", "decoded", "pointer"} : Start(h,k)) \/ Release(h))
        \/ Observe \/ Register \/ Return \/ Cancel \/ Publish \/ CloseSnapshot
        \/ Collect \/ SelectEviction \/ DeleteLocal \/ ExpireRecent
        \/ EvictDecoded \/ Restart \/ UNCHANGED vars
ReadContract == ~badRead
SnapshotSafe == ~badCollect
LocalDurable == ~badLocal
NoReleasedHold == held \subseteq active
Spec == Init /\ [][Next]_vars
======================================================================
