---- MODULE PortalCapture ----
\* One registered peer owns capture and input. A read of the grant permits
\* Start, while the backend can still refuse it or fail to answer. Model the
\* deadline independently of the reply and the FD handoff independently of
\* Start's acceptance.

VARIABLES state, grant, session, request, expired
vars == <<state, grant, session, request, expired>>

Init ==
  /\ state = "registered"
  /\ grant \in BOOLEAN
  /\ session = FALSE
  /\ request = FALSE
  /\ expired = FALSE

Prepare ==
  /\ state = "registered"
  /\ state' = IF grant THEN "prepared" ELSE "not_authorized"
  /\ session' = grant
  /\ UNCHANGED <<grant, request, expired>>

Check ==
  /\ state = "prepared"
  /\ state' = IF grant THEN "checked" ELSE "closing_unauthorized"
  /\ UNCHANGED <<grant, session, request, expired>>

Start ==
  /\ state = "checked"
  /\ state' = "starting"
  /\ request' = TRUE
  /\ UNCHANGED <<grant, session, expired>>

Deadline ==
  /\ state = "starting"
  /\ ~expired
  /\ expired' = TRUE
  /\ UNCHANGED <<state, grant, session, request>>

Accept ==
  /\ state = "starting"
  /\ grant
  /\ ~expired
  /\ state' = "opening_remote"
  /\ request' = FALSE
  /\ UNCHANGED <<grant, session, expired>>

OpenRemote ==
  /\ state = "opening_remote"
  /\ state' = "capturing"
  /\ UNCHANGED <<grant, session, request, expired>>

DenyOrExpire ==
  /\ state = "starting"
  /\ state' = "closing_unauthorized"
  /\ request' = FALSE
  /\ UNCHANGED <<grant, session, expired>>

Revoke ==
  /\ grant
  /\ grant' = FALSE
  /\ UNCHANGED <<state, session, request, expired>>

ObserveRevocation ==
  /\ ~grant
  /\ state \in {"prepared", "checked", "starting", "opening_remote", "capturing"}
  /\ state' = "closing_unauthorized"
  /\ request' = FALSE
  /\ UNCHANGED <<grant, session, expired>>

CloseUnauthorized ==
  /\ state = "closing_unauthorized"
  /\ state' = "not_authorized"
  /\ session' = FALSE
  /\ UNCHANGED <<grant, request, expired>>

\* While Start is pending, a separate Closed signal can precede its refusal
\* in the caller's view. Both signals therefore lead to closing_unauthorized.
SessionClosed ==
  /\ state \in {"prepared", "checked", "starting", "opening_remote", "capturing"}
  /\ state' = IF state = "starting" THEN "closing_unauthorized" ELSE "closed"
  /\ session' = (state = "starting")
  /\ request' = FALSE
  /\ UNCHANGED <<grant, expired>>

Cancel ==
  /\ state \in {"registered", "prepared", "checked", "starting", "opening_remote", "capturing"}
  /\ state' = "closed"
  /\ session' = FALSE
  /\ request' = FALSE
  /\ UNCHANGED <<grant, expired>>

Done ==
  /\ state \in {"not_authorized", "closed"}
  /\ UNCHANGED vars

ResolveStart == Accept \/ DenyOrExpire
Next == Prepare \/ Check \/ Start \/ Deadline \/ ResolveStart \/ OpenRemote \/ Revoke
        \/ ObserveRevocation \/ CloseUnauthorized \/ SessionClosed \/ Cancel \/ Done
Spec == Init /\ [][Next]_vars /\ WF_vars(ResolveStart) /\ WF_vars(ObserveRevocation) /\ WF_vars(CloseUnauthorized)

OnlyStartHasRequest == request = (state = "starting")
TerminalHasNoSession == state \in {"not_authorized", "closed"} => ~session
CaptureAndInputShareSession == state = "capturing" => session
ExpiredStartCannotCapture == expired => state \notin {"opening_remote", "capturing"}
StartResolves == (state = "starting") ~> (state # "starting")
RevocationEndsCapture == (state = "capturing" /\ ~grant) ~> (state \in {"closed", "not_authorized"})
====
