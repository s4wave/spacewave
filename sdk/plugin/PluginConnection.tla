------------------------- MODULE PluginConnection -------------------------
EXTENDS Integers, FiniteSets

\* Quickstart and WorldOp callers retain the registration's installation and
\* executable together. Unpinned SDK calls already retain their installation.
CONSTANTS PreserveInstance, GlobalAvailable
VARIABLES phase, wanted, requested, connected
vars == <<phase, wanted, requested, connected>>
Instances == {"", "space-a", "space-b"}
Roots == {"", "revision-one"}
Targets == Instances \X Roots
Available == {target \in Targets : GlobalAvailable \/ target[1] # ""}
Init == /\ phase = "idle" /\ wanted = <<"", "">>
        /\ requested = <<"", "">> /\ connected = <<"", "">>
Request(target) ==
    /\ phase = "idle" /\ target \in Targets
    /\ wanted' = target /\ phase' = "loading"
    /\ requested' = <<IF PreserveInstance \/ target[2] = "" THEN target[1] ELSE "", target[2]>>
    /\ UNCHANGED connected
Resolve ==
    /\ phase = "loading"
    /\ phase' = IF requested \in Available THEN "ready" ELSE "failed"
    /\ connected' = requested
    /\ UNCHANGED <<wanted, requested>>
Release == /\ phase \in {"ready", "failed"} /\ phase' = "idle"
           /\ UNCHANGED <<wanted, requested, connected>>
Next == (\E target \in Targets : Request(target)) \/ Resolve \/ Release \/ UNCHANGED vars
TypeOK == /\ phase \in {"idle", "loading", "ready", "failed"}
          /\ wanted \in Targets /\ requested \in Targets /\ connected \in Targets
ReadyIdentity == phase = "ready" => connected = wanted
AvailableTargetWorks == phase = "failed" => wanted \notin Available
Spec == Init /\ [][Next]_vars
=============================================================================
