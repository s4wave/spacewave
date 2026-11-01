---- MODULE SharedInterval ----
\* Receipt/recount submodel selected by SharedCounts Compose="interval".
\* Callers: replay.nameWorld -> worldDeaths.take -> pendingNet.net; replay
\* buildHold/save -> rootHoldBuilder (HoldsRoots); ledger lost -> publication
\* Recount -> RecountSharing; collector applies deaths and resolves releases.
\* The sequence configs cover tree push-down. This model isolates the receipt
\* cases that per-commit records cannot express: put-only netting, a failed
\* put's child lists, forfeiture, and a hold that first stores a root.
\* The alphabet has a base A -> L, new root B -> L, empty root N. L is pruned
\* by the writer: only the record, not B's put, accounts for B's edge to L.
\* An interval's joined steps are represented by its carried immutable page
\* lists. Original pages stay stored under the base hold until Name. Original
\* entries are Counted and add no found count. A failed receipt contributes
\* lists to the live walk but never to the stored/dead set. The matrix checks
\* the first/last successful receipt being found or fresh and one failed put.
\* SharingWalk supplies the recount assumption: uncovered intervals orphan;
\* the atomic Recount here is its completed result, not one walk slice.
EXTENDS Integers, FiniteSets
CONSTANT Guard
VARIABLE state
Pages == {"A", "B", "L", "N"}
Kids(p) == IF p \in {"A", "B"} THEN {"L"} ELSE {}
Init == state = [scenario |-> "choose", stage |-> 0,
  stored |-> {"A", "L"}, live |-> {"A", "L"},
  extra |-> [p \in Pages |-> 0], rel |-> {},
  carried |-> {}, successful |-> {}, found |-> {},
  record |-> {}, held |-> {}, recount |-> FALSE,
  resolving |-> FALSE, delta |-> 0, underflow |-> FALSE]
Choose(s) ==
  /\ state.scenario = "choose"
  /\ state' = [state EXCEPT !.scenario = s]
\* The failed publication contributes B's page list, but stored no B.
FailedPut ==
  /\ state.scenario \in {"failed-live", "failed-dead"} /\ state.stage = 0
  /\ state' = [state EXCEPT !.carried = {"B"}, !.stage = 1]
\* Replay may be the first successful writer of a root whose own put failed.
\* The hold itself and its owner are abstracted to live B and its lineage.
HoldsRoots ==
  /\ state.stage = 1 /\ state.scenario = "failed-live"
  /\ state' = [state EXCEPT !.stored = @ \cup {"B"},
       !.live = {"A", "B", "L"}, !.stage = 2,
       !.extra["B"] = IF Guard = "found-only" THEN 0 ELSE 1]
\* Success receipts report found on the repeat; OR preserves that receipt.
SuccessfulPut ==
  /\ state.scenario = "receipts" /\ state.stage < 2
  /\ state' = [state EXCEPT !.stage = @ + 1,
       !.carried = @ \cup {"B"}, !.successful = @ \cup {"B"},
       !.stored = @ \cup {"B"}, !.live = {"A", "B", "L"},
       !.found = IF state.stage = 1 /\ Guard = "reported"
                THEN {"B"} ELSE {},
       !.extra["B"] = @ + 1]
\* Name nets the whole carried interval. In the put-only case all carried
\* pages are created; walking failed lists still discovers B -> L.
Name ==
  /\ (state.scenario = "failed-live" /\ state.stage = 2)
      \/ (state.scenario = "failed-dead" /\ state.stage = 1)
      \/ (state.scenario = "receipts" /\ state.stage = 2)
  /\ LET lists == IF Guard = "successful-only" THEN state.successful
                  ELSE state.carried
         alive == IF state.scenario = "failed-dead" THEN {"N"}
                  ELSE {"B"} \cup UNION {Kids(p) : p \in lists}
         candidates == IF Guard = "failed-stored" THEN state.carried
                       ELSE state.successful
         dead == candidates \ alive
         held == IF "L" \in alive /\ "B" \notin state.found THEN {"L"} ELSE {}
     IN state' = [state EXCEPT !.record = dead, !.held = held, !.stage = 3,
           !.extra["L"] = @ + (IF "L" \in held THEN 1 ELSE 0)]
ReleaseBase ==
  /\ (state.scenario = "failed-live" /\ state.stage = 3)
      \/ (state.scenario = "receipts" /\ state.stage = 3)
      \/ (state.scenario = "forfeit" /\ state.stage = 3)
  /\ state' = [state EXCEPT !.live = @ \ {"A"}, !.rel = {"A"}, !.stage = 5]
\* Apply the lineage from B while a replay hold continues to name it.
ApplyLineage ==
  /\ state.scenario = "failed-live" /\ state.stage = 5
      /\ state.rel = {}
  /\ state' = [state EXCEPT !.rel = {"B"}, !.stage = 6]
Forfeit ==
  /\ state.scenario = "forfeit" /\ state.stage = 0
  /\ state' = [state EXCEPT !.stored = @ \cup {"B"},
       !.live = {"A", "B", "L"}, !.recount = Guard # "drop-recount", !.stage = 1]
Recount ==
  /\ state.scenario = "forfeit" /\ state.stage = 1
  /\ state' = [state EXCEPT !.stage = 3,
       !.extra["L"] = IF state.recount THEN 1 ELSE 0, !.recount = FALSE]
\* Release resolution is separate from deaths' read and ChangeHolds. The
\* correct caller excludes it from that interval; the mutant lets it enter.
StartResolveDeaths ==
  /\ state.scenario = "apply-race" /\ state.stage = 0
  /\ state' = [state EXCEPT !.extra["A"] = 1,
       !.rel = {"A"}, !.resolving = TRUE, !.delta = -1, !.stage = 1,
       !.stored = @ \cup {"B"}, !.live = {"B", "L"}]
ChangeHolds ==
  /\ state.scenario = "apply-race" /\ state.stage = 1
  /\ state' = [state EXCEPT !.underflow = state.extra["A"] + state.delta < 0,
       !.extra["A"] = IF state.extra["A"] + state.delta < 0 THEN 0
                     ELSE state.extra["A"] + state.delta,
       !.extra["L"] = @ + 1, !.resolving = FALSE, !.stage = 2]
ResolveReleases(p) ==
  /\ p \in state.rel
  /\ ~state.resolving \/ Guard = "resolve-during-apply"
  /\ state' = [state EXCEPT
       !.rel = (@ \ {p}) \cup (IF state.extra[p] = 0 THEN Kids(p) ELSE {}),
       !.stored = IF state.extra[p] = 0 THEN @ \ {p} ELSE @,
       !.extra[p] = IF @ > 0 THEN @ - 1 ELSE 0]
Next == (\E s \in {"failed-live", "failed-dead", "receipts", "forfeit", "apply-race"} : Choose(s))
  \/ FailedPut \/ HoldsRoots \/ SuccessfulPut \/ Name
  \/ ReleaseBase \/ ApplyLineage \/ Forfeit \/ Recount
  \/ StartResolveDeaths \/ ChangeHolds
  \/ (\E p \in Pages : ResolveReleases(p)) \/ UNCHANGED state
Spec == Init /\ [][Next]_state
NoFreedRead == state.live \subseteq state.stored
ReleasesStored == state.rel \cup state.record \subseteq state.stored
CountsNonNegative == ~state.underflow /\ \A p \in Pages : state.extra[p] >= 0
CountsCover == (state.stage >= 5 /\ state.rel = {} /\
                  state.scenario \in {"failed-live", "forfeit", "receipts"}) =>
  state.extra["L"] >= Cardinality({p \in state.stored : "L" \in Kids(p)}) - 1
====
