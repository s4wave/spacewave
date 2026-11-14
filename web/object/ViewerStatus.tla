---- MODULE ViewerStatus ----
\* What an object viewer shows before and instead of its object: waiting while
\* the record is read, failed when the read fails, empty when the record holds
\* nothing to show, and ready once the viewer draws the object itself.
\*
\* Waiting names what the person waits for. Failed says what went wrong and
\* offers Retry only when the source can read again; a failure no retry can
\* fix, such as a malformed record, says so and offers nothing. Retry starts a
\* new read, so it leaves the screen while that read runs and cannot be
\* pressed twice.
\*
\* A press shows in the states where it can succeed. A press on the record,
\* such as Edit, Add or Export, needs a record drawn. A standing press, such as
\* Refresh or opening a query editor, needs no record, so it shows in every
\* state; some viewers have none.
\*
\* A re-read of the same record keeps it on screen while it runs. A failed
\* resource reports no value, only the last one it had, so a failure keeps
\* the record only where the viewer opts in by drawing that last value: the
\* record stays under the failure's notice and makes no empty claim there,
\* and its Retry reads again behind the notice, which stays until the read
\* returns. A viewer that draws only the value lets the failure take the
\* body; its record returns only when a Retry reads it again, and that Retry
\* waits. Another object in the pane clears the record either way.

People == {"person"}

Statuses == {"waiting", "failed", "empty", "ready"}

\* retry starts a new read; act is a press on the record; standing is a press
\* that needs no record.
Buttons == {"retry", "act", "standing"}

VARIABLES
  status,    \* what the viewer shows
  retryable, \* in failed, whether the source can read again
  drawn,     \* whether the viewer draws a record that arrived
  got,       \* history: a record arrived since the pane last changed
  standing,  \* whether this viewer has a standing press
  keeps      \* whether the viewer draws a failed resource's last value
vars == <<status, retryable, drawn, got, standing, keeps>>

Arrived == status \in {"empty", "ready"}

Shown(p) ==
  (IF status = "failed" /\ retryable THEN {"retry"} ELSE {})
    \cup (IF Arrived \/ drawn THEN {"act"} ELSE {})
    \cup (IF standing THEN {"standing"} ELSE {})

\* Retry works only while the source can read again; a press on the record
\* works only on a record drawn; a standing press works in every state.
Allowed(p, b) ==
  \/ b = "retry" /\ status = "failed" /\ retryable
  \/ b = "act" /\ (Arrived \/ drawn)
  \/ b = "standing" /\ standing

\* What a state tells the person beside its presses.
Hint(p) ==
  CASE status = "waiting" /\ ~drawn -> "Names what it is reading"
    [] status = "failed" /\ ~retryable -> "Says why reading again cannot help"
    [] OTHER -> ""

Init ==
  /\ status = "waiting"
  /\ retryable = FALSE
  /\ drawn = FALSE
  /\ got = FALSE
  /\ standing \in BOOLEAN
  /\ keeps \in BOOLEAN

\* The read finishes with the record, which may hold nothing to show. A read
\* behind the notice over a kept record finishes the same way.
Arrive ==
  /\ status = "waiting" \/ (status = "failed" /\ drawn)
  /\ status' \in {"empty", "ready"}
  /\ retryable' = FALSE
  /\ drawn' = TRUE
  /\ got' = TRUE
  /\ UNCHANGED <<standing, keeps>>
\* The read fails, or a watched record fails after it arrived. What was drawn
\* stays under the notice only when the viewer draws the last value.
Fail ==
  /\ status # "failed"
  /\ status' = "failed"
  /\ retryable' \in BOOLEAN
  /\ drawn' = (drawn /\ keeps)
  /\ UNCHANGED <<got, standing, keeps>>
\* The record is read again, as a refetch or a reconnecting watch; what was
\* drawn stays while it runs.
Reread ==
  /\ Arrived
  /\ status' = "waiting"
  /\ UNCHANGED <<retryable, drawn, got, standing, keeps>>
\* Another object opens in the pane; nothing of the old one stays.
Change ==
  /\ status' = "waiting"
  /\ retryable' = FALSE
  /\ drawn' = FALSE
  /\ got' = FALSE
  /\ standing' \in BOOLEAN
  /\ keeps' \in BOOLEAN
\* Retry reads again: behind the notice over a kept record, else in a wait.
Retry(p) ==
  /\ "retry" \in Shown(p)
  /\ status' = IF drawn THEN "failed" ELSE "waiting"
  /\ retryable' = drawn
  /\ UNCHANGED <<drawn, got, standing, keeps>>

Next ==
  \/ Arrive
  \/ Fail
  \/ Reread
  \/ Change
  \/ \E p \in People : Retry(p)

S == INSTANCE Surface WITH Actions <- Buttons, Next <- Hint
EveryShownActionWorks == S!EveryShownActionWorks
NobodyIsStuck == S!NobodyIsStuck

\* Retry shows exactly where a read can run again.
RetryWhereItCanSucceed ==
  "retry" \in Shown("person") <=> status = "failed" /\ retryable
\* In a viewer that draws the last value, a record that arrived stays drawn
\* until another object opens.
ArrivedStaysDrawn == keeps /\ got => drawn
\* In a viewer that draws only the value, a failure draws no record.
DroppedFailureDrawsNothing == ~keeps /\ status = "failed" => ~drawn
TypeOK ==
  /\ status \in Statuses
  /\ retryable \in BOOLEAN
  /\ drawn \in BOOLEAN
  /\ got \in BOOLEAN
  /\ standing \in BOOLEAN
  /\ keeps \in BOOLEAN
\* Only a failure carries whether it can be retried.
RetryableOnlyFailed == retryable => status = "failed"
====
