---- MODULE PackReadAdmission ----
\* One block and a split read, with a competing admission.
\* Location lookup and drop hold the log lock. CheckRead through FinishRead
\* hold the pool shard lock, so other admissions cannot observe the
\* provisional slot. A drop can publish a generation while the shard is held.
EXTENDS Naturals

CONSTANTS MaxDrops, RequireSync, InvalidateDrop
VARIABLES location, generation, sampled, read, cached, cacheFloor, lastAdmission
vars == <<location, generation, sampled, read, cached, cacheFloor, lastAdmission>>

Backing == {"synced", "removed", "remote"}
DropGeneration == IF InvalidateDrop THEN generation + 1 ELSE generation

Init ==
  /\ location = "absent"
  /\ generation = 1
  /\ sampled = 0
  /\ read = "idle"
  /\ cached = 0
  /\ cacheFloor = 0
  /\ lastAdmission = <<0, 0>>

Append ==
  /\ location = "absent"
  /\ location' = "unsynced"
  /\ UNCHANGED <<generation, sampled, read, cached, cacheFloor, lastAdmission>>

Sync ==
  /\ location = "unsynced"
  /\ location' = "synced"
  /\ UNCHANGED <<generation, sampled, read, cached, cacheFloor, lastAdmission>>

Remove ==
  /\ location = "synced"
  /\ generation < MaxDrops
  /\ location' = "removed"
  /\ generation' = generation + 1
  /\ UNCHANGED <<sampled, read, cached, cacheFloor, lastAdmission>>

Evict ==
  /\ location = "synced"
  /\ location' = "remote"
  /\ UNCHANGED <<generation, sampled, read, cached, cacheFloor, lastAdmission>>

Drop ==
  /\ location # "absent"
  /\ generation < MaxDrops
  /\ location' = "absent"
  /\ generation' = DropGeneration
  /\ UNCHANGED <<sampled, read, cached, cacheFloor, lastAdmission>>

Replace ==
  /\ location \in {"synced", "removed"}
  /\ generation < MaxDrops
  /\ location' = "unsynced"
  /\ generation' = DropGeneration
  /\ UNCHANGED <<sampled, read, cached, cacheFloor, lastAdmission>>

BeginRead ==
  /\ read = "idle"
  /\ read' = "started"
  /\ sampled' = generation
  /\ UNCHANGED <<location, generation, cached, cacheFloor, lastAdmission>>

Lookup ==
  /\ read = "started"
  /\ read' = IF location \in Backing \/
                (~RequireSync /\ location = "unsynced")
             THEN "durable" ELSE "unmarked"
  /\ UNCHANGED <<location, generation, sampled, cached, cacheFloor, lastAdmission>>

CheckRead ==
  /\ read \in {"durable", "unmarked"}
  /\ read' = IF read = "durable" /\ sampled = generation THEN "checked" ELSE "idle"
  /\ UNCHANGED <<location, generation, sampled, cached, cacheFloor, lastAdmission>>

StageRead ==
  /\ read = "checked"
  /\ read' = "staged"
  /\ UNCHANGED <<location, generation, sampled, cached, cacheFloor, lastAdmission>>

VerifyRead ==
  /\ read = "staged"
  /\ \/ /\ sampled = generation
        /\ read' = "verified"
        /\ lastAdmission' = <<sampled, generation>>
     \/ /\ sampled # generation
        /\ read' = "idle"
        /\ UNCHANGED lastAdmission
  /\ UNCHANGED <<location, generation, sampled, cached, cacheFloor>>

FinishRead ==
  /\ read = "verified"
  /\ cached' = sampled
  /\ cacheFloor' = IF sampled > cacheFloor THEN sampled ELSE cacheFloor
  /\ read' = "idle"
  /\ UNCHANGED <<location, generation, sampled, lastAdmission>>

AdmitOther ==
  /\ read \notin {"checked", "staged", "verified"}
  /\ location \in Backing
  /\ cached' = generation
  /\ cacheFloor' = generation
  /\ UNCHANGED <<location, generation, sampled, read, lastAdmission>>

Next == Append \/ Sync \/ Remove \/ Evict \/ Drop \/ Replace \/
        BeginRead \/ Lookup \/ CheckRead \/ StageRead \/ VerifyRead \/ FinishRead \/ AdmitOther
Spec == Init /\ [][Next]_vars

CacheBacked == cached = generation => location \in Backing
AdmissionOrder == cached = cacheFloor
AdmissionCurrent == lastAdmission[1] = lastAdmission[2]
====
