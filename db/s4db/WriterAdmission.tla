---- MODULE WriterAdmission ----
\* Proposed shared arbiter, NOT native writer correspondence. Native s4db
\* has process-local served/waiting counters and aggregate remote intent
\* bytes; it has no shared request queue, cohort boundary or consumed turn.
\* These atomic actions require a new cross-process coordination primitive.
\*
\* Two foreground actors repeatedly register/cancel; two background calls
\* start queued behind W. Requests may belong to different processes: every
\* actor uses this ONE abstract arbiter, not one arbiter per handle/process.
\* A cohort snapshots the foreground queue. Later arrivals wait for the next
\* cohort; after a cohort drains, exactly one background slice is reserved.
\* Consuming that turn clears it for ALL background calls. With no foreground
\* demand, background calls run directly. Neither class has priority levels.
\*
\* Cancellation removes a queued request, including a cohort member, without
\* consuming a background turn. Finish represents release on success, failure
\* or owner death after admission. Cancel also abstracts atomic removal of a
\* dead queued process. Real crash detection, slot reuse, grant-vs-cancel
\* linearization, notifications and OS-lock acquisition are obligations of
\* the missing native primitive, not established by this abstract model.
\* Refresh, transaction bodies, disk errors and durations are omitted. Weak
\* fairness assumes every holder releases and every enabled arbiter action
\* runs. Counters saturate at two; they witness violations, not scheduling.
\* SharedAging=FALSE is the rejected per-call policy: cohort completion gives
\* each background invocation an independent permission to bypass a waiter.
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
  /\ passed' = [passed EXCEPT ![f] = IF holder \in Background THEN 1 ELSE 0]
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
