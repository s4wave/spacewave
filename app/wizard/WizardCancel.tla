---- MODULE WizardCancel ----
EXTENDS Naturals

\* A wizard's Delete control racing its Finish control, in each tab that
\* shows the wizard. WizardFinish.tla models Finish alone, with the lagging
\* settings watch; this spec keeps Finish's atomic list and adds Cancel, so
\* it leaves the watch out and has no chart, since the two together pass the
\* chart's state budget.
\*
\* Delete prepares one list, the Space index move and the wizard delete,
\* and the executor commits or discards it whole, as it does Finish's. Both
\* lists expect the wizard to exist: one refused because the wizard is gone
\* writes nothing and ends in the shared gone screen, so a Cancel and a
\* Finish, or two Cancels, never both report that they ended the wizard.
\*
\* A wizard's Finish may create an object, or create nothing, as an
\* introduction does; Creates picks which, with a config for each. Every
\* Finish expects the wizard, so one that creates nothing also ends in gone
\* after a Cancel rather than reporting done for a list that did nothing.
\*
\* A tab's cancel control: idle offers Delete; cancelling waits on the list
\* and offers nothing more, so one press makes at most one delete;
\* cancelled means this press deleted the wizard and the tab leaves; gone
\* means another writer removed it first; failed keeps the wizard, says why,
\* and offers Delete again. Delete is not offered while the tab's own Finish
\* runs or is done; a Finish in another tab is covered by the refusal.

\* Creates is whether this wizard's Finish creates an object.
CONSTANT Creates

Tabs == {"a", "b"}
People == Tabs

\* finish starts the first press; retry presses again after a failure;
\* delete cancels the wizard; close closes a tab whose wizard is gone.
Buttons == {"finish", "retry", "delete", "close"}

Controls == {"idle", "finishing", "failed", "done", "gone"}
Cancels == {"idle", "cancelling", "cancelled", "gone", "failed"}

\* The Space index names the wizard, the object Finish made, nothing after a
\* Cancel cleared it, or something another writer chose.
Indexes == {"wizard", "object", "cleared", "other"}

VARIABLES
  control, \* each tab's Finish control
  live,    \* whether the tab's Finish control waits on a list in flight
  orphan,  \* whether a Finish list is in flight from a tab that reloaded
  cancel,  \* each tab's Delete control
  clive,   \* how many of the tab's Delete lists are in flight
  corphan, \* whether a Delete list is in flight from a tab that reloaded
  wizard,  \* whether the wizard object exists
  objects, \* how many Finishes ended the wizard, making its object if any
  cancels, \* how many Delete lists deleted the wizard
  index    \* what the Space index names
vars == <<control, live, orphan, cancel, clive, corphan, wizard, objects,
          cancels, index>>

\* The tab learns the wizard is gone while no Delete of its own removes it.
\* The model's watch sees the World at once, so a refused Delete, which
\* ends in gone, shows the gone screen through it; the code also goes to
\* gone on the refusal itself, for a watch that lags.
WizardGone(t) == ~wizard /\ cancel[t] \notin {"cancelling", "cancelled"}

\* The tab shows the gone screen: its Finish was refused, or the wizard is
\* gone while no Finish of its own removes it.
Gone(t) ==
  \/ control[t] = "gone"
  \/ WizardGone(t) /\ control[t] \notin {"finishing", "done"}

\* A tab whose Delete landed leaves the wizard and shows nothing more.
Shown(t) ==
  IF cancel[t] = "cancelled" THEN {} ELSE
    (IF control[t] = "idle" /\ wizard THEN {"finish"} ELSE {})
      \cup (IF control[t] = "failed" /\ wizard THEN {"retry"} ELSE {})
      \cup (IF /\ control[t] \notin {"finishing", "done"}
               /\ cancel[t] \in {"idle", "failed"}
               /\ wizard
            THEN {"delete"} ELSE {})
      \cup (IF Gone(t) THEN {"close"} ELSE {})

\* A press works only while the wizard exists and the control waits on
\* nothing.
Allowed(t, b) ==
  \/ b = "finish" /\ control[t] = "idle" /\ wizard /\ ~live[t]
  \/ b = "retry" /\ control[t] = "failed" /\ wizard /\ ~live[t]
  \/ /\ b = "delete"
     /\ control[t] \notin {"finishing", "done"}
     /\ cancel[t] \in {"idle", "failed"}
     /\ wizard
     /\ clive[t] = 0
  \/ b = "close" /\ Gone(t)

\* What a tab tells the person beside its presses.
Hint(t) ==
  CASE cancel[t] = "cancelled" -> "Leaves the wizard"
    [] control[t] = "finishing" -> "Says it is finishing"
    [] control[t] = "done" -> "Opens the object"
    [] cancel[t] = "cancelling" -> "Says it is deleting"
    [] control[t] = "failed" /\ wizard -> "Says what failed"
    [] cancel[t] = "failed" /\ wizard -> "Says the delete failed"
    [] OTHER -> ""

Init ==
  /\ control = [t \in Tabs |-> "idle"]
  /\ live = [t \in Tabs |-> FALSE]
  /\ orphan = FALSE
  /\ cancel = [t \in Tabs |-> "idle"]
  /\ clive = [t \in Tabs |-> 0]
  /\ corphan = FALSE
  /\ wizard = TRUE
  /\ objects = 0
  /\ cancels = 0
  /\ index \in {"wizard", "other"}

\* Every Finish expects the wizard to exist, whether or not it creates.
MustExist == TRUE

\* Finish's list commits atomically, or is refused when the wizard is gone,
\* whether or not it creates.
FinishApply ==
  IF wizard
  THEN /\ objects' = objects + 1 \* the object, when Creates
       /\ wizard' = FALSE
       /\ index' = IF index = "wizard" THEN "object" ELSE index
  ELSE UNCHANGED <<objects, wizard, index>>

\* Delete's list commits atomically, or is refused when the wizard is gone.
CancelApply ==
  IF wizard
  THEN /\ cancels' = cancels + 1
       /\ wizard' = FALSE
       /\ index' = IF index = "wizard" THEN "cleared" ELSE index
  ELSE UNCHANGED <<cancels, wizard, index>>

Press(t) ==
  /\ Shown(t) \cap {"finish", "retry"} # {}
  /\ control' = [control EXCEPT ![t] = "finishing"]
  /\ live' = [live EXCEPT ![t] = TRUE]
  /\ UNCHANGED <<orphan, cancel, clive, corphan, wizard, objects, cancels,
                 index>>

Commit(t) ==
  /\ live[t]
  /\ FinishApply
  /\ control' = [control EXCEPT ![t] =
                   IF wizard \/ ~MustExist THEN "done" ELSE "gone"]
  /\ live' = [live EXCEPT ![t] = FALSE]
  /\ UNCHANGED <<orphan, cancel, clive, corphan, cancels>>

Discard(t) ==
  /\ live[t]
  /\ control' = [control EXCEPT ![t] = "failed"]
  /\ live' = [live EXCEPT ![t] = FALSE]
  /\ UNCHANGED <<orphan, cancel, clive, corphan, wizard, objects, cancels,
                 index>>

\* Delete hands its list to the executor.
CancelPress(t) ==
  /\ "delete" \in Shown(t)
  /\ cancel' = [cancel EXCEPT ![t] = "cancelling"]
  /\ clive' = [clive EXCEPT ![t] = clive[t] + 1]
  /\ UNCHANGED <<control, live, orphan, corphan, wizard, objects, cancels,
                 index>>

\* The tab's Delete list commits, or is refused because the wizard is gone,
\* and the control shows which.
CancelCommit(t) ==
  /\ clive[t] > 0
  /\ CancelApply
  /\ cancel' = [cancel EXCEPT ![t] = IF wizard THEN "cancelled" ELSE "gone"]
  /\ clive' = [clive EXCEPT ![t] = clive[t] - 1]
  /\ UNCHANGED <<control, live, orphan, corphan, objects>>

\* A write in the Delete list throws and the executor discards it.
CancelDiscard(t) ==
  /\ clive[t] > 0
  /\ cancel' = [cancel EXCEPT ![t] = "failed"]
  /\ clive' = [clive EXCEPT ![t] = clive[t] - 1]
  /\ UNCHANGED <<control, live, orphan, corphan, wizard, objects, cancels,
                 index>>

\* The tab reloads; a list in flight carries on without it.
Reload(t) ==
  /\ ~(live[t] /\ orphan)
  /\ ~(clive[t] > 0 /\ corphan)
  /\ control' = [control EXCEPT ![t] = "idle"]
  /\ live' = [live EXCEPT ![t] = FALSE]
  /\ orphan' = (orphan \/ live[t])
  /\ cancel' = [cancel EXCEPT ![t] = "idle"]
  /\ clive' = [clive EXCEPT ![t] = 0]
  /\ corphan' = (corphan \/ clive[t] > 0)
  /\ UNCHANGED <<wizard, objects, cancels, index>>

\* A list whose tab reloaded commits, is refused or discards on the server.
OrphanEnds ==
  /\ orphan
  /\ orphan' = FALSE
  /\ \/ FinishApply
     \/ UNCHANGED <<wizard, objects, index>>
  /\ UNCHANGED <<control, live, cancel, clive, corphan, cancels>>

CancelOrphanEnds ==
  /\ corphan
  /\ corphan' = FALSE
  /\ \/ CancelApply
     \/ UNCHANGED <<wizard, cancels, index>>
  /\ UNCHANGED <<control, live, orphan, cancel, clive, objects>>

\* Another writer points the index somewhere else.
MoveIndex ==
  /\ index # "other"
  /\ index' = "other"
  /\ UNCHANGED <<control, live, orphan, cancel, clive, corphan, wizard,
                 objects, cancels>>

\* Close leaves the tab on nothing; the next tab opens elsewhere.
Close(t) ==
  /\ "close" \in Shown(t)
  /\ UNCHANGED vars

Next ==
  \/ \E t \in Tabs :
       \/ Press(t) \/ Commit(t) \/ Discard(t)
       \/ CancelPress(t) \/ CancelCommit(t) \/ CancelDiscard(t)
       \/ Reload(t) \/ Close(t)
  \/ OrphanEnds
  \/ CancelOrphanEnds
  \/ MoveIndex

S == INSTANCE Surface WITH Actions <- Buttons, Next <- Hint
EveryShownActionWorks == S!EveryShownActionWorks
NobodyIsStuck == S!NobodyIsStuck

\* A Cancel and a Finish never both report that they ended the wizard.
NotBothWin ==
  ~(/\ \E t \in Tabs : control[t] = "done"
    /\ \E u \in Tabs : cancel[u] = "cancelled")
\* Two Cancels never both report that they deleted the wizard.
OneCancelWins ==
  \A t, u \in Tabs : cancel[t] = "cancelled" /\ cancel[u] = "cancelled" => t = u
\* The wizard ends exactly once: it exists, or one Finish ended it, or one
\* Delete removed it.
ExactlyOneEnding == (IF wizard THEN 1 ELSE 0) + objects + cancels = 1
\* One press of Delete sends at most one delete.
OneDeletePerPress == \A t \in Tabs : clive[t] <= 1
\* A tab says cancelled only once a Delete removed the wizard.
CancelledMeansDeleted ==
  \A t \in Tabs : cancel[t] = "cancelled" => cancels = 1
\* A failed Delete keeps the wizard and offers Delete again.
FailedCancelOffersDelete ==
  \A t \in Tabs :
    cancel[t] = "failed" /\ wizard /\ control[t] \notin {"finishing", "done"}
      => "delete" \in Shown(t)
\* The index never names an object that is gone.
IndexNamesLive ==
  /\ index = "wizard" => wizard
  /\ index = "object" => objects = 1
TypeOK ==
  /\ control \in [Tabs -> Controls]
  /\ live \in [Tabs -> BOOLEAN]
  /\ orphan \in BOOLEAN
  /\ cancel \in [Tabs -> Cancels]
  /\ clive \in [Tabs -> 0..2]
  /\ corphan \in BOOLEAN
  /\ wizard \in BOOLEAN
  /\ objects \in 0..2
  /\ cancels \in 0..2
  /\ index \in Indexes
\* A tab waits on a list exactly while its control says so.
LiveOnlyFinishing == \A t \in Tabs : live[t] <=> control[t] = "finishing"
LiveOnlyCancelling == \A t \in Tabs : clive[t] > 0 <=> cancel[t] = "cancelling"
====
