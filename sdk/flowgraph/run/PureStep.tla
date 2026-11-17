------------------------------ MODULE PureStep ------------------------------
EXTENDS Naturals, Sequences, FiniteSets
CONSTANTS Devices, MaxArrivals, Poison
VARIABLES phase, pending, arrived, consumed, rev, eval, dstate, registered, fixed
vars == <<phase, pending, arrived, consumed, rev, eval, dstate, registered, fixed>>

(* Models one pure Step of a standing run on every Device hosting the run.
   A run controller with the Step's function registered reads the Step's
   arrivals and its state object revision, evaluates the function outside
   any World transaction, and commits the result in one World operation that
   consumes exactly the arrivals it read, writes the next state guarded by
   the revision it read, and routes the chosen port. A commit whose arrivals
   or revision moved is discarded and the Step is evaluated again.

   An evaluation is volatile: a restart forgets it. The function is
   deterministic, so it faults on exactly the arrival sets holding Poison;
   a fault pauses the Step with its arrivals kept until an operator resumes
   it with the function fixed. A Device without the function leaves the
   Step Ready under a binding watch. *)
NoEval == [in |-> {}, rev |-> 0]
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

\* Arrive: an upstream route or DeliverEvents stores one arrival at the Step.
Arrive ==
  /\ arrived < MaxArrivals
  /\ arrived' = arrived + 1
  /\ pending' = pending \cup {arrived + 1}
  /\ phase' = IF phase \in {"Waiting", "Routed"} THEN "Ready" ELSE phase
  /\ UNCHANGED <<consumed, rev, eval, dstate, registered, fixed>>

\* Settle: the routed Step waits for, or is ready with, its next arrivals.
Settle ==
  /\ phase = "Routed"
  /\ phase' = IF pending = {} THEN "Waiting" ELSE "Ready"
  /\ UNCHANGED <<pending, arrived, consumed, rev, eval, dstate, registered, fixed>>

\* Unavailable: a Device without the function waits on its binding.
Unavailable(d) ==
  /\ phase = "Ready"
  /\ ~registered[d]
  /\ dstate[d] = "Idle"
  /\ dstate' = [dstate EXCEPT ![d] = "Unavailable"]
  /\ UNCHANGED <<phase, pending, arrived, consumed, rev, eval, registered, fixed>>

\* Register: the binding watch sees the function registered on the Device.
Register(d) ==
  /\ ~registered[d]
  /\ registered' = [registered EXCEPT ![d] = TRUE]
  /\ dstate' = [dstate EXCEPT ![d] = "Idle"]
  /\ UNCHANGED <<phase, pending, arrived, consumed, rev, eval, fixed>>

\* Evaluate: the Device reads a ready set of the arrivals, one per input port
\* in the code, and the state revision.
Evaluate(d) ==
  /\ phase = "Ready"
  /\ registered[d]
  /\ dstate[d] = "Idle"
  /\ \E S \in SUBSET pending \ {{}} :
       eval' = [eval EXCEPT ![d] = [in |-> S, rev |-> rev]]
  /\ dstate' = [dstate EXCEPT ![d] = "Evaluating"]
  /\ UNCHANGED <<phase, pending, arrived, consumed, rev, registered, fixed>>

\* Faults is TRUE when the deterministic function fails on the inputs.
Faults(in) == Poison \in in /\ ~fixed

\* Commit: one World operation consumes the evaluated arrivals, writes the
\* next state at the revision read, and routes the chosen port.
Commit(d) ==
  /\ dstate[d] = "Evaluating"
  /\ phase = "Ready"
  /\ eval[d].in \subseteq pending /\ eval[d].rev = rev
  /\ ~Faults(eval[d].in)
  /\ consumed' = Append(consumed, eval[d].in)
  /\ pending' = pending \ eval[d].in
  /\ rev' = rev + 1
  /\ phase' = "Routed"
  /\ eval' = [eval EXCEPT ![d] = NoEval]
  /\ dstate' = [dstate EXCEPT ![d] = "Idle"]
  /\ UNCHANGED <<arrived, registered, fixed>>

\* Fault: one World operation pauses the Step and keeps its arrivals.
Fault(d) ==
  /\ dstate[d] = "Evaluating"
  /\ phase = "Ready"
  /\ eval[d].in \subseteq pending /\ eval[d].rev = rev
  /\ Faults(eval[d].in)
  /\ phase' = "Faulted"
  /\ eval' = [eval EXCEPT ![d] = NoEval]
  /\ dstate' = [dstate EXCEPT ![d] = "Idle"]
  /\ UNCHANGED <<pending, arrived, consumed, rev, registered, fixed>>

\* LostRace: the commit finds the arrivals, revision or Step moved, so the
\* evaluation is discarded.
LostRace(d) ==
  /\ dstate[d] = "Evaluating"
  /\ \/ ~(eval[d].in \subseteq pending)
     \/ eval[d].rev # rev
     \/ phase # "Ready"
  /\ eval' = [eval EXCEPT ![d] = NoEval]
  /\ dstate' = [dstate EXCEPT ![d] = "Idle"]
  /\ UNCHANGED <<phase, pending, arrived, consumed, rev, registered, fixed>>

\* Restart: the Device's run controller restarts and forgets its evaluation.
Restart(d) ==
  /\ eval' = [eval EXCEPT ![d] = NoEval]
  /\ dstate' = [dstate EXCEPT ![d] = "Idle"]
  /\ UNCHANGED <<phase, pending, arrived, consumed, rev, registered, fixed>>

\* Resume: an operator fixes the function and resumes the paused Step.
Resume ==
  /\ phase = "Faulted"
  /\ phase' = "Ready"
  /\ fixed' = TRUE
  /\ UNCHANGED <<pending, arrived, consumed, rev, eval, dstate, registered>>

Next == \/ Arrive \/ Settle \/ Resume
        \/ \E d \in Devices :
             \/ Unavailable(d) \/ Register(d) \/ Evaluate(d) \/ Commit(d)
             \/ Fault(d) \/ LostRace(d) \/ Restart(d)
Spec == Init /\ [][Next]_vars

Range(s) == {s[i] : i \in DOMAIN s}

TypeOK ==
  /\ phase \in {"Waiting", "Ready", "Routed", "Faulted"}
  /\ dstate \in [Devices -> {"Idle", "Evaluating", "Unavailable"}]
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
