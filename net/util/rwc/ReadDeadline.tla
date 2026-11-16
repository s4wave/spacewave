---- MODULE ReadDeadline ----
EXTENDS Naturals
(* PacketConn.ReadFrom and Conn.Read share readDeadline.read. Both deadline
   setters (including quic Transport.Close and maybeStopListening) use set.
   Snapshot and subscribe are one Broadcast lock section; set changes the
   deadline and closes the subscription in another. Timers only wake the loop,
   which checks the current deadline again. Packet delivery and cancellation
   end a read independently. The stream pumps and writes do not access this
   state and are outside the model. Two setters can run before or after a
   reader snapshot. DropWake retains the former one-shot deadline snapshot. *)
CONSTANT DropWake
VARIABLES deadline, epoch, watched, snapshot, phase, writes
vars == <<deadline, epoch, watched, snapshot, phase, writes>>
Init == /\ deadline = "none" /\ epoch = 0 /\ watched = 0
        /\ snapshot = "none" /\ phase = "snapshot" /\ writes = 0
Set == /\ writes < 2
       /\ deadline' \in {"none", "future", "expired"}
       /\ writes' = writes + 1 /\ epoch' = epoch + 1
       /\ UNCHANGED <<watched, snapshot, phase>>
Snapshot == /\ phase = "snapshot"
            /\ snapshot' = deadline /\ watched' = epoch
            /\ phase' = IF deadline = "expired" THEN "done" ELSE "wait"
            /\ UNCHANGED <<deadline, epoch, writes>>
Changed == /\ phase = "wait" /\ watched # epoch /\ ~DropWake
           /\ phase' = "snapshot"
           /\ UNCHANGED <<deadline, epoch, watched, snapshot, writes>>
Timer == /\ phase = "wait" /\ snapshot = "future"
         /\ phase' = "snapshot"
         /\ UNCHANGED <<deadline, epoch, watched, snapshot, writes>>
Finish == /\ phase # "done" /\ phase' = "done"
          /\ UNCHANGED <<deadline, epoch, watched, snapshot, writes>>
Done == /\ phase = "done" /\ UNCHANGED vars
Next == Set \/ Snapshot \/ Changed \/ Timer \/ Finish \/ Done
Spec == Init /\ [][Next]_vars /\ WF_vars(Snapshot) /\ WF_vars(Changed)
        /\ WF_vars(Timer)
Expires == (writes = 2 /\ deadline = "expired") ~> (phase = "done")
Bounded == writes <= 2 /\ watched <= epoch
====
