------------------------ MODULE PersistentBoot ------------------------
EXTENDS Naturals, TLC
\* Seed Sync and tracked atomic HeadState publication retain the image durably.
\* Reopen uses one read-only s4db handle, a bare store and an immutable World Tx:
\* no controller, writer, GC or active reader-pin writer exists during serving.
\* Cancellation contains the child; server failure first disconnects FUSE, then
\* contains the child. Root alone closes storage after join and snapshot release.
\* Bounds: one image/identity, two boots. Linux syscalls, provider caches and the
\* seed publisher's backend ordering remain separate implementation assumptions.
CONSTANTS BlocksBeforeHead, AtomicRetention, ReadOnlyServe, IndependentCloser
VARIABLES phase, exists, pending, durable, saved, retained, storeOpen,
          pinned, mounted, child, server, boots, damaged, missingDesc,
          cancelled, refused, modified
vars == <<phase, exists, pending, durable, saved, retained, storeOpen,
          pinned, mounted, child, server, boots, damaged, missingDesc,
          cancelled, refused, modified>>
Init == /\ phase = "absent" /\ exists = FALSE /\ pending = FALSE
        /\ durable = FALSE /\ saved = FALSE /\ retained = FALSE
        /\ storeOpen = FALSE /\ pinned = FALSE /\ mounted = FALSE
        /\ child = FALSE /\ server = FALSE /\ boots = 0
        /\ damaged = FALSE /\ missingDesc = FALSE /\ cancelled = FALSE
        /\ refused = FALSE /\ modified = FALSE
Create == /\ phase = "absent" /\ phase' = "seeding"
          /\ exists' = TRUE /\ storeOpen' = TRUE
          /\ UNCHANGED <<pending, durable, saved, retained, pinned, mounted,
                         child, server, boots, damaged, missingDesc, cancelled,
                         refused, modified>>
Prepare == /\ phase = "seeding" /\ ~pending /\ pending' = TRUE
           /\ UNCHANGED <<phase, exists, durable, saved, retained, storeOpen,
                          pinned, mounted, child, server, boots, damaged,
                          missingDesc, cancelled, refused, modified>>
Fence == /\ phase = "seeding" /\ pending /\ durable' = TRUE
         /\ phase' = "fenced"
         /\ UNCHANGED <<exists, pending, saved, retained, storeOpen, pinned,
                        mounted, child, server, boots, damaged, missingDesc,
                        cancelled, refused, modified>>
Publish == /\ phase \in {"seeding", "fenced"} /\ pending
           /\ (BlocksBeforeHead => durable)
           /\ phase' = "selected" /\ saved' = TRUE
           /\ retained' = AtomicRetention
           /\ UNCHANGED <<exists, pending, durable, storeOpen, pinned, mounted,
                          child, server, boots, damaged, missingDesc, cancelled,
                          refused, modified>>
SeedClose == /\ phase = "selected" /\ phase' = "off" /\ storeOpen' = FALSE
             /\ UNCHANGED <<exists, pending, durable, saved, retained, pinned,
                            mounted, child, server, boots, damaged, missingDesc,
                            cancelled, refused, modified>>
Interrupt == /\ phase \in {"seeding", "fenced", "selected"}
             /\ phase' = "off" /\ storeOpen' = FALSE /\ pending' = FALSE
             /\ UNCHANGED <<exists, durable, saved, retained, pinned, mounted,
                            child, server, boots, damaged, missingDesc, cancelled,
                            refused, modified>>
\* The run-through corrupts disposable storage both at the root and below it.
PoisonRoot == /\ phase = "off" /\ saved /\ ~damaged /\ boots = 0
              /\ damaged' = TRUE /\ durable' = FALSE
              /\ UNCHANGED <<phase, exists, pending, saved, retained, storeOpen,
                             pinned, mounted, child, server, boots, missingDesc,
                             cancelled, refused, modified>>
PoisonDesc == /\ phase = "off" /\ saved /\ ~damaged /\ boots = 0
              /\ damaged' = TRUE /\ missingDesc' = TRUE
              /\ UNCHANGED <<phase, exists, pending, durable, saved, retained,
                             storeOpen, pinned, mounted, child, server, boots,
                             cancelled, refused, modified>>
Open == /\ phase = "off" /\ boots < 2 /\ saved /\ durable /\ retained
        /\ phase' = "inspecting" /\ storeOpen' = TRUE /\ pinned' = TRUE
        /\ cancelled' = FALSE /\ modified' = ~ReadOnlyServe
        /\ UNCHANGED <<exists, pending, durable, saved, retained, mounted, child,
                       server, boots, damaged, missingDesc, refused>>
Refuse == /\ phase = "off" /\ ~(saved /\ durable /\ retained)
          /\ phase' = "refused" /\ refused' = TRUE
          /\ UNCHANGED <<exists, pending, durable, saved, retained, storeOpen,
                         pinned, mounted, child, server, boots, damaged,
                         missingDesc, cancelled, modified>>
Validate == /\ phase = "inspecting" /\ ~missingDesc /\ phase' = "ready"
            /\ UNCHANGED <<exists, pending, durable, saved, retained, storeOpen,
                           pinned, mounted, child, server, boots, damaged,
                           missingDesc, cancelled, refused, modified>>
RefuseContents == /\ phase = "inspecting" /\ missingDesc
                  /\ phase' = "stopping" /\ refused' = TRUE
                  /\ UNCHANGED <<exists, pending, durable, saved, retained,
                                 storeOpen, pinned, mounted, child, server,
                                 boots, damaged, missingDesc, cancelled, modified>>
Mount == /\ phase = "ready" /\ phase' = "serving"
         /\ mounted' = TRUE /\ server' = TRUE
         /\ UNCHANGED <<exists, pending, durable, saved, retained, storeOpen,
                        pinned, child, boots, damaged, missingDesc, cancelled,
                        refused, modified>>
Exec == /\ phase = "serving" /\ phase' = "running" /\ child' = TRUE
        /\ UNCHANGED <<exists, pending, durable, saved, retained, storeOpen,
                       pinned, mounted, server, boots, damaged, missingDesc,
                       cancelled, refused, modified>>
Cancel == /\ phase \in {"ready", "serving", "running"}
          /\ phase' = "containing" /\ cancelled' = TRUE
          /\ UNCHANGED <<exists, pending, durable, saved, retained, storeOpen,
                         pinned, mounted, child, server, boots, damaged,
                         missingDesc, refused, modified>>
ServerFailure == /\ phase \in {"serving", "running"}
                 /\ phase' = "containing" /\ server' = FALSE
                 /\ cancelled' = TRUE
                 /\ UNCHANGED <<exists, pending, durable, saved, retained,
                                storeOpen, pinned, mounted, child, boots,
                                damaged, missingDesc, refused, modified>>
\* Mutant of the removed controller-owned Close on caller cancellation/failure.
IndependentClose == /\ IndependentCloser /\ cancelled /\ storeOpen
                    /\ storeOpen' = FALSE
                    /\ UNCHANGED <<phase, exists, pending, durable, saved,
                                   retained, pinned, mounted, child, server,
                                   boots, damaged, missingDesc, cancelled,
                                   refused, modified>>
Exit == /\ phase \in {"ready", "serving", "running", "containing"}
        /\ phase' = "stopping" /\ child' = FALSE
        /\ UNCHANGED <<exists, pending, durable, saved, retained, storeOpen,
                       pinned, mounted, server, boots, damaged, missingDesc,
                       cancelled, refused, modified>>
Unmount == /\ phase = "stopping" /\ mounted /\ ~child /\ mounted' = FALSE
           /\ UNCHANGED <<phase, exists, pending, durable, saved, retained,
                          storeOpen, pinned, child, server, boots, damaged,
                          missingDesc, cancelled, refused, modified>>
Join == /\ phase = "stopping" /\ ~mounted /\ ~child
        /\ phase' = "joined" /\ server' = FALSE
        /\ UNCHANGED <<exists, pending, durable, saved, retained,
                       storeOpen, pinned, mounted, child, boots, damaged,
                       missingDesc, cancelled, refused, modified>>
Release == /\ phase = "joined" /\ ~mounted /\ ~child /\ ~server /\ pinned
           /\ pinned' = FALSE
           /\ UNCHANGED <<phase, exists, pending, durable, saved, retained,
                          storeOpen, mounted, child, server, boots, damaged,
                          missingDesc, cancelled, refused, modified>>
Close == /\ phase = "joined" /\ ~mounted /\ ~child /\ ~server /\ ~pinned
         /\ phase' = IF refused THEN "refused" ELSE "off"
         /\ storeOpen' = FALSE /\ boots' = boots + 1
         /\ UNCHANGED <<exists, pending, durable, saved, retained, pinned,
                        mounted, child, server, damaged, missingDesc, cancelled,
                        refused, modified>>
Next == Create \/ Prepare \/ Fence \/ Publish \/ SeedClose \/ Interrupt \/
        Open \/ Refuse \/ PoisonRoot \/ PoisonDesc \/ Validate \/ RefuseContents \/
        Mount \/ Exec \/ Cancel \/ ServerFailure \/ IndependentClose \/ Exit \/
        Unmount \/ Join \/ Release \/ Close
Spec == Init /\ [][Next]_vars /\ WF_vars(Next)
SavedRootHasBlocks == ~damaged => (saved => durable)
SelectedRootRetained == saved => retained
ReadyMeansComplete == (phase \in {"ready", "serving", "running"} \/ mounted \/ child) =>
                       (saved /\ durable /\ ~missingDesc /\ retained /\ storeOpen)
WorkBeforeUnmount == child => mounted /\ pinned /\ storeOpen
ClosedMeansUnused == ~storeOpen => ~pinned /\ ~mounted /\ ~child /\ ~server
RefusedPreservesStore == phase = "refused" => exists /\ ~storeOpen /\ ~modified
ServingPreservesStore == ~modified
EventuallyQuiescent == <> (phase = "refused" \/ boots = 2)
=============================================================================
