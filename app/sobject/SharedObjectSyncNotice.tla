---- MODULE SharedObjectSyncNotice ----
\* The replay state the sync notice shows, as the Space's health carries it
\* from the body driver, and the save of an editor in the Space, which the
\* same state decides. Only caught up takes a write now. While the Space
\* opens, a save waits in the Space's gate and lands once replay catches up,
\* so the notice says edits are saved then; in every wait the Space is read
\* only, a save waiting in the gate is refused, and the editor offers none.
\*
\* Opening is replay before its first step reached the end of the order the
\* Space holds. Handler is replay stopped at a change whose type no handler on
\* this device resolves yet, such as one from a plugin this device has not
\* started; nothing needs the person. Approval names the plugin the device
\* waits for the person to approve; the approval itself is made where plugin
\* approvals are made, so the notice offers no press. Gap is replay stopped at
\* a change whose data this device lacks; retry is replay stopped at a change
\* that failed on this device, until its retry deadline. CaughtUp shows
\* nothing.
\*
\* A wait can begin after caught up, at a change another device ordered
\* later, and the Space is read only again until replay passes it. A restart
\* opens the Space again and keeps every change in the order.
\*
\* A save once pressed is decided by the Space, saved or refused, whether or
\* not its editor is still open: closing the editor does not drop it. A
\* closed editor shows a refusal in a toast. An edit never saved dies with
\* the editor.
\*
\* The component holds the opening notice back for its first two seconds, so
\* a Space that opens at once flashes none.

People == {"member"}

Waits == {"handler", "approval", "gap", "retry"}

Phases == {"opening", "caughtUp"} \cup Waits

Saves == {"none", "held", "saved", "refused"}

\* The editor's press; the notice itself offers none.
Buttons == {"Save"}

VARIABLES
  phase, \* where replay stands
  save, \* the last save pressed: none yet, held in the gate, or decided
  open, \* whether the editor is open
  sent \* whether a save was ever pressed
vars == <<phase, save, open, sent>>

\* Whether a write lands now.
Writable == phase = "caughtUp"

\* Whether the editor offers Save: the Space takes it now or once it opens,
\* and no save of this editor is in flight.
SaveOffered == open /\ phase \in {"opening", "caughtUp"} /\ save # "held"

Shown(p) == IF SaveOffered THEN {"Save"} ELSE {}

Allowed(p, b) == b = "Save" /\ SaveOffered

\* What the notice tells the person.
Hint(p) ==
  CASE phase = "caughtUp" -> "Shows the Space with no notice"
    [] phase = "opening" -> "Says the Space is opening, and edits made now are saved once it opens"
    [] phase = "handler" -> "Says the Space waits for the app that handles a change, and is read only until then"
    [] phase = "approval" -> "Names the plugin waiting for approval, and is read only until then"
    [] phase = "gap" -> "Says the Space waits for data it lacks, and is read only until then"
    [] phase = "retry" -> "Says the Space tries a change again shortly, and is read only until then"

Init ==
  /\ phase = "opening"
  /\ save = "none"
  /\ open = TRUE
  /\ sent = FALSE

\* The person presses Save: it lands at once when caught up, and waits in the
\* gate while the Space opens.
Press ==
  /\ SaveOffered
  /\ save' = IF phase = "caughtUp" THEN "saved" ELSE "held"
  /\ sent' = TRUE
  /\ UNCHANGED <<phase, open>>
\* The person closes the editor; a save it sent goes on.
Close ==
  /\ open
  /\ open' = FALSE
  /\ UNCHANGED <<phase, save, sent>>
\* Replay's first step reaches the end of the order: the gate admits a held
\* save.
Open ==
  /\ phase = "opening"
  /\ phase' = "caughtUp"
  /\ save' = IF save = "held" THEN "saved" ELSE save
  /\ UNCHANGED <<open, sent>>
\* Replay stops at a change it cannot apply yet, while opening or at a change
\* ordered after it caught up: the gate refuses a held save.
Stall ==
  /\ phase \in {"opening", "caughtUp"}
  /\ phase' \in {"handler", "gap", "retry"}
  /\ save' = IF save = "held" THEN "refused" ELSE save
  /\ UNCHANGED <<open, sent>>
\* The plugin that could handle it is not approved on this device.
NeedApproval ==
  /\ phase = "handler"
  /\ phase' = "approval"
  /\ UNCHANGED <<save, open, sent>>
\* The person approves the plugin elsewhere; it starts.
Approve ==
  /\ phase = "approval"
  /\ phase' = "handler"
  /\ UNCHANGED <<save, open, sent>>
\* The handler registers, the data arrives, the retry succeeds, or a
\* checkpoint covers the change: replay passes it and reaches the end of the
\* order, and the Space takes edits again.
Pass ==
  /\ phase \in Waits
  /\ phase' = "caughtUp"
  /\ UNCHANGED <<save, open, sent>>
\* The app restarts: replay opens again, with the editor open again.
Restart ==
  /\ phase # "opening"
  /\ phase' = "opening"
  /\ open' = TRUE
  /\ UNCHANGED <<save, sent>>

Next ==
  \/ Press
  \/ Close
  \/ Open
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

\* A pressed save is never dropped, by closing its editor or otherwise.
PressedSaveKept == sent => save # "none"

\* A save waits in the gate only while the Space opens: replay leaving the
\* opening decides it.
HeldOnlyWhileOpening == save = "held" => phase = "opening"

TypeOK ==
  /\ phase \in Phases
  /\ save \in Saves
  /\ open \in BOOLEAN
  /\ sent \in BOOLEAN
====
