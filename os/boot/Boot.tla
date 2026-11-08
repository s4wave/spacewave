---------------------------- MODULE Boot ----------------------------
EXTENDS TLC
\* Models boot.Run and the bootstrap EnterRoot child with one Go processor.
\* Direct exec holds that processor in vfork while the child waits for FUSE;
\* an independent bootstrap exec releases it before entering the served tree.
\* Omits storage internals, syscall failures and unrelated adopted processes.
CONSTANT IndependentExec
VARIABLES phase, pinned, mounted, child, serverAvailable, execWait
vars == <<phase, pinned, mounted, child, serverAvailable, execWait>>
Init == /\ phase = "bootstrap" /\ pinned = FALSE /\ mounted = FALSE
        /\ child = FALSE /\ serverAvailable = FALSE /\ execWait = FALSE
Build == /\ phase = "bootstrap" /\ phase' = "frozen" /\ pinned' = TRUE
         /\ UNCHANGED <<mounted, child, serverAvailable, execWait>>
Mount == /\ phase = "frozen" /\ phase' = "serving"
         /\ mounted' = TRUE /\ serverAvailable' = TRUE
         /\ UNCHANGED <<pinned, child, execWait>>
StartExec == /\ phase = "serving" /\ phase' = "entering"
             /\ child' = TRUE /\ execWait' = TRUE
             /\ serverAvailable' = IndependentExec
             /\ UNCHANGED <<pinned, mounted>>
ServeExec == /\ phase = "entering" /\ execWait /\ serverAvailable
             /\ phase' = "workload" /\ execWait' = FALSE
             /\ UNCHANGED <<pinned, mounted, child, serverAvailable>>
Exit == /\ phase = "workload" /\ phase' = "stopping" /\ child' = FALSE
        /\ UNCHANGED <<pinned, mounted, serverAvailable, execWait>>
\* Parent cancellation cannot run while its sole processor is held in vfork.
Fail == /\ phase \in {"bootstrap", "frozen", "serving", "entering", "workload"}
        /\ (~mounted \/ serverAvailable)
        /\ phase' = "stopping" /\ child' = FALSE /\ execWait' = FALSE
        /\ UNCHANGED <<pinned, mounted, serverAvailable>>
Unmount == /\ phase = "stopping" /\ mounted /\ ~child
           /\ mounted' = FALSE /\ serverAvailable' = FALSE
           /\ UNCHANGED <<phase, pinned, child, execWait>>
Stop == /\ phase = "stopping" /\ ~mounted /\ ~child
        /\ phase' = "stopped" /\ pinned' = FALSE
        /\ UNCHANGED <<mounted, child, serverAvailable, execWait>>
Next == Build \/ Mount \/ StartExec \/ ServeExec \/ Exit \/ Fail \/ Unmount \/ Stop
        \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars /\ WF_vars(Next)
Safe == /\ (child => mounted) /\ (mounted => pinned)
        /\ (phase = "stopped" => ~mounted /\ ~child /\ ~pinned)
NoSelfWait == execWait => serverAvailable
EventuallyStopped == <> (phase = "stopped")
=============================================================================
