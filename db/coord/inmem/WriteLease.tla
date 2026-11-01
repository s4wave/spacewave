---- MODULE WriteLease ----
EXTENDS Naturals, FiniteSets
(* coordinator.go TryAcquireWriteLease/WaitAcquireWriteLease, lease.go Publish
   and Release. Two lease handles on one unkeyed scope. Scope consumers in
   volume, object and provider layers acquire through coord.Coordinator.
   Mutex sections linearize ownership, generation changes and idempotent
   release. Publish rejects released handles even after another acquires.
   Caller cancellation terminates a wait; cond wake scheduling and Watch's
   bounded event channel are excluded. Keyed scopes do not publish generations.
   At most two publishes, requested generation equal to the current generation:
   this exercises Publish's current+1 branch; arbitrary generation jumps and
   uint64 overflow are outside this bound. *)
CONSTANT CheckReleased
VARIABLES owner, held, released, generation, writes, staleWrite
vars == <<owner, held, released, generation, writes, staleWrite>>
Init == /\ owner = 0 /\ held = {} /\ released = {}
        /\ generation = 0 /\ writes = 0 /\ staleWrite = FALSE
Acquire(a) == /\ owner = 0 /\ a \notin released /\ a \notin held
              /\ owner' = a /\ held' = held \cup {a}
              /\ UNCHANGED <<released, generation, writes, staleWrite>>
Publish(a) ==
  /\ a \in held /\ owner # 0 /\ writes < 2
  /\ (CheckReleased => a \notin released)
  /\ generation' = generation + 1 /\ writes' = writes + 1
  /\ staleWrite' = (staleWrite \/ a \in released \/ owner # a)
  /\ UNCHANGED <<owner, held, released>>
Release(a) ==
  /\ a \in held /\ a \notin released
  /\ owner' = 0 /\ released' = released \cup {a}
  /\ UNCHANGED <<held, generation, writes, staleWrite>>
Next == (\E a \in {1, 2}: Acquire(a) \/ Publish(a) \/ Release(a)) \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars
Exclusive == Cardinality(held \ released) <= 1
ReleasedCannotPublish == ~staleWrite
GenerationAdvances == generation = writes
====
