--------------------------- MODULE Preparation ---------------------------
EXTENDS Naturals, FiniteSets
CONSTANT Design
\* Two callers in one process: lobby start and wanted-release prefetch.
\* Join, interface install and live changes attach through the same Store.
\* Teardown is separate from cancellation: a new attach waits for reaping.
VARIABLES phase, refs, caller, leader, published, lost, overlap
vars == <<phase, refs, caller, leader, published, lost, overlap>>
Clients == {"host", "prefetch"}
Working == {"checking", "fetching", "assembling", "verifying", "publishing"}
Init == /\ phase = "idle" /\ refs = {}
        /\ caller = [c \in Clients |-> "absent"]
        /\ leader = "none"
        /\ published = FALSE /\ lost = FALSE /\ overlap = FALSE

Attach(c) == /\ caller[c] = "absent"
             /\ caller' = [caller EXCEPT ![c] = "waiting"]
             /\ refs' = refs \cup {c}
             /\ UNCHANGED <<phase, leader, published, lost, overlap>>
Start(c) == /\ (phase = "idle" \/ (Design = "overlap" /\ phase = "stopping")) /\ c \in refs
            /\ phase' = IF published THEN "ready" ELSE "checking"
            /\ leader' = c
            /\ overlap' = (overlap \/ phase = "stopping")
            /\ UNCHANGED <<refs, caller, published, lost>>
Advance == /\ phase \in Working
           /\ phase' = IF Design = "premature-ready" /\ phase = "assembling"
                        THEN "ready"
                        ELSE CASE phase = "checking" -> "fetching"
                       [] phase = "fetching" -> "assembling"
                       [] phase = "assembling" -> "verifying"
                       [] phase = "verifying" -> "publishing"
                       [] OTHER -> "ready"
           /\ published' = (published \/ phase = "publishing")
           /\ UNCHANGED <<refs, caller, leader, lost, overlap>>
Cancel(c) ==
  /\ c \in refs
  /\ refs' = refs \ {c}
  /\ caller' = [caller EXCEPT ![c] = "cancelled"]
  /\ LET stop == phase \in Working /\
          (refs' = {} \/ (Design = "caller-owned" /\ c = leader))
     IN /\ phase' = IF stop THEN "stopping" ELSE phase
        /\ lost' = (lost \/ (stop /\ refs' # {}))
  /\ UNCHANGED <<leader, published, overlap>>
Fail == /\ phase \in Working /\ phase' = "failed"
        /\ UNCHANGED <<refs, caller, leader, published, lost, overlap>>
Observe(c) == /\ c \in refs /\ phase \in {"ready", "failed"}
              /\ refs' = refs \ {c}
              /\ caller' = [caller EXCEPT ![c] = phase]
              /\ UNCHANGED <<phase, leader, published, lost, overlap>>
Retire == /\ refs = {} /\ phase \in {"ready", "failed"}
          /\ phase' = "stopping"
          /\ UNCHANGED <<refs, caller, leader, published, lost, overlap>>
Reap == /\ phase = "stopping" /\ phase' = "idle"
        /\ UNCHANGED <<refs, caller, leader, published, lost, overlap>>
Retry(c) == /\ caller[c] \in {"failed", "cancelled"}
            /\ caller' = [caller EXCEPT ![c] = "absent"]
            /\ UNCHANGED <<phase, refs, leader, published, lost, overlap>>
Crash == /\ phase # "idle" /\ phase' = "idle" /\ refs' = {}
          /\ caller' = [c \in Clients |-> IF caller[c] = "waiting" THEN "failed" ELSE caller[c]]
          /\ UNCHANGED <<leader, published, lost, overlap>>
Next == \/ \E c \in Clients : Attach(c) \/ Start(c) \/ Cancel(c) \/ Observe(c) \/ Retry(c)
        \/ Advance \/ Fail \/ Retire \/ Reap \/ Crash \/ UNCHANGED vars
IndependentCancellation == ~lost
OnlyPublishedSuccess == \A c \in Clients : caller[c] = "ready" => published
ReapBeforeRestart == ~overlap
\* Under completion and scheduling fairness, a continuously subscribed host
\* reaches a terminal result. Network failure is a terminal result, not a hang.
HostSettles == (caller["host"] = "waiting") ~> (caller["host"] # "waiting")
Spec == Init /\ [][Next]_vars /\ WF_vars(Advance) /\ WF_vars(Reap)
        /\ (\A c \in Clients : WF_vars(Start(c)) /\ WF_vars(Observe(c)))
=============================================================================
