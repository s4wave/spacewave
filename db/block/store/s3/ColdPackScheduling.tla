-------------------------- MODULE ColdPackScheduling ---------------------------
EXTENDS Naturals, FiniteSets

(***************************************************************************
 Model the shared demand lane, the mover's reservation, and the two tail
  caches in one mount. Counters represent channel tokens; independent live
  owners explain every token. A move can cancel at either wait.
 An index fill abstracts authentication and decoding into publication. One
 resident slot per cache suffices to test cross-cache eviction. The separate
 background tail worker, deadlines and bucket bytes are outside this model.
***************************************************************************)
CONSTANTS Slots, MoveSlots, Moves, Tails, NoTail
VARIABLES queued, reserved, moving, users, demandPermits, movePermits,
          userTail, moveTail, moveEvictedUser, userWaited
vars == <<queued, reserved, moving, users, demandPermits, movePermits,
          userTail, moveTail, moveEvictedUser, userWaited>>

Init == /\ queued = {} /\ reserved = {} /\ moving = {} /\ users = 0
        /\ demandPermits = 0 /\ movePermits = 0
        /\ userTail = NoTail /\ moveTail = NoTail
        /\ moveEvictedUser = FALSE /\ userWaited = FALSE

Full == demandPermits = Slots

QueueMove(m) == /\ m \notin (queued \cup reserved \cup moving)
                /\ queued' = queued \cup {m}
                /\ UNCHANGED <<reserved, moving, users, demandPermits,
                               movePermits, userTail, moveTail,
                               moveEvictedUser, userWaited>>

ReserveMove(m) == /\ m \in queued /\ movePermits < MoveSlots
                  /\ queued' = queued \ {m} /\ reserved' = reserved \cup {m}
                  /\ movePermits' = movePermits + 1
                  /\ UNCHANGED <<moving, users, demandPermits, userTail,
                                 moveTail, moveEvictedUser, userWaited>>

AdmitMove(m) == /\ m \in reserved /\ ~Full
                /\ reserved' = reserved \ {m} /\ moving' = moving \cup {m}
                /\ demandPermits' = demandPermits + 1
                /\ UNCHANGED <<queued, users, movePermits, userTail, moveTail,
                               moveEvictedUser, userWaited>>

CancelMove(m) == /\ m \in (queued \cup reserved)
                 /\ queued' = queued \ {m} /\ reserved' = reserved \ {m}
                 /\ movePermits' = IF m \in reserved THEN movePermits - 1 ELSE movePermits
                 /\ UNCHANGED <<moving, users, demandPermits, userTail,
                                moveTail, moveEvictedUser, userWaited>>

(* Finishing a range includes errors, timeouts and an unwind. *)
FinishMove(m) == /\ m \in moving /\ moving' = moving \ {m}
                 /\ demandPermits' = demandPermits - 1
                 /\ movePermits' = movePermits - 1
                 /\ UNCHANGED <<queued, reserved, users, userTail, moveTail,
                                moveEvictedUser, userWaited>>

AdmitUser == /\ users' = IF Full THEN users ELSE users + 1
             /\ demandPermits' = IF Full THEN demandPermits ELSE demandPermits + 1
             /\ userWaited' = FALSE
             /\ UNCHANGED <<queued, reserved, moving, movePermits, userTail,
                            moveTail, moveEvictedUser>>

FinishUser == /\ users > 0 /\ users' = users - 1
              /\ demandPermits' = demandPermits - 1
              /\ UNCHANGED <<queued, reserved, moving, movePermits, userTail,
                             moveTail, moveEvictedUser, userWaited>>

CacheUser(t) == /\ users > 0 /\ userTail' = t
                /\ UNCHANGED <<queued, reserved, moving, users, demandPermits,
                               movePermits, moveTail, moveEvictedUser, userWaited>>

CacheMove(t) == /\ moving # {} /\ moveTail' = t
                /\ userTail' = userTail
                /\ moveEvictedUser' = (userTail # NoTail /\ userTail' # userTail)
                /\ UNCHANGED <<queued, reserved, moving, users, demandPermits,
                               movePermits, userWaited>>

Next == \/ \E m \in Moves: QueueMove(m) \/ ReserveMove(m) \/ AdmitMove(m)
                          \/ CancelMove(m) \/ FinishMove(m)
        \/ AdmitUser \/ FinishUser
        \/ \E t \in Tails: CacheUser(t) \/ CacheMove(t)

Spec == Init /\ [][Next]_vars
AtMostDemandSlots == demandPermits <= Slots
MoveReservationsBounded == movePermits <= MoveSlots
PermitOwnersMatch == /\ demandPermits = users + Cardinality(moving)
                     /\ movePermits = Cardinality(reserved \cup moving)
NoUserWaiting == ~userWaited
NoMoveEviction == ~moveEvictedUser
=============================================================================
