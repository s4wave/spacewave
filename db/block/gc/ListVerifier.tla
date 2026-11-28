---- MODULE ListVerifier ----
\* records.go GetState is the one generation-proof boundary for every caller.
\* Earlier verifiers may leave Open, Sweeping or Settled, with any adoption
\* state. A retained unknown-field parent's short list omits a stored child.
\* A current mark is also reachable after an interrupted current walk; its
\* pending front entry must survive until an undecodable read revokes it.
\* Current settle and copied reopen preserve and invalidate the flip mark.
\* Counts are abstracted by adoption. Settle.tla covers their individual moves.
EXTENDS Integers
CONSTANT Skip
VARIABLE s
vars == <<s>>
Current == 2
Marked == s.list = Current \/ Skip = "list"
Finished == s.visit = Current \/ Skip = "visit"
Complete == s.complete = Current \/ Skip = "run"
Writer == s.revision = Current /\ (s.gen = "open" \/ s.flip = Current)
Generation == IF Writer \/ Skip = s.gen
                THEN s.gen ELSE "open"
Adoption == IF Writer \/ Skip = s.gen
              THEN s.adoption ELSE "unadopted"

Init ==
  \/ \E gen \in {"open", "sweeping", "settled"},
        adoption \in {"unadopted", "adopting", "adopted"},
        list \in {0, 1}:
       s = [list |-> list, visit |-> 1, complete |-> 1, run |-> gen = "sweeping",
            front |-> FALSE, seeded |-> FALSE, walked |-> FALSE,
            skipped |-> FALSE, gen |-> gen, revision |-> 1,
            adoption |-> adoption, flip |-> 1, keep |-> FALSE, child |-> TRUE]
  \/ s = [list |-> Current, visit |-> 1, complete |-> 0, run |-> FALSE,
          front |-> TRUE, seeded |-> FALSE, walked |-> FALSE,
          skipped |-> FALSE, gen |-> "open", revision |-> Current,
          adoption |-> "adopted", flip |-> 0, keep |-> FALSE, child |-> TRUE]

  \/ \E gen \in {"sweeping", "settled"}:
       s = [list |-> Current, visit |-> 1, complete |-> Current, run |-> FALSE,
            front |-> FALSE, seeded |-> FALSE, walked |-> FALSE,
            skipped |-> FALSE, gen |-> gen, revision |-> Current,
            adoption |-> "adopted", flip |-> Current, keep |-> TRUE, child |-> TRUE]

\* A state write persists GetState's normalized generation and starts recount.
Recount ==
  /\ Adoption # "adopted"
  /\ s' = [s EXCEPT !.gen = Generation, !.revision = Current,
                     !.adoption = "adopted", !.keep = FALSE]

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
  /\ s' = [s EXCEPT !.gen = "sweeping", !.revision = Current,
                     !.flip = Current, !.keep = TRUE]

\* Sweep and the ordinary Settled zero-holder free share the same state read.
Free ==
  /\ Generation \in {"sweeping", "settled"} /\ Adoption = "adopted"
  /\ s.child /\ s.list # Current
  /\ s' = [s EXCEPT !.child = FALSE]

\* An older writer preserves the unknown revision but writes its own proof.
\* Before revision fencing it can accept a carried run-completion entry;
\* neither form writes the distinct mark that a current flip requires.
OlderFlip ==
  /\ s.gen = "open" /\ s.adoption = "adopted"
  /\ s' = [s EXCEPT !.gen = "sweeping", !.complete = 1,
                     !.flip = IF @ = Current THEN Current ELSE 1,
                     !.list = 1, !.run = TRUE, !.keep = FALSE]

\* The old writer deletes the completion entry when its run settles.
OlderFinish ==
  /\ s.gen = "sweeping" /\ s.flip # Current
  /\ s' = [s EXCEPT !.gen = "settled", !.complete = 0, !.flip = 0, !.keep = FALSE]

\* Finishing a current sweep retains its writer certificate.
Settle ==
  /\ Generation = "sweeping"
  /\ s' = [s EXCEPT !.gen = "settled"]

\* A copied short list reopens a current generation and replaces its mark.
Reopen ==
  /\ Generation = "settled"
  /\ s' = [s EXCEPT !.gen = "open", !.flip = 0, !.complete = Current,
                     !.list = 1, !.run = TRUE, !.front = TRUE,
                     !.seeded = FALSE, !.walked = FALSE, !.keep = FALSE]

Next == Settle \/ Reopen \/ OlderFlip \/ OlderFinish \/ Recount \/ BuildRun \/ Seed \/ Walk \/ Crash \/ Flip \/ Free
Spec == Init /\ [][Next]_vars
ChildSurvives == s.child
VisitsCurrent == ~s.skipped
RunIncludesStale == Complete /\ s.list # Current => s.run
CurrentRevoked == s.walked /\ ~s.skipped => s.list # Current /\ s.front /\ s.run
\* keep records a current proof not superseded by recount, reopen or old flip.
SettledProof == s.keep => s.flip = Current
====
