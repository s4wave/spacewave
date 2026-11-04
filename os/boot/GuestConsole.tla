------------------------- MODULE GuestConsole -------------------------
EXTENDS Naturals, TLC
\* RunConsole composition: syscall mounts and two FUSE servers, a bootstrap
\* exec outside FUSE, then one SIGCHLD-driven Wait4 authority. Helpers finish
\* before that authority starts; os.Process.Release never waits for children.
\* Two abstract jobs include shell descendants that fork/setsid and become
\* adopted orphans. SIGKILL containment is legal only in actual guest PID1.
\* Kernel delivery/reap is fair; unkillable kernel tasks and syscall hangs are
\* outside the bound. DataVolume covers writes/publication, Console the reader.
\* servers counts retained server joins, including a failed Serve that has
\* already disconnected. Close includes explicit cursor invalidation (FUSE may
\* omit Forget), watcher/controller joins, then the sole physical store close.
CONSTANTS IsPID1, GuestGuard, SingleWaiter, JoinBeforeClose
VARIABLES phase, store, verified, mounts, servers, jobs, zombies, signalled,
          globalWait, commandWait, failed
vars == <<phase, store, verified, mounts, servers, jobs, zombies, signalled,
          globalWait, commandWait, failed>>
Init == /\ phase = "opening" /\ store = FALSE /\ verified = FALSE
        /\ mounts = 0 /\ servers = 0 /\ jobs = 0 /\ zombies = 0
        /\ signalled = FALSE /\ globalWait = FALSE /\ commandWait = FALSE /\ failed = FALSE
Open == /\ phase = "opening" /\ IsPID1 /\ ~store /\ store' = TRUE
        /\ UNCHANGED <<phase, verified, mounts, servers, jobs, zombies,
                       signalled, globalWait, commandWait, failed>>
Verify == /\ phase = "opening" /\ store /\ ~verified /\ verified' = TRUE
          /\ UNCHANGED <<phase, store, mounts, servers, jobs, zombies,
                         signalled, globalWait, commandWait, failed>>
Mount == /\ phase = "opening" /\ verified /\ mounts < 2
         /\ mounts' = mounts + 1 /\ servers' = servers + 1
         /\ UNCHANGED <<phase, store, verified, jobs, zombies, signalled,
                        globalWait, commandWait, failed>>
Enter == /\ phase = "opening" /\ mounts = 2 /\ phase' = "console"
         /\ jobs' = 2 /\ globalWait' = TRUE /\ commandWait' = ~SingleWaiter
         /\ UNCHANGED <<store, verified, mounts, servers, zombies, signalled, failed>>
\* A child may exit before its SIGCHLD is consumed; only the global waiter reaps.
ChildExit == /\ phase = "console" /\ jobs > 0
             /\ jobs' = jobs - 1 /\ zombies' = zombies + 1
             /\ UNCHANGED <<phase, store, verified, mounts, servers, signalled,
                            globalWait, commandWait, failed>>
Reap == /\ globalWait /\ zombies > 0 /\ zombies' = zombies - 1
        /\ UNCHANGED <<phase, store, verified, mounts, servers, jobs, signalled,
                       globalWait, commandWait, failed>>
\* Shell exit, cancellation, failed startup, server exit or controller stop all
\* enter the same cleanup. Failed FUSE disconnects before killing blocked users.
Stop == /\ phase \in {"opening", "console"} /\ phase' = "stopping"
        /\ failed' \in BOOLEAN
        /\ UNCHANGED <<store, verified, mounts, servers, jobs, zombies,
                       signalled, globalWait, commandWait>>
Contain == /\ phase = "stopping" /\ ~signalled
           /\ (GuestGuard => IsPID1) /\ signalled' = TRUE
           /\ zombies' = zombies + jobs /\ jobs' = 0 /\ globalWait' = TRUE
           /\ UNCHANGED <<phase, store, verified, mounts, servers, commandWait, failed>>
Unmount == /\ phase = "stopping" /\ jobs = 0 /\ zombies = 0 /\ mounts > 0
           /\ mounts' = mounts - 1
           /\ UNCHANGED <<phase, store, verified, servers, jobs, zombies,
                          signalled, globalWait, commandWait, failed>>
Join == /\ phase = "stopping" /\ mounts = 0 /\ servers > 0
        /\ servers' = servers - 1
        /\ UNCHANGED <<phase, store, verified, mounts, jobs, zombies,
                       signalled, globalWait, commandWait, failed>>
Close == /\ phase = "stopping" /\ jobs = 0 /\ zombies = 0
         /\ (JoinBeforeClose => mounts = 0 /\ servers = 0)
         /\ store' = FALSE /\ phase' = "stopped"
         /\ UNCHANGED <<verified, mounts, servers, jobs, zombies, signalled,
                        globalWait, commandWait, failed>>
RefuseHosted == /\ phase = "opening" /\ ~IsPID1 /\ phase' = "stopped"
                /\ UNCHANGED <<store, verified, mounts, servers, jobs, zombies,
                               signalled, globalWait, commandWait, failed>>
Next == Open \/ Verify \/ Mount \/ Enter \/ ChildExit \/ Reap \/ Stop \/
        Contain \/ Unmount \/ Join \/ Close \/ RefuseHosted
Spec == Init /\ [][Next]_vars /\ WF_vars(Stop) /\ WF_vars(Contain) /\
        WF_vars(Reap) /\ WF_vars(Unmount) /\ WF_vars(Join) /\ WF_vars(Close) /\
        WF_vars(RefuseHosted)
GuestOnlySignals == signalled => IsPID1
OneWaiter == ~(globalWait /\ commandWait)
VerifiedBeforeMount == mounts > 0 => verified
StorageOutlivesUsers == (mounts > 0 \/ servers > 0 \/ jobs > 0 \/ zombies > 0) => store
StoppedIsJoined == phase = "stopped" => mounts = 0 /\ servers = 0 /\ jobs = 0 /\ zombies = 0
EventuallyStopped == <> (phase = "stopped")
=======================================================================
