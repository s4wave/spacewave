------------------------------ MODULE PureStep ------------------------------
EXTENDS Naturals, Sequences, FiniteSets
CONSTANTS Devices, MaxArrivals, Poison
VARIABLES phase, pending, arrived, consumed, rev, eval, dstate, registered, fixed,
          ver, batch
vars == <<phase, pending, arrived, consumed, rev, eval, dstate, registered, fixed,
          ver, batch>>

(* Models one pure Step of a standing run on every Device hosting the run.
   A run controller with the Step's function registered reads the Step's
   arrivals and its state object revision, evaluates the function outside
   any World transaction, and commits the result in one World operation that
   consumes exactly the arrivals it read, writes the next state guarded by
   the revision it read, and routes the chosen port. A commit whose arrivals
   or revision moved is discarded and the Step is evaluated again.

   The commit is not one atomic compare-and-set on the World. The Device
   decides it in a write transaction on its own head, which records whole
   object writes (batch), and the Space places the operation later; when
   another Device's operation was placed first, the Space replays the
   recorded batch onto the new head. ver counts the writes to the run and
   state objects, so a batch's base is the root it expects. The recorded
   expected-root guard (world/expect-roots) refuses a replay onto a moved
   root, and the writer then decides again on the new head.

   An evaluation is volatile: a restart forgets it. The function is
   deterministic, so it faults on exactly the arrival sets holding Poison;
   a fault pauses the Step with its arrivals kept until an operator resumes
   it with the function fixed. A Device without the function leaves the
   Step Ready under a binding watch. *)
NoEval == [in |-> {}, rev |-> 0]
NoBatch == [in |-> {}, base |-> 0, fault |-> FALSE, pend |-> {}, rev |-> 0, phase |-> "None"]
Arrivals == 1..MaxArrivals

Init == /\ phase = "Waiting"
        /\ pending = {}
        /\ arrived = 0
        /\ consumed = <<>>
        /\ rev = 0
        /\ eval = [d \in Devices |-> NoEval]
        /\ dstate = [d \in Devices |-> "Idle"]
        /\ registered \in [Devices -> BOOLEAN]
        /\ fixed = FALSE
        /\ ver = 0
        /\ batch = [d \in Devices |-> NoBatch]

\* Arrive: an upstream route or DeliverEvents stores one arrival at the Step.
Arrive ==
  /\ arrived < MaxArrivals
  /\ arrived' = arrived + 1
  /\ pending' = pending \cup {arrived + 1}
  /\ phase' = IF phase \in {"Waiting", "Routed"} THEN "Ready" ELSE phase
  /\ ver' = ver + 1
  /\ UNCHANGED <<consumed, rev, eval, dstate, registered, fixed, batch>>

\* Settle: the routed Step waits for, or is ready with, its next arrivals.
Settle ==
  /\ phase = "Routed"
  /\ phase' = IF pending = {} THEN "Waiting" ELSE "Ready"
  /\ ver' = ver + 1
  /\ UNCHANGED <<pending, arrived, consumed, rev, eval, dstate, registered, fixed, batch>>

\* Unavailable: a Device without the function waits on its binding.
Unavailable(d) ==
  /\ phase = "Ready"
  /\ ~registered[d]
  /\ dstate[d] = "Idle"
  /\ dstate' = [dstate EXCEPT ![d] = "Unavailable"]
  /\ UNCHANGED <<phase, pending, arrived, consumed, rev, eval, registered, fixed, ver, batch>>

\* Register: the binding watch sees the function registered on the Device.
Register(d) ==
  /\ ~registered[d]
  /\ registered' = [registered EXCEPT ![d] = TRUE]
  /\ dstate' = IF dstate[d] = "Unavailable" THEN [dstate EXCEPT ![d] = "Idle"] ELSE dstate
  /\ UNCHANGED <<phase, pending, arrived, consumed, rev, eval, fixed, ver, batch>>

\* Evaluate: the Device reads a ready set of the arrivals, one per input port
\* in the code, and the state revision.
Evaluate(d) ==
  /\ phase = "Ready"
  /\ registered[d]
  /\ dstate[d] = "Idle"
  /\ \E S \in SUBSET pending \ {{}} :
       eval' = [eval EXCEPT ![d] = [in |-> S, rev |-> rev]]
  /\ dstate' = [dstate EXCEPT ![d] = "Evaluating"]
  /\ UNCHANGED <<phase, pending, arrived, consumed, rev, registered, fixed, ver, batch>>

\* Faults is TRUE when the deterministic function fails on the inputs.
Faults(in) == Poison \in in /\ ~fixed

\* Decided is the batch the Device's write transaction records: whole writes
\* of the run's arrivals and phase and of the state, with the expected root.
Decided(d, f) == [in |-> eval[d].in, base |-> ver, fault |-> f,
                  pend |-> IF f THEN pending ELSE pending \ eval[d].in,
                  rev |-> IF f THEN rev ELSE rev + 1,
                  phase |-> IF f THEN "Faulted" ELSE "Routed"]

\* Commit: the Device's write transaction finds the evaluated arrivals and
\* revision current on its head and records the commit: consume the arrivals,
\* write the next state, and route the chosen port.
Commit(d) ==
  /\ dstate[d] = "Evaluating"
  /\ phase = "Ready"
  /\ eval[d].in \subseteq pending /\ eval[d].rev = rev
  /\ ~Faults(eval[d].in)
  /\ batch' = [batch EXCEPT ![d] = Decided(d, FALSE)]
  /\ dstate' = [dstate EXCEPT ![d] = "Placing"]
  /\ UNCHANGED <<phase, pending, arrived, consumed, rev, eval, registered, fixed, ver>>

\* Fault: the write transaction records the pause, keeping the arrivals.
Fault(d) ==
  /\ dstate[d] = "Evaluating"
  /\ phase = "Ready"
  /\ eval[d].in \subseteq pending /\ eval[d].rev = rev
  /\ Faults(eval[d].in)
  /\ batch' = [batch EXCEPT ![d] = Decided(d, TRUE)]
  /\ dstate' = [dstate EXCEPT ![d] = "Placing"]
  /\ UNCHANGED <<phase, pending, arrived, consumed, rev, eval, registered, fixed, ver>>

\* Placed: the Space places the operation onto a head where the run and state
\* still hold the expected root, so the recorded writes apply as decided.
Placed(d) ==
  /\ dstate[d] = "Placing"
  /\ batch[d].base = ver
  /\ pending' = batch[d].pend
  /\ rev' = batch[d].rev
  /\ phase' = batch[d].phase
  /\ consumed' = IF batch[d].fault THEN consumed ELSE Append(consumed, batch[d].in)
  /\ ver' = ver + 1
  /\ batch' = [batch EXCEPT ![d] = NoBatch]
  /\ eval' = [eval EXCEPT ![d] = NoEval]
  /\ dstate' = [dstate EXCEPT ![d] = "Idle"]
  /\ UNCHANGED <<arrived, registered, fixed>>

\* Refused: the replay onto a moved root fails the expected-root guard, so the
\* Space refuses the operation and the writer decides again on the new head.
Refused(d) ==
  /\ dstate[d] = "Placing"
  /\ batch[d].base # ver
  /\ batch' = [batch EXCEPT ![d] = NoBatch]
  /\ dstate' = [dstate EXCEPT ![d] = "Evaluating"]
  /\ UNCHANGED <<phase, pending, arrived, consumed, rev, eval, registered, fixed, ver>>

\* LostRace: the write transaction finds the arrivals, revision or Step moved,
\* so the evaluation is discarded.
LostRace(d) ==
  /\ dstate[d] = "Evaluating"
  /\ \/ ~(eval[d].in \subseteq pending)
     \/ eval[d].rev # rev
     \/ phase # "Ready"
  /\ eval' = [eval EXCEPT ![d] = NoEval]
  /\ dstate' = [dstate EXCEPT ![d] = "Idle"]
  /\ UNCHANGED <<phase, pending, arrived, consumed, rev, registered, fixed, ver, batch>>

\* Restart: the Device's run controller restarts and forgets its evaluation;
\* an operation already submitted to the Space is still placed.
Restart(d) ==
  /\ dstate[d] # "Placing"
  /\ eval' = [eval EXCEPT ![d] = NoEval]
  /\ dstate' = [dstate EXCEPT ![d] = "Idle"]
  /\ UNCHANGED <<phase, pending, arrived, consumed, rev, registered, fixed, ver, batch>>

\* Resume: an operator fixes the function and resumes the paused Step.
Resume ==
  /\ phase = "Faulted"
  /\ phase' = "Ready"
  /\ fixed' = TRUE
  /\ ver' = ver + 1
  /\ UNCHANGED <<pending, arrived, consumed, rev, eval, dstate, registered, batch>>

Next == \/ Arrive \/ Settle \/ Resume
        \/ \E d \in Devices :
             \/ Unavailable(d) \/ Register(d) \/ Evaluate(d) \/ Commit(d)
             \/ Fault(d) \/ Placed(d) \/ Refused(d) \/ LostRace(d) \/ Restart(d)
Spec == Init /\ [][Next]_vars

Range(s) == {s[i] : i \in DOMAIN s}

TypeOK ==
  /\ phase \in {"Waiting", "Ready", "Routed", "Faulted"}
  /\ dstate \in [Devices -> {"Idle", "Evaluating", "Placing", "Unavailable"}]
  /\ pending \subseteq Arrivals

\* RevisionPerCommit: the state revision advances exactly once per consumed
\* arrival set, and only through the commit.
RevisionPerCommit == rev = Len(consumed)

\* ExactlyOnce: no arrival is consumed twice, or consumed and still pending.
ExactlyOnce ==
  /\ \A i, j \in DOMAIN consumed : i # j => consumed[i] \cap consumed[j] = {}
  /\ \A i \in DOMAIN consumed : consumed[i] \cap pending = {}

\* NoLoss: every arrival is pending or consumed.
NoLoss == 1..arrived = pending \cup UNION Range(consumed)

\* FaultKeepsInputs: a paused Step still holds the arrivals that faulted.
FaultKeepsInputs == phase = "Faulted" => Poison \in pending
=============================================================================
