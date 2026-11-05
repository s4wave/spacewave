---- MODULE TypedWatchAdmission ----
\* WatchTypedObject binds a caller to mount and client lifetimes. AfterFunc
\* cancellation may run after owner startup, even for an already-closed grant.
\* This bounded admission model covers authorities closed before the RPC;
\* ongoing watch replacement and child retirement remain separate lifecycles.
EXTENDS Naturals
CheckAuthorities == TRUE
VARIABLES mountOpen, clientOpen, cancelled, phase, published
vars == <<mountOpen, clientOpen, cancelled, phase, published>>
Init == /\ mountOpen \in BOOLEAN /\ clientOpen \in BOOLEAN
        /\ cancelled = FALSE /\ phase = "binding" /\ published = FALSE
ObserveCancellation == /\ ~mountOpen \/ ~clientOpen
                       /\ ~cancelled /\ cancelled' = TRUE
                       /\ UNCHANGED <<mountOpen, clientOpen, phase, published>>
Admit == /\ phase = "binding"
         /\ phase' = IF CheckAuthorities /\ (~mountOpen \/ ~clientOpen)
                      THEN "done" ELSE "owner"
         /\ UNCHANGED <<mountOpen, clientOpen, cancelled, published>>
Publish == /\ phase = "owner" /\ ~cancelled
           /\ published' = TRUE /\ phase' = "done"
           /\ UNCHANGED <<mountOpen, clientOpen, cancelled>>
Cancel == /\ phase = "owner" /\ cancelled /\ phase' = "done"
          /\ UNCHANGED <<mountOpen, clientOpen, cancelled, published>>
Done == /\ phase = "done" /\ UNCHANGED vars
Next == ObserveCancellation \/ Admit \/ Publish \/ Cancel \/ Done
Spec == Init /\ [][Next]_vars
AlreadyClosedNeverPublishes == published => mountOpen /\ clientOpen
=============================================================================
