---- MODULE ListVerifier ----
\* records.go GetState is the one generation-proof boundary for every caller.
\* Earlier verifiers may leave Open, Sweeping or Settled, with any adoption
\* state. A retained unknown-field parent's short list omits a stored child.
\* A current mark is also reachable after an interrupted current walk; its
\* pending front entry must survive until an undecodable read revokes it.
\* Counts are abstracted by adoption. Settle.tla covers their individual moves.
EXTENDS Integers
CONSTANT Skip
VARIABLE s
vars == <<s>>
Current == 2
Marked == s.list = Current \/ Skip = "list"
Finished == s.visit = Current \/ Skip = "visit"
Complete == s.complete = Current \/ Skip = "run"
Generation == IF s.revision = Current \/ Skip = s.gen
                THEN s.gen ELSE "open"
Adoption == IF s.revision = Current \/ Skip = s.gen
              THEN s.adoption ELSE "unadopted"

Init ==
  \/ \E gen \in {"open", "sweeping", "settled"},
        adoption \in {"unadopted", "adopting", "adopted"},
        list \in {0, 1}:
       s = [list |-> list, visit |-> 1, complete |-> 1, run |-> gen = "sweeping",
            front |-> FALSE, seeded |-> FALSE, walked |-> FALSE,
            skipped |-> FALSE, gen |-> gen, revision |-> 1,
            adoption |-> adoption, child |-> TRUE]
  \/ s = [list |-> Current, visit |-> 1, complete |-> 0, run |-> FALSE,
          front |-> TRUE, seeded |-> FALSE, walked |-> FALSE,
          skipped |-> FALSE, gen |-> "open", revision |-> Current,
          adoption |-> "adopted", child |-> TRUE]

\* A state write persists GetState's normalized generation and starts recount.
Recount ==
  /\ Adoption # "adopted"
  /\ s' = [s EXCEPT !.gen = Generation, !.revision = Current,
                     !.adoption = "adopted"]

BuildRun ==
  /\ Generation = "open" /\ ~Complete
  /\ s' = [s EXCEPT !.run = ~Marked, !.complete = Current]

Seed ==
  /\ Generation = "open" /\ Complete /\ ~s.seeded
  /\ s' = [s EXCEPT !.front = @ \/ ~Marked, !.seeded = TRUE]

Walk ==
  /\ Generation = "open" /\ ~s.walked
  /\ IF Finished
       THEN s' = [s EXCEPT !.walked = TRUE, !.skipped = TRUE]
       ELSE s' = [s EXCEPT !.list = -1, !.front = TRUE, !.run = TRUE,
                            !.walked = TRUE]

Crash ==
  /\ s.seeded \/ s.walked
  /\ s' = [s EXCEPT !.seeded = FALSE, !.walked = FALSE]

Flip ==
  /\ Generation = "open" /\ Adoption = "adopted"
  /\ Complete /\ s.seeded /\ ~s.front
  /\ s' = [s EXCEPT !.gen = "sweeping", !.revision = Current]

\* Sweep and the ordinary Settled zero-holder free share the same state read.
Free ==
  /\ Generation \in {"sweeping", "settled"} /\ Adoption = "adopted"
  /\ s.child
  /\ s' = [s EXCEPT !.child = FALSE]

Next == Recount \/ BuildRun \/ Seed \/ Walk \/ Crash \/ Flip \/ Free
Spec == Init /\ [][Next]_vars
ChildSurvives == s.child
VisitsCurrent == ~s.skipped
RunIncludesStale == Complete /\ s.list # Current => s.run
CurrentRevoked == s.walked /\ ~s.skipped => s.list # Current /\ s.front /\ s.run
====
