-------------------------- MODULE DataVolume --------------------------
EXTENDS Naturals, FiniteSets, TLC
\* Boot Data composition, before implementation: CreateData exclusively creates
\* a fixture; OpenData inspects format, identity, head, retention and root using
\* a read-only s4db handle before starting normal Volume/World controllers.
\* FUSE Fsync/Flush -> FSHandle.Sync -> FSWriter -> EngineTx.Commit waits for
\* the tracked bucket publication receipt. s4db atomically commits prepared
\* blocks, generated HeadState and retention. Large preparation spills retain
\* ownership but cannot acknowledge a head; abstract them as Prepare below.
\* Controller cancellation/failure is independent of FS users and physical
\* storage: borrowedNativeVolume.Close is inert; Data.Close alone closes it
\* after its caller stops users, releases handles and joins both controllers.
\* Bounds: two immutable file records, two opens; one writer and one FS user.
\* "partial" includes interruption before FsInit publishes a UnixFS root; later
\* OpenData refuses it. modified records writes within the current open attempt.
\* A failed controller cancels Data's buffered WriteAt context. Idle flush and
\* FSHandle.Release also use that writer, even with their background contexts;
\* they must fail rather than wait for an unavailable writer/revision watcher.
\* Substrate queue/CAS interleavings are covered in db/world/block and
\* db/volume/common/kvtx models. Mounted Linux/QEMU containment is not modeled.
CONSTANTS DurableBeforeAck, BootOwnsClose, AtomicRetention, CancelStoppedWriter
VARIABLES phase, kind, storeOpen, volumeRun, engineRun, users, syncPhase,
          record, prepared, durable, retained, acknowledged, boots, modified
vars == <<phase, kind, storeOpen, volumeRun, engineRun, users, syncPhase,
          record, prepared, durable, retained, acknowledged, boots, modified>>
Init == /\ phase = "off" /\ kind \in {"absent", "unknown", "missing-root", "partial", "valid"}
        /\ storeOpen = FALSE /\ volumeRun = FALSE /\ engineRun = FALSE
        /\ users = FALSE /\ syncPhase = "none" /\ record = 1
        /\ prepared = {} /\ durable = {} /\ retained = {}
        /\ acknowledged = {} /\ boots = 0 /\ modified = FALSE
CreateData == /\ phase = "off" /\ kind = "absent" /\ boots = 0
              /\ kind' = "partial" /\ modified' = TRUE /\ phase' = "opening"
              /\ UNCHANGED <<storeOpen, volumeRun, engineRun, users, syncPhase,
                             record, prepared, durable, retained, acknowledged, boots>>
OpenData == /\ phase = "off" /\ kind = "valid" /\ boots < 2
            /\ phase' = "opening"
            /\ UNCHANGED <<kind, storeOpen, volumeRun, engineRun, users, syncPhase,
                           record, prepared, durable, retained, acknowledged, boots, modified>>
Refuse == /\ phase = "off" /\ kind \in {"absent", "unknown", "missing-root", "partial"}
          /\ phase' = "refused"
          /\ UNCHANGED <<kind, storeOpen, volumeRun, engineRun, users, syncPhase,
                         record, prepared, durable, retained, acknowledged, boots, modified>>
StartVolume == /\ phase = "opening" /\ ~storeOpen
               /\ storeOpen' = TRUE /\ volumeRun' = TRUE
               /\ UNCHANGED <<phase, kind, engineRun, users, syncPhase, record,
                              prepared, durable, retained, acknowledged, boots, modified>>
StartEngine == /\ phase = "opening" /\ volumeRun /\ ~engineRun
               /\ engineRun' = TRUE /\ phase' = "ready" /\ boots' = boots + 1
               /\ kind' = "valid"
               /\ UNCHANGED <<storeOpen, volumeRun, users, syncPhase, record,
                              prepared, durable, retained, acknowledged, modified>>
AcquireUser == /\ phase = "ready" /\ phase' = "serving" /\ users' = TRUE
               /\ UNCHANGED <<kind, storeOpen, volumeRun, engineRun, syncPhase, record,
                              prepared, durable, retained, acknowledged, boots, modified>>
WriteAt == /\ phase = "serving" /\ volumeRun /\ engineRun /\ record <= 2
           /\ syncPhase = "none" /\ syncPhase' = "buffered"
           /\ UNCHANGED <<phase, kind, storeOpen, volumeRun, engineRun, users, record,
                          prepared, durable, retained, acknowledged, boots, modified>>
Prepare == /\ syncPhase = "buffered" /\ volumeRun /\ engineRun
           /\ syncPhase' = "preparing" /\ prepared' = prepared \cup {record}
           /\ UNCHANGED <<phase, kind, storeOpen, volumeRun, engineRun, users, record,
                          durable, retained, acknowledged, boots, modified>>
Admit == /\ syncPhase = "preparing" /\ volumeRun /\ engineRun
         /\ syncPhase' = "admitted"
         /\ UNCHANGED <<phase, kind, storeOpen, volumeRun, engineRun, users, record,
                        prepared, durable, retained, acknowledged, boots, modified>>
PublishAtomic == /\ syncPhase = "admitted" /\ storeOpen
                 /\ durable' = durable \cup prepared
                 /\ retained' = IF AtomicRetention THEN retained \cup prepared ELSE retained
                 /\ syncPhase' = "durable"
                 /\ UNCHANGED <<phase, kind, storeOpen, volumeRun, engineRun, users, record,
                                prepared, acknowledged, boots, modified>>
SyncReturn == /\ syncPhase \in {"admitted", "durable"}
              /\ (DurableBeforeAck => syncPhase = "durable")
              /\ acknowledged' = acknowledged \cup {record}
              /\ syncPhase' = "none" /\ record' = record + 1
              /\ UNCHANGED <<phase, kind, storeOpen, volumeRun, engineRun, users,
                             prepared, durable, retained, boots, modified>>
SyncFailure == /\ syncPhase \in {"buffered", "preparing", "admitted", "durable"}
               /\ ((volumeRun /\ engineRun) \/ CancelStoppedWriter)
               /\ syncPhase' = "none" /\ record' = record + 1
               /\ UNCHANGED <<phase, kind, storeOpen, volumeRun, engineRun, users,
                              prepared, durable, retained, acknowledged, boots, modified>>
\* Both controller failures/cancellation can occur while users still hold files.
ControllerStop == /\ phase \in {"opening", "ready", "serving", "stopping"}
                  /\ volumeRun \/ engineRun
                  /\ (\/ /\ volumeRun /\ volumeRun' = FALSE /\ UNCHANGED engineRun
                      \/ /\ engineRun /\ engineRun' = FALSE /\ UNCHANGED volumeRun)
                  /\ phase' = "stopping"
                  /\ storeOpen' = IF BootOwnsClose THEN storeOpen ELSE FALSE
                  /\ UNCHANGED <<kind, users, syncPhase, record, prepared, durable,
                                 retained, acknowledged, boots, modified>>
StopUsers == /\ phase \in {"ready", "serving", "stopping"}
             /\ syncPhase = "none" /\ phase' = "joined" /\ users' = FALSE
             /\ UNCHANGED <<kind, storeOpen, volumeRun, engineRun, syncPhase, record,
                            prepared, durable, retained, acknowledged, boots, modified>>
CloseControllers == /\ phase = "joined" /\ phase' = "closing"
                    /\ volumeRun' = FALSE /\ engineRun' = FALSE
                    /\ UNCHANGED <<kind, storeOpen, users, syncPhase, record, prepared,
                                   durable, retained, acknowledged, boots, modified>>
CloseStorage == /\ phase = "closing" /\ phase' = "off" /\ storeOpen' = FALSE
                /\ prepared' = {} /\ modified' = FALSE
                /\ UNCHANGED <<kind, volumeRun, engineRun, users, syncPhase, record,
                               durable, retained, acknowledged, boots>>
Crash == /\ phase \in {"opening", "ready", "serving", "stopping"}
         /\ phase' = "off" /\ storeOpen' = FALSE /\ users' = FALSE
         /\ volumeRun' = FALSE /\ engineRun' = FALSE /\ prepared' = {}
         /\ syncPhase' = "none" /\ modified' = FALSE
         /\ UNCHANGED <<kind, record, durable, retained, acknowledged, boots>>
Done == /\ (phase = "refused" \/ (phase = "off" /\ boots = 2)) /\ UNCHANGED vars
Next == CreateData \/ OpenData \/ Refuse \/ StartVolume \/ StartEngine \/ AcquireUser \/
        WriteAt \/ Prepare \/ Admit \/ PublishAtomic \/ SyncReturn \/ SyncFailure \/
        ControllerStop \/ StopUsers \/ CloseControllers \/ CloseStorage \/ Crash \/ Done
Spec == Init /\ [][Next]_vars /\ WF_vars(SyncFailure) /\ WF_vars(StopUsers) /\
        WF_vars(CloseControllers) /\ WF_vars(CloseStorage)
AcknowledgedSurvives == acknowledged \subseteq durable /\ acknowledged \subseteq retained
StorageOutlivesUsers == users => storeOpen
CloseAfterJoin == ~storeOpen => ~volumeRun /\ ~engineRun /\ ~users
RefusalPreserves == phase = "refused" => ~modified /\ ~storeOpen
ContainmentCompletes == [] (phase = "stopping" ~> phase = "off")
=============================================================================
