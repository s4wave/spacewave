---- MODULE PackReadAdmission ----
\* One block and a split read, with a competing admission.
\* Location lookup and drop hold the log lock. CheckRead and InsertRead
\* hold the same pool shard lock, which serializes competing insertions
\* but can overlap a drop.
EXTENDS Naturals

CONSTANTS MaxDrops, RequireSync, InvalidateDrop
VARIABLES location, generation, sampled, read, cached, cacheFloor
vars == <<location, generation, sampled, read, cached, cacheFloor>>

Backing == {"synced", "removed", "remote"}

Init ==
  /\ location = "absent"
  /\ generation = 1
  /\ sampled = 0
  /\ read = "idle"
  /\ cached = 0
  /\ cacheFloor = 0

Append ==
  /\ location = "absent"
  /\ location' = "unsynced"
  /\ UNCHANGED <<generation, sampled, read, cached, cacheFloor>>

Sync ==
  /\ location = "unsynced"
  /\ location' = "synced"
  /\ UNCHANGED <<generation, sampled, read, cached, cacheFloor>>

Remove ==
  /\ location = "synced"
  /\ generation < MaxDrops
  /\ location' = "removed"
  /\ generation' = generation + 1
  /\ UNCHANGED <<sampled, read, cached, cacheFloor>>

Evict ==
  /\ location = "synced"
  /\ location' = "remote"
  /\ UNCHANGED <<generation, sampled, read, cached, cacheFloor>>

\* DroppedGeneration is the generation after dropLocationLocked drops a
\* location: invalidating bumps it, so an in-flight read cannot admit.
DroppedGeneration == IF InvalidateDrop THEN generation + 1 ELSE generation

Drop ==
  /\ location # "absent"
  /\ generation < MaxDrops
  /\ location' = "absent"
  /\ generation' = DroppedGeneration
  /\ UNCHANGED <<sampled, read, cached, cacheFloor>>

Replace ==
  /\ location \in {"synced", "removed"}
  /\ generation < MaxDrops
  /\ location' = "unsynced"
  /\ generation' = DroppedGeneration
  /\ UNCHANGED <<sampled, read, cached, cacheFloor>>

BeginRead ==
  /\ read = "idle"
  /\ read' = "started"
  /\ sampled' = generation
  /\ UNCHANGED <<location, generation, cached, cacheFloor>>

Lookup ==
  /\ read = "started"
  /\ read' = IF location \in Backing \/
                (~RequireSync /\ location = "unsynced")
             THEN "durable" ELSE "unmarked"
  /\ UNCHANGED <<location, generation, sampled, cached, cacheFloor>>

CheckRead ==
  /\ read \in {"durable", "unmarked"}
  /\ read' = IF read = "durable" /\ sampled = generation THEN "checked" ELSE "idle"
  /\ UNCHANGED <<location, generation, sampled, cached, cacheFloor>>

InsertRead ==
  /\ read = "checked"
  /\ cached' = sampled
  /\ cacheFloor' = IF sampled > cacheFloor THEN sampled ELSE cacheFloor
  /\ read' = "idle"
  /\ UNCHANGED <<location, generation, sampled>>

AdmitOther ==
  /\ read # "checked"
  /\ location \in Backing
  /\ cached' = generation
  /\ cacheFloor' = generation
  /\ UNCHANGED <<location, generation, sampled, read>>

Next == Append \/ Sync \/ Remove \/ Evict \/ Drop \/ Replace \/
        BeginRead \/ Lookup \/ CheckRead \/ InsertRead \/ AdmitOther
Spec == Init /\ [][Next]_vars

CacheBacked == cached = generation => location \in Backing
AdmissionOrder == cached = cacheFloor
====
