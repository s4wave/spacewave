---- MODULE SharedObjectSyncNotice ----
\* The replay wait the sync notice shows. This device's replay of the Space's
\* changes stops at a change whose type no handler on this device resolves
\* yet, such as one a plugin this device has not started or the person has not
\* approved. The Space stays open at the content before that change, and edits
\* made meanwhile are saved and apply, in order, once replay passes it.
\*
\* None shows no wait. Handler says the Space waits for the app that handles a
\* change and nothing needs the person. Approval names the plugin the device
\* waits for the person to approve; the approval itself is made where plugin
\* approvals are made, so the notice offers no press. Both waiting states say
\* that edits made now are saved and apply once the wait ends, since a write
\* made while waiting is ordered at once and decided after the change.
\*
\* The wait ends when the handler registers or a checkpoint covers the change.
\* A restart keeps the change and any saved edit in the order, so the wait
\* starts again until the handler arrives.

People == {"member"}

Waits == {"none", "handler", "approval"}

\* The notice offers no press in any state.
Buttons == {}

VARIABLE wait \* what replay waits for
vars == <<wait>>

Shown(p) == {}

Allowed(p, b) == FALSE

\* What the notice tells the person.
Hint(p) ==
  CASE wait = "none" -> "Shows the Space with no wait"
    [] wait = "handler" -> "Says the Space waits for the app that handles a change, and edits apply after"
    [] wait = "approval" -> "Names the plugin waiting for approval, and edits apply after"

Init == wait = "none"

\* Replay reaches a change no handler on this device resolves.
Stall ==
  /\ wait = "none"
  /\ wait' = "handler"
\* The plugin that could handle it is not approved on this device.
NeedApproval ==
  /\ wait = "handler"
  /\ wait' = "approval"
\* The person approves the plugin elsewhere; it starts.
Approve ==
  /\ wait = "approval"
  /\ wait' = "handler"
\* The handler registers, or a checkpoint covers the change: replay passes
\* it and applies the saved edits.
Pass ==
  /\ wait # "none"
  /\ wait' = "none"
\* The app restarts: replay stops at the same change again, edits kept.
Restart ==
  /\ wait = "approval"
  /\ wait' = "handler"

Next ==
  \/ Stall
  \/ NeedApproval
  \/ Approve
  \/ Pass
  \/ Restart

S == INSTANCE Surface WITH Actions <- Buttons, Next <- Hint
EveryShownActionWorks == S!EveryShownActionWorks
NobodyIsStuck == S!NobodyIsStuck

TypeOK == wait \in Waits
====
