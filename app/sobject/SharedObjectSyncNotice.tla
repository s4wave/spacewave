---- MODULE SharedObjectSyncNotice ----
\* The replay state the sync notice shows, as the Space's health carries it
\* from the body driver. The same state decides whether the Space takes
\* writes: only caught up does, so the notice says read only in every other
\* state and never while a write would be saved.
\*
\* Opening is replay before its first step reached the end of the order the
\* Space holds. Handler is replay stopped at a change whose type no handler on
\* this device resolves yet, such as one from a plugin this device has not
\* started; nothing needs the person. Approval names the plugin the device
\* waits for the person to approve; the approval itself is made where plugin
\* approvals are made, so the notice offers no press. CaughtUp shows nothing.
\*
\* CatchingUp is replay that caught up while this device holds another
\* device's change it has not applied. It begins when the state holding that
\* change arrives, before replay steps it, so no write is taken on a World
\* that lacks it, and ends when the step reaches the end of the order. A wait
\* can begin at that change, and the Space is read only again until replay
\* passes it. A restart
\* opens the Space again and keeps every change in the order.
\*
\* The component holds the opening and catching up notices back for their
\* first two seconds, so a Space that opens or applies a change at once
\* flashes none; a write refused then says why in
\* its own error, and an editor's save control reads the same state.

People == {"member"}

Phases == {"opening", "catchingUp", "handler", "approval", "caughtUp"}

\* The notice offers no press in any state.
Buttons == {}

VARIABLE phase \* where replay stands
vars == <<phase>>

\* Whether the Space takes writes.
Writable == phase = "caughtUp"

Shown(p) == {}

Allowed(p, b) == FALSE

\* What the notice tells the person.
Hint(p) ==
  CASE phase = "caughtUp" -> "Shows the Space with no notice"
    [] phase = "opening" -> "Says the Space is opening, and is read only until then"
    [] phase = "catchingUp" -> "Says the Space is catching up with another device, and is read only until then"
    [] phase = "handler" -> "Says the Space waits for the app that handles a change, and is read only until then"
    [] phase = "approval" -> "Names the plugin waiting for approval, and is read only until then"

Init == phase = "opening"

\* Replay's first step reaches the end of the order.
Open ==
  /\ phase = "opening"
  /\ phase' = "caughtUp"
\* A state arrives holding another device's change replay has not applied.
Arrive ==
  /\ phase = "caughtUp"
  /\ phase' = "catchingUp"
\* The step reaches the end of the order with that change applied.
Apply ==
  /\ phase = "catchingUp"
  /\ phase' = "caughtUp"
\* Replay reaches a change no handler on this device resolves, while opening
\* or stepping a change another device ordered after it caught up.
Stall ==
  /\ phase \in {"opening", "catchingUp"}
  /\ phase' = "handler"
\* The plugin that could handle it is not approved on this device.
NeedApproval ==
  /\ phase = "handler"
  /\ phase' = "approval"
\* The person approves the plugin elsewhere; it starts.
Approve ==
  /\ phase = "approval"
  /\ phase' = "handler"
\* The handler registers, or a checkpoint covers the change: replay passes
\* it and reaches the end of the order, and the Space takes edits again.
Pass ==
  /\ phase \in {"handler", "approval"}
  /\ phase' = "caughtUp"
\* The app restarts: replay opens again.
Restart ==
  /\ phase # "opening"
  /\ phase' = "opening"

Next ==
  \/ Open
  \/ Arrive
  \/ Apply
  \/ Stall
  \/ NeedApproval
  \/ Approve
  \/ Pass
  \/ Restart

S == INSTANCE Surface WITH Actions <- Buttons, Next <- Hint
EveryShownActionWorks == S!EveryShownActionWorks
NobodyIsStuck == S!NobodyIsStuck

\* The notice shows nothing exactly while the Space takes writes.
NoticeMatchesWrites == (Hint("member") = "Shows the Space with no notice") <=> Writable

TypeOK == phase \in Phases
====
