---- MODULE DeadlistLedger ----
\* deadlists.go's save generations: three records and two saves in flight.
\* Callers found in replay.go: nameWorld -> name; buildHold/save -> resolve,
\* advance, carry, saved; accept -> adopt; confirmSave -> confirm; unsave ->
\* unsave; newReplay/restoreDeadlists -> lost. engine.go publication failure
\* calls lost when a hold's recount did not submit. Admission unwinds through
\* replay.unsave. These paths are represented below; encoding error leaves a
\* record waiting, and the next adoption may orphan it. The limit orphan is
\* represented by Orphan. Span payloads and netting are in SharedCounts.
\* A save snapshots names, puts, cursor and recount atomically under the
\* writer lock; carry and saved have no interleaving caller between them.
\* Confirm reports durability through a generation, never submission alone.
\* Publication order is assumed, as the model-check sidecar requires.
EXTENDS Naturals, FiniteSets
CONSTANTS ResetOnUnsave, ConfirmGeneration, CarryBlocks
Records == 1..3
Saves == 1..2
None == "none"
VARIABLES status, carried, stored, durable, adopted, pin, gen, flight,
          snapshot, puts, asks, recount, recountGen, lostAt, covered, next,
          dueAt, pinAt
vars == <<status, carried, stored, durable, adopted, pin, gen, flight,
          snapshot, puts, asks, recount, recountGen, lostAt, covered, next,
          dueAt, pinAt>>
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
Name ==
  /\ next \in Records
  /\ status' = [status EXCEPT ![next] = "waiting"] /\ next' = next + 1
  /\ UNCHANGED <<carried, stored, durable, adopted, pin, gen, flight,
      snapshot, puts, asks, recount, recountGen, lostAt, covered, dueAt, pinAt>>
Resolve(r) ==
  /\ status[r] = "waiting"
  /\ status' = [status EXCEPT ![r] = "pending"]
  /\ UNCHANGED <<carried, stored, durable, adopted, pin, gen, flight,
      snapshot, puts, asks, recount, recountGen, lostAt, covered, next, dueAt, pinAt>>
\* One lost event may follow an earlier carried recount: older confirmation
\* must not clear the newer request. Two events suffice for that race.
Lost ==
  /\ lostAt < 2
  /\ lostAt' = lostAt + 1 /\ recount' = TRUE /\ recountGen' = 0
  /\ UNCHANGED <<status, carried, stored, durable, adopted, pin, gen, flight,
      snapshot, puts, asks, covered, next, dueAt, pinAt>>
Adopt(r) ==
  /\ status[r] \in {"waiting", "pending"} /\ r > adopted
  /\ adopted' = r
  /\ UNCHANGED <<status, carried, stored, durable, pin, gen, flight,
      snapshot, puts, asks, recount, recountGen, lostAt, covered, next, dueAt, pinAt>>
\* The one reader opens at epoch zero; releasing it opens the watermark.
ReleasePin ==
  /\ pin = 0 /\ pin' = 3
  /\ UNCHANGED <<status, carried, stored, durable, adopted, gen, flight,
      snapshot, puts, asks, recount, recountGen, lostAt, covered, next, dueAt, pinAt>>
Advance(r) ==
  /\ status[r] = "pending" /\ r <= adopted /\ r <= pin
  /\ \A q \in 1..(r - 1) : status[q] # "waiting"
  /\ status' = [status EXCEPT ![r] = "due"]
  /\ carried' = [carried EXCEPT ![r] = 0]
  /\ dueAt' = [dueAt EXCEPT ![r] = adopted]
  /\ pinAt' = [pinAt EXCEPT ![r] = pin]
  /\ UNCHANGED <<stored, durable, adopted, pin, gen, flight,
      snapshot, puts, asks, recount, recountGen, lostAt, covered, next>>
Orphan(r) ==
  /\ status[r] \in {"waiting", "pending"}
  /\ status' = [status EXCEPT ![r] = "orphaned"]
  /\ carried' = [carried EXCEPT ![r] = 0]
  /\ recount' = IF status[r] = "waiting" THEN TRUE ELSE recount
  /\ recountGen' = IF status[r] = "waiting" THEN 0 ELSE recountGen
  /\ lostAt' = IF status[r] = "waiting" THEN 2 ELSE lostAt
  /\ UNCHANGED <<stored, durable, adopted, pin, gen, flight,
      snapshot, puts, asks, covered, next, dueAt, pinAt>>
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
        /\ asks' = [asks EXCEPT ![g] = IF recount /\ recountGen = 0
                     THEN lostAt ELSE 0]
        /\ recountGen' = IF recount /\ recountGen = 0 THEN g ELSE recountGen
        /\ carried' = [r \in Records |-> IF r \in selected THEN g ELSE carried[r]]
        /\ flight' = flight \cup {g} /\ gen' = g
  /\ UNCHANGED <<status, stored, durable, adopted, pin, recount, lostAt,
                  covered, next, dueAt, pinAt>>
\* A durable save stores its puts before its names. Saves land in order.
Confirm(g) ==
  /\ g \in flight /\ \A h \in flight : g <= h
  /\ stored' = stored \cup puts[g]
  /\ durable' = [r \in Records |->
       IF snapshot[g][r] = None THEN durable[r]
       ELSE IF snapshot[g][r] = "orphaned" THEN None ELSE snapshot[g][r]]
  /\ covered' = IF asks[g] > covered THEN asks[g] ELSE covered
  /\ LET clear == recountGen # 0 /\
           (IF ConfirmGeneration THEN recountGen <= g ELSE TRUE)
     IN /\ recount' = IF clear THEN FALSE ELSE recount
        /\ recountGen' = IF clear THEN 0 ELSE recountGen
  /\ flight' = flight \ {g}
  /\ UNCHANGED <<status, carried, adopted, pin, gen, snapshot, puts, asks,
                  lostAt, next, dueAt, pinAt>>
\* Failure discards every unconfirmed save, and resets their carry markers.
Unsave ==
  /\ flight # {}
  /\ flight' = {} /\ carried' = [r \in Records |-> 0]
  /\ recountGen' = IF ResetOnUnsave THEN 0 ELSE recountGen
  /\ UNCHANGED <<status, stored, durable, adopted, pin, gen, snapshot, puts,
                  asks, recount, lostAt, covered, next, dueAt, pinAt>>
\* Restart restores only durable pending names; every other interval leaks.
RestoreDeadlists ==
  /\ flight # {} /\ lostAt < 2
  /\ flight' = {} /\ carried' = [r \in Records |-> 0]
  /\ status' = [r \in Records |-> IF durable[r] = "pending"
                                  THEN "pending" ELSE "absent"]
  /\ recount' = TRUE /\ recountGen' = 0 /\ lostAt' = lostAt + 1
  /\ UNCHANGED <<stored, durable, adopted, pin, gen, snapshot, puts, asks,
                  covered, next, dueAt, pinAt>>
Next == Name \/ Lost \/ ReleasePin \/ CarrySaved \/ Unsave \/ RestoreDeadlists
  \/ (\E r \in Records : Resolve(r) \/ Adopt(r) \/ Advance(r) \/ Orphan(r))
  \/ (\E g \in Saves : Confirm(g)) \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars
DurableNamesStored == {r \in Records : durable[r] # None} \subseteq stored
DueWatermark == \A r \in Records : status[r] = "due" =>
                 r <= dueAt[r] /\ r <= pinAt[r]
\* Request responsibility cannot disappear between lost and a durable save.
RecountCarried == lostAt > covered =>
  recount /\ (recountGen = 0 \/
    (recountGen \in flight /\ asks[recountGen] >= lostAt))
\* When an orphan's removal is what the durable save carries, no name survives.
OrphanReleased == \A r \in Records :
  (status[r] = "orphaned" /\ carried[r] # 0 /\ carried[r] \notin flight
      /\ snapshot[carried[r]][r] = "orphaned") => durable[r] = None
====
