--------------------------- MODULE ProvisionalNoop ---------------------------
(* The two writers in Engine.NewBlockEngineTransaction and EngineTx.admit.
   Space's Adapter.Begin snapshots the provisional or durable writer head;
   the recorder can carry B's delete even when sealing returns the base root.
   A local or caller-owned base follows the durable no-op path. Changing
   commits and durability waiting beyond A's decision are outside this model:
   neither uses the unchanged-root return. The edge stands for root data. *)

CONSTANT Guard
VARIABLES phaseA, edge, baseB, resultB
vars == <<phaseA, edge, baseB, resultB>>

Init == /\ phaseA = "open"
        /\ edge = TRUE
        /\ baseB = "none"
        /\ resultB = "waiting"

AdmitA == /\ phaseA = "open"
          /\ phaseA' = "pending"
          /\ UNCHANGED <<edge, baseB, resultB>>

DecideA == /\ phaseA = "pending"
           /\ phaseA' \in {"accepted", "refused"}
           /\ edge' = (phaseA' = "refused")
           /\ UNCHANGED <<baseB, resultB>>

OpenB == /\ baseB = "none"
         /\ phaseA \in {"pending", "accepted"}
         /\ baseB' = IF phaseA = "pending" THEN "provisional" ELSE "durable"
         /\ UNCHANGED <<phaseA, edge, resultB>>

CommitB == /\ baseB # "none"
           /\ resultB = "waiting"
           /\ resultB' = IF Guard /\ baseB = "provisional"
                            THEN "superseded" ELSE "success"
           /\ UNCHANGED <<phaseA, edge, baseB>>

Done == /\ phaseA \in {"accepted", "refused"}
        /\ (resultB # "waiting" \/ (phaseA = "refused" /\ baseB = "none"))
        /\ UNCHANGED vars

Next == AdmitA \/ DecideA \/ OpenB \/ CommitB \/ Done
Spec == Init /\ [][Next]_vars

NoFalseSuccess == (phaseA = "refused" /\ resultB = "success") => ~edge
=============================================================================
