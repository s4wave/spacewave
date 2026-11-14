---- MODULE ViewerBar ----
\* The one-line bar over an object viewer: its title, at most a few presses,
\* and More for the rest. The pane's width decides which presses sit in the bar
\* and which fold into More, through the @pane-* container variants; More lists
\* exactly the presses folded, read when it opens and on each resize while
\* open, and closes when nothing is folded. Below 28rem the bar's muted
\* description folds too, and More shows it above the presses. The title never
\* folds.
\*
\* A disabled press is one whose action is running; its label says so and the
\* viewer, not the bar, holds that state. A press that cannot work in the
\* object's state is left out of the bar, so More never lists a dead end.
\*
\* Keyboard: every press in the bar and More are buttons in tab order, and a
\* folded press is display: none, so Tab skips it. More opens on Enter, Space
\* or ArrowDown; the arrows move through its items, Enter picks one, and
\* Escape closes the menu and returns focus to More.

People == {"person"}
Presses == {"a", "b", "c"}

\* The pane's width band: cramped is under 20rem, narrow 20rem to 28rem, wide
\* 28rem and up. A bar mounted outside any pane matches no variant, which
\* folds the same presses as cramped.
Widths == {"cramped", "narrow", "wide"}

\* Where a press folds: "xs" below 20rem, "md" below 28rem, "menu" always.
FoldPoints == {"xs", "md", "menu"}

Buttons == Presses \cup {"more", "dismiss"}

VARIABLES
  width,     \* the pane's width band; the person resizes it at any time
  fold,      \* each press's fold point, fixed by the viewer
  described, \* whether the viewer gives a description, fixed by the viewer
  menu,      \* "closed" or "open"
  listed,    \* the presses More lists
  noted      \* whether More shows the description
vars == <<width, fold, described, menu, listed, noted>>

Folded(w) ==
  {a \in Presses :
    \/ fold[a] = "menu"
    \/ fold[a] = "md" /\ w # "wide"
    \/ fold[a] = "xs" /\ w = "cramped"}
Inline(w) == Presses \ Folded(w)
NoteFolded(w) == described /\ w # "wide"
Folds(w) == Folded(w) # {} \/ NoteFolded(w)

\* Closed: the bar's presses, and More while any press is folded.
\* Open: the presses More lists, and Escape or a click away.
\* With no press at all the bar is only the object's title.
Shown(p) ==
  IF menu = "open"
  THEN listed \cup {"dismiss"}
  ELSE Inline(width) \cup (IF Folds(width) THEN {"more"} ELSE {})

\* A press runs its action wherever it sits; More and dismiss always work.
Allowed(p, b) == b \in Buttons

Hint(p) == IF Shown(p) = {} THEN "The bar names the object" ELSE ""

Init ==
  /\ width \in Widths
  /\ fold \in [Presses -> FoldPoints]
  /\ described \in BOOLEAN
  /\ menu = "closed"
  /\ listed = {}
  /\ noted = FALSE

\* The person drags a grid edge. An open menu rereads what is folded, and
\* closes once nothing is, since its trigger has left the bar.
Resize ==
  /\ \E w \in Widths :
       /\ w # width /\ width' = w
       /\ IF menu = "open" /\ Folds(w)
          THEN listed' = Folded(w) /\ noted' = NoteFolded(w) /\ menu' = menu
          ELSE listed' = {} /\ noted' = FALSE /\ menu' = "closed"
  /\ UNCHANGED <<fold, described>>
Open(p) ==
  /\ menu = "closed" /\ "more" \in Shown(p)
  /\ menu' = "open" /\ listed' = Folded(width) /\ noted' = NoteFolded(width)
  /\ UNCHANGED <<width, fold, described>>
PressInline(p) ==
  /\ menu = "closed"
  /\ \E a \in Inline(width) : a \in Shown(p)
  /\ UNCHANGED vars
PickListed(p) ==
  /\ menu = "open"
  /\ \E a \in listed : a \in Shown(p)
  /\ menu' = "closed" /\ listed' = {} /\ noted' = FALSE
  /\ UNCHANGED <<width, fold, described>>
Dismiss(p) ==
  /\ "dismiss" \in Shown(p)
  /\ menu' = "closed" /\ listed' = {} /\ noted' = FALSE
  /\ UNCHANGED <<width, fold, described>>

Next ==
  \/ Resize
  \/ \E p \in People : Open(p) \/ PressInline(p) \/ PickListed(p) \/ Dismiss(p)

S == INSTANCE Surface WITH Actions <- Buttons, Next <- Hint
EveryShownActionWorks == S!EveryShownActionWorks
NobodyIsStuck == S!NobodyIsStuck

\* Every press is in the bar or one More press away, at every width.
EveryPressReachable ==
  menu = "closed" =>
    \A a \in Presses :
      a \in Shown("person") \/ ("more" \in Shown("person") /\ a \in Folded(width))
\* The description is in the bar or one More press away.
DescriptionReachable ==
  menu = "closed" /\ described =>
    width = "wide" \/ "more" \in Shown("person")
\* More is never open empty.
MenuNeverEmpty == menu = "open" => listed # {} \/ noted
\* An open More lists exactly what the pane folds now.
MenuMatchesPane ==
  menu = "open" => listed = Folded(width) /\ noted = NoteFolded(width)
\* A closed More holds no stale list; it reads the folded presses on each open.
ClosedMenuListsNothing == menu = "closed" => listed = {} /\ ~noted
\* A wider pane folds no press that a narrower one shows.
WiderShowsMore == Folded("wide") \subseteq Folded("narrow") /\ Folded("narrow") \subseteq Folded("cramped")
====
