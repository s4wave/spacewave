---- MODULE WriterAdmission ----
\* Native single-process writerAdmission: one writable handle owns each file.
\* W starts as an already-running background slice; it counts toward the bound.
\* All queue changes are protected by writerAdmission.mtx; bodies and native
\* writer-byte acquisition happen after that mutex releases.
\* Want maps to lock's enqueue. StartCohort captures the foreground FIFO tail;
\* the implementation keeps one linked FIFO and a boundary, not a queue copy.
\* TakeForeground/TakeBackground map to grant, including close(ready).
\* CloseCohort maps to dispatch observing a drained boundary after release.
\* dispatch executes several of these abstract actions in one mutex hold.
\* Cancel maps to unlink plus moving a cancelled tail to its predecessor.
\* A cancellation after grant maps to Finish, just as native lock I/O failure,
\* transaction commit/discard and scheduler-slice completion return the grant.
\* Returning a grant and dispatching its successor share one mutex hold.
\* The optional closing mark only grants when both queues are empty.
\* Shutdown stops new transactions, drains counted writers, then cancels and
\* joins scheduler callers. Process death drops ownership and all local queues.
\* The model has two recurring foreground actors and two background slices.
\* Caller bodies, storage, scheduling and memtable backpressure have no time
\* bound. Weak fairness assumes granted holders eventually finish.
\* SharedAging=FALSE retains the independent per-call-aging counterexample.
EXTENDS Naturals, Sequences, FiniteSets
Foreground == {"F1", "F2"}
Background == {"B1", "B2"}
Actors == Foreground \cup Background
SharedAging == TRUE
AgeKeys == IF SharedAging THEN {"store"} ELSE Background
AgeKey(b) == IF SharedAging THEN "store" ELSE b
VARIABLES holder, pending, cohort, backgrounds, stage, aged, done, passed
vars == <<holder, pending, cohort, backgrounds, stage, aged, done, passed>>
Members(q) == {q[i] : i \in 1..Len(q)}
Waiting == Members(pending) \cup Members(cohort)
ReadyBackground == IF Len(backgrounds) = 0 THEN FALSE
  ELSE aged[AgeKey(Head(backgrounds))]
Init ==
  /\ holder = "W" /\ pending = <<>> /\ cohort = <<>>
  /\ backgrounds = <<"B1", "B2">> /\ stage = "open"
  /\ aged = [k \in AgeKeys |-> FALSE] /\ done = {}
  /\ passed = [f \in Foreground |-> 0]
Want(f) ==
  /\ f \notin Waiting /\ holder # f
  /\ pending' = Append(pending, f)
  \* Count a slice already holding writer, as well as later grants.
  /\ passed' = [passed EXCEPT ![f] = IF holder \in Background \cup {"W"} THEN 1 ELSE 0]
  /\ UNCHANGED <<holder, cohort, backgrounds, stage, aged, done>>
StartCohort ==
  /\ holder = "none" /\ stage = "open" /\ Len(pending) > 0
  /\ ~ReadyBackground
  /\ cohort' = pending /\ pending' = <<>> /\ stage' = "foreground"
  /\ UNCHANGED <<holder, backgrounds, aged, done, passed>>
TakeForeground ==
  /\ holder = "none" /\ stage = "foreground" /\ Len(cohort) > 0
  /\ holder' = Head(cohort) /\ cohort' = Tail(cohort)
  /\ UNCHANGED <<pending, backgrounds, stage, aged, done, passed>>
CloseCohort ==
  /\ holder = "none" /\ stage = "foreground" /\ Len(cohort) = 0
  /\ stage' = "open"
  /\ aged' = [k \in AgeKeys |-> TRUE]
  /\ UNCHANGED <<holder, pending, cohort, backgrounds, done, passed>>
TakeBackground ==
  /\ holder = "none" /\ stage = "open" /\ Len(backgrounds) > 0
  /\ (Len(pending) = 0 \/ ReadyBackground)
  /\ LET b == Head(backgrounds) IN
       /\ holder' = b /\ backgrounds' = Tail(backgrounds)
       /\ aged' = [aged EXCEPT ![AgeKey(b)] = FALSE]
  /\ passed' = [f \in Foreground |-> IF f \in Waiting
       THEN IF passed[f] < 2 THEN passed[f] + 1 ELSE 2 ELSE passed[f]]
  /\ UNCHANGED <<pending, cohort, stage, done>>
Finish ==
  /\ holder # "none" /\ holder' = "none"
  /\ done' = IF holder \in Background THEN done \cup {holder} ELSE done
  /\ UNCHANGED <<pending, cohort, backgrounds, stage, aged, passed>>
Cancel(f) ==
  /\ f \in Waiting
  /\ pending' = SelectSeq(pending, LAMBDA a : a # f)
  /\ cohort' = SelectSeq(cohort, LAMBDA a : a # f)
  /\ UNCHANGED <<holder, backgrounds, stage, aged, done, passed>>
CancelBackground(b) ==
  /\ b \in Members(backgrounds)
  /\ backgrounds' = SelectSeq(backgrounds, LAMBDA a : a # b)
  /\ done' = done \cup {b}
  /\ UNCHANGED <<holder, pending, cohort, stage, aged, passed>>
\* Dispatch is one enabled arbiter operation even while arrivals/cancellation
\* change which branch it selects; separate branch fairness is insufficient.
Dispatch == StartCohort \/ TakeBackground
Next == Dispatch \/ TakeForeground \/ CloseCohort \/ Finish
  \/ (\E f \in Foreground : Want(f) \/ Cancel(f))
  \/ (\E b \in Background : CancelBackground(b))
Spec == Init /\ [][Next]_vars
  /\ WF_vars(Dispatch) /\ WF_vars(TakeForeground) /\ WF_vars(CloseCohort)
  /\ WF_vars(Finish)
QueueState ==
  /\ holder \in Actors \cup {"W", "none"}
  /\ stage \in {"open", "foreground"}
  /\ pending \in Seq(Foreground) /\ cohort \in Seq(Foreground)
  /\ backgrounds \in Seq(Background) /\ done \subseteq Background
  /\ aged \in [AgeKeys -> BOOLEAN] /\ passed \in [Foreground -> 0..2]
  /\ Len(pending) = Cardinality(Members(pending))
  /\ Len(cohort) = Cardinality(Members(cohort))
  /\ Members(pending) \intersect Members(cohort) = {}
  /\ holder \notin Waiting /\ holder \notin Members(backgrounds)
  /\ Len(backgrounds) = Cardinality(Members(backgrounds))
  /\ Members(backgrounds) \intersect done = {} /\ holder \notin done
  /\ Background = Members(backgrounds) \cup done \cup ({holder} \intersect Background)
  /\ (stage = "open" => Len(cohort) = 0)
\* Stronger than the head-only bound: includes every announced waiter and an
\* already-running slice. Existing foreground work ahead is not background.
ForegroundPassedAtMostOnce == \A f \in Waiting : passed[f] <= 1
\* Completion includes explicit cancellation; a call that stays queued must
\* acquire and release. Foreground arrivals may continue forever.
BackgroundFinishes == \A b \in Background : <>(b \in done)
ForegroundFinishes == \A f \in Foreground : (f \in Waiting) ~> (f \notin Waiting)
====
