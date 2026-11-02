------------------------ MODULE PersistentBoot ------------------------
EXTENDS Naturals, TLC
\* SeedRoot's final Engine.Sync is the block barrier from RootPublication;
\* Publish is the following durable HeadState CAS and named-root publication.
\* Backend flush correctness is assumed here, modeled separately by
\* db/s4db/Durability.tla. This model adds selection, interrupted seed, retained
\* reopen and the boot workload/store lifetime. It does not prove Linux syscalls,
\* provider cache durability, module dependencies, general GC or concurrent seed.
\* One immutable image, one Volume identity and at most two boots are sufficient.
CONSTANTS BlocksBeforeHead, AtomicRetention
VARIABLES phase, exists, pending, durable, saved, retained, storeOpen,
          pinned, mounted, child, boots, damaged
vars == <<phase, exists, pending, durable, saved, retained, storeOpen,
          pinned, mounted, child, boots, damaged>>
Init == /\ phase = "absent" /\ exists = FALSE /\ pending = FALSE
        /\ durable = FALSE /\ saved = FALSE /\ retained = FALSE
        /\ storeOpen = FALSE /\ pinned = FALSE /\ mounted = FALSE
        /\ child = FALSE /\ boots = 0 /\ damaged = FALSE
Create == /\ phase = "absent" /\ phase' = "seeding"
          /\ exists' = TRUE /\ storeOpen' = TRUE
          /\ UNCHANGED <<pending, durable, saved, retained, pinned,
                         mounted, child, boots, damaged>>
Prepare == /\ phase = "seeding" /\ ~pending /\ pending' = TRUE
           /\ UNCHANGED <<phase, exists, durable, saved, retained, storeOpen,
                          pinned, mounted, child, boots, damaged>>
Fence == /\ phase = "seeding" /\ pending /\ durable' = TRUE
         /\ phase' = "fenced"
         /\ UNCHANGED <<exists, pending, saved, retained, storeOpen,
                        pinned, mounted, child, boots, damaged>>
Publish == /\ phase \in {"seeding", "fenced"} /\ pending
           /\ (BlocksBeforeHead => durable)
           /\ phase' = "selected" /\ saved' = TRUE
           /\ retained' = AtomicRetention
           /\ UNCHANGED <<exists, pending, durable, storeOpen, pinned,
                          mounted, child, boots, damaged>>
SeedClose == /\ phase = "selected" /\ phase' = "off" /\ storeOpen' = FALSE
             /\ UNCHANGED <<exists, pending, durable, saved, retained,
                            pinned, mounted, child, boots, damaged>>
\* Crash before publication can leave construction data but cannot select it.
Interrupt == /\ phase \in {"seeding", "fenced", "selected"}
             /\ phase' = "off" /\ storeOpen' = FALSE /\ pending' = FALSE
             /\ UNCHANGED <<exists, durable, saved, retained, pinned,
                            mounted, child, boots, damaged>>
Open == /\ phase = "off" /\ boots < 2 /\ saved /\ durable /\ retained
        /\ phase' = "ready" /\ storeOpen' = TRUE /\ pinned' = TRUE
        /\ UNCHANGED <<exists, pending, durable, saved, retained,
                       mounted, child, boots, damaged>>
Refuse == /\ phase = "off" /\ ~(saved /\ durable /\ retained)
          /\ phase' = "refused"
          /\ UNCHANGED <<exists, pending, durable, saved, retained, storeOpen,
                         pinned, mounted, child, boots, damaged>>
\* The test deliberately bypasses publication to install a missing root.
\* Inspection must preserve that disposable store and refuse to serve it.
Poison == /\ phase = "off" /\ saved /\ ~damaged /\ boots = 0
          /\ damaged' = TRUE /\ durable' = FALSE
          /\ UNCHANGED <<phase, exists, pending, saved, retained, storeOpen,
                         pinned, mounted, child, boots>>
Mount == /\ phase = "ready" /\ phase' = "serving" /\ mounted' = TRUE
         /\ UNCHANGED <<exists, pending, durable, saved, retained, storeOpen,
                        pinned, child, boots, damaged>>
Exec == /\ phase = "serving" /\ phase' = "running" /\ child' = TRUE
        /\ UNCHANGED <<exists, pending, durable, saved, retained, storeOpen,
                       pinned, mounted, boots, damaged>>
Exit == /\ phase \in {"ready", "serving", "running"}
        /\ phase' = "stopping" /\ child' = FALSE
        /\ UNCHANGED <<exists, pending, durable, saved, retained, storeOpen,
                       pinned, mounted, boots, damaged>>
Unmount == /\ phase = "stopping" /\ mounted /\ ~child /\ mounted' = FALSE
           /\ UNCHANGED <<phase, exists, pending, durable, saved, retained,
                          storeOpen, pinned, child, boots, damaged>>
Close == /\ phase = "stopping" /\ ~mounted /\ ~child
         /\ phase' = "off" /\ storeOpen' = FALSE /\ pinned' = FALSE /\ boots' = boots + 1
         /\ UNCHANGED <<exists, pending, durable, saved, retained,
                        mounted, child, damaged>>
Next == Create \/ Prepare \/ Fence \/ Publish \/ SeedClose \/ Interrupt \/
        Open \/ Refuse \/ Poison \/ Mount \/ Exec \/ Exit \/ Unmount \/ Close
Spec == Init /\ [][Next]_vars /\ WF_vars(Next)
SavedRootHasBlocks == ~damaged => (saved => durable)
SelectedRootRetained == saved => retained
ReadyMeansComplete == (pinned \/ mounted \/ child) =>
                       (saved /\ durable /\ retained /\ storeOpen)
WorkBeforeUnmount == child => mounted /\ pinned /\ storeOpen
ClosedMeansUnused == ~storeOpen => ~pinned /\ ~mounted /\ ~child
RefusedPreservesStore == phase = "refused" => exists /\ ~storeOpen
EventuallyQuiescent == <> (phase = "refused" \/ boots = 2)
=============================================================================
