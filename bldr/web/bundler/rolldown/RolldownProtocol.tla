------------------------- MODULE RolldownProtocol -------------------------
EXTENDS Naturals

(* Build owns two control files until its runner exits and its result is read.
   Two calls represent any pair of the plugin, GoScript, browser, Electron,
   Saucer, Wasm, Vite bootstrap and QuickJS callers of Build. Their source and
   artifact contents are outside this model. Workspace cleanup can interleave
   with either call; it must not erase their control files. Process launch and
   cancellation requests do not change file ownership. Start is the runner's
   request read; Abort is its reaped exit followed by deferred Cleanup. Setup
   failure before Prepare closes a call without any allocated control files.
   Fresh temporary directories never reuse an orphan from an earlier process. *)
CONSTANTS Calls, InsideWorkspace, PrivateDirectory
VARIABLES stage, requests, results
vars == <<stage, requests, results>>
Stages == {"new", "ready", "running", "finished", "read", "closed"}

Init == /\ stage = [i \in Calls |-> "new"]
        /\ requests = {} /\ results = {}

Prepare(i) == /\ stage[i] = "new"
              /\ stage' = [stage EXCEPT ![i] = "ready"]
              /\ requests' = requests \cup {i}
              /\ UNCHANGED results
Start(i) == /\ stage[i] = "ready" /\ i \in requests
            /\ stage' = [stage EXCEPT ![i] = "running"]
            /\ UNCHANGED <<requests, results>>
Finish(i) == /\ stage[i] = "running"
             /\ stage' = [stage EXCEPT ![i] = "finished"]
             /\ results' = results \cup {i}
             /\ UNCHANGED requests
Read(i) == /\ stage[i] = "finished" /\ i \in results
           /\ stage' = [stage EXCEPT ![i] = "read"]
           /\ UNCHANGED <<requests, results>>
Abort(i) == /\ stage[i] \in {"new", "ready", "running", "finished"}
            /\ stage' = [stage EXCEPT ![i] = IF @ = "new" THEN "closed" ELSE "read"]
            /\ UNCHANGED <<requests, results>>
Cleanup(i) == /\ stage[i] = "read"
              /\ stage' = [stage EXCEPT ![i] = "closed"]
              /\ requests' = IF PrivateDirectory THEN requests \ {i} ELSE {}
              /\ results' = IF PrivateDirectory THEN results \ {i} ELSE {}
CleanWorkspace == /\ UNCHANGED stage
                  /\ IF InsideWorkspace
                        THEN /\ requests' = {} /\ results' = {}
                        ELSE UNCHANGED <<requests, results>>
Next == (\E i \in Calls: Prepare(i) \/ Start(i) \/ Finish(i) \/ Read(i)
                         \/ Abort(i) \/ Cleanup(i))
        \/ CleanWorkspace \/ UNCHANGED vars

TypeOK == /\ stage \in [Calls -> Stages]
          /\ requests \subseteq Calls /\ results \subseteq Calls
RequestAvailable == \A i \in Calls:
    stage[i] \in {"ready", "running", "finished"} => i \in requests
ResultAvailable == \A i \in Calls: stage[i] = "finished" => i \in results
ClosedReleasesFiles == \A i \in Calls:
    stage[i] = "closed" => i \notin (requests \cup results)
Spec == Init /\ [][Next]_vars
=============================================================================
