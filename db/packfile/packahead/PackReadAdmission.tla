---- MODULE PackReadAdmission ----
\* One block, one read in flight, and at most MaxDrops replacements.
\* The lookup and drop each hold the log lock. Sampling the global
\* generation and admitting the decoded block run outside that lock.
EXTENDS Naturals

CONSTANTS MaxDrops, RequireSync, InvalidateDrop
VARIABLES location, generation, sampled, read, cached
vars == <<location, generation, sampled, read, cached>>

Init ==
  /\ location = "absent"
  /\ generation = 1
  /\ sampled = 0
  /\ read = "idle"
  /\ cached = 0

Append ==
  /\ location = "absent"
  /\ location' = "unsynced"
  /\ UNCHANGED <<generation, sampled, read, cached>>

Sync ==
  /\ location = "unsynced"
  /\ location' = "synced"
  /\ UNCHANGED <<generation, sampled, read, cached>>

Remove ==
  /\ location = "synced"
  /\ generation < MaxDrops
  /\ location' = "removed"
  /\ generation' = generation + 1
  /\ UNCHANGED <<sampled, read, cached>>

Drop ==
  /\ location # "absent"
  /\ generation < MaxDrops
  /\ location' = "absent"
  /\ generation' = IF InvalidateDrop THEN generation + 1 ELSE generation
  /\ UNCHANGED <<sampled, read, cached>>

Replace ==
  /\ location \in {"synced", "removed"}
  /\ generation < MaxDrops
  /\ location' = "unsynced"
  /\ generation' = IF InvalidateDrop THEN generation + 1 ELSE generation
  /\ UNCHANGED <<sampled, read, cached>>

BeginRead ==
  /\ read = "idle"
  /\ read' = "started"
  /\ sampled' = generation
  /\ UNCHANGED <<location, generation, cached>>

Lookup ==
  /\ read = "started"
  /\ read' = IF location \in {"synced", "removed"} \/
                (~RequireSync /\ location = "unsynced")
             THEN "durable" ELSE "unmarked"
  /\ UNCHANGED <<location, generation, sampled, cached>>

FinishRead ==
  /\ read \in {"durable", "unmarked"}
  /\ cached' = IF read = "durable" /\ sampled = generation THEN sampled ELSE cached
  /\ read' = "idle"
  /\ UNCHANGED <<location, generation, sampled>>

Next == Append \/ Sync \/ Remove \/ Drop \/ Replace \/ BeginRead \/ Lookup \/ FinishRead
Spec == Init /\ [][Next]_vars

CacheBacked == cached = generation => location \in {"synced", "removed"}
====
