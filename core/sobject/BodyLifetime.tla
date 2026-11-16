--------------------------- MODULE BodyLifetime ---------------------------
EXTENDS Naturals, TLC

\* BodyHost.SetBody claims one body for Adapter.Execute. The adapter may fail
\* before opened, or run until cancellation. Stop clears the routine state and
\* joins execute (driver plus group control) before cleanup; release frees the
\* claim last. Local, Spacewave and CDN providers forward this same contract.
\* Other SetBody callers in tests release directly, which includes Stop.
\* The two workers abstract serial replay calls and concurrent vote calls.
\* One checkpoint store represents the pending set drained by waitBackground; disk,
\* roots, operation contents and repeated restart cycles are outside this model.

VARIABLES phase, claim, workers, opened, resources, store
vars == <<phase, claim, workers, opened, resources, store>>
Actors == {"driver", "voter"}

Init == /\ phase = "opening"
        /\ claim = "first"
        /\ workers = [a \in Actors |-> "waiting"]
        /\ opened = FALSE
        /\ resources = TRUE
        /\ store = "idle"

Open == /\ phase = "opening"
        /\ phase' = "active"
        /\ opened' = TRUE
        /\ UNCHANGED <<claim, workers, resources, store>>

Call(a) == /\ phase \in {"opening", "active"}
           /\ opened /\ workers[a] = "waiting"
           /\ workers' = [workers EXCEPT ![a] = "using"]
           /\ UNCHANGED <<phase, claim, opened, resources, store>>

Return(a) == /\ workers[a] = "using"
             /\ workers' = [workers EXCEPT ![a] = "waiting"]
             /\ UNCHANGED <<phase, claim, opened, resources, store>>

Cancel == /\ phase \in {"opening", "active"}
          /\ phase' = "stopping"
          /\ UNCHANGED <<claim, workers, opened, resources, store>>

Exit(a) == /\ phase = "stopping" /\ workers[a] = "waiting"
           /\ workers' = [workers EXCEPT ![a] = "exited"]
           /\ UNCHANGED <<phase, claim, opened, resources, store>>

Join == /\ phase = "stopping"
        /\ workers["driver"] = "exited"
        /\ workers["voter"] = "exited"
        /\ phase' = "stopped"
        /\ UNCHANGED <<claim, workers, opened, resources, store>>

Cleanup == /\ phase = "stopped"
           /\ store # "running"
           /\ phase' = "cleaned"
           /\ resources' = FALSE
           /\ UNCHANGED <<claim, workers, opened, store>>

Release == /\ phase = "cleaned"
           /\ phase' = "released"
           /\ claim' = "none"
           /\ UNCHANGED <<workers, opened, resources, store>>

Successor == /\ claim = "none"
             /\ claim' = "second"
             /\ UNCHANGED <<phase, workers, opened, resources, store>>

StartStore == /\ phase = "active" /\ store = "idle"
              /\ \E a \in Actors : workers[a] = "using"
              /\ store' = "running"
              /\ UNCHANGED <<phase, claim, workers, opened, resources>>

FinishStore == /\ store = "running"
               /\ store' = "done"
               /\ UNCHANGED <<phase, claim, workers, opened, resources>>

Next == Open \/ Cancel \/ Join \/ Cleanup \/ Release \/ Successor
        \/ StartStore \/ FinishStore
        \/ (\E a \in Actors : Call(a) \/ Return(a) \/ Exit(a))

BackgroundResourcesLive == store = "running" => resources
BodyResourcesLive == (\E a \in Actors : workers[a] = "using") => resources
CleanupRetainsClaim == phase \in {"opening", "active", "stopping", "stopped"}
                       => claim = "first"
SuccessorAfterCleanup == claim = "second" => ~resources
StopsBeforeCleanup == ~resources => \A a \in Actors : workers[a] = "exited"
Completes == <> (claim = "second")

Spec == Init /\ [][Next]_vars
        /\ WF_vars(Cancel) /\ WF_vars(Join) /\ WF_vars(Cleanup)
        /\ WF_vars(Release) /\ WF_vars(Successor) /\ WF_vars(FinishStore)
        /\ (\A a \in Actors : WF_vars(Return(a)) /\ WF_vars(Exit(a)))
=============================================================================
