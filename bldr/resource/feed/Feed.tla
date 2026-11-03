---- MODULE Feed ----
\* Hub, joinFeed, leave and expire share one source per descriptor/key.
\* A publication replaces the head; slow readers skip intermediate versions.
\* The linger generation prevents an expired timer from stopping a rejoin.
EXTENDS Naturals, FiniteSets, TLC
Readers == {1,2}
VARIABLES joined, running, version, delivered, generation, pending, expired
vars == <<joined,running,version,delivered,generation,pending,expired>>
Init == /\ joined = {} /\ running = FALSE /\ version = 0
        /\ delivered = [r \in Readers |-> 0]
        /\ generation = 0 /\ pending = 0 /\ expired = 0
Join(r) == /\ r \notin joined /\ generation < 3
           /\ joined' = joined \cup {r} /\ running' = TRUE
           /\ pending' = 0
           /\ UNCHANGED <<version,delivered,generation,expired>>
Leave(r) == /\ r \in joined /\ generation < 3
            /\ joined' = joined \ {r}
            /\ generation' = IF joined' = {} THEN generation+1 ELSE generation
            /\ pending' = IF joined' = {} THEN generation' ELSE pending
            /\ UNCHANGED <<running,version,delivered,expired>>
Publish == /\ running /\ version < 3 /\ version' = version+1
           /\ UNCHANGED <<joined,running,delivered,generation,pending,expired>>
Deliver(r) == /\ r \in joined /\ delivered[r] < version
              /\ delivered' = [delivered EXCEPT ![r] = version]
              /\ UNCHANGED <<joined,running,version,generation,pending,expired>>
CaptureTimer == /\ pending # 0 /\ expired' = pending
                /\ UNCHANGED <<joined,running,version,delivered,generation,pending>>
Expire == /\ expired # 0
          /\ running' = IF pending = expired THEN FALSE ELSE running
          /\ pending' = IF pending = expired THEN 0 ELSE pending
          /\ expired' = 0
          /\ UNCHANGED <<joined,version,delivered,generation>>
Next == (\E r \in Readers: Join(r) \/ Leave(r) \/ Deliver(r)) \/ Publish \/ CaptureTimer \/ Expire
Spec == Init /\ [][Next]_vars
JoinedHasSource == joined # {} => running
KnownPublications == \A r \in Readers: delivered[r] <= version
=============================================================================
