---- MODULE ViewerSide ----
\* The side column of an object viewer and the bar's Details popover: two
\* places for the same facts, picked by the pane's width. From a 42rem pane
\* the column sits at the pane's left edge, open with the facts or collapsed
\* to a rail of state glyphs; below 42rem it is gone and the bar's Details
\* press opens the facts in a popover. The column's open or rail state
\* outlives a narrow spell, and a popover open when the pane widens past
\* 42rem closes, since its press has left the bar and the column shows the
\* facts.
\*
\* Keyboard: Collapse, Expand and Details are buttons in tab order. Details
\* opens on Enter or Space and moves focus into the popover; Tab walks its
\* copy presses, and Escape or a click away closes it and returns focus to
\* Details. A popover that closes because the pane widened past 42rem has no
\* Details to return to, so focus goes to the column's Collapse or Expand
\* press. Collapse and Expand replace each other and focus follows the press.
\* A press that leaves on a resize while it holds focus, with the popover
\* closed, is left to the browser and modeled as focus going away. The list
\* beside the column keeps its own keys.

People == {"person"}

\* The pane's width band: narrow is under 42rem, wide 42rem and up. A viewer
\* outside any pane matches no variant, which is the same as narrow.
Widths == {"narrow", "wide"}

Buttons == {"collapse", "expand", "details", "dismiss", "copy"}

\* Where focus is: a button, the bar itself, or away.
Focuses == Buttons \cup {"bar", "away"}

VARIABLES
  width, \* the pane's width band; the person resizes it at any time
  side,  \* the column: "open" with its facts or a "rail" of glyphs
  pop,   \* the Details popover: "closed" or "open"
  focus  \* where keyboard focus is
vars == <<width, side, pop, focus>>

\* Open popover: its facts, each with its copy press, and a way out.
\* Wide pane: the open column's facts and Collapse, or the rail's Expand.
\* Narrow pane: the bar's Details press.
ShownAt(w, s, o) ==
  IF o = "open" THEN {"copy", "dismiss"}
  ELSE IF w = "wide"
    THEN IF s = "open" THEN {"copy", "collapse"} ELSE {"expand"}
  ELSE {"details"}
Shown(p) == ShownAt(width, side, pop)

Allowed(p, b) == b \in Buttons

Hint(p) == ""

Init ==
  /\ width \in Widths
  /\ side = "open"
  /\ pop = "closed"
  /\ focus = "away"

\* The column's press: Collapse on the open column, Expand on the rail.
SidePress(s) == IF s = "open" THEN "collapse" ELSE "expand"

\* The person drags a grid edge. Widening past 42rem closes the popover and
\* sends focus to the column's press; a focused press that leaves otherwise
\* drops focus.
Resize ==
  /\ \E w \in Widths :
       /\ w # width /\ width' = w
       /\ IF w = "wide" /\ pop = "open"
            THEN /\ pop' = "closed"
                 /\ focus' = SidePress(side)
            ELSE /\ pop' = pop
                 /\ focus' = IF focus \in Buttons /\ focus \notin ShownAt(w, side, pop)
                              THEN "away" ELSE focus
  /\ UNCHANGED side

Collapse(p) ==
  /\ "collapse" \in Shown(p)
  /\ side' = "rail"
  /\ focus' = "expand"
  /\ UNCHANGED <<width, pop>>

Expand(p) ==
  /\ "expand" \in Shown(p)
  /\ side' = "open"
  /\ focus' = "collapse"
  /\ UNCHANGED <<width, pop>>

OpenDetails(p) ==
  /\ "details" \in Shown(p)
  /\ pop' = "open"
  /\ focus' = "copy"
  /\ UNCHANGED <<width, side>>

Dismiss(p) ==
  /\ "dismiss" \in Shown(p)
  /\ pop' = "closed"
  /\ focus' = "details"
  /\ UNCHANGED <<width, side>>

\* Copy takes the whole value and leaves the layout as it was.
Copy(p) ==
  /\ "copy" \in Shown(p)
  /\ UNCHANGED vars

\* Tab moves focus to another shown press, or the bar while no popover is
\* open.
Move(p) ==
  /\ \E b \in Shown(p) \cup (IF pop = "open" THEN {} ELSE {"bar"}) :
       b # focus /\ focus' = b
  /\ UNCHANGED <<width, side, pop>>

Next ==
  \/ Resize
  \/ \E p \in People :
       Collapse(p) \/ Expand(p) \/ OpenDetails(p) \/ Dismiss(p) \/ Copy(p)
       \/ Move(p)

S == INSTANCE Surface WITH Actions <- Buttons, Next <- Hint
EveryShownActionWorks == S!EveryShownActionWorks
NobodyIsStuck == S!NobodyIsStuck

\* The facts show, or are one press away, at every width and column state.
FactsReachable ==
  LET sh == Shown("person") IN
    "copy" \in sh \/ "expand" \in sh \/ "details" \in sh
\* The popover opens only where the column is gone.
PopoverOnlyNarrow == pop = "open" => width = "narrow"
\* The facts are in one place at a time: the column or the popover.
FactsInOnePlace == ~(pop = "open" /\ width = "wide" /\ side = "open")
\* Focus is on a shown press, the bar, or away, and inside an open popover.
FocusShown ==
  /\ focus \in Focuses
  /\ focus \in Buttons => focus \in Shown("person")
  /\ pop = "open" => focus \in {"copy", "dismiss"}
====
