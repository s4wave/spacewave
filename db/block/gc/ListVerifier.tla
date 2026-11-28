---- MODULE ListVerifier ----
\* Certificate checks during an interrupted Open-generation reconcile upgrade.
\* records.go listMarked/listKeys, settle.go runSlice/seed/flip, Cloud's
\* listVisits.Finished and ListRebuild.Undecodable are the modeled callers.
\* One held parent has an unknown-field child omitted by the old verifier.
\* The old list, visit and run certificates are durable before this binary
\* starts. Crash drops traversal state but preserves every committed record.
\* Counts and byte decoding are abstracted: the omitted child has no holder,
\* and the fixed exact decoder refuses its parent. Settle.tla covers counts.
\* This model covers Open, not a generation an earlier verifier already flipped.
EXTENDS Integers
CONSTANT Skip
VARIABLE s
vars == <<s>>
Current == 1
Marked == s.list = Current \/ Skip = "list"
Finished == s.visit = Current \/ Skip = "visit"
Complete == s.complete = Current \/ Skip = "run"

Init == s = [list |-> 0, visit |-> 0, complete |-> 0, run |-> FALSE,
             front |-> FALSE, seeded |-> FALSE, walked |-> FALSE,
             skipped |-> FALSE, gen |-> "open", child |-> TRUE]

\* A stale run proof cannot claim a scan that skipped earlier marked lists.
BuildRun ==
  /\ s.gen = "open"
  /\ ~Complete
  /\ s' = [s EXCEPT !.run = ~Marked, !.complete = Current]

\* Seeding happens after the run build, as in Collector.settle.
Seed ==
  /\ s.gen = "open" /\ Complete /\ ~s.seeded
  /\ s' = [s EXCEPT !.front = @ \/ ~Marked, !.seeded = TRUE]

\* A stale visit is no evidence for skipping a subtree under this decoder.
Walk ==
  /\ s.gen = "open" /\ ~s.walked
  /\ IF Finished
       THEN s' = [s EXCEPT !.walked = TRUE, !.skipped = TRUE]
       ELSE s' = [s EXCEPT !.list = -1, !.front = TRUE, !.run = TRUE,
                            !.walked = TRUE]

\* A process can stop after any transaction, including before its walk.
Crash ==
  /\ s.gen = "open" /\ (s.seeded \/ s.walked)
  /\ s' = [s EXCEPT !.seeded = FALSE, !.walked = FALSE]

Flip ==
  /\ s.gen = "open" /\ Complete /\ s.seeded /\ ~s.front
  /\ s' = [s EXCEPT !.gen = "sweeping"]

Free ==
  /\ s.gen = "sweeping" /\ s.child
  /\ s' = [s EXCEPT !.child = FALSE]

Next == BuildRun \/ Seed \/ Walk \/ Crash \/ Flip \/ Free
Spec == Init /\ [][Next]_vars
ChildSurvives == s.child
VisitsCurrent == ~s.skipped
RunIncludesStale == Complete /\ s.list # Current => s.run
====
