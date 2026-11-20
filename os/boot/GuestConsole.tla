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
\* An image holding the daemon adds one supervised child: the same waiter reaps
\* it, a restart follows only while the console runs, and stopping sends it
\* SIGTERM before containment, which still kills it after a bounded wait.
\* A reboot request stops the console exactly as poweroff does; only the
\* final kernel call differs, so poweroff models both.
CONSTANTS IsPID1, GuestGuard, SingleWaiter, JoinBeforeClose, PoweroffHandled, PoweroffCancels,
          TermBeforeKill
VARIABLES phase, store, verified, mounts, servers, jobs, zombies, signalled,
          globalWait, commandWait, failed, poweroff, daemon, restarts, killedRunning
console == <<verified, mounts, servers, jobs, zombies, signalled, globalWait,
             commandWait, failed, poweroff>>
vars == <<phase, store, console, daemon, restarts, killedRunning>>
dvars == <<daemon, restarts, killedRunning>>
MaxRestarts == 2
Init == /\ phase = "opening" /\ store = FALSE /\ verified = FALSE
        /\ mounts = 0 /\ servers = 0 /\ jobs = 0 /\ zombies = 0
        /\ signalled = FALSE /\ globalWait = FALSE /\ commandWait = FALSE /\ failed = FALSE
        /\ poweroff = "idle" /\ daemon = "absent" /\ restarts = 0 /\ killedRunning = FALSE
Open == /\ phase = "opening" /\ IsPID1 /\ ~store /\ store' = TRUE
        /\ UNCHANGED <<phase, verified, mounts, servers, jobs, zombies,
                       signalled, globalWait, commandWait, failed, poweroff>>
        /\ UNCHANGED dvars
Verify == /\ phase = "opening" /\ store /\ ~verified /\ verified' = TRUE
          /\ UNCHANGED <<phase, store, mounts, servers, jobs, zombies,
                         signalled, globalWait, commandWait, failed, poweroff>>
          /\ UNCHANGED dvars
Mount == /\ phase = "opening" /\ verified /\ mounts < 2
         /\ mounts' = mounts + 1 /\ servers' = servers + 1
         /\ UNCHANGED <<phase, store, verified, jobs, zombies, signalled,
                        globalWait, commandWait, failed, poweroff>>
         /\ UNCHANGED dvars
Enter == /\ phase = "opening" /\ mounts = 2 /\ phase' = "console"
         /\ jobs' = 2 /\ globalWait' = TRUE /\ commandWait' = ~SingleWaiter
         /\ daemon' \in {"absent", "running"}
         /\ UNCHANGED <<store, verified, mounts, servers, zombies, signalled, failed, poweroff,
                        restarts, killedRunning>>
\* A child may exit before its SIGCHLD is consumed; only the global waiter reaps.
ChildExit == /\ phase = "console" /\ jobs > 0
             /\ jobs' = jobs - 1 /\ zombies' = zombies + 1
             /\ UNCHANGED <<phase, store, verified, mounts, servers, signalled,
                            globalWait, commandWait, failed, poweroff>>
             /\ UNCHANGED dvars
Reap == /\ globalWait /\ zombies > 0 /\ zombies' = zombies - 1
        /\ UNCHANGED <<phase, store, verified, mounts, servers, jobs, signalled,
                       globalWait, commandWait, failed, poweroff>>
        /\ UNCHANGED dvars
\* Shell exit, cancellation, failed startup, server exit or controller stop all
\* enter the same cleanup. Failed FUSE disconnects before killing blocked users.
\* Leaving the console cancels a pending restart.
DropBackoff == IF daemon = "backoff" THEN "absent" ELSE daemon
Stop == /\ phase \in {"opening", "console"} /\ poweroff # "requested" /\ phase' = "stopping"
        /\ failed' \in BOOLEAN /\ daemon' = DropBackoff
        /\ UNCHANGED <<store, verified, mounts, servers, jobs, zombies,
                       signalled, globalWait, commandWait, poweroff, restarts, killedRunning>>
Contain == /\ phase = "stopping" /\ ~signalled
           /\ (GuestGuard => IsPID1) /\ signalled' = TRUE
           /\ (TermBeforeKill => daemon # "running")
           /\ zombies' = zombies + jobs /\ jobs' = 0 /\ globalWait' = TRUE
           /\ killedRunning' = (killedRunning \/ daemon = "running")
           /\ daemon' = IF daemon \in {"running", "terminating"} THEN "exited" ELSE daemon
           /\ UNCHANGED <<phase, store, verified, mounts, servers, commandWait, failed, poweroff,
                          restarts>>
Unmount == /\ phase = "stopping" /\ jobs = 0 /\ zombies = 0 /\ daemon = "absent" /\ mounts > 0
           /\ mounts' = mounts - 1
           /\ UNCHANGED <<phase, store, verified, servers, jobs, zombies,
                          signalled, globalWait, commandWait, failed, poweroff>>
           /\ UNCHANGED dvars
Join == /\ phase = "stopping" /\ mounts = 0 /\ servers > 0
        /\ servers' = servers - 1
        /\ UNCHANGED <<phase, store, verified, mounts, jobs, zombies,
                       signalled, globalWait, commandWait, failed, poweroff>>
        /\ UNCHANGED dvars
Close == /\ phase = "stopping" /\ jobs = 0 /\ zombies = 0
         /\ (JoinBeforeClose => mounts = 0 /\ servers = 0 /\ daemon = "absent")
         /\ store' = FALSE /\ phase' = "stopped"
         /\ UNCHANGED <<verified, mounts, servers, jobs, zombies, signalled,
                        globalWait, commandWait, failed, poweroff>>
         /\ UNCHANGED dvars
RefuseHosted == /\ phase = "opening" /\ ~IsPID1 /\ phase' = "stopped"
                /\ UNCHANGED <<store, verified, mounts, servers, jobs, zombies,
                               signalled, globalWait, commandWait, failed, poweroff>>
                /\ UNCHANGED dvars
\* Bound a poweroff request without a coincident independent failure. The signal
\* is latched until the shell waiter consumes it; healthy FUSE stays live.
RequestPoweroff == /\ phase = "console" /\ poweroff = "idle"
                   /\ poweroff' = "requested"
                   /\ UNCHANGED <<phase, store, verified, mounts, servers, jobs,
                                  zombies, signalled, globalWait, commandWait, failed>>
                   /\ UNCHANGED dvars
HandlePoweroff == /\ phase = "console" /\ poweroff = "requested" /\ PoweroffHandled
                  /\ phase' = "stopping" /\ poweroff' = "handled"
                  /\ failed' = PoweroffCancels /\ daemon' = DropBackoff
                  /\ UNCHANGED <<store, verified, mounts, servers, jobs, zombies,
                                 signalled, globalWait, commandWait, restarts, killedRunning>>
\* The daemon may exit at any time it runs, including after SIGTERM.
DaemonExit == /\ phase \in {"console", "stopping"} /\ daemon \in {"running", "terminating"}
              /\ daemon' = "exited"
              /\ UNCHANGED <<phase, store, console, restarts, killedRunning>>
\* The single waiter reaps it; only the running console schedules a restart.
ReapDaemon == /\ globalWait /\ daemon = "exited"
              /\ daemon' = IF phase = "console" THEN "backoff" ELSE "absent"
              /\ UNCHANGED <<phase, store, console, restarts, killedRunning>>
Restart == /\ phase = "console" /\ poweroff = "idle" /\ daemon = "backoff"
           /\ restarts < MaxRestarts
           /\ daemon' = "running" /\ restarts' = restarts + 1
           /\ UNCHANGED <<phase, store, console, killedRunning>>
\* Stopping asks the daemon to finish its writes before containment.
Terminate == /\ phase = "stopping" /\ daemon = "running" /\ ~signalled
             /\ daemon' = "terminating"
             /\ UNCHANGED <<phase, store, console, restarts, killedRunning>>
Next == Open \/ Verify \/ Mount \/ Enter \/ ChildExit \/ Reap \/ Stop \/
        Contain \/ Unmount \/ Join \/ Close \/ RefuseHosted \/ RequestPoweroff \/ HandlePoweroff \/
        DaemonExit \/ ReapDaemon \/ Restart \/ Terminate
Spec == Init /\ [][Next]_vars /\ WF_vars(Stop) /\ WF_vars(Contain) /\
        WF_vars(Reap) /\ WF_vars(Unmount) /\ WF_vars(Join) /\ WF_vars(Close) /\
        WF_vars(RefuseHosted) /\ WF_vars(HandlePoweroff) /\
        WF_vars(ReapDaemon) /\ WF_vars(Terminate)
GuestOnlySignals == signalled => IsPID1
OneWaiter == ~(globalWait /\ commandWait)
VerifiedBeforeMount == mounts > 0 => verified
StorageOutlivesUsers == (mounts > 0 \/ servers > 0 \/ jobs > 0 \/ zombies > 0 \/
                         daemon \in {"running", "terminating", "exited"}) => store
StoppedIsJoined == phase = "stopped" => mounts = 0 /\ servers = 0 /\ jobs = 0 /\ zombies = 0
                                         /\ daemon = "absent"
DaemonAskedToStop == ~killedRunning
NoRestartAfterConsole == phase # "console" => daemon # "backoff"
PoweroffPreservesService == poweroff = "handled" => ~failed
PoweroffIsHandled == (poweroff = "requested") ~> (poweroff = "handled")
EventuallyStopped == <> (phase = "stopped")
=======================================================================
