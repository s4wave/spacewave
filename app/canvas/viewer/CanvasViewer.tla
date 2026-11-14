---- MODULE CanvasViewer ----
EXTENDS FiniteSets
\* What the Canvas viewer draws from its two sources: the object handle and
\* the canvas state watch on it.
\*
\* The handle opens first and the watch runs on it. The watch reports its
\* parent's failure as its own, with the parent's Retry, so the viewer reads
\* one failure from the watch and draws it once. Until a canvas arrives, a
\* wait or a failure takes the body. Once one arrived, the stage keeps it:
\* the watch reconnects after a failure and keeps its last value while it
\* backs off, and a handle that fails leaves the watch's value in place, so
\* the failure is a notice above the stage and the edits the person made stay
\* drawn. Only a watch that fails before its first value takes the body.
\*
\* An edit is drawn at once and sent; a send that fails rolls the edit back
\* and leaves a notice above the stage, which the next edit clears.

People == {"person"}

\* Each Retry restarts the source that failed; edit is any change to the
\* canvas.
Buttons == {"retryHandle", "retryWatch", "edit"}

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

\* The row above the stage: a read that failed, the watch reconnecting, or
\* nothing.
Notice ==
  CASE Body # "stage" -> "none"
    [] Reported # "none" -> "failed"
    [] handle = "open" /\ watch = "reconnecting" -> "reconnecting"
    [] OTHER -> "none"

\* The stage's own hint: a canvas with no node says how to add one.
Hint0 == Body = "stage" /\ ~nodes

\* The failure each element draws, or none.
Draws(e) ==
  CASE e = "body" -> IF Body = "failed" THEN Reported ELSE "none"
    [] e = "notice" -> IF Notice = "failed" THEN Reported ELSE "none"
    [] e = "action" -> IF Body = "stage" /\ action = "failed"
                       THEN "send" ELSE "none"
Elements == {"body", "notice", "action"}

\* The sources that have failed.
Failed ==
  (IF handle = "failed" THEN {"handle"} ELSE {})
    \cup (IF handle = "open" /\ watch = "failed" THEN {"watch"} ELSE {})
    \cup (IF action = "failed" THEN {"send"} ELSE {})

Shown(p) ==
  (IF Reported = "handle" THEN {"retryHandle"} ELSE {})
    \cup (IF Reported = "watch" THEN {"retryWatch"} ELSE {})
    \cup (IF Body = "stage" THEN {"edit"} ELSE {})

\* Each Retry works on its own failed source; an edit works on a drawn
\* canvas, and while the handle is gone its send fails and rolls back.
Allowed(p, b) ==
  \/ b = "retryHandle" /\ handle = "failed"
  \/ b = "retryWatch" /\ handle = "open" /\ watch = "failed"
  \/ b = "edit" /\ kept

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
\* The handle fails, before or after it opened; the watch stops but keeps
\* the canvas it held.
HandleFail ==
  /\ handle \in {"waiting", "open"}
  /\ handle' = "failed"
  /\ watch' = "none"
  /\ UNCHANGED <<kept, nodes, pending, action>>
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
\* The watch fails after a value and reconnects, keeping the value.
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
\* earlier failed send leaves.
Edit(p) ==
  /\ "edit" \in Shown(p)
  /\ pending' = TRUE
  /\ nodes' \in BOOLEAN
  /\ action' = "none"
  /\ UNCHANGED <<handle, watch, kept>>
\* The canvas confirms the sent edits.
Saved ==
  /\ pending
  /\ handle = "open"
  /\ pending' = FALSE
  /\ UNCHANGED <<handle, watch, kept, nodes, action>>
\* A send fails: the edit rolls back and a notice says so.
SendFail ==
  /\ pending
  /\ pending' = FALSE
  /\ action' = "failed"
  /\ UNCHANGED <<handle, watch, kept, nodes>>

Next ==
  \/ HandleOpen
  \/ HandleFail
  \/ WatchArrive
  \/ WatchDrop
  \/ WatchFail
  \/ Saved
  \/ SendFail
  \/ \E p \in People :
       RetryHandle(p) \/ RetryWatch(p) \/ Edit(p)

S == INSTANCE Surface WITH Actions <- Buttons, Next <- Hint
EveryShownActionWorks == S!EveryShownActionWorks
NobodyIsStuck == S!NobodyIsStuck

\* Every failure is drawn by exactly one element.
EachFailureDrawnOnce ==
  \A f \in Failed : Cardinality({e \in Elements : Draws(e) = f}) = 1
\* No element draws a failure that did not happen.
NothingDrawnUnfailed ==
  \A e \in Elements : Draws(e) = "none" \/ Draws(e) \in Failed
\* A canvas that arrived stays on the stage, so a failed read never takes
\* away edits the person made.
ArrivedStaysDrawn == kept => Body = "stage"
PendingDrawn == pending => Body = "stage"
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
