---- MODULE InodeWrites ----
\* fs-inode-write.go: one buffered inode, an idle callback, FSHandle.Sync
\* (file or root: both snapshot the same tree), and Release of a shared handle.
\* Another handle stays attached. AccessOps/writeAt/buffered consume an error
\* under the same inode mutex; Observe models that consumption without a flush.
\*
\* Start with an accepted small write. Bytes, extent sizes, cursor resolution
\* and final inode teardown are outside this bounded pending-error lifecycle.
\* Full/discontiguous/caller flushes use the sync actor's flush transition:
\* failure is returned directly, cancellation keeps the buffer. Idle/Release
\* retain the first failure. A retained error does not retry a discarded write.
\*
\* Acquire/Return are w.mtx boundaries; Snapshot takes only treeWrites.mtx.
\* Flush publishes pending under treeWrites.mtx after changing buf/error while
\* still holding w.mtx. Snapshot may run during I/O, but sees the previous
\* pending entry until that publication. It never observes buf/error directly.
EXTENDS Naturals

RetainFailures == TRUE
ReleaseReports == FALSE
Actors == {"idle", "sync", "release", "observe"}
VARIABLES buf, error, pending, owner, pc, selected, result, unreported, badAck
vars == <<buf, error, pending, owner, pc, selected, result, unreported, badAck>>

Init ==
  /\ buf = TRUE /\ error = FALSE /\ pending = TRUE /\ owner = "none"
  /\ pc = [a \in Actors |-> "start"]
  /\ selected = FALSE /\ result = [a \in Actors |-> "ok"]
  /\ unreported = FALSE /\ badAck = FALSE

Snapshot ==
  /\ pc["sync"] = "start"
  /\ selected' = pending
  /\ pc' = [pc EXCEPT !["sync"] = "acquire"]
  /\ UNCHANGED <<buf, error, pending, owner, result, unreported, badAck>>

Acquire(a) ==
  /\ a \in Actors /\ owner = "none"
  /\ IF a = "sync" THEN pc[a] = "acquire" /\ selected ELSE pc[a] = "start"
  /\ owner' = a
  /\ pc' = [pc EXCEPT ![a] = IF a = "observe" THEN "return" ELSE "flush"]
  /\ UNCHANGED <<buf, error, pending, selected, result, unreported, badAck>>

Flush(a, outcome) ==
  /\ owner = a /\ pc[a] = "flush"
  /\ outcome \in IF a = "sync" THEN {"ok", "fail", "cancel"} ELSE {"ok", "fail"}
  /\ buf' = (buf /\ outcome = "cancel")
  /\ error' = (error \/ (buf /\ outcome = "fail" /\ a # "sync"))
  /\ pending' = (buf' \/ (RetainFailures /\ error'))
  /\ result' = [result EXCEPT ![a] =
       IF buf /\ outcome # "ok" THEN outcome ELSE "ok"]
  /\ unreported' = (unreported \/ (buf /\ outcome = "fail" /\ a # "sync"))
  /\ pc' = [pc EXCEPT ![a] = "return"]
  /\ UNCHANGED <<owner, selected, badAck>>

Return(a) ==
  /\ owner = a /\ pc[a] = "return"
  /\ LET reports == a \in {"sync", "observe"} \/ (a = "release" /\ ReleaseReports)
         consumes == reports /\ result[a] = "ok" /\ error
         returnsOK == result[a] = "ok" /\ ~error
     IN /\ error' = (error /\ ~consumes)
        /\ unreported' = (unreported /\ ~(consumes /\ a # "release"))
        /\ result' = [result EXCEPT ![a] =
             IF consumes THEN "fail" ELSE @]
        /\ badAck' = (badAck \/ (a = "sync" /\ returnsOK /\ unreported))
  /\ pending' = (buf \/ (RetainFailures /\ error'))
  /\ pc' = [pc EXCEPT ![a] = "done"]
  /\ owner' = "none"
  /\ UNCHANGED <<buf, selected>>

SkipSync ==
  /\ pc["sync"] = "acquire" /\ ~selected
  /\ pc' = [pc EXCEPT !["sync"] = "done"]
  /\ badAck' = (badAck \/ unreported)
  /\ UNCHANGED <<buf, error, pending, owner, selected, result, unreported>>

CancelSync ==
  /\ pc["sync"] \in {"start", "acquire"}
  /\ pc' = [pc EXCEPT !["sync"] = "done"]
  /\ result' = [result EXCEPT !["sync"] = "cancel"]
  /\ UNCHANGED <<buf, error, pending, owner, selected, unreported, badAck>>

Actor(a) == Acquire(a) \/ (\E outcome \in {"ok", "fail", "cancel"}: Flush(a, outcome)) \/ Return(a)
Finished == (\A a \in Actors: pc[a] = "done") /\ UNCHANGED vars
Next == Snapshot \/ SkipSync \/ CancelSync \/ (\E a \in Actors: Actor(a)) \/ Finished
Spec == Init /\ [][Next]_vars /\ WF_vars(Snapshot) /\ WF_vars(SkipSync)
        /\ (\A a \in Actors: WF_vars(Actor(a)))

SyncReportsFailure == ~badAck
CleanEvicted == owner = "none" /\ ~buf /\ ~error => ~pending
AllReturn == <>(\A a \in Actors: pc[a] = "done")
=============================================================================
