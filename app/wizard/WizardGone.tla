---- MODULE WizardGone ----
\* The screen a wizard's tab shows once the wizard is removed. It watches the
\* World's record of how the wizard ended, rereading it on each revision:
\* checking while the first read runs, unreadable when it fails, finished as
\* the object a Finish recorded, or deleted when the World records none. A
\* wizard records no ending when it was cancelled or deleted, and its ending
\* goes when the object it finished as is deleted, since the World deletes
\* every quad that names a deleted object.
\*
\* Finished offers Open, deleted offers Open Space, unreadable offers Retry,
\* and every state offers Close tab. Between a revision and the reread the
\* screen still shows the last read; a watch that fails after its first
\* result reconnects and keeps it.

People == {"person"}
Buttons == {"open", "openSpace", "retry", "close"}

VARIABLES
  ending, \* what the World records: "finished" or "deleted"
  view,   \* what the screen shows
  fresh   \* whether view shows the World's latest revision
vars == <<ending, view, fresh>>

Views == {"checking", "unreadable", "finished", "deleted"}

Shown(p) ==
  {"close"}
    \cup (IF view = "unreadable" THEN {"retry"} ELSE {})
    \cup (IF view = "finished" THEN {"open"} ELSE {})
    \cup (IF view = "deleted" THEN {"openSpace"} ELSE {})

Allowed(p, b) ==
  \/ b = "close"
  \/ b = "retry" /\ view = "unreadable"
  \/ b = "open" /\ view = "finished"
  \/ b = "openSpace" /\ view = "deleted"

Hint(p) ==
  CASE view = "checking" -> "Says it is checking how the wizard ended"
    [] view = "unreadable" -> "Says the check failed"
    [] view = "finished" -> "Names the object it finished with"
    [] OTHER -> "Says the wizard no longer exists"

Init ==
  /\ ending \in {"finished", "deleted"}
  /\ view = "checking"
  /\ fresh = FALSE

\* The watch reads the World's ending.
Read ==
  /\ ~fresh
  /\ view # "unreadable"
  /\ view' = ending
  /\ fresh' = TRUE
  /\ UNCHANGED ending

\* The first read fails; later failures reconnect and keep the last read.
ReadFails ==
  /\ view = "checking"
  /\ view' = "unreadable"
  /\ UNCHANGED <<ending, fresh>>

\* Retry reads again.
Retry ==
  /\ "retry" \in Shown("person")
  /\ view' = "checking"
  /\ UNCHANGED <<ending, fresh>>

\* Someone deletes the object the wizard finished as, and its ending with it;
\* the new revision makes the screen's read stale.
DeleteTarget ==
  /\ ending = "finished"
  /\ ending' = "deleted"
  /\ fresh' = FALSE
  /\ UNCHANGED view

\* Open, Open Space and Close tab leave the screen.
Leave ==
  /\ Shown("person") \cap {"open", "openSpace", "close"} # {}
  /\ UNCHANGED vars

Next == Read \/ ReadFails \/ Retry \/ DeleteTarget \/ Leave

S == INSTANCE Surface WITH Actions <- Buttons, Next <- Hint
EveryShownActionWorks == S!EveryShownActionWorks
NobodyIsStuck == S!NobodyIsStuck

TypeOK ==
  /\ ending \in {"finished", "deleted"}
  /\ view \in Views
  /\ fresh \in BOOLEAN
\* A current screen never offers an object that is gone, nor calls a wizard
\* deleted that finished as an object that exists.
FreshViewIsTrue == fresh => view = ending
\* A wizard shows as deleted only when it is: a failed read is not an
\* ending, and an ending, once gone, never comes back.
DeletedIsTrue == view = "deleted" => ending = "deleted"
====
