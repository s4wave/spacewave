---- MODULE WizardFinish ----
EXTENDS Naturals

\* A wizard's Finish control, in each tab that shows the wizard, and the
\* World writes behind it. Finish prepares a list of writes (the creates, a
\* Space index move when the index named the wizard, and the wizard delete)
\* and one executor applies the whole list as one atomic step: it commits
\* every write or discards them all. The list carries everything prepared, so
\* no entry waits on the network or the person while the writer is held.
\*
\* idle offers Finish; finishing waits on the list; failed says what failed
\* and offers Retry, which prepares and applies the whole list again; done
\* opens the object. A tab whose wizard is gone, finished or cancelled
\* elsewhere, says so and offers to close the tab.
\*
\* Another writer may move the index off the wizard at any time; the index
\* move expects the index the tab read, so the commit leaves a moved index
\* alone. A reload drops the tab's control back to idle while its list may
\* still commit or discard on the server.

Tabs == {"a", "b"}
People == Tabs

\* finish starts the first press; retry presses again after a failure;
\* close closes a tab whose wizard is gone.
Buttons == {"finish", "retry", "close"}

Controls == {"idle", "finishing", "failed", "done"}

\* The Space index names the wizard, the object Finish made, or something
\* another writer chose.
Indexes == {"wizard", "object", "other"}

VARIABLES
  control, \* each tab's Finish control
  live,    \* whether the tab's control waits on a list in flight
  read,    \* the index each tab read when it pressed
  orphan,  \* the index read by a list in flight from a tab that reloaded,
           \* or none
  wizard,  \* whether the wizard object exists
  objects, \* how many objects Finish made
  index,   \* what the Space index names
  moved    \* history: another writer moved the index
vars == <<control, live, read, orphan, wizard, objects, index, moved>>

Gone(t) == ~wizard /\ control[t] \in {"idle", "failed"}

Shown(t) ==
  (IF control[t] = "idle" /\ wizard THEN {"finish"} ELSE {})
    \cup (IF control[t] = "failed" /\ wizard THEN {"retry"} ELSE {})
    \cup (IF Gone(t) THEN {"close"} ELSE {})

\* A press works only while the wizard exists and the tab waits on nothing.
Allowed(t, b) ==
  \/ b = "finish" /\ control[t] = "idle" /\ wizard /\ ~live[t]
  \/ b = "retry" /\ control[t] = "failed" /\ wizard /\ ~live[t]
  \/ b = "close" /\ Gone(t)

\* What a tab tells the person beside its presses.
Hint(t) ==
  CASE control[t] = "finishing" -> "Says it is finishing"
    [] control[t] = "done" -> "Opens the object"
    [] control[t] = "failed" /\ wizard -> "Says what failed"
    [] OTHER -> ""

Init ==
  /\ control = [t \in Tabs |-> "idle"]
  /\ live = [t \in Tabs |-> FALSE]
  /\ read = [t \in Tabs |-> "wizard"]
  /\ orphan = "none"
  /\ wizard = TRUE
  /\ objects = 0
  /\ index \in {"wizard", "other"}
  /\ moved = FALSE

\* Finish or Retry prepares the list and hands it to the executor.
Press(t) ==
  /\ Shown(t) \cap {"finish", "retry"} # {}
  /\ control' = [control EXCEPT ![t] = "finishing"]
  /\ live' = [live EXCEPT ![t] = TRUE]
  /\ read' = [read EXCEPT ![t] = index]
  /\ UNCHANGED <<orphan, wizard, objects, index, moved>>

\* Apply is the executor's one atomic step: the creates, the index move and
\* the delete commit together. A list that creates refuses itself when the
\* wizard is already gone, so a second tab makes no second object.
Apply(r) ==
  IF wizard
  THEN /\ objects' = objects + 1
       /\ wizard' = FALSE
       /\ index' = IF r = "wizard" /\ index = r THEN "object" ELSE index
  ELSE UNCHANGED <<objects, wizard, index>>

\* The tab's own list commits, or is refused, and the control shows which.
Commit(t) ==
  /\ live[t]
  /\ Apply(read[t])
  /\ control' = [control EXCEPT ![t] = IF wizard THEN "done" ELSE "failed"]
  /\ live' = [live EXCEPT ![t] = FALSE]
  /\ UNCHANGED <<read, orphan, moved>>

\* A write in the list throws, or the tab closes, and the executor discards
\* the list: nothing it staged lands.
Discard(t) ==
  /\ live[t]
  /\ control' = [control EXCEPT ![t] = "failed"]
  /\ live' = [live EXCEPT ![t] = FALSE]
  /\ UNCHANGED <<read, orphan, wizard, objects, index, moved>>

\* The tab reloads; a list in flight carries on without it.
Reload(t) ==
  /\ control' = [control EXCEPT ![t] = "idle"]
  /\ live' = [live EXCEPT ![t] = FALSE]
  /\ ~(live[t] /\ orphan # "none")
  /\ orphan' = IF live[t] THEN read[t] ELSE orphan
  /\ UNCHANGED <<read, wizard, objects, index, moved>>

\* A list whose tab reloaded commits, is refused or discards on the server,
\* with the index its tab read before the reload.
OrphanEnds ==
  /\ orphan # "none"
  /\ orphan' = "none"
  /\ \/ Apply(orphan)
     \/ UNCHANGED <<wizard, objects, index>>
  /\ UNCHANGED <<control, live, read, moved>>

\* Another writer points the index somewhere else.
MoveIndex ==
  /\ index # "other"
  /\ index' = "other"
  /\ moved' = TRUE
  /\ UNCHANGED <<control, live, read, orphan, wizard, objects>>

\* Close leaves the tab on nothing; the next tab opens elsewhere.
Close(t) ==
  /\ "close" \in Shown(t)
  /\ UNCHANGED vars

Next ==
  \/ \E t \in Tabs : Press(t) \/ Commit(t) \/ Discard(t) \/ Reload(t)
  \/ \E t \in Tabs : Close(t)
  \/ OrphanEnds
  \/ MoveIndex

S == INSTANCE Surface WITH Actions <- Buttons, Next <- Hint
EveryShownActionWorks == S!EveryShownActionWorks
NobodyIsStuck == S!NobodyIsStuck

\* The wizard exists exactly until its one object does.
WizardXorObject == wizard <=> objects = 0
AtMostOneObject == objects <= 1
\* The index never names an object that is gone.
IndexNamesLive ==
  /\ index = "wizard" => wizard
  /\ index = "object" => objects = 1
\* Once Finish made the object, an index that named the wizard names it,
\* unless another writer moved it.
FinishedIndexNamesObject == objects = 1 /\ ~moved => index # "wizard"
\* A moved index stays where its writer put it.
MovedIndexStays == moved => index = "other"
\* A tab says done only once the object exists.
DoneMeansMade == \A t \in Tabs : control[t] = "done" => objects = 1
TypeOK ==
  /\ control \in [Tabs -> Controls]
  /\ live \in [Tabs -> BOOLEAN]
  /\ read \in [Tabs -> Indexes]
  /\ orphan \in {"none"} \cup Indexes
  /\ wizard \in BOOLEAN
  /\ objects \in 0..2
  /\ index \in Indexes
  /\ moved \in BOOLEAN
\* A tab waits on a list exactly while its control says finishing.
LiveOnlyFinishing == \A t \in Tabs : live[t] <=> control[t] = "finishing"
====
