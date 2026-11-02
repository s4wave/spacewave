---- MODULE UploadPressure ----
EXTENDS Naturals
(* Store.scanHead admits entries after their settle age or while pending bytes
   exceed the placement's byte/resident limit. queueHead.plan sends a short
   batch before pack linger when any admitted entry relieves that pressure.
   A block may already be settled when PlaceBlockStore lowers the byte limit.
   This bounded model follows one marker, whether its local block survived or
   was swept: uploadChunk uploads the surviving block or drops the swept marker,
   then clear ends the pending state. Remote failures retain the marker.
   CountSettledPressure=FALSE reproduces the previous classification. *)
CONSTANT CountSettledPressure
VARIABLES settled, pressure, phase, forced, local, remote, pending
vars == <<settled, pressure, phase, forced, local, remote, pending>>
Init == /\ settled \in BOOLEAN /\ pressure = FALSE /\ phase = "queued"
        /\ forced = FALSE /\ local \in BOOLEAN /\ remote = FALSE /\ pending = TRUE
SetPolicy == /\ phase = "queued" /\ pressure' = TRUE
             /\ UNCHANGED <<settled, phase, forced, local, remote, pending>>
Settle == /\ phase = "queued" /\ settled' = TRUE
          /\ UNCHANGED <<pressure, phase, forced, local, remote, pending>>
ScanHead == /\ phase = "queued" /\ pressure
            /\ forced' = (pressure /\ (CountSettledPressure \/ ~settled))
            /\ phase' = "head"
            /\ UNCHANGED <<settled, pressure, local, remote, pending>>
Plan == /\ phase = "head" /\ phase' = IF forced THEN "upload" ELSE "linger"
        /\ UNCHANGED <<settled, pressure, forced, local, remote, pending>>
UploadChunk == /\ phase = "upload" /\ phase' = "clear" /\ remote' = local
               /\ UNCHANGED <<settled, pressure, forced, local, pending>>
Clear == /\ phase = "clear" /\ phase' = "done" /\ pending' = FALSE
         /\ UNCHANGED <<settled, pressure, forced, local, remote>>
Next == SetPolicy \/ Settle \/ ScanHead \/ Plan \/ UploadChunk \/ Clear \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars
FairSpec == Spec /\ WF_vars(SetPolicy) /\ WF_vars(ScanHead) /\ WF_vars(Plan)
            /\ WF_vars(UploadChunk) /\ WF_vars(Clear)
PressureBypassesLinger == phase # "linger"
KeptUntilUploaded == local /\ ~pending => remote
QueueDrains == pending ~> ~pending
====
