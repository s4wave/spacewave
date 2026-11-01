---- MODULE DeadlistLedger ----
\* deadlists.go's save generations: up to three records and two saves in flight.
\* Callers found in replay.go: nameWorld -> name; buildHold/save -> resolve,
\* advance, carry, saved; accept -> adopt; confirmSave -> confirm; unsave ->
\* unsave; newReplay/restoreDeadlists -> lost. engine.go publication failure
\* calls lost when a hold's recount did not submit. Admission unwinds through
\* replay.unsave. These paths are represented below; encoding error leaves a
\* record waiting, and the next adoption may orphan it. Adoption of a disconnected chain orphans records. Capacity eviction is
\* excluded: the bound never fills the production limit of 64. SharedCounts
\* covers span payloads and interval netting.
\* A save snapshots names, puts, cursor and recount atomically under the
\* writer lock; carry and saved have no interleaving caller between them.
\* Confirm reports durability through a generation, never submission alone.
\* Publication order is assumed, as the model-check sidecar requires.
EXTENDS Naturals, FiniteSets
CONSTANTS ResetOnUnsave, ConfirmGeneration, CarryBlocks, MaxRecords
Records == 1..MaxRecords
Saves == 1..2
None == "none"
VARIABLES status, carried, stored, durable, adopted, pin, gen, flight, snapshot, puts,
          asks, recount, recountGen, lostAt, covered, next, dueAt, pinAt, parent,
          nameLost, savedCursor, driveCursor, landed, rebuilding, restarted, applied,
          used, seq, savedNext, savedAdopted, driveNext, driveAdopted
vars == <<status, carried, stored, durable, adopted, pin, gen, flight, snapshot, puts,
      asks, recount, recountGen, lostAt, covered, next, dueAt, pinAt, parent,
      nameLost, savedCursor, driveCursor, landed, rebuilding, restarted, applied,
      used, seq, savedNext, savedAdopted, driveNext, driveAdopted>>
Init ==
  /\ status = [r \in Records |-> "absent"]
  /\ carried = [r \in Records |-> 0] /\ stored = {}
  /\ durable = [r \in Records |-> None]
  /\ adopted = 0 /\ pin = 0 /\ gen = 0 /\ flight = {}
  /\ snapshot = [g \in Saves |-> [r \in Records |-> None]]
  /\ puts = [g \in Saves |-> {}] /\ asks = [g \in Saves |-> 0]
  /\ recount = FALSE /\ recountGen = 0 /\ lostAt = 0 /\ covered = 0
  /\ next = 1 /\ dueAt = [r \in Records |-> 0]
  /\ pinAt = [r \in Records |-> 0]
  /\ parent = [r \in Records |-> 0] /\ nameLost = [r \in Records |-> 0]
  /\ savedCursor = [g \in Saves |-> [r \in Records |-> "absent"]]
  /\ driveCursor = [r \in Records |-> "absent"]
  /\ landed = {} /\ rebuilding = FALSE /\ restarted = FALSE /\ applied = {}
  /\ used = {} /\ seq = [r \in Records |-> 0]
  /\ savedNext = [g \in Saves |-> 1] /\ savedAdopted = [g \in Saves |-> 0]
  /\ driveNext = 1 /\ driveAdopted = 0
Name(q, net) ==
  /\ Cardinality(used) < MaxRecords /\ q \in used \cup {0}
  /\ ~net => lostAt < 2
  /\ status' = [status EXCEPT ![Cardinality(used) + 1] = IF net THEN "waiting" ELSE "forfeited"] /\ next' = next + 1
  /\ used' = used \cup {Cardinality(used) + 1}
  /\ seq' = [seq EXCEPT ![Cardinality(used) + 1] = next]
  /\ parent' = [parent EXCEPT ![Cardinality(used) + 1] = q]
  /\ nameLost' = [nameLost EXCEPT ![Cardinality(used) + 1] = lostAt]
  /\ recount' = IF net THEN recount ELSE TRUE
  /\ recountGen' = IF net THEN recountGen ELSE 0
  /\ lostAt' = IF net THEN lostAt ELSE lostAt + 1
  /\ UNCHANGED <<carried, stored, durable, adopted, pin, gen, flight, snapshot, puts, asks,
      covered, dueAt, pinAt, savedCursor, driveCursor, landed, rebuilding,
      restarted, applied, savedNext, savedAdopted, driveNext, driveAdopted>>
Resolve(r) ==
  /\ status[r] = "waiting"
  /\ status' = [status EXCEPT ![r] = "pending"]
  /\ UNCHANGED <<carried, stored, durable, adopted, pin, gen, flight, snapshot, puts, asks,
      recount, recountGen, lostAt, covered, next, dueAt, pinAt, parent, nameLost,
      savedCursor, driveCursor, landed, rebuilding, restarted, applied, used, seq,
      savedNext, savedAdopted, driveNext, driveAdopted>>
\* One lost event may follow an earlier carried recount: older confirmation
\* must not clear the newer request. Two events suffice for that race.
Lost ==
  /\ lostAt < 2
  /\ lostAt' = lostAt + 1 /\ recount' = TRUE /\ recountGen' = 0
  /\ UNCHANGED <<status, carried, stored, durable, adopted, pin, gen, flight, snapshot, puts,
      asks, covered, next, dueAt, pinAt, parent, nameLost, savedCursor,
      driveCursor, landed, rebuilding, restarted, applied, used, seq, savedNext,
      savedAdopted, driveNext, driveAdopted>>
Pending == {r \in Records : status[r] \in {"waiting", "pending", "forfeited"}}
Prev(r) == LET ps == {q \in Pending : seq[q] < seq[r]}
           IN IF ps = {} THEN 0 ELSE CHOOSE q \in ps : \A x \in ps : seq[x] <= seq[q]
RECURSIVE Back(_)
Back(r) == {r} \cup IF Prev(r) # 0 /\ status[r] # "forfeited" /\ parent[r] = Prev(r)
                   THEN Back(Prev(r)) ELSE {}
Adopt(r) ==
  /\ r \in Pending /\ seq[r] > adopted
  /\ LET chain == Back(r)
         first == CHOOSE q \in chain : \A x \in chain : seq[q] <= seq[x]
         orphan == {q \in Pending : seq[q] < seq[first] /\ seq[q] > adopted}
         loses == \E q \in orphan : status[q] = "waiting"
     IN /\ loses => lostAt < 2
        /\ adopted' = seq[r]
        /\ status' = [q \in Records |-> IF q \in orphan THEN "orphaned" ELSE status[q]]
        /\ carried' = [q \in Records |-> IF q \in orphan THEN 0 ELSE carried[q]]
        /\ recount' = IF loses THEN TRUE ELSE recount
        /\ recountGen' = IF loses THEN 0 ELSE recountGen
        /\ lostAt' = IF loses THEN lostAt + 1 ELSE lostAt
  /\ UNCHANGED <<stored, durable, pin, gen, flight, snapshot, puts, asks, covered, next,
      dueAt, pinAt, parent, nameLost, savedCursor, driveCursor, landed,
      rebuilding, restarted, applied, used, seq, savedNext, savedAdopted,
      driveNext, driveAdopted>>
\* The oldest reader is pinned at epoch zero; releasing it opens the watermark.
\* A later epoch changes only the numeric bound, checked by DueWatermark.
ReleasePin ==
  /\ pin = 0 /\ pin' = 3
  /\ UNCHANGED <<status, carried, stored, durable, adopted, gen, flight, snapshot, puts,
      asks, recount, recountGen, lostAt, covered, next, dueAt, pinAt, parent,
      nameLost, savedCursor, driveCursor, landed, rebuilding, restarted, applied,
      used, seq, savedNext, savedAdopted, driveNext, driveAdopted>>
Advance(r) ==
  /\ status[r] \in {"pending", "forfeited"} /\ seq[r] <= adopted /\ seq[r] <= pin
  /\ \A q \in Pending : seq[q] < seq[r] => status[q] # "waiting"
  /\ status' = [status EXCEPT ![r] = IF @ = "forfeited" THEN "absent" ELSE "due"]
  /\ carried' = [carried EXCEPT ![r] = 0]
  /\ dueAt' = [dueAt EXCEPT ![r] = adopted]
  /\ pinAt' = [pinAt EXCEPT ![r] = pin]
  /\ UNCHANGED <<stored, durable, adopted, pin, gen, flight, snapshot, puts, asks, recount,
      recountGen, lostAt, covered, next, parent, nameLost, savedCursor,
      driveCursor, landed, rebuilding, restarted, applied, used, seq, savedNext,
      savedAdopted, driveNext, driveAdopted>>
\* carry/saved snapshot precisely the entries not carried by an earlier save.
CarrySaved ==
  /\ gen < 2
  /\ LET g == gen + 1
         selected == {r \in Records : carried[r] = 0 /\
                       status[r] \in {"pending", "due", "orphaned"}}
     IN /\ snapshot' = [snapshot EXCEPT ![g] = [r \in Records |->
                    IF r \in selected THEN status[r] ELSE None]]
        /\ puts' = [puts EXCEPT ![g] = IF CarryBlocks
                   THEN {r \in selected : r \notin stored /\ status[r] # "orphaned"}
                   ELSE {}]
        /\ savedNext' = [savedNext EXCEPT ![g] = next]
        /\ savedAdopted' = [savedAdopted EXCEPT ![g] = adopted]
        /\ savedCursor' = [savedCursor EXCEPT ![g] = [r \in Records |->
             IF status[r] = "waiting" THEN "forfeited"
             ELSE IF status[r] \in {"pending", "forfeited"} THEN status[r] ELSE "absent"]]
        /\ asks' = [asks EXCEPT ![g] = IF recount /\ recountGen = 0
                     THEN lostAt ELSE 0]
        /\ recountGen' = IF recount /\ recountGen = 0 THEN g ELSE recountGen
        /\ carried' = [r \in Records |-> IF r \in selected THEN g ELSE carried[r]]
        /\ flight' = flight \cup {g} /\ gen' = g
  /\ UNCHANGED <<status, stored, durable, adopted, pin, recount, lostAt, covered, next,
      dueAt, pinAt, parent, nameLost, driveCursor, landed, rebuilding, restarted,
      applied, used, seq, driveNext, driveAdopted>>
\* Completed or discarded save payloads are forgotten: the engine no longer
\* reads their snapshots. This removes history that cannot change a future
\* transition, keeping the three-record model tractable.
\* A durable save stores its puts before its names. Saves land in order.
LandSave(g) ==
  /\ g \in flight \ landed
  /\ \A h \in flight : h < g => h \in landed
  /\ stored' = stored \cup puts[g]
  /\ driveCursor' = savedCursor[g]
  /\ driveNext' = savedNext[g] /\ driveAdopted' = savedAdopted[g]
  /\ durable' = [r \in Records |->
       IF snapshot[g][r] = None THEN durable[r]
       ELSE IF snapshot[g][r] = "orphaned" THEN None ELSE snapshot[g][r]]
  /\ covered' = IF asks[g] > covered THEN asks[g] ELSE covered
  /\ landed' = landed \cup {g}
  /\ rebuilding' = (rebuilding \/ asks[g] > 0)
  /\ UNCHANGED <<status, carried, adopted, pin, gen, flight, snapshot, puts, asks, recount,
      recountGen, lostAt, next, dueAt, pinAt, parent, nameLost, savedCursor,
      restarted, applied, used, seq, savedNext, savedAdopted>>
Confirm(g) ==
  /\ g \in flight \cap landed /\ \A h \in flight : g <= h
  /\ LET clear == recountGen # 0 /\
           (IF ConfirmGeneration THEN recountGen <= g ELSE TRUE)
     IN /\ recount' = IF clear THEN FALSE ELSE recount
        /\ recountGen' = IF clear THEN 0 ELSE recountGen
  /\ flight' = flight \ {g}
  /\ snapshot' = [snapshot EXCEPT ![g] = [r \in Records |-> None]]
  /\ puts' = [puts EXCEPT ![g] = {}]
  /\ asks' = [asks EXCEPT ![g] = 0]
  /\ savedCursor' = [savedCursor EXCEPT ![g] = [r \in Records |-> "absent"]]
  /\ savedNext' = [savedNext EXCEPT ![g] = 1]
  /\ savedAdopted' = [savedAdopted EXCEPT ![g] = 0]
  /\ UNCHANGED <<status, carried, adopted, pin, gen, lostAt, next,
      dueAt, pinAt, parent, nameLost, stored, durable, driveCursor,
      covered, landed, rebuilding, restarted, applied, used, seq, driveNext, driveAdopted>>
\* Failure discards every unconfirmed save, and resets their carry markers.
Unsave ==
  /\ flight # {}
  /\ flight' = {} /\ landed' = {} /\ carried' = [r \in Records |-> 0]
  /\ recountGen' = IF ResetOnUnsave THEN 0 ELSE recountGen
  /\ snapshot' = [g \in Saves |-> [r \in Records |-> None]]
  /\ puts' = [g \in Saves |-> {}]
  /\ asks' = [g \in Saves |-> 0]
  /\ savedCursor' = [g \in Saves |-> [r \in Records |-> "absent"]]
  /\ savedNext' = [g \in Saves |-> 1]
  /\ savedAdopted' = [g \in Saves |-> 0]
  /\ UNCHANGED <<status, stored, durable, adopted, pin, gen, recount,
      lostAt, covered, next, dueAt, pinAt, parent, nameLost, driveCursor, rebuilding, restarted, applied, used, seq, driveNext, driveAdopted>>
\* Restart restores the saved cursor: an unresolved record becomes forfeited.
\* Names and cursor are separate durable state; a pending cursor with no
\* record has no blocks to carry and its deaths leak. Sequence numbers restore with the cursor and may be reused; unique model
\* IDs distinguish new record blocks from older blocks with the same number.
\* Record payload encoding is excluded.
RestoreDeadlists ==
  /\ ~restarted /\ lostAt < 2
  /\ flight' = {} /\ landed' = {} /\ restarted' = TRUE /\ carried' = [r \in Records |-> 0]
  /\ status' = driveCursor /\ next' = driveNext /\ adopted' = driveAdopted
  /\ gen' = 0 /\ pin' = 3
  /\ recount' = TRUE /\ recountGen' = 0 /\ lostAt' = lostAt + 1
  /\ snapshot' = [g \in Saves |-> [r \in Records |-> None]]
  /\ puts' = [g \in Saves |-> {}]
  /\ asks' = [g \in Saves |-> 0]
  /\ savedCursor' = [g \in Saves |-> [r \in Records |-> "absent"]]
  /\ savedNext' = [g \in Saves |-> 1]
  /\ savedAdopted' = [g \in Saves |-> 0]
  /\ UNCHANGED <<stored, durable, covered, dueAt, pinAt, parent,
      nameLost, driveCursor, rebuilding, applied, used, seq,
      driveNext, driveAdopted>>
\* Completing the store's recount is independent of confirming its receipt.
FinishRecount ==
  /\ rebuilding /\ rebuilding' = FALSE
  /\ UNCHANGED <<status, carried, stored, durable, adopted, pin, gen, flight, snapshot, puts,
      asks, recount, recountGen, lostAt, covered, next, dueAt, pinAt, parent,
      nameLost, savedCursor, driveCursor, landed, restarted, applied, used, seq,
      savedNext, savedAdopted, driveNext, driveAdopted>>
Apply(r) ==
  /\ durable[r] = "due" /\ ~rebuilding /\ r \notin applied
  /\ applied' = applied \cup {r}
  /\ durable' = [durable EXCEPT ![r] = None]
  /\ UNCHANGED <<status, carried, stored, adopted, pin, gen, flight, snapshot, puts, asks,
      recount, recountGen, lostAt, covered, next, dueAt, pinAt, parent, nameLost,
      savedCursor, driveCursor, landed, rebuilding, restarted, used, seq,
      savedNext, savedAdopted, driveNext, driveAdopted>>
Next == (\E q \in 0..MaxRecords, net \in BOOLEAN : Name(q, net)) \/ Lost \/ ReleasePin \/ CarrySaved \/ Unsave \/ RestoreDeadlists \/ FinishRecount
  \/ (\E r \in Records : Resolve(r) \/ Adopt(r) \/ Advance(r) \/ Apply(r))
  \/ (\E g \in Saves : LandSave(g) \/ Confirm(g)) \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars
DurableNamesStored == {r \in Records : durable[r] # None} \subseteq stored
DueWatermark == \A r \in Records : status[r] = "due" =>
                 seq[r] <= dueAt[r] /\ seq[r] <= pinAt[r]
\* Request responsibility cannot disappear between lost and a durable save.
RecountCarried == lostAt > covered =>
  recount /\ (recountGen = 0 \/
    (recountGen \in flight /\ asks[recountGen] >= lostAt))
\* When an orphan's removal is what the durable save carries, no name survives.
RecountBeforeDue == \A r \in Records : durable[r] = "due" => covered >= nameLost[r]
RecountBeforeApply == \A r \in applied : covered >= nameLost[r]
OrphanReleased == \A r \in Records :
  (status[r] = "orphaned" /\ carried[r] # 0 /\ carried[r] \notin flight
      ) => durable[r] = None
====
