---- MODULE ViewerBar ----
\* The one-line bar over an object viewer: its title, at most a few presses,
\* and More for the rest. The pane's width decides which presses sit in the bar
\* and which fold into More, through the @pane-* container variants; More lists
\* exactly the presses folded, read when it opens and on each resize or change
\* of the presses while open, and closes when nothing is folded. Below 28rem the bar's muted
\* description folds too, and More shows it above the presses. The title never
\* folds.
\*
\* A disabled press is one whose action is running; its label says so and the
\* viewer, not the bar, holds that state. It stays in the bar and focusable
\* (aria-disabled, its click ignored), so a running press is not a state here. A press that cannot work in the
\* object's state is left out of the bar, so More never lists a dead end.
\*
\* Keyboard: every press in the bar and More are buttons in tab order, and a
\* folded press is display: none, so Tab skips it. More opens on Enter, Space
\* or ArrowDown; the arrows move through its items, Enter picks one, and
\* Escape closes the menu and returns focus to More. When More closes because
\* its press left the bar, focus goes to the first shown press, or the bar.
\* A press that folds or leaves while it holds focus, with More closed, is left
\* to the browser and modeled as focus going away.
\*
\* A narrow press, such as Pages beside a side column, shows only below 42rem,
\* where the column it opens is gone. From 42rem it is not folded but gone:
\* More never lists it, as Details' narrow press never folds into More. The
\* xs press is the narrow one here; a press that is not narrow behaves at
\* 42rem as at 28rem. ViewerBarChoice.tla holds the bar's choice.

People == {"person"}
Presses == {"a", "b", "c"}
\* The narrow presses, shown only below 42rem.
Narrow == {"a"}

\* The pane's width band: cramped is under 20rem, narrow 20rem to 28rem, wide
\* 28rem to 42rem, full 42rem and up. A bar mounted outside any pane matches
\* no variant, which folds the same presses as cramped.
Widths == {"cramped", "narrow", "wide", "full"}
Wide(w) == w \in {"wide", "full"}

Buttons == Presses \cup {"more", "dismiss"}

\* Where focus is: a button, the open menu, the bar itself, or away.
Focuses == Buttons \cup {"menu", "bar", "away"}

VARIABLES
  width,     \* the pane's width band; the person resizes it at any time
  fold,      \* each press's fold point, fixed by the viewer; one press per
             \* point, so the subsets of present cover every mix of points
  present,   \* the presses the viewer gives now; it changes them at any time
  described, \* whether the viewer gives a description, fixed by the viewer
  menu,      \* "closed" or "open"
  listed,    \* the presses More lists
  noted,     \* whether More shows the description
  focus      \* where keyboard focus is
vars == <<width, fold, present, described, menu, listed, noted, focus>>

\* The narrow presses gone at width w; gone is neither in the bar nor folded.
Gone(w) == IF w = "full" THEN Narrow ELSE {}
\* The presses of s the pane hides at width w, folded or gone.
HiddenIn(s, w) ==
  {a \in s :
    \/ fold[a] = "menu"
    \/ fold[a] = "md" /\ ~Wide(w)
    \/ fold[a] = "xs" /\ w = "cramped"
    \/ a \in Gone(w)}
FoldedIn(s, w) == HiddenIn(s, w) \ Gone(w)
Folded(w) == FoldedIn(present, w)
Inline(w) == present \ HiddenIn(present, w)
NoteFolded(w) == described /\ ~Wide(w)
FoldsIn(s, w) == FoldedIn(s, w) # {} \/ NoteFolded(w)
Folds(w) == FoldsIn(present, w)

\* Where focus goes when More closes with its press gone.
Fallback(s, w) ==
  IF s \ HiddenIn(s, w) # {} THEN CHOOSE a \in s \ HiddenIn(s, w) : TRUE ELSE "bar"
\* Focus after a closed-bar change: kept while its button is still shown.
Kept(s, w) ==
  IF focus \in Buttons /\ focus \notin (s \ HiddenIn(s, w)) \cup (IF FoldsIn(s, w) THEN {"more"} ELSE {})
  THEN "away"
  ELSE focus

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
  \* A press folds at "xs" below 20rem, "md" below 28rem, "menu" always.
  /\ fold = [a |-> "xs", b |-> "md", c |-> "menu"]
  /\ present \in SUBSET Presses
  /\ described \in BOOLEAN
  /\ menu = "closed"
  /\ listed = {}
  /\ noted = FALSE
  /\ focus = "away"

\* The bar settles on presses s at width w. An open menu rereads what is
\* folded, and closes once nothing is, since its trigger has left the bar;
\* focus then goes to the first shown press, or the bar.
Settle(s, w) ==
  IF menu = "open"
  THEN IF FoldsIn(s, w)
       THEN /\ listed' = FoldedIn(s, w) /\ noted' = NoteFolded(w)
            /\ UNCHANGED <<menu, focus>>
       ELSE /\ listed' = {} /\ noted' = FALSE /\ menu' = "closed"
            /\ focus' = Fallback(s, w)
  ELSE /\ focus' = Kept(s, w) /\ UNCHANGED <<menu, listed, noted>>

\* The person drags a grid edge.
Resize ==
  /\ \E w \in Widths : w # width /\ width' = w /\ Settle(present, w)
  /\ UNCHANGED <<fold, present, described>>
\* The viewer adds or removes presses, such as when the object's state changes.
Change ==
  /\ \E s \in SUBSET Presses : s # present /\ present' = s /\ Settle(s, width)
  /\ UNCHANGED <<width, fold, described>>
Focus(p) ==
  /\ menu = "closed"
  /\ \E b \in Shown(p) \cup {"bar"} : b # focus /\ focus' = b
  /\ UNCHANGED <<width, fold, present, described, menu, listed, noted>>
Open(p) ==
  /\ menu = "closed" /\ "more" \in Shown(p)
  /\ menu' = "open" /\ listed' = Folded(width) /\ noted' = NoteFolded(width)
  /\ focus' = "menu"
  /\ UNCHANGED <<width, fold, present, described>>
PressInline(p) ==
  /\ menu = "closed"
  /\ \E a \in Inline(width) : a \in Shown(p) /\ focus' = a
  /\ UNCHANGED <<width, fold, present, described, menu, listed, noted>>
PickListed(p) ==
  /\ menu = "open"
  /\ \E a \in listed : a \in Shown(p)
  /\ menu' = "closed" /\ listed' = {} /\ noted' = FALSE /\ focus' = "more"
  /\ UNCHANGED <<width, fold, present, described>>
Dismiss(p) ==
  /\ "dismiss" \in Shown(p)
  /\ menu' = "closed" /\ listed' = {} /\ noted' = FALSE /\ focus' = "more"
  /\ UNCHANGED <<width, fold, present, described>>

Next ==
  \/ Resize
  \/ Change
  \/ \E p \in People :
       Open(p) \/ PressInline(p) \/ PickListed(p) \/ Dismiss(p) \/ Focus(p)

S == INSTANCE Surface WITH Actions <- Buttons, Next <- Hint
EveryShownActionWorks == S!EveryShownActionWorks
NobodyIsStuck == S!NobodyIsStuck

\* Every press is in the bar or one More press away, at every width, except a
\* narrow press from 42rem, where the side column it opens shows instead.
EveryPressReachable ==
  menu = "closed" =>
    \A a \in present \ Gone(width) :
      a \in Shown("person") \/ ("more" \in Shown("person") /\ a \in Folded(width))
\* The description is in the bar or one More press away.
DescriptionReachable ==
  menu = "closed" /\ described =>
    Wide(width) \/ "more" \in Shown("person")
\* More is never open empty.
MenuNeverEmpty == menu = "open" => listed # {} \/ noted
\* An open More lists exactly what the pane folds now.
MenuMatchesPane ==
  menu = "open" => listed = Folded(width) /\ noted = NoteFolded(width)
\* A closed More holds no stale list; it reads the folded presses on each open.
ClosedMenuListsNothing == menu = "closed" => listed = {} /\ ~noted
\* A wider pane folds no press that a narrower one shows.
WiderShowsMore ==
  /\ Folded("full") \subseteq Folded("wide")
  /\ Folded("wide") \subseteq Folded("narrow")
  /\ Folded("narrow") \subseteq Folded("cramped")
\* More never lists a narrow press the pane removed.
GoneNeverListed == listed \cap Gone(width) = {}
\* Focus sits on a shown button, the open menu, the bar, or away; never on a
\* press or More the bar hides.
FocusShown ==
  /\ focus \in Focuses
  /\ menu = "open" => focus = "menu"
  /\ menu = "closed" /\ focus \in Buttons => focus \in Shown("person")
====
