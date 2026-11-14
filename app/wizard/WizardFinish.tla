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
\* opens the object; gone says the wizard was finished or cancelled elsewhere
\* and offers to close the tab. A list refused because the wizard is gone
\* goes to gone directly, and a tab that sees the wizard gone shows gone.
\*
\* Another writer may move the index off the wizard at any time. The index
\* move always goes in the list and expects the wizard, so the World decides
\* inside the commit: it moves an index that names the wizard and leaves a
\* moved one alone. The tab's settings watch may not have loaded, or may lag,
\* so what the tab saw when it pressed decides nothing. A reload drops the
\* tab's control back to idle while its list may still commit or discard on
\* the server.

Tabs == {"a", "b"}
People == Tabs

\* finish starts the first press; retry presses again after a failure;
\* close closes a tab whose wizard is gone.
Buttons == {"finish", "retry", "close"}

Controls == {"idle", "finishing", "failed", "done", "gone"}

\* The Space index names the wizard, the object Finish made, or something
\* another writer chose.
Indexes == {"wizard", "object", "other"}

\* What a tab's settings watch shows: an index, or nothing before it loads.
Seen == {"unloaded"} \cup Indexes

VARIABLES
  control, \* each tab's Finish control
  live,    \* whether the tab's control waits on a list in flight
  seen,    \* the index the tabs' settings watch shows, which may lag;
           \* one watch for both tabs shows the lag with fewer states
  orphan,  \* whether a list is still in flight from a tab that reloaded
  wizard,  \* whether the wizard object exists
  objects, \* how many objects Finish made
  index,   \* what the Space index names
  moved    \* history: another writer moved the index
vars == <<control, live, seen, orphan, wizard, objects, index, moved>>

Gone(t) ==
  \/ control[t] = "gone"
  \/ ~wizard /\ control[t] \in {"idle", "failed"}

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
  /\ seen = "unloaded"
  /\ orphan = FALSE
  /\ wizard = TRUE
  /\ objects = 0
  /\ index \in {"wizard", "other"}
  /\ moved = FALSE

\* The settings watch catches up with the World's index. Until it does it
\* lags every write, which is the case the list must survive.
Watch ==
  /\ seen' = index
  /\ UNCHANGED <<control, live, orphan, wizard, objects, index, moved>>

\* Finish or Retry prepares the list and hands it to the executor. The list
\* reads nothing from the tab's settings watch.
Press(t) ==
  /\ Shown(t) \cap {"finish", "retry"} # {}
  /\ control' = [control EXCEPT ![t] = "finishing"]
  /\ live' = [live EXCEPT ![t] = TRUE]
  /\ UNCHANGED <<seen, orphan, wizard, objects, index, moved>>

\* Apply is the executor's one atomic step: the creates, the index move and
\* the delete commit together. The index move reads the World's index in the
\* commit, not r, what the tab's watch shows. A list that creates refuses itself when
\* the wizard is already gone, so a second tab makes no second object.
Apply(r) ==
  IF wizard
  THEN /\ objects' = objects + 1
       /\ wizard' = FALSE
       /\ index' = IF index = "wizard" THEN "object" ELSE index
  ELSE UNCHANGED <<objects, wizard, index>>

\* The tab's own list commits, or is refused because the wizard is gone, and
\* the control shows which.
Commit(t) ==
  /\ live[t]
  /\ Apply(seen)
  /\ control' = [control EXCEPT ![t] = IF wizard THEN "done" ELSE "gone"]
  /\ live' = [live EXCEPT ![t] = FALSE]
  /\ UNCHANGED <<seen, orphan, moved>>

\* A write in the list throws, or the tab closes, and the executor discards
\* the list: nothing it staged lands.
Discard(t) ==
  /\ live[t]
  /\ control' = [control EXCEPT ![t] = "failed"]
  /\ live' = [live EXCEPT ![t] = FALSE]
  /\ UNCHANGED <<seen, orphan, wizard, objects, index, moved>>

\* The tab reloads; a list in flight carries on without it.
Reload(t) ==
  /\ control' = [control EXCEPT ![t] = "idle"]
  /\ live' = [live EXCEPT ![t] = FALSE]
  /\ ~(live[t] /\ orphan)
  /\ orphan' = (orphan \/ live[t])
  /\ UNCHANGED <<seen, wizard, objects, index, moved>>

\* A list whose tab reloaded commits, is refused or discards on the server,
\* the same way.
OrphanEnds ==
  /\ orphan
  /\ orphan' = FALSE
  /\ \/ Apply(seen)
     \/ UNCHANGED <<wizard, objects, index>>
  /\ UNCHANGED <<control, live, seen, moved>>

\* Another writer points the index somewhere else.
MoveIndex ==
  /\ index # "other"
  /\ index' = "other"
  /\ moved' = TRUE
  /\ UNCHANGED <<control, live, seen, orphan, wizard, objects>>

\* Close leaves the tab on nothing; the next tab opens elsewhere.
Close(t) ==
  /\ "close" \in Shown(t)
  /\ UNCHANGED vars

Next ==
  \/ \E t \in Tabs : Press(t) \/ Commit(t) \/ Discard(t) \/ Reload(t)
  \/ \E t \in Tabs : Close(t)
  \/ Watch
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
  /\ seen \in Seen
  /\ orphan \in BOOLEAN
  /\ wizard \in BOOLEAN
  /\ objects \in 0..2
  /\ index \in Indexes
  /\ moved \in BOOLEAN
\* A tab waits on a list exactly while its control says finishing.
LiveOnlyFinishing == \A t \in Tabs : live[t] <=> control[t] = "finishing"
====
