---- MODULE Registry ----
\* Only generation -> registry nesting is allowed. Drain owns detached maps.
EXTENDS Naturals, FiniteSets, TLC
CONSTANT HoldDuringDrain
Ids == 1..3
VARIABLES live, owned, detached, released, registered, init, registry, generation, reg, retire
vars == <<live, owned, detached, released, registered, init, registry, generation, reg, retire>>
Init == /\ live = TRUE /\ owned = {} /\ detached = {}
        /\ released = [i \in Ids |-> 0] /\ registered = {}
        /\ init = "waiting" /\ registry = "free" /\ generation = "free"
        /\ reg = "waiting" /\ retire = "waiting"
RegAcquire == /\ reg = "waiting" /\ generation = "free"
              /\ reg' = "generation" /\ generation' = "register"
              /\ UNCHANGED <<live, owned, detached, released, registered, init, registry, retire>>
RegIndex == /\ reg = "generation" /\ registry = "free"
            /\ reg' = "indexed" /\ registry' = "register"
            /\ UNCHANGED <<live, owned, detached, released, registered, init, generation, retire>>
Register(i) == /\ reg = "indexed" /\ i \notin registered
               /\ owned' = IF live THEN owned \cup {i} ELSE owned
               /\ registered' = IF live THEN registered \cup {i} ELSE registered
               /\ reg' = "committed"
               /\ UNCHANGED <<live, detached, released, init, registry, generation, retire>>
RegUnindex == /\ reg = "committed" /\ registry' = "free" /\ reg' = "unlock"
              /\ UNCHANGED <<live, owned, detached, released, registered, init, generation, retire>>
RegRelease == /\ reg = "unlock" /\ generation' = "free" /\ reg' = "done"
              /\ UNCHANGED <<live, owned, detached, released, registered, init, registry, retire>>
RetireAcquire == /\ retire = "waiting" /\ generation = "free"
                 /\ retire' = "generation" /\ generation' = "retire"
                 /\ UNCHANGED <<live, owned, detached, released, registered, init, registry, reg>>
RetireIndex == /\ retire = "generation" /\ registry = "free"
               /\ retire' = "indexed" /\ registry' = "retire"
               /\ UNCHANGED <<live, owned, detached, released, registered, init, generation, reg>>
Detach == /\ retire = "indexed" /\ live
          /\ live' = FALSE /\ detached' = owned /\ owned' = {} /\ retire' = "detached"
          /\ UNCHANGED <<released, registered, init, registry, generation, reg>>
RetireUnindex == /\ retire = "detached" /\ retire' = "unlock"
                 /\ registry' = IF HoldDuringDrain THEN "drain" ELSE "free"
                 /\ UNCHANGED <<live, owned, detached, released, registered, init, generation, reg>>
RetireRelease == /\ retire = "unlock" /\ retire' = "drain" /\ generation' = "free"
                 /\ UNCHANGED <<live, owned, detached, released, registered, init, registry, reg>>
Drain(i) == /\ retire = "drain" /\ i \in detached /\ detached' = detached \ {i}
            /\ released' = [released EXCEPT ![i] = @ + 1]
            /\ UNCHANGED <<live, owned, registered, init, registry, generation, reg, retire>>
Finish == /\ retire = "drain" /\ detached = {} /\ retire' = "done"
          /\ registry' = IF registry = "drain" THEN "free" ELSE registry
          /\ UNCHANGED <<live, owned, detached, released, registered, init, generation, reg>>
InitAcquire == /\ init = "waiting" /\ registry = "free"
               /\ init' = "locked" /\ registry' = "init"
               /\ UNCHANGED <<live, owned, detached, released, registered, generation, reg, retire>>
InitRelease == /\ init = "locked" /\ init' = "done" /\ registry' = "free"
               /\ UNCHANGED <<live, owned, detached, released, registered, generation, reg, retire>>
Next == (\E i \in Ids: Register(i) \/ Drain(i)) \/ RegAcquire \/ RegIndex \/ RegUnindex \/ RegRelease
        \/ RetireAcquire \/ RetireIndex \/ Detach \/ RetireUnindex \/ RetireRelease \/ Finish
        \/ InitAcquire \/ InitRelease
Spec == Init /\ [][Next]_vars
        /\ WF_vars(InitAcquire) /\ WF_vars(InitRelease)
        /\ WF_vars(RegUnindex) /\ WF_vars(RegRelease)
        /\ WF_vars(\E i \in Ids: Register(i))
        /\ WF_vars(RetireUnindex) /\ WF_vars(RetireRelease) /\ WF_vars(Detach)
ExclusiveOwner == /\ owned \intersect detached = {}
                  /\ \A i \in registered: (i \in owned \/ i \in detached) <=> released[i] = 0
ExactlyOnce == \A i \in Ids: released[i] <= 1
DetachedAdmission == ~live => owned = {}
BoundedRegistry == registry # "drain"
LockOrder == registry \in {"register", "retire"} => generation # "free"
InitCompletes == <>(init = "done")
=============================================================================
