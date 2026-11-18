-------------------------- MODULE ColdPackScheduling ---------------------------
EXTENDS Naturals, FiniteSets

(***************************************************************************
 Model the shared demand lane, the mover's reservation, and the two tail
 caches in one mount. A move can wait with or without a reservation; it can
 cancel at either wait. User admission does not create a waiting state.
 An index fill abstracts authentication and decoding into publication. One
 resident slot per cache suffices to test cross-cache eviction. The separate
 background tail worker, deadlines and bucket bytes are outside this model.
***************************************************************************)
CONSTANTS Slots, MoveSlots, Moves, Tails, NoTail
VARIABLES queued, reserved, moving, users, userTail, moveTail,
          moveEvictedUser, userWaited
vars == <<queued, reserved, moving, users, userTail, moveTail,
          moveEvictedUser, userWaited>>

Init == /\ queued = {} /\ reserved = {} /\ moving = {} /\ users = 0
        /\ userTail = NoTail /\ moveTail = NoTail
        /\ moveEvictedUser = FALSE /\ userWaited = FALSE

Full == users + Cardinality(moving) = Slots

QueueMove(m) == /\ m \notin (queued \cup reserved \cup moving)
                /\ queued' = queued \cup {m}
                /\ UNCHANGED <<reserved, moving, users, userTail, moveTail,
                               moveEvictedUser, userWaited>>

ReserveMove(m) == /\ m \in queued
                  /\ Cardinality(reserved \cup moving) < MoveSlots
                  /\ queued' = queued \ {m} /\ reserved' = reserved \cup {m}
                  /\ UNCHANGED <<moving, users, userTail, moveTail,
                                 moveEvictedUser, userWaited>>

AdmitMove(m) == /\ m \in reserved /\ ~Full
                /\ reserved' = reserved \ {m} /\ moving' = moving \cup {m}
                /\ UNCHANGED <<queued, users, userTail, moveTail,
                               moveEvictedUser, userWaited>>

CancelMove(m) == /\ m \in (queued \cup reserved)
                 /\ queued' = queued \ {m} /\ reserved' = reserved \ {m}
                 /\ UNCHANGED <<moving, users, userTail, moveTail,
                                moveEvictedUser, userWaited>>

FinishMove(m) == /\ m \in moving /\ moving' = moving \ {m}
                 /\ UNCHANGED <<queued, reserved, users, userTail, moveTail,
                                moveEvictedUser, userWaited>>

AdmitUser == /\ users' = IF Full THEN users ELSE users + 1
             /\ userWaited' = FALSE
             /\ UNCHANGED <<queued, reserved, moving, userTail, moveTail,
                            moveEvictedUser>>

FinishUser == /\ users > 0 /\ users' = users - 1
              /\ UNCHANGED <<queued, reserved, moving, userTail, moveTail,
                             moveEvictedUser, userWaited>>

CacheUser(t) == /\ users > 0 /\ userTail' = t
                /\ UNCHANGED <<queued, reserved, moving, users, moveTail,
                               moveEvictedUser, userWaited>>

CacheMove(t) == /\ moving # {} /\ moveTail' = t
                /\ userTail' = userTail
                /\ moveEvictedUser' = (userTail # NoTail /\ userTail' # userTail)
                /\ UNCHANGED <<queued, reserved, moving, users, userWaited>>

Next == \/ \E m \in Moves: QueueMove(m) \/ ReserveMove(m) \/ AdmitMove(m)
                          \/ CancelMove(m) \/ FinishMove(m)
        \/ AdmitUser \/ FinishUser
        \/ \E t \in Tails: CacheUser(t) \/ CacheMove(t)

Spec == Init /\ [][Next]_vars
AtMostDemandSlots == users + Cardinality(moving) <= Slots
MoveReservationsBounded == Cardinality(reserved \cup moving) <= MoveSlots
NoUserWaiting == ~userWaited
NoMoveEviction == ~moveEvictedUser
=============================================================================
