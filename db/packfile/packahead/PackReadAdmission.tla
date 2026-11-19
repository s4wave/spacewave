---- MODULE PackReadAdmission ----
\* One block, one split admission and one delayed stale lookup.
\* The shard lock hides a candidate's provisional charge until commit.
\* A drop can advance the generation while the shard is held.
\* For a stale lookup in this model the removal generation identifies
\* the old entry. The implementation compares a per-store version.
EXTENDS Naturals

CONSTANTS MaxDrops, RequireSync, InvalidateDrop
VARIABLES location, generation, sampled, read, cached, cacheFloor, lastAdmission, seenStale, lastPurge
vars == <<location, generation, sampled, read, cached, cacheFloor, lastAdmission, seenStale, lastPurge>>

StableAtRead == {"synced", "removed", "remote"}
Backing == StableAtRead \cup {"recovering"}
DropGeneration == IF InvalidateDrop THEN generation + 1 ELSE generation

Init ==
  /\ location = "absent"
  /\ generation = 1
  /\ sampled = 0
  /\ read = "idle"
  /\ cached = 0
  /\ cacheFloor = 0
  /\ lastAdmission = <<0, 0>>
  /\ seenStale = 0
  /\ lastPurge = <<0, 0>>

Append ==
  /\ location = "absent"
  /\ location' = "unsynced"
  /\ UNCHANGED <<generation, sampled, read, cached, cacheFloor, lastAdmission, seenStale, lastPurge>>

Sync ==
  /\ location = "unsynced"
  /\ location' = "synced"
  /\ UNCHANGED <<generation, sampled, read, cached, cacheFloor, lastAdmission, seenStale, lastPurge>>

FailWrite ==
  /\ location = "synced"
  /\ location' = "recovering"
  /\ UNCHANGED <<generation, sampled, read, cached, cacheFloor, lastAdmission, seenStale, lastPurge>>

VerifyBatch ==
  /\ location = "recovering"
  /\ location' = "synced"
  /\ UNCHANGED <<generation, sampled, read, cached, cacheFloor, lastAdmission, seenStale, lastPurge>>

Remove ==
  /\ location = "synced"
  /\ generation < MaxDrops
  /\ location' = "removed"
  /\ generation' = generation + 1
  /\ UNCHANGED <<sampled, read, cached, cacheFloor, lastAdmission, seenStale, lastPurge>>

Evict ==
  /\ location = "synced"
  /\ location' = "remote"
  /\ UNCHANGED <<generation, sampled, read, cached, cacheFloor, lastAdmission, seenStale, lastPurge>>

Drop ==
  /\ location # "absent"
  /\ generation < MaxDrops
  /\ location' = "absent"
  /\ generation' = DropGeneration
  /\ UNCHANGED <<sampled, read, cached, cacheFloor, lastAdmission, seenStale, lastPurge>>

Replace ==
  /\ location \in {"synced", "removed"}
  /\ generation < MaxDrops
  /\ location' = "unsynced"
  /\ generation' = DropGeneration
  /\ UNCHANGED <<sampled, read, cached, cacheFloor, lastAdmission, seenStale, lastPurge>>

BeginRead ==
  /\ read = "idle"
  /\ read' = "started"
  /\ sampled' = generation
  /\ UNCHANGED <<location, generation, cached, cacheFloor, lastAdmission, seenStale, lastPurge>>

Lookup ==
  /\ read = "started"
  /\ read' = IF location \in StableAtRead \/
                (~RequireSync /\ location = "unsynced")
             THEN "durable" ELSE "unmarked"
  /\ UNCHANGED <<location, generation, sampled, cached, cacheFloor, lastAdmission, seenStale, lastPurge>>

CheckRead ==
  /\ read \in {"durable", "unmarked"}
  /\ read' = IF read = "durable" /\ sampled = generation THEN "checked" ELSE "idle"
  /\ UNCHANGED <<location, generation, sampled, cached, cacheFloor, lastAdmission, seenStale, lastPurge>>

ChargeRead ==
  /\ read = "checked"
  /\ read' = "charged"
  /\ UNCHANGED <<location, generation, sampled, cached, cacheFloor, lastAdmission, seenStale, lastPurge>>

CommitRead ==
  /\ read = "charged"
  /\ read' = "idle"
  /\ \/ /\ sampled = generation
        /\ cached' = sampled
        /\ cacheFloor' = IF sampled > cacheFloor THEN sampled ELSE cacheFloor
        /\ lastAdmission' = <<sampled, generation>>
     \/ /\ sampled # generation
        /\ UNCHANGED <<cached, cacheFloor, lastAdmission>>
  /\ UNCHANGED <<location, generation, sampled, seenStale, lastPurge>>

AdmitOther ==
  /\ read \notin {"checked", "charged"}
  /\ location \in StableAtRead
  /\ cached' = generation
  /\ cacheFloor' = generation
  /\ UNCHANGED <<location, generation, sampled, read, lastAdmission, seenStale, lastPurge>>

ObserveStale ==
  /\ read \notin {"checked", "charged"}
  /\ cached > 0 /\ cached < generation /\ seenStale = 0
  /\ seenStale' = cached
  /\ UNCHANGED <<location, generation, sampled, read, cached, cacheFloor, lastAdmission, lastPurge>>

PurgeStale ==
  /\ read \notin {"checked", "charged"}
  /\ seenStale # 0
  /\ IF cached = seenStale
        THEN /\ cached' = 0
             /\ lastPurge' = <<seenStale, cached>>
        ELSE UNCHANGED <<cached, lastPurge>>
  /\ seenStale' = 0
  /\ UNCHANGED <<location, generation, sampled, read, cacheFloor, lastAdmission>>

Next == Append \/ Sync \/ FailWrite \/ VerifyBatch \/ Remove \/ Evict \/ Drop \/ Replace \/
        BeginRead \/ Lookup \/ CheckRead \/ ChargeRead \/ CommitRead \/ AdmitOther \/
        ObserveStale \/ PurgeStale
Spec == Init /\ [][Next]_vars

CacheBacked == cached = generation => location \in Backing
AdmissionOrder == cached = 0 \/ cached = cacheFloor
AdmissionCurrent == lastAdmission[1] = lastAdmission[2]
PurgeCurrent == lastPurge[1] = lastPurge[2]
====
