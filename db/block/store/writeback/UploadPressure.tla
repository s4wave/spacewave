---- MODULE UploadPressure ----
EXTENDS Naturals
(* Store.scanHead admits entries after their settle age or while pending bytes
   exceed the placement's byte/resident limit. queueHead.plan sends a short
   batch before pack linger when any admitted entry relieves that pressure.
   A block may already be settled when PlaceBlockStore lowers the byte limit.
   This bounded model follows one marker, whether its local block survived or
   was swept: uploadChunk uploads the surviving block or drops the swept marker,
   then clear ends the pending state. CountSettledPressure=FALSE reproduces
   the previous classification.

   uploadChunk and clear can fail: the remote errs, or the SQLite store
   reports a write it could not finish. Either failure ends the round with
   the marker kept, so the entry queues and uploads again; the remote keeps
   the repeat as another copy. Failures bounds the failures in a run, and
   uploads counts the sends. *)
CONSTANTS CountSettledPressure, Failures
VARIABLES settled, pressure, phase, forced, local, remote, pending,
          failed, uploads
vars == <<settled, pressure, phase, forced, local, remote, pending,
          failed, uploads>>
Store == <<failed, uploads>>
Init == /\ settled \in BOOLEAN /\ pressure = FALSE /\ phase = "queued"
        /\ forced = FALSE /\ local \in BOOLEAN /\ remote = FALSE /\ pending = TRUE
        /\ failed = 0 /\ uploads = 0
SetPolicy == /\ phase = "queued" /\ pressure' = TRUE
             /\ UNCHANGED <<settled, phase, forced, local, remote, pending, Store>>
Settle == /\ phase = "queued" /\ settled' = TRUE
          /\ UNCHANGED <<pressure, phase, forced, local, remote, pending, Store>>
ScanHead == /\ phase = "queued" /\ pressure
            /\ forced' = (pressure /\ (CountSettledPressure \/ ~settled))
            /\ phase' = "head"
            /\ UNCHANGED <<settled, pressure, local, remote, pending, Store>>
Plan == /\ phase = "head" /\ phase' = IF forced THEN "upload" ELSE "linger"
        /\ UNCHANGED <<settled, pressure, forced, local, remote, pending, Store>>
\* Fail ends the round with the marker kept, so the entry queues again.
Fail == /\ failed < Failures /\ failed' = failed + 1 /\ phase' = "queued"
        /\ UNCHANGED pending
UploadChunk == /\ phase = "upload"
               /\ \/ /\ phase' = "clear" /\ remote' = local
                     /\ uploads' = uploads + 1 /\ UNCHANGED <<pending, failed>>
                  \/ Fail /\ UNCHANGED <<remote, uploads>>
               /\ UNCHANGED <<settled, pressure, forced, local>>
Clear == /\ phase = "clear"
         /\ \/ phase' = "done" /\ pending' = FALSE /\ UNCHANGED failed
            \/ Fail
         /\ UNCHANGED <<settled, pressure, forced, local, remote, uploads>>
Next == SetPolicy \/ Settle \/ ScanHead \/ Plan \/ UploadChunk \/ Clear \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars
FairSpec == Spec /\ WF_vars(SetPolicy) /\ WF_vars(ScanHead) /\ WF_vars(Plan)
            /\ WF_vars(UploadChunk) /\ WF_vars(Clear)
TypeOK == failed \in 0..Failures /\ uploads \in 0..(Failures + 1)
PressureBypassesLinger == phase # "linger"
\* A pack goes to the remote again only after a failure.
ResendOnlyAfterFailure == uploads <= failed + 1
KeptUntilUploaded == local /\ ~pending => remote
QueueDrains == pending ~> ~pending
====
