---- MODULE CanvasViewer ----
EXTENDS FiniteSets
\* What the Canvas viewer draws from its two sources: the object handle and
\* the canvas state watch on it.
\*
\* The handle opens first and the watch runs on it. The watch reports its
\* parent's failure as its own, with the parent's Retry, so the viewer reads
\* one failure from the watch and draws it once. Until a canvas arrives, a
\* wait or a failure takes the body. A watch that fails after a canvas
\* reopens after a backoff and keeps the canvas, with a line saying it reads
\* again; it never reports that failure, however long it lasts. A handle
\* that fails or reloads, as on a lost connection, drops the watch's canvas:
\* its failure takes the body with the handle's Retry, its reload the wait.
\* The edits the person made stay queued in the viewer while the canvas is
\* gone and draw again on the canvas that next arrives.
\*
\* An edit is drawn at once and sent; a send that fails rolls the edit back
\* and leaves a notice over the stage, so the stage's box never changes. The
\* notice stays through later edits, since a drag sends many, and leaves when
\* the person dismisses it or a later send is saved.

People == {"person"}

\* Each Retry restarts the source that failed; edit is any change to the
\* canvas; dismiss clears the failed send's notice.
Buttons == {"retryHandle", "retryWatch", "edit", "dismiss"}

VARIABLES
  handle,  \* the object handle: waiting, open or failed
  watch,   \* the watch on an open handle: none, waiting, arrived,
           \* reconnecting or failed
  kept,    \* a canvas arrived and the watch still holds it
  nodes,   \* the canvas holds at least one node
  pending, \* edits drawn but not yet confirmed
  action   \* the last send: none or failed
vars == <<handle, watch, kept, nodes, pending, action>>

\* The failure the viewer reads from the watch: its parent's first.
Reported ==
  CASE handle = "failed" -> "handle"
    [] watch = "failed" -> "watch"
    [] OTHER -> "none"

\* The body: the stage once a canvas arrived, else the wait or the failure.
Body ==
  CASE kept -> "stage"
    [] Reported # "none" -> "failed"
    [] OTHER -> "waiting"

\* The line over the stage while the watch reopens.
Rereading == Body = "stage" /\ watch = "reconnecting"

\* The stage's own hint: a canvas with no node says how to add one.
Hint0 == Body = "stage" /\ ~nodes

\* The failure each element draws, or none.
Draws(e) ==
  CASE e = "body" -> IF Body = "failed" THEN Reported ELSE "none"
    [] e = "action" -> IF Body = "stage" /\ action = "failed"
                       THEN "send" ELSE "none"
Elements == {"body", "action"}

\* The failures the viewer can draw: a failed source, and a failed send once
\* a canvas is on the stage to draw its notice over.
Failed ==
  (IF handle = "failed" THEN {"handle"} ELSE {})
    \cup (IF handle = "open" /\ watch = "failed" THEN {"watch"} ELSE {})
    \cup (IF action = "failed" /\ Body = "stage" THEN {"send"} ELSE {})

Shown(p) ==
  (IF Reported = "handle" THEN {"retryHandle"} ELSE {})
    \cup (IF Reported = "watch" THEN {"retryWatch"} ELSE {})
    \cup (IF Body = "stage" THEN {"edit"} ELSE {})
    \cup (IF Draws("action") = "send" THEN {"dismiss"} ELSE {})

\* Each Retry works on its own failed source; an edit works on a drawn
\* canvas, and while the handle is gone its send fails and rolls back.
Allowed(p, b) ==
  \/ b = "retryHandle" /\ handle = "failed"
  \/ b = "retryWatch" /\ handle = "open" /\ watch = "failed"
  \/ b = "edit" /\ kept
  \/ b = "dismiss" /\ action = "failed"

\* What a state with nothing to press tells the person.
Hint(p) ==
  CASE Body = "waiting" -> "Names the canvas it reads"
    [] Hint0 -> "Says how to add an object"
    [] OTHER -> ""

Init ==
  /\ handle = "waiting"
  /\ watch = "none"
  /\ kept = FALSE
  /\ nodes = FALSE
  /\ pending = FALSE
  /\ action = "none"

\* The handle opens and the watch starts on it.
HandleOpen ==
  /\ handle = "waiting"
  /\ handle' = "open"
  /\ watch' = "waiting"
  /\ UNCHANGED <<kept, nodes, pending, action>>
\* The handle fails, before or after it opened; the watch stops and drops
\* the canvas it held, while the queued edits stay.
HandleFail ==
  /\ handle \in {"waiting", "open"}
  /\ handle' = "failed"
  /\ watch' = "none"
  /\ kept' = FALSE
  /\ UNCHANGED <<nodes, pending, action>>
\* The open handle reloads, as on a lost connection; the watch stops and
\* drops the canvas it held until the new handle's watch yields.
HandleReload ==
  /\ handle = "open"
  /\ handle' = "waiting"
  /\ watch' = "none"
  /\ kept' = FALSE
  /\ UNCHANGED <<nodes, pending, action>>
RetryHandle(p) ==
  /\ "retryHandle" \in Shown(p)
  /\ handle' = "waiting"
  /\ UNCHANGED <<watch, kept, nodes, pending, action>>
\* The watch yields the canvas, again on each change.
WatchArrive ==
  /\ handle = "open"
  /\ watch \in {"waiting", "arrived", "reconnecting"}
  /\ watch' = "arrived"
  /\ kept' = TRUE
  /\ nodes' \in BOOLEAN
  /\ UNCHANGED <<handle, pending, action>>
\* The watch fails after a value and reconnects, keeping the value. It
\* reconnects for as long as it fails and never reports the failure.
WatchDrop ==
  /\ handle = "open"
  /\ watch = "arrived"
  /\ watch' = "reconnecting"
  /\ UNCHANGED <<handle, kept, nodes, pending, action>>
\* The watch fails before its first value and reports it.
WatchFail ==
  /\ handle = "open"
  /\ watch = "waiting"
  /\ ~kept
  /\ watch' = "failed"
  /\ UNCHANGED <<handle, kept, nodes, pending, action>>
RetryWatch(p) ==
  /\ "retryWatch" \in Shown(p)
  /\ watch' = "waiting"
  /\ UNCHANGED <<handle, kept, nodes, pending, action>>
\* The person edits the canvas: the edit draws at once, and the notice of an
\* earlier failed send stays.
Edit(p) ==
  /\ "edit" \in Shown(p)
  /\ pending' = TRUE
  /\ nodes' \in BOOLEAN
  /\ UNCHANGED <<handle, watch, kept, action>>
\* The person dismisses the failed send's notice.
Dismiss(p) ==
  /\ "dismiss" \in Shown(p)
  /\ action' = "none"
  /\ UNCHANGED <<handle, watch, kept, nodes, pending>>
\* The canvas confirms the sent edits, which ends an earlier failure's notice.
Saved ==
  /\ pending
  /\ handle = "open"
  /\ pending' = FALSE
  /\ action' = "none"
  /\ UNCHANGED <<handle, watch, kept, nodes>>
\* A send fails: the edit rolls back and a notice says so.
SendFail ==
  /\ pending
  /\ pending' = FALSE
  /\ action' = "failed"
  /\ UNCHANGED <<handle, watch, kept, nodes>>

Next ==
  \/ HandleOpen
  \/ HandleFail
  \/ HandleReload
  \/ WatchArrive
  \/ WatchDrop
  \/ WatchFail
  \/ Saved
  \/ SendFail
  \/ \E p \in People :
       RetryHandle(p) \/ RetryWatch(p) \/ Edit(p) \/ Dismiss(p)

S == INSTANCE Surface WITH Actions <- Buttons, Next <- Hint
EveryShownActionWorks == S!EveryShownActionWorks
NobodyIsStuck == S!NobodyIsStuck

\* Every failure is drawn by exactly one element.
EachFailureDrawnOnce ==
  \A f \in Failed : Cardinality({e \in Elements : Draws(e) = f}) = 1
\* No element draws a failure that did not happen.
NothingDrawnUnfailed ==
  \A e \in Elements : Draws(e) = "none" \/ Draws(e) \in Failed
\* A canvas the watch holds is on the stage, under any reconnecting line.
ArrivedStaysDrawn == kept => Body = "stage"
\* A failed send's notice leaves only by Dismiss or a later save, never on
\* an edit, so a drag cannot hide it before the person reads it.
SendNoticeStays ==
  [][action = "failed" /\ action' = "none" =>
       Saved \/ \E p \in People : Dismiss(p)]_vars
TypeOK ==
  /\ handle \in {"waiting", "open", "failed"}
  /\ watch \in {"none", "waiting", "arrived", "reconnecting", "failed"}
  /\ kept \in BOOLEAN
  /\ nodes \in BOOLEAN
  /\ pending \in BOOLEAN
  /\ action \in {"none", "failed"}
\* The watch runs only on an open handle.
WatchNeedsHandle == watch # "none" => handle = "open"
====
