---- MODULE LogSpace ----
\* placeLog backs ordinary extents and a standby before publication. Bounded
\* free/mark records can spend that standby on a full disk. Backing assumes
\* in-place writes: filesystem metadata failure and CoW snapshots needing more
\* allocation are outside this model. Free/Pins/Durability cover reachability
\* and crash ordering. Reservation and truncate callers must account for log
\* allocations made by the record that publishes their operation.
EXTENDS Naturals, FiniteSets
Max(a,b) == IF a > b THEN a ELSE b
VARIABLES active, spare, backed, free, room, full, pending, length,
 output, published, owner, allocated
vars == <<active, spare, backed, free, room, full, pending, length, output, published, owner, allocated>>
Init == /\ active = 1 /\ spare = 2 /\ backed = {1,2,3}
 /\ free = {4} /\ room = TRUE /\ full = FALSE /\ pending = TRUE /\ length = 5
 /\ output = "free" /\ published = FALSE /\ owner \in {1,2} /\ allocated = FALSE
FillDisk == /\ ~full /\ full' = TRUE
 /\ UNCHANGED <<active, spare, backed, free, room, pending, length, output, published, owner, allocated>>
FillLog == /\ room /\ room' = FALSE
 /\ UNCHANGED <<active, spare, backed, free, full, pending, length, output, published, owner, allocated>>
FreeRecord ==
 /\ pending /\ (room \/ spare \in backed)
 /\ active' = IF room THEN active ELSE spare
 /\ spare' = IF room THEN spare ELSE 0
 /\ pending' = FALSE /\ room' = TRUE /\ free' = free \cup {3}
 /\ UNCHANGED <<backed, full, length, output, published, owner, allocated>>
OrdinaryRoll ==
 /\ ~full /\ ~room /\ spare # 0 /\ free # {}
 /\ \E c \in free :
  /\ active' = spare /\ spare' = c /\ backed' = backed \cup {c}
  /\ free' = free \ {c} /\ room' = TRUE /\ length' = Max(length, c+1)
 /\ UNCHANGED <<full, pending, output, published, owner, allocated>>
Replenish ==
 /\ spare = 0 /\ free \cap backed # {}
 /\ \E c \in free \cap backed : /\ spare' = c /\ free' = free \ {c}
 /\ UNCHANGED <<active, backed, room, full, pending, length, output, published, owner, allocated>>
ReserveOutput ==
 /\ output = "free" /\ 4 \in free /\ ~published
 /\ output' = "reserved" /\ allocated' = TRUE /\ free' = free \ {4}
 /\ UNCHANGED <<active, spare, backed, room, full, pending, length, published, owner>>
PublishOutput ==
 /\ output = "reserved" /\ output' = "live" /\ published' = TRUE
 /\ UNCHANGED <<active, spare, backed, free, room, full, pending, length, owner, allocated>>
CleanupOutput ==
 /\ output = "reserved" /\ owner = 1 /\ output' = "free" /\ free' = free \cup {4}
 /\ UNCHANGED <<active, spare, backed, room, full, pending, length, published, owner, allocated>>
TruncateRecord ==
 /\ ~pending /\ ~room /\ (spare # 0 \/ free # {})
 /\ \E c \in (IF spare # 0 THEN {spare} ELSE free) :
  /\ ~full \/ c \in backed
  /\ active' = c /\ spare' = 0 /\ room' = TRUE
  /\ LET target == Max(active+1, spare+1)
          finish == Max(target, c+1)
      IN /\ length' = finish
         /\ free' = {f \in free \ {c} : f < finish}
         /\ backed' = {b \in backed \cup {c} : b < finish}
 /\ UNCHANGED <<full, pending, output, published, owner, allocated>>
Next == FillDisk \/ FillLog \/ FreeRecord \/ OrdinaryRoll \/ Replenish
 \/ ReserveOutput \/ PublishOutput \/ CleanupOutput \/ TruncateRecord \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars /\ WF_vars(FreeRecord)
FreeCanProgress == pending => room \/ spare \in backed
NoLiveReuse == (active # 3 /\ spare # 3) \/ ~pending
OutputSafe == (published => output = "live") /\ ((owner = 2 /\ allocated) => output # "free")
OutputSeparate == output \in {"reserved", "live"} => active # 4 /\ spare # 4
LogWithinFile == active < length /\ spare < length
PendingDrains == pending ~> ~pending
====
