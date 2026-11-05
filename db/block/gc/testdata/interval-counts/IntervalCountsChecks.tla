---- MODULE IntervalCountsChecks ----
\* Directed source-action checks for the interval model. Each prefix is a
\* conjunction with the original actions; no state is injected after Init.
\* The ordinary case saves kept pages through a foreground publication,
\* retains both the old wrapper and the empty record header elsewhere, then
\* checks coverage through actual retirement. Historical names absent Found
\* metadata and requires safe retirement. Stray checks the separately stated
\* counted-free exact branch for a successful unused construction put.
\*
\* Run from this directory through the bounded checker, which copies the
\* exact main model and these fixtures into a fresh evidence directory:
\*   uv run --no-project --python 3.11 check_interval_counts.py
\*       --case ordinary --output <new-directory>
\* Repeat for historical and stray; add --mutant for the omission recipes.
\* Expected rejections are CountsCover, RecordsRetire and NoLeak. Setup errors
\* never count as sensitivity. This testdata directory is deliberately outside
\* broad positive-model discovery; the checker preserves every input and trace.
\* The full three-commit obligation uses the unchanged discard-full3 config on
\* IntervalCounts itself and is not replaced by these directed checks.
EXTENDS IntervalCounts
VARIABLE checkStep
CheckVars == <<vars, checkStep>>

OrdinaryInit == Init /\ Shadow /\ checkStep = 0
OrdinaryNext ==
  \/ /\ checkStep = 0 /\ AliasWrapper /\ aliases' = {<<"R0">>} /\ checkStep' = 1
  \/ /\ checkStep = 1 /\ Commit /\ cur' = "R1" /\ putSet' = {} /\ kept' = {"R1", "B"} /\ checkStep' = 2
  \/ /\ checkStep = 2 /\ WrittenAdvance /\ save'.roots = <<"R0", "R1">> /\ checkStep' = 3
  \/ /\ checkStep = 3 /\ Commit /\ cur' = "R2" /\ putSet' = {}
     /\ wrapper' = <<"R0", "R1">> /\ counting' /\ ~doubt' /\ checkStep' = 4
  \/ /\ checkStep = 4 /\ Commit /\ cur' = "R1" /\ putSet' = {} /\ checkStep' = 5
  \/ /\ checkStep = 5 /\ ResolveWrapper /\ counting' /\ checkStep' = 6
  \/ /\ checkStep = 6 /\ SealCheckpoint /\ Len(waiting') = 1
     /\ waiting'[1].record.from = None /\ waiting'[1].record.parts = {} /\ ~waiting'[1].record.large
     /\ ~waiting'[1].record.current /\ counting' /\ checkStep' = 7
  \/ /\ checkStep = 7 /\ CarryRecord /\ checkStep' = 8
  \/ /\ checkStep = 8 /\ AliasHeader /\ headerAliases' = {curEpoch} /\ checkStep' = 9
  \/ /\ checkStep = 9 /\ StartApply /\ checkStep' = 10
  \/ /\ checkStep = 10 /\ EndApply /\ counting' /\ checkStep' = 11
  \/ /\ checkStep = 11 /\ ResolveRecord /\ counting' /\ checkStep' = 12
  \/ /\ checkStep = 12 /\ UNCHANGED vars /\ checkStep' = 12
OrdinarySpec == OrdinaryInit /\ [][OrdinaryNext]_CheckVars /\ WF_CheckVars(OrdinaryNext)
OrdinaryReached == checkStep = 12
OrdinarySettles == <>OrdinaryReached

HistoricalInit == Init /\ storageMode = "shadow" /\ checkStep = 0
HistoricalNext ==
  \/ /\ checkStep = 0 /\ Discard
     /\ stored' = {"R0", "A", "L", "R2", "B", "M"}
     /\ putSet' = {"R2", "B", "M"} /\ foundSet' = {}
     /\ basisCurrent' /\ firstCurrent' = {"R2", "B", "M"}
     /\ checkStep' = 1
  \/ /\ checkStep = 1 /\ Discard
     /\ putSet' = {"R2", "B", "M"} /\ foundSet' = {"R2"}
     /\ foundPuts' = [p \in Pages |-> IF p = "R2" THEN 1 ELSE 0]
     /\ basisCurrent' /\ checkStep' = 2
  \/ /\ checkStep = 2 /\ SealCheckpoint
     /\ Len(waiting') = 1 /\ ~waiting'[1].record.large
     /\ Cardinality(waiting'[1].record.parts) = 1
     /\ waiting'[1].record.current /\ checkStep' = 3
  \/ /\ checkStep = 3 /\ ReleaseStage /\ staged' = {} /\ checkStep' = 4
  \/ /\ checkStep = 4 /\ Resolve("R2") /\ counting' /\ checkStep' = 5
  \/ /\ checkStep = 5 /\ Erase
     /\ {"R0", "A", "L"} \subseteq stored'
     /\ wrappers' = wrappers /\ retired' = retired
     /\ checkStep' = IF stored' = {"R0", "A", "L"} THEN 6 ELSE 5
  \/ /\ checkStep = 6 /\ PrepareRecord
     /\ waiting'[1].prepared = waiting'[1].record.parts /\ checkStep' = 7
  \/ /\ checkStep = 7 /\ Sight /\ checkStep' = 8
  \/ /\ checkStep = 8 /\ FinishRecount /\ doubt' /\ checkStep' = 9
  \/ /\ checkStep = 9 /\ CarryRecord
     /\ waiting' = <<>> /\ Len(queue') = 1 /\ ~queue'[1].current
     /\ checkStep' = 10
  \/ /\ checkStep = 10 /\ Sight /\ checkStep' = 11
  \/ /\ checkStep = 11 /\ FinishRecount /\ doubt' /\ checkStep' = 12
  \/ /\ checkStep = 12 /\ ProgressNext /\ checkStep' = 12
HistoricalSpec == HistoricalInit /\ [][HistoricalNext]_CheckVars /\ RetirementFair
                  /\ WF_CheckVars(HistoricalNext)
HistoricalPrefixReached == <>(checkStep = 12)

StrayInit == Init /\ ~Shadow /\ checkStep = 0
StrayNext ==
  \/ /\ checkStep = 0 /\ Discard
     /\ putSet' = {"R2", "B", "M"} /\ freshSet' = putSet'
     /\ basisCurrent' /\ checkStep' = 1
  \/ /\ checkStep = 1 /\ Name /\ checkStep' = 2
  \/ /\ checkStep = 2 /\ StartApply /\ checkStep' = 3
  \/ /\ checkStep = 3 /\ WalkStep /\ checkStep' = 3
  \/ /\ checkStep = 3 /\ EndApply /\ checkStep' = 4
  \/ /\ checkStep = 4 /\ (\E p \in Pages : Resolve(p)) /\ checkStep' = 4
  \/ /\ checkStep = 4 /\ (\A p \in Pages : rel[p] = 0) /\ UNCHANGED vars /\ checkStep' = 4
StraySpec == StrayInit /\ [][StrayNext]_CheckVars /\ WF_CheckVars(StrayNext)
StraySettles == <>Quiet

====
