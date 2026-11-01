---- MODULE StreamLifecycle ----
EXTENDS Naturals
(* sync.go runStream and joinSyncWorkers: authentication precedes data;
   watchAuthority is independent of a blocked data writer. Context cancellation
   closes the transport, then teardown joins the watcher and data worker.
   Callers are outgoing solicitation and incoming handleSolicitedStream.
   Authentication proof and admission math stay in Lean Sync/Auth; here an
   authenticated result is opaque. Backend state watches obey cancellation and
   transport Close interrupts SendMsg. Deadline errors and auth failures enter
   the same cleanup path. There is one stream and one blocked send; successful
   exchange may continue indefinitely until external cancellation/revocation.
   Revocation observation is separate from cancellation and close. *)
CONSTANT CloseBeforeJoin
VARIABLES phase, authenticated, revoked, cancelled, closed, worker, watcher, returned
vars == <<phase, authenticated, revoked, cancelled, closed, worker, watcher, returned>>
Init == /\ phase = "auth" /\ authenticated = FALSE /\ revoked = FALSE
        /\ cancelled = FALSE /\ closed = FALSE /\ worker = "absent"
        /\ watcher = "absent" /\ returned = FALSE
Authenticate == /\ phase = "auth" /\ ~cancelled
                /\ authenticated' = TRUE /\ phase' = "data"
                /\ worker' = "blocked" /\ watcher' = "running"
                /\ UNCHANGED <<revoked, cancelled, closed, returned>>
Revoke == /\ phase = "data" /\ ~revoked /\ revoked' = TRUE
          /\ UNCHANGED <<phase, authenticated, cancelled, closed, worker, watcher, returned>>
Cancel == /\ ~cancelled /\ cancelled' = TRUE /\ phase' = "stop"
          /\ UNCHANGED <<authenticated, revoked, closed, worker, watcher, returned>>
ObserveRevocation == /\ revoked /\ ~cancelled /\ Cancel
Close == /\ phase = "stop" /\ ~closed /\ CloseBeforeJoin
         /\ closed' = TRUE
         /\ UNCHANGED <<phase, authenticated, revoked, cancelled, worker, watcher, returned>>
WorkerExit == /\ worker = "blocked" /\ closed /\ worker' = "exited"
              /\ UNCHANGED <<phase, authenticated, revoked, cancelled, closed, watcher, returned>>
WatcherExit == /\ watcher = "running" /\ cancelled /\ watcher' = "exited"
               /\ UNCHANGED <<phase, authenticated, revoked, cancelled, closed, worker, returned>>
Join == /\ phase = "stop" /\ worker \in {"absent", "exited"}
        /\ watcher \in {"absent", "exited"} /\ returned' = TRUE
        /\ closed' = TRUE /\ phase' = "done"
        /\ UNCHANGED <<authenticated, revoked, cancelled, worker, watcher>>
Next == Authenticate \/ Revoke \/ Cancel \/ ObserveRevocation \/ Close
        \/ WorkerExit \/ WatcherExit \/ Join \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars /\ WF_vars(Close) /\ WF_vars(WorkerExit)
        /\ WF_vars(WatcherExit) /\ WF_vars(Join) /\ WF_vars(ObserveRevocation)
DataAuthenticated == worker # "absent" => authenticated
JoinedBeforeReturn == returned => worker \in {"absent", "exited"} /\ watcher \in {"absent", "exited"}
CancellationCompletes == cancelled ~> returned
RevocationCompletes == revoked ~> returned
====
