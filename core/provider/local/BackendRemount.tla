---------------------------- MODULE BackendRemount ----------------------------
VARIABLES mount, uploadExited, remote
vars == <<mount, uploadExited, remote>>

(* A tracker reuses its remote slot across two mounts. RestartRoutine starts
   the next mount after the first returns. runUpload can clear the slot while
   handling cancellation, independently of the mount's own cleanup. This
   model assumes the replacement opens successfully. BackendAvailability
   models readers, failed opens and notifications during a mount. *)
Init == /\ mount = "mounted"
        /\ uploadExited = FALSE
        /\ remote = "old"

CancelMount == /\ mount = "mounted"
               /\ mount' = "stopping"
               /\ UNCHANGED <<uploadExited, remote>>

ExitUpload == /\ mount # "mounted"
              /\ ~uploadExited
              /\ uploadExited' = TRUE
              /\ remote' = "none"
              /\ UNCHANGED mount

Unmount == /\ mount = "stopping"
           /\ uploadExited
           /\ mount' = "unmounted"
           /\ remote' = "none"
           /\ UNCHANGED uploadExited

RestartMount == /\ mount = "unmounted"
                /\ mount' = "opening"
                /\ UNCHANGED <<uploadExited, remote>>

OpenBackend == /\ mount = "opening"
               /\ mount' = "ready"
               /\ remote' = "new"
               /\ UNCHANGED uploadExited

Done == /\ mount = "ready" /\ uploadExited
        /\ UNCHANGED vars
Next == CancelMount \/ ExitUpload \/ Unmount \/ RestartMount \/ OpenBackend \/ Done
Spec == Init /\ [][Next]_vars
        /\ WF_vars(CancelMount) /\ WF_vars(ExitUpload) /\ WF_vars(Unmount)
        /\ WF_vars(RestartMount) /\ WF_vars(OpenBackend)

ReplacementKeepsBackend == mount = "ready" => remote = "new"
RestartOpensBackend == <>(mount = "ready")
=============================================================================
