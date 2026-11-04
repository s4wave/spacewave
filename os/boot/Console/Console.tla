---------------------------- MODULE Console ----------------------------
EXTENDS Naturals, TLC
\* Reader journey for the trusted, isolated QEMU serial console. Persist is the
\* existing World's durable atomic publication, not admission or buffer flush.
\* Omits block layout, kernel faults, job-control implementation and media loss;
\* source lifecycle/durability models refine those boundaries before code.
CONSTANTS DurableBeforeAck, FreezeSystem
VARIABLES phase, store, mounted, child, pending, durable, acknowledged, systemRev
vars == <<phase, store, mounted, child, pending, durable, acknowledged, systemRev>>
Init == /\ phase = "off" /\ store = FALSE /\ mounted = FALSE /\ child = FALSE
        /\ pending = 0 /\ durable = 0 /\ acknowledged = 0 /\ systemRev = 0
Start == /\ phase \in {"off", "recovery"} /\ phase' = "opening"
         /\ UNCHANGED <<store, mounted, child, pending, durable, acknowledged, systemRev>>
Open == /\ phase = "opening" /\ phase' = "ready" /\ store' = TRUE /\ mounted' = TRUE
        /\ UNCHANGED <<child, pending, durable, acknowledged, systemRev>>
Enter == /\ phase = "ready" /\ phase' = "console" /\ child' = TRUE
         /\ UNCHANGED <<store, mounted, pending, durable, acknowledged, systemRev>>
Write == /\ phase = "console" /\ pending = acknowledged /\ pending < 2
         /\ pending' = pending + 1
         /\ UNCHANGED <<phase, store, mounted, child, durable, acknowledged, systemRev>>
Persist == /\ phase = "console" /\ pending > durable /\ durable' = pending
           /\ UNCHANGED <<phase, store, mounted, child, pending, acknowledged, systemRev>>
Ack == /\ phase = "console" /\ pending > acknowledged
       /\ (DurableBeforeAck => durable >= pending) /\ acknowledged' = pending
       /\ UNCHANGED <<phase, store, mounted, child, pending, durable, systemRev>>
Stop == /\ phase \in {"ready", "console"} /\ phase' = "stopping"
        /\ UNCHANGED <<store, mounted, child, pending, durable, acknowledged, systemRev>>
JoinChild == /\ phase = "stopping" /\ child /\ child' = FALSE
             /\ UNCHANGED <<phase, store, mounted, pending, durable, acknowledged, systemRev>>
Unmount == /\ phase = "stopping" /\ ~child /\ mounted /\ mounted' = FALSE
           /\ UNCHANGED <<phase, store, child, pending, durable, acknowledged, systemRev>>
Close == /\ phase = "stopping" /\ ~child /\ ~mounted /\ phase' = "off"
         /\ store' = FALSE /\ pending' = durable
         /\ UNCHANGED <<mounted, child, durable, acknowledged, systemRev>>
FailOpen == /\ phase = "opening" /\ phase' = "recovery"
            /\ UNCHANGED <<store, mounted, child, pending, durable, acknowledged, systemRev>>
\* Host termination drops volatile state; acknowledged receipts stay external.
Crash == /\ phase \in {"opening", "ready", "console", "stopping"}
         /\ phase' = "off" /\ store' = FALSE /\ mounted' = FALSE /\ child' = FALSE
         /\ pending' = durable /\ UNCHANGED <<durable, acknowledged, systemRev>>
ChangeSystem == /\ ~FreezeSystem /\ phase = "console" /\ systemRev' = 1
                /\ UNCHANGED <<phase, store, mounted, child, pending, durable, acknowledged>>
Next == Start \/ Open \/ Enter \/ Write \/ Persist \/ Ack \/ Stop \/ JoinChild \/
        Unmount \/ Close \/ FailOpen \/ Crash \/ ChangeSystem
Spec == Init /\ [][Next]_vars
ConsoleHasStorage == child => mounted /\ store
CloseAfterUsers == ~store => ~mounted /\ ~child
AcknowledgedSurvives == acknowledged <= durable
SystemStaysFrozen == systemRev = 0
=============================================================================
