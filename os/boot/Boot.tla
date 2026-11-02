---------------------------- MODULE Boot ----------------------------
EXTENDS TLC
VARIABLES phase, pinned, mounted, child
vars == <<phase, pinned, mounted, child>>
Init == /\ phase = "bootstrap" /\ pinned = FALSE /\ mounted = FALSE /\ child = FALSE
Build == /\ phase = "bootstrap" /\ phase' = "frozen"
         /\ pinned' = TRUE /\ UNCHANGED <<mounted, child>>
Mount == /\ phase = "frozen" /\ phase' = "serving"
         /\ mounted' = TRUE /\ UNCHANGED <<pinned, child>>
Run == /\ phase = "serving" /\ phase' = "workload"
       /\ child' = TRUE /\ UNCHANGED <<pinned, mounted>>
Exit == /\ phase = "workload" /\ phase' = "stopping"
        /\ child' = FALSE /\ UNCHANGED <<pinned, mounted>>
Fail == /\ phase \in {"bootstrap", "frozen", "serving", "workload"}
        /\ phase' = "stopping" /\ child' = FALSE
        /\ UNCHANGED <<pinned, mounted>>
Unmount == /\ phase = "stopping" /\ mounted /\ ~child
           /\ mounted' = FALSE /\ UNCHANGED <<phase, pinned, child>>
Stop == /\ phase = "stopping" /\ ~mounted /\ ~child
        /\ phase' = "stopped" /\ pinned' = FALSE
        /\ UNCHANGED <<mounted, child>>
Next == Build \/ Mount \/ Run \/ Exit \/ Fail \/ Unmount \/ Stop
Spec == Init /\ [][Next]_vars
Safe == /\ (child => mounted) /\ (mounted => pinned)
        /\ (phase = "stopped" => ~mounted /\ ~child /\ ~pinned)
=============================================================================
