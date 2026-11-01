---- MODULE SharingWalk ----
\* Sliced recount, with two independent World lineages and three pages.
\* Source: refgraph-sharing.go, refgraph-releases.go, kvtx/sharing.go,
\* root-retention.go:addExistedHolds and engine/world-deaths.go:take/pendingNet.
\* Callers: format.go upgrade and format-copy.go call BeginSharing; Volume.open
\* stops/starts it; publicationWriter.apply calls RecountSharing before puts;
\* Volume.walkSharing runs slices; the property test rewalk stops/starts behind
\* the engine. All enter BeginSharing here. Registered block roots and unknown
\* child lists (Doubt) are excluded: they prohibit trusting any completed count.
\* Child lists are immutable; Put adds a new parent, never mutates its contents.
\* Owner removals can leave surplus sightings: equality holds only without a
\* removal. SharedCounts covers push-down and interval netting, Ledger saves.
\*
\* Boundary "none" is current code. "start" forfeits unnamed intervals only
\* at recount submission. "birth" additionally forbids taking a lineage from
\* any root born before counts became complete unless its record was named
\* before recount. This is an assumption about the engine fix, not built Go.
\* Forfeit removes an unobservable lineage without lost(): recount already
\* includes the surviving physical edges. A future interval may establish a
\* new lineage only from a root first stored with complete counts.
EXTENDS Integers, FiniteSets
CONSTANTS Boundary, SightBehind, UseImplied, SightClearing
Pages == 1..3
Lineages == 1..2
\* Each World root has one replay hold (child list key 1 or 2). Both may
\* also acquire a new child-list edge (key 3 or 4) to the shared payload 3.
Keys == 1..4
Target(k) == IF k <= 2 THEN k ELSE 3
VARIABLES phase, cursor, edges, owners, record, eligible, put, seen, extra,
          stored, release, removed, began
vars == <<phase, cursor, edges, owners, record, eligible, put, seen, extra,
          stored, release, removed, began>>
Counting == phase \in {"complete", "forget"}
Clearing == phase \in {"holds", "releases", "seen"}
Holders(p) == Cardinality({k \in edges : Target(k) = p})
              + Cardinality({k \in owners : k = p})
              + (IF p \in Lineages /\ record[p] \in {"open", "named"}
                    THEN 1 ELSE 0)
Init ==
  /\ phase = "complete" /\ cursor = 1 /\ began = FALSE
  /\ edges = {} /\ owners = {} /\ stored = {} /\ put = {}
  /\ record = [l \in Lineages |-> "absent"]
  /\ eligible = [l \in Lineages |-> FALSE]
  /\ seen = {} /\ extra = [p \in Pages |-> 0]
  /\ release = [p \in Pages |-> 0] /\ removed = {}
\* A sight leaves zero for its first holder and increments for each later one.
Sight(es) == [p \in Pages |-> extra[p] +
  Cardinality({k \in es : Target(k) = p}) -
  (IF p \notin seen /\ \E k \in es : Target(k) = p THEN 1 ELSE 0)]
Seen(es) == seen \cup {Target(k) : k \in es}
BeginSharing ==
  /\ ~began /\ Counting
  /\ began' = TRUE /\ phase' = "holds" /\ cursor' = 1
  /\ record' = [l \in Lineages |->
       IF Boundary \in {"start", "birth"} /\ record[l] = "open"
       THEN "orphan" ELSE record[l]]
  /\ eligible' = [l \in Lineages |-> record[l] = "named"]
  /\ UNCHANGED <<edges, owners, put, seen, extra, stored, release, removed>>
\* One physical transaction per action: cursor reads and sightings are atomic.
WalkSharing ==
  /\ began /\ phase # "complete"
  /\ CASE phase = "holds" ->
       /\ extra' = [p \in Pages |-> 0] /\ phase' = "releases"
       /\ UNCHANGED <<seen, release, cursor>>
     [] phase = "releases" ->
       /\ release' = [p \in Pages |-> 0] /\ phase' = "seen"
       /\ UNCHANGED <<seen, extra, cursor>>
     [] phase = "seen" ->
       /\ seen' = {} /\ phase' = "children" /\ cursor' = 1
       /\ UNCHANGED <<extra, release>>
     [] phase = "children" ->
       LET es == edges \cap {cursor} IN
       /\ extra' = Sight(es) /\ seen' = Seen(es)
       /\ phase' = IF cursor = 4 THEN "owners" ELSE phase
       /\ cursor' = IF cursor = 4 THEN 1 ELSE cursor + 1
       /\ UNCHANGED release
     [] phase = "owners" ->
       LET es == (owners \cap {cursor}) \cup
                 (IF UseImplied /\ cursor \in Lineages /\ record[cursor] = "named"
                  THEN {cursor + 4} ELSE {})
           target(k) == IF k > 4 THEN k - 4 ELSE k
           n(p) == Cardinality({k \in es : target(k) = p})
       IN /\ extra' = [p \in Pages |-> extra[p] + n(p)
                       - (IF p \notin seen /\ n(p) > 0 THEN 1 ELSE 0)]
          /\ seen' = seen \cup {target(k) : k \in es}
          /\ phase' = IF cursor = 3 THEN "forget" ELSE phase
          /\ cursor' = IF cursor = 3 THEN 1 ELSE cursor + 1
          /\ UNCHANGED release
     [] phase = "forget" ->
       /\ seen' = {} /\ phase' = "complete"
       /\ UNCHANGED <<extra, release, cursor>>
  /\ UNCHANGED <<edges, owners, record, eligible, put, stored, removed, began>>
\* Put stores a fresh root and its replay hold. AddHolds is dropped in a walk.
\* A root's lineage is not itself an edge the walk can sight.
Put(l) ==
  /\ l \notin put
  /\ put' = put \cup {l} /\ edges' = edges \cup {l}
  /\ stored' = stored \cup {l}
  /\ eligible' = [eligible EXCEPT ![l] = Counting]
  /\ record' = [record EXCEPT ![l] = "open"]
  /\ LET sight == (SightBehind /\
                    (phase = "owners" \/ (phase = "children" /\ l < cursor)))
                    \/ (SightClearing /\ Clearing)
     IN /\ extra' = IF Counting THEN [extra EXCEPT ![l] = 1]
                   ELSE IF sight THEN Sight({l}) ELSE extra
        /\ seen' = IF sight /\ ~Counting THEN Seen({l}) ELSE seen
  /\ UNCHANGED <<phase, cursor, owners, release, removed, began>>
\* Found put/new child list; repeated identical lists add nothing.
PutChild(k) ==
  /\ k \in 3..4 /\ k \notin edges
  /\ edges' = edges \cup {k} /\ stored' = stored \cup {3}
  /\ LET sight == (SightBehind /\ (phase = "owners" \/
                   (phase = "children" /\ k < cursor)))
                   \/ (SightClearing /\ Clearing)
     IN /\ extra' = IF Counting THEN
                      [extra EXCEPT ![3] = IF 3 \in stored THEN @ + 1 ELSE 0]
                    ELSE IF sight THEN Sight({k}) ELSE extra
        /\ seen' = IF sight /\ ~Counting THEN Seen({k}) ELSE seen
  /\ UNCHANGED <<phase, cursor, owners, record, eligible, put, release, removed, began>>
Name(l) ==
  /\ record[l] = "open"
  /\ record' = [record EXCEPT ![l] =
       IF Boundary = "birth" /\ ~eligible[l] THEN "orphan" ELSE "named"]
  /\ LET sight == UseImplied /\ phase = "owners" /\ l < cursor
                    /\ (Boundary # "birth" \/ eligible[l])
     IN /\ extra' = IF sight THEN Sight({l}) ELSE extra
        /\ seen' = IF sight THEN Seen({l}) ELSE seen
  /\ UNCHANGED <<phase, cursor, edges, owners, eligible, put, stored, release, removed, began>>
SettleOwnerCounts(p) ==
  /\ p \in stored /\ p \notin owners /\ p \notin removed
  /\ owners' = owners \cup {p}
  /\ LET sight == SightBehind /\ phase = "owners" /\ p < cursor IN
       /\ extra' = IF Counting THEN [extra EXCEPT ![p] = @ + 1]
                    ELSE IF sight THEN Sight({p}) ELSE extra
       /\ seen' = IF sight THEN Seen({p}) ELSE seen
  /\ UNCHANGED <<phase, cursor, edges, record, eligible, put, stored, release, removed, began>>
RemoveOwner(p) ==
  /\ p \in owners /\ p \notin removed
  /\ owners' = owners \ {p} /\ removed' = removed \cup {p}
  /\ release' = IF Counting THEN [release EXCEPT ![p] = @ + 1] ELSE release
  /\ UNCHANGED <<phase, cursor, edges, record, eligible, put, stored, seen, extra, began>>
Apply(l) ==
  /\ Counting /\ began /\ record[l] = "named"
  /\ record' = [record EXCEPT ![l] = "applied"]
  /\ release' = [release EXCEPT ![l] = @ + 1]
  /\ UNCHANGED <<phase, cursor, edges, owners, eligible, put, seen, extra, stored, removed, began>>
ResolveReleases(p) ==
  /\ Counting /\ release[p] > 0
  /\ release' = [release EXCEPT ![p] = @ - 1]
  /\ extra' = [extra EXCEPT ![p] = IF @ > 0 THEN @ - 1 ELSE 0]
  /\ stored' = IF extra[p] = 0 THEN stored \ {p} ELSE stored
  /\ UNCHANGED <<phase, cursor, edges, owners, record, eligible, put, seen, removed, began>>
Next == BeginSharing \/ WalkSharing
  \/ (\E l \in Lineages : Put(l) \/ Name(l) \/ Apply(l))
  \/ (\E k \in 3..4 : PutChild(k))
  \/ (\E p \in Pages : SettleOwnerCounts(p) \/ RemoveOwner(p) \/ ResolveReleases(p))
  \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars
NoFreedRead == {Target(k) : k \in edges} \cup owners \subseteq stored
CountsNonNegative == \A p \in Pages : extra[p] >= 0
\* Ineligible open intervals must forfeit, so they promise no future death.
CountsCover == (began /\ Counting) => \A p \in stored :
  extra[p] >= Holders(p) - 1 + release[p] -
    (IF Boundary = "birth" /\ p \in Lineages /\ record[p] = "open"
          /\ ~eligible[p] THEN 1 ELSE 0)
CountsExact == (began /\ Counting /\ removed = {}) => \A p \in stored :
  extra[p] = Holders(p) - 1 + release[p] -
    (IF Boundary = "birth" /\ p \in Lineages /\ record[p] = "open"
          /\ ~eligible[p] THEN 1 ELSE 0)
====
