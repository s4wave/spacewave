---- MODULE WorkerGeneration ----
EXTENDS Naturals, FiniteSets
(* sync.go solicitationGeneration: startWorker registers under bcast before
   launching its body; stopAndWait sets stopping, cancels, then samples count
   and wait channel together. workerDone decrements and broadcasts.
   Callers: runSolicitationGeneration owns teardown; handleSolicitedStream
   registers streams. Two callbacks race teardown, each starts at most once.
   Worker return after cancellation is a fairness assumption about transport
   closure, not a proof that arbitrary worker code honors cancellation.
   Stream authentication and message exchange are outside this mechanism. *)
CONSTANT FenceStarts
VARIABLES phase, workers, used, sampled, waiting, returned
vars == <<phase, workers, used, sampled, waiting, returned>>
Init == /\ phase = "running" /\ workers = {} /\ used = {}
        /\ sampled = 0 /\ waiting = FALSE /\ returned = FALSE
StartWorker(a) ==
  /\ a \notin used /\ (~FenceStarts \/ phase = "running")
  /\ workers' = workers \cup {a} /\ used' = used \cup {a}
  /\ UNCHANGED <<phase, sampled, waiting, returned>>
Fence == /\ phase = "running" /\ phase' = "stopping"
         /\ UNCHANGED <<workers, used, sampled, waiting, returned>>
Cancel == /\ phase = "stopping" /\ phase' = "cancelled"
          /\ UNCHANGED <<workers, used, sampled, waiting, returned>>
WorkerDone(a) ==
  /\ a \in workers /\ workers' = workers \ {a} /\ waiting' = FALSE
  /\ UNCHANGED <<phase, used, sampled, returned>>
Sample == /\ phase = "cancelled" /\ ~waiting /\ ~returned
          /\ sampled' = Cardinality(workers)
          /\ waiting' = (workers # {}) /\ returned' = (workers = {})
          /\ UNCHANGED <<phase, workers, used>>
Next == (\E a \in {1, 2}: StartWorker(a) \/ WorkerDone(a))
        \/ Fence \/ Cancel \/ Sample \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars /\ WF_vars(Fence) /\ WF_vars(Cancel)
        /\ WF_vars(Sample) /\ \A a \in {1, 2}: WF_vars(WorkerDone(a))
Joined == returned => workers = {}
WakeMatchesCount == waiting => sampled = Cardinality(workers)
ShutdownCompletes == <>returned
====
