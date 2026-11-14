---- MODULE TerminalViewer ----
\* What the Terminal viewer draws from its three sources: the object handle,
\* the session state watch, and the screen's connection.
\*
\* The handle opens first; the watch and the connection run only on an open
\* handle and restart with it. While the handle waits or fails, its status
\* takes the body. Once it is open the screen takes the body, a failed watch
\* is a notice above it, and the connection's own card covers the screen
\* while it connects, after it closes and when it fails.
\*
\* The watch reports its parent's failure as its own, so the viewer reads the
\* watch only while the handle is open; otherwise one failure would draw
\* twice, each with a Retry. A failure is drawn by exactly one element, and
\* each Retry restarts the source that failed. The bar names the session's
\* target only once the watch has said what it is.

People == {"person"}

\* Each Retry restarts one source; type is input to a live screen.
Buttons == {"retryHandle", "retryWatch", "retryConn", "type"}

VARIABLES
  handle, \* the object handle: waiting, open or failed
  watch,  \* the state watch's own state; none while the handle is not open
  conn    \* the screen's connection; none while the handle is not open
vars == <<handle, watch, conn>>

\* The watch's error as the viewer receives it: its parent's first.
WatchReported == IF handle = "failed" THEN "failed" ELSE watch

\* The viewer's one derived state: the body, the notice and the card.
Body == IF handle = "open" THEN "screen" ELSE "status"
Notice ==
  IF handle = "open" /\ WatchReported = "failed" THEN "watch" ELSE "none"
Card ==
  IF handle = "open" /\ conn \in {"connecting", "closed", "failed"}
  THEN conn
  ELSE "none"
\* The bar names a target only from the session state.
Described == handle = "open" /\ watch = "arrived"

\* The failure each element draws, or none.
Draws(e) ==
  CASE e = "body" -> IF Body = "status" /\ handle = "failed"
                     THEN "handle" ELSE "none"
    [] e = "notice" -> IF Notice = "none" THEN "none"
                       ELSE IF handle = "failed" THEN "handle" ELSE "watch"
    [] e = "card" -> IF Card = "failed" THEN "conn" ELSE "none"
Elements == {"body", "notice", "card"}

\* The sources that have failed.
Failed ==
  (IF handle = "failed" THEN {"handle"} ELSE {})
    \cup (IF handle = "open" /\ watch = "failed" THEN {"watch"} ELSE {})
    \cup (IF handle = "open" /\ conn = "failed" THEN {"conn"} ELSE {})

Shown(p) ==
  (IF Draws("body") = "handle" THEN {"retryHandle"} ELSE {})
    \cup (IF Notice = "watch" THEN {"retryWatch"} ELSE {})
    \cup (IF Card \in {"failed", "closed"} THEN {"retryConn"} ELSE {})
    \cup (IF Body = "screen" /\ Card = "none" THEN {"type"} ELSE {})

\* Each Retry works only on its own failed source under an open handle.
Allowed(p, b) ==
  \/ b = "retryHandle" /\ handle = "failed"
  \/ b = "retryWatch" /\ handle = "open" /\ watch = "failed"
  \/ b = "retryConn" /\ handle = "open" /\ conn \in {"failed", "closed"}
  \/ b = "type" /\ handle = "open" /\ conn = "active"

\* What a state with nothing to press tells the person.
Hint(p) ==
  CASE handle = "waiting" -> "Names the terminal it opens"
    [] Card = "connecting" -> "Says the screen is connecting"
    [] OTHER -> ""

Init ==
  /\ handle = "waiting"
  /\ watch = "none"
  /\ conn = "none"

\* The handle opens; the watch and the connection start on it.
HandleOpen ==
  /\ handle = "waiting"
  /\ handle' = "open"
  /\ watch' = "waiting"
  /\ conn' = "connecting"
\* The handle fails, before or after it opened; what ran on it stops.
HandleFail ==
  /\ handle \in {"waiting", "open"}
  /\ handle' = "failed"
  /\ watch' = "none"
  /\ conn' = "none"
RetryHandle(p) ==
  /\ "retryHandle" \in Shown(p)
  /\ handle' = "waiting"
  /\ UNCHANGED <<watch, conn>>
\* The watch yields the session state, again on each change.
WatchArrive ==
  /\ handle = "open"
  /\ watch \in {"waiting", "arrived"}
  /\ watch' = "arrived"
  /\ UNCHANGED <<handle, conn>>
\* The watch fails and drops its value.
WatchFail ==
  /\ handle = "open"
  /\ watch \in {"waiting", "arrived"}
  /\ watch' = "failed"
  /\ UNCHANGED <<handle, conn>>
RetryWatch(p) ==
  /\ "retryWatch" \in Shown(p)
  /\ watch' = "waiting"
  /\ UNCHANGED <<handle, conn>>
\* The connection goes live, ends or fails.
ConnStep ==
  /\ handle = "open"
  /\ \/ conn = "connecting" /\ conn' \in {"active", "failed"}
     \/ conn = "active" /\ conn' \in {"closed", "failed"}
  /\ UNCHANGED <<handle, watch>>
RetryConn(p) ==
  /\ "retryConn" \in Shown(p)
  /\ conn' = "connecting"
  /\ UNCHANGED <<handle, watch>>

Next ==
  \/ HandleOpen
  \/ HandleFail
  \/ WatchArrive
  \/ WatchFail
  \/ ConnStep
  \/ \E p \in People :
       RetryHandle(p) \/ RetryWatch(p) \/ RetryConn(p)

S == INSTANCE Surface WITH Actions <- Buttons, Next <- Hint
EveryShownActionWorks == S!EveryShownActionWorks
NobodyIsStuck == S!NobodyIsStuck

\* Every failure is drawn by exactly one element.
EachFailureDrawnOnce ==
  \A f \in Failed : \E e \in Elements :
    /\ Draws(e) = f
    /\ \A o \in Elements \ {e} : Draws(o) # f
\* No element draws a failure that has not happened.
NothingDrawnUnfailed == \A e \in Elements : Draws(e) \in Failed \cup {"none"}
\* The bar names a target only from the session state.
TargetOnlyFromSession == Described => watch = "arrived"
\* The watch and the connection run only on an open handle.
SourcesNeedHandle == handle # "open" => watch = "none" /\ conn = "none"
TypeOK ==
  /\ handle \in {"waiting", "open", "failed"}
  /\ watch \in {"none", "waiting", "arrived", "failed"}
  /\ conn \in {"none", "connecting", "active", "closed", "failed"}
====
