---- MODULE SubscriptionWriter ----
EXTENDS Naturals
(* stream.go queueSubscriptions/takeSubscriptions and executeSession's sole
   SendMsg writer. reconcile and initial peer startup call queueSubscriptions.
   One topic, two desired-state changes, one in-flight packet; network can stall
   between take and send while the producer reverses its desired state.
   announced advances at take, before transport delivery. Publication queue
   drops are allowed and do not affect subscription order. Cancellation ends
   this session; reconnect seeds a fresh session, outside this model.
   Eventual convergence assumes transport resumes and producer quiescence.
   Each mutex section is atomic. Wake is a size-one coalesced channel. *)
CONSTANT AdvanceAtTake
VARIABLES desired, announced, pending, flight, remote, changes, wake, phase, closed
vars == <<desired, announced, pending, flight, remote, changes, wake, phase, closed>>
Init == /\ desired = 0 /\ announced = 0 /\ pending = 2
        /\ flight = 2 /\ remote = 0 /\ changes = 0
        /\ wake = FALSE /\ phase = "take" /\ closed = FALSE
QueueSubscriptions ==
  /\ ~closed /\ changes < 2
  /\ desired' = 1 - desired /\ changes' = changes + 1
  /\ pending' = IF announced = 1 - desired THEN 2 ELSE 1 - desired
  /\ wake' = TRUE
  /\ UNCHANGED <<announced, flight, remote, phase, closed>>
TakeSubscriptions ==
  /\ ~closed /\ phase = "take"
  /\ flight' = pending
  /\ announced' = IF AdvanceAtTake /\ pending # 2 THEN pending ELSE announced
  /\ pending' = 2
  /\ phase' = IF pending = 2 THEN "select" ELSE "send"
  /\ UNCHANGED <<desired, remote, changes, wake, closed>>
SendMsg ==
  /\ ~closed /\ phase = "send"
  /\ remote' = flight
  /\ announced' = IF AdvanceAtTake THEN announced ELSE flight
  /\ flight' = 2 /\ phase' = "select"
  /\ UNCHANGED <<desired, pending, changes, wake, closed>>
SelectWake == /\ ~closed /\ phase = "select" /\ wake
              /\ wake' = FALSE /\ phase' = "take"
              /\ UNCHANGED <<desired, announced, pending, flight, remote, changes, closed>>
Cancel == /\ ~closed /\ closed' = TRUE
          /\ UNCHANGED <<desired, announced, pending, flight, remote, changes, wake, phase>>
Next == QueueSubscriptions \/ TakeSubscriptions \/ SendMsg \/ SelectWake \/ Cancel \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars /\ WF_vars(TakeSubscriptions)
        /\ WF_vars(SendMsg) /\ WF_vars(SelectWake)
Coalesced == pending = 2 => desired = announced
Bounded == changes <= 2
Converges == <>[](closed \/ (remote = desired /\ pending = 2 /\ flight = 2))
====
