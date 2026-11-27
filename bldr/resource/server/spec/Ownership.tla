---- MODULE Ownership ----
\* Two generations, one offered child per generation, and two local holders.
\* Controls retain the existing generation-wide callback completion barrier.
EXTENDS Naturals, FiniteSets, TLC
CONSTANT ParentFreesOffer, ReuseOldReference
Generations == 1..2
VARIABLES generation, live, parent, offer, allocated, holders, released, controls, ack, calls
vars == <<generation, live, parent, offer, allocated, holders, released, controls, ack, calls>>
Init == /\ generation = 1 /\ live = {1} /\ parent = {1}
        /\ offer = [g \in Generations |-> "none"] /\ allocated = {}
        /\ holders = [g \in Generations |-> 0] /\ released = {}
        /\ controls = 0 /\ ack = 0 /\ calls = {}
OpenOffer == /\ generation \in live /\ generation \in parent
             /\ offer[generation] = "none"
             /\ offer' = [offer EXCEPT ![generation] = "unpublished"]
             /\ allocated' = allocated \cup {generation}
             /\ UNCHANGED <<generation, live, parent, holders, released, controls, ack, calls>>
Publish == /\ offer[generation] = "unpublished" /\ generation \in live
           /\ offer' = [offer EXCEPT ![generation] = "published"]
           /\ UNCHANGED <<generation, live, parent, allocated, holders, released, controls, ack, calls>>
Receive == /\ offer[generation] = "published" /\ generation \in live /\ holders[generation] = 0
           /\ generation \notin released
           /\ offer' = [offer EXCEPT ![generation] = "received"]
           /\ holders' = [holders EXCEPT ![generation] = 2]
           /\ controls' = controls + 1
           /\ UNCHANGED <<generation, live, parent, allocated, released, ack, calls>>
Adopt == /\ offer[generation] = "received" /\ holders[generation] > 0 /\ generation \in live
         /\ offer' = [offer EXCEPT ![generation] = "adopted"]
         /\ ack' = 1
         /\ UNCHANGED <<generation, live, parent, allocated, holders, released, controls, calls>>
ReleaseParent == /\ generation \in parent /\ parent' = parent \ {generation}
                 /\ released' = IF ParentFreesOffer /\ offer[generation] \in {"published", "received"}
                                THEN released \cup {generation} ELSE released
                 /\ UNCHANGED <<generation, live, offer, allocated, holders, controls, ack, calls>>
ReleaseHolder == /\ generation \in live /\ holders[generation] > 0
                 /\ holders' = [holders EXCEPT ![generation] = @ - 1]
                 /\ controls' = controls + 1
                 /\ UNCHANGED <<generation, live, parent, offer, allocated, released, ack, calls>>
FinishRelease == /\ generation \in allocated /\ holders[generation] = 0
                 /\ offer[generation] = "adopted" /\ generation \notin released
                 /\ released' = released \cup {generation} /\ ack' = controls
                 /\ UNCHANGED <<generation, live, parent, offer, allocated, holders, controls, calls>>
Abandon == /\ generation \in allocated /\ holders[generation] = 0
           /\ offer[generation] \in {"unpublished", "published"}
           /\ released' = released \cup {generation}
           /\ UNCHANGED <<generation, live, parent, offer, allocated, holders, controls, ack, calls>>
Drop == /\ generation \in live /\ live' = live \ {generation}
        /\ released' = released \cup (allocated \cap {generation})
        /\ holders' = [holders EXCEPT ![generation] = 0]
        /\ UNCHANGED <<generation, parent, offer, allocated, controls, ack, calls>>
Reconnect == /\ generation = 1 /\ 1 \notin live
             /\ generation' = 2 /\ live' = {2} /\ parent' = parent \cup {2}
             /\ controls' = 0 /\ ack' = 0
             /\ UNCHANGED <<offer, allocated, holders, released, calls>>
Call(g) == /\ g \in Generations /\ holders[g] > 0 /\ ack = controls
           /\ ((g = generation /\ g \in live) \/ (ReuseOldReference /\ g < generation))
           /\ calls' = calls \cup {<<g, generation>>}
           /\ UNCHANGED <<generation, live, parent, offer, allocated, holders, released, controls, ack>>
\* A cached old client is callable after its local holders retired only in the mutant.
OldCall == /\ ReuseOldReference /\ generation = 2 /\ 1 \in allocated
           /\ calls' = calls \cup {<<1, 2>>}
           /\ UNCHANGED <<generation, live, parent, offer, allocated, holders, released, controls, ack>>
Next == OpenOffer \/ Publish \/ Receive \/ Adopt \/ ReleaseParent \/ ReleaseHolder
        \/ FinishRelease \/ Abandon \/ Drop \/ Reconnect \/ (\E g \in Generations: Call(g)) \/ OldCall
Spec == Init /\ [][Next]_vars
NoEarlyFree == \A g \in live: holders[g] > 0 => g \notin released
NoGenerationEscape == \A pair \in calls: pair[1] = pair[2]
NoDisconnectedLeak == \A g \in allocated \ live: g \in released
=============================================================================
