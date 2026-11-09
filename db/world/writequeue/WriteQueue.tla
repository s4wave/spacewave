---- MODULE WriteQueue ----
\* Queue admission and durable receipt with a caller-provided transaction adapter.
\* The adapter pairs auxiliary SQL writes with the admitted World revision.
\* No durable wait holds the SQL writer. A retry discards before preparing.
\* Admitted replayable bodies remain obligations after their caller returns.
\* A body whose read only a peer serves leaves its batch and fetches with the
\* writer released; the writes of its actor wait for it, others go on.
EXTENDS Naturals
CONSTANTS EarlyApply, HoldSQL, DiscardOnRetry
\* The guards the deferred fetch relies on; the mutants clear them.
FetchReleasesWriter == TRUE
ActorGuard == TRUE
Writes == {1, 2}
Kinds == {"apply", "report"}
VARIABLE s
Init == s = [pc |-> [i \in Writes |-> "queued"],
             kind |-> [i \in Writes |-> CHOOSE k \in Kinds : TRUE],
             actor |-> [i \in Writes |-> 1],
             sql |-> [i \in Writes |-> FALSE],
             lock |-> 0, durable |-> {}, answered |-> {}, read |-> {},
             badRead |-> FALSE, failed |-> FALSE, cancelled |-> {}]
\* Enumerate the caller kinds and whether each body uses SQL independently.
Start ==
  /\ \A i \in Writes : s.pc[i] = "queued"
  /\ s.lock = 0 /\ s.answered = {} /\ s.cancelled = {}
  /\ \E kinds \in [Writes -> Kinds], rows \in [Writes -> BOOLEAN], actors \in [Writes -> {1, 2}] :
       s' = [s EXCEPT !.kind = kinds, !.sql = rows, !.actor = actors,
                     !.pc = [i \in Writes |-> "ready"]]
\* The states that hold the engine's writer; a fetch that kept the writer
\* would be one.
Writer == IF FetchReleasesWriter THEN {"body", "sql"} ELSE {"body", "sql", "fetching"}
\* A write waits while an earlier write of its actor fetches.
ActorFetching(i) == \E j \in Writes : j # i /\ s.actor[j] = s.actor[i] /\ s.pc[j] = "fetching"
\* NewTransaction serializes bodies, but an earlier receipt may be pending.
Body(i) ==
  /\ s.pc[i] = "ready" /\ s.lock = 0
  /\ \A j \in Writes : s.pc[j] \notin Writer
  /\ ActorGuard => ~ActorFetching(i)
  /\ s' = [s EXCEPT !.pc[i] = "body", !.lock = IF s.sql[i] THEN i ELSE 0]
\* A read only a peer serves defers: the batch is discarded, releasing the
\* writer and any SQL writer, and the write fetches. An answered replay may
\* have been seen, so it never moves behind later writes.
Defer(i) ==
  /\ s.pc[i] = "body" /\ i \notin s.answered
  /\ s' = [s EXCEPT !.pc[i] = "fetching",
                    !.lock = IF s.lock = i /\ FetchReleasesWriter THEN 0 ELSE @]
Fetched(i) ==
  /\ s.pc[i] = "fetching"
  /\ s' = [s EXCEPT !.pc[i] = "ready",
                    !.lock = IF s.lock = i THEN 0 ELSE @]
Admit(i) ==
  /\ s.pc[i] = "body"
  /\ s' = [s EXCEPT !.pc[i] = IF s.sql[i] THEN "sql" ELSE "waiting"]
\* A full publisher or busy state owner returns without admitting anything.
RetryPreparation(i) ==
  /\ s.pc[i] = "body"
  /\ s' = [s EXCEPT !.pc[i] = "preparing", !.lock = IF DiscardOnRetry THEN 0 ELSE @]
Prepared(i) ==
  /\ s.pc[i] = "preparing"
  /\ s' = [s EXCEPT !.pc[i] = "ready"]
\* SQL commits locally at admission and releases before the receipt wait.
CommitSQL(i) ==
  /\ s.pc[i] = "sql"
  /\ s' = [s EXCEPT !.pc[i] = "waiting", !.lock = IF HoldSQL THEN i ELSE 0]
\* report answers after admit returns; apply waits for settle.
AnswerEarly(i) ==
  /\ s.pc[i] = "waiting" /\ i \notin s.answered
  /\ (s.kind[i] = "report" \/ EarlyApply)
  /\ s' = [s EXCEPT !.answered = @ \cup {i}]
Settle(i) ==
  /\ s.pc[i] = "waiting"
  /\ s' = [s EXCEPT !.pc[i] = "durable", !.durable = @ \cup {i},
                    !.lock = IF s.lock = i THEN 0 ELSE @]
AnswerDurable(i) ==
  /\ s.pc[i] = "durable" /\ i \notin s.answered
  /\ s' = [s EXCEPT !.answered = @ \cup {i}]
ReadOwn(i) ==
  /\ i \in s.answered /\ s.kind[i] = "apply" /\ i \notin s.read
  /\ s' = [s EXCEPT !.read = @ \cup {i},
                    !.badRead = @ \/ i \notin s.durable]
\* A failed publication reinstalls the durable head. Pending apply callers
\* hear failure; acknowledged reports replay independently of their callers.
Drop ==
  /\ ~s.failed /\ s.lock = 0
  /\ \E i \in Writes : s.pc[i] = "waiting"
  /\ s' = [s EXCEPT !.failed = TRUE,
       !.pc = [i \in Writes |-> IF s.pc[i] = "waiting"
          THEN IF s.kind[i] = "report" /\ i \in s.answered THEN "ready" ELSE "error"
          ELSE s.pc[i]]]
Cancel(i) ==
  /\ i \notin s.answered /\ i \notin s.cancelled
  /\ s.pc[i] \in {"ready", "preparing", "fetching", "waiting", "durable"}
  /\ s' = [s EXCEPT !.cancelled = @ \cup {i},
       !.pc[i] = IF @ \in {"ready", "preparing"} THEN "cancelled" ELSE @]
Next == Start \/ Drop \/ (\E i \in Writes : Body(i) \/ Defer(i) \/ Fetched(i) \/ Admit(i) \/ RetryPreparation(i) \/ Prepared(i) \/ CommitSQL(i)
        \/ AnswerEarly(i) \/ Settle(i) \/ AnswerDurable(i) \/ ReadOwn(i) \/ Cancel(i))
        \/ UNCHANGED s
Spec == Init /\ [][Next]_s
NoWaitUnderSQLWriter == \A i \in Writes : s.pc[i] \in {"waiting", "preparing"} => s.lock # i
ReadYourWrites == ~s.badRead
\* A fetching write holds neither the SQL writer nor the engine's writer.
NoFetchUnderWriter == \A i \in Writes : s.pc[i] = "fetching" => s.lock # i
\* No write runs while an earlier write of its actor fetches, and no write
\* answered at admission fetches.
ActorWaitsForFetch == \A i \in Writes :
  (s.pc[i] = "fetching" => i \notin s.answered) /\
  (s.pc[i] \in {"body", "sql"} => ~ActorFetching(i))
DurableSurvives == \A i \in s.durable : s.pc[i] = "durable"
====
