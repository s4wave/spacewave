---- MODULE ViewerBarChoice ----
\* The viewer bar's choice: a group of exclusive options, such as a
\* Dashboard's time range. From a 28rem pane its options sit in the bar with
\* the current one pressed; below 28rem one press names the current option
\* and opens a menu of them. The choice never folds into More, so it is in the
\* bar at every width, as one press or as the group.
\*
\* Keyboard: the options and the press are buttons in tab order. The press
\* opens its menu on Enter, Space or ArrowDown; the arrows move through the
\* options, Enter picks one, which closes the menu and returns focus to the
\* press, now naming it. Escape closes the menu and returns focus to the press.
\* A pane that widens past 28rem while the menu is open closes it and sends
\* focus to the current option in the group, since the press is gone. A pane
\* that narrows while an option holds focus is left to the browser and modeled
\* as focus going away.

People == {"person"}
Options == {"o1", "o2"}

\* The pane's width band: narrow under 28rem, wide from 28rem.
Widths == {"narrow", "wide"}

\* "press" is the one press below 28rem; "pick" is an option, in the group or
\* in the open menu; "dismiss" is Escape or a click away.
Buttons == {"press", "pick", "dismiss"}
Focuses == Buttons \cup {"menu", "away"}

VARIABLES
  width, \* the pane's width band; the person resizes it at any time
  value, \* the current option, held by the viewer
  menu,  \* "closed" or "open"
  focus  \* where keyboard focus is
vars == <<width, value, menu, focus>>

\* Closed: the group's options from 28rem, the press below it.
\* Open: the options in the menu, and Escape or a click away.
Shown(p) ==
  IF menu = "open" THEN {"pick", "dismiss"}
  ELSE IF width = "wide" THEN {"pick"} ELSE {"press"}

Allowed(p, b) == b \in Buttons

Hint(p) == ""

Init ==
  /\ width \in Widths
  /\ value \in Options
  /\ menu = "closed"
  /\ focus = "away"

\* The person drags a grid edge. An open menu closes once its press leaves the
\* bar; focus goes to the current option. A focused button the pane hides
\* loses focus.
Resize ==
  /\ \E w \in Widths : w # width /\ width' = w
  /\ IF menu = "open" /\ width' = "wide"
     THEN menu' = "closed" /\ focus' = "pick"
     ELSE /\ UNCHANGED menu
          /\ focus' = IF menu = "closed" /\ focus \in Buttons
                         /\ focus \notin (IF width' = "wide" THEN {"pick"} ELSE {"press"})
                      THEN "away" ELSE focus
  /\ UNCHANGED value
\* The person moves focus to a shown button.
Focus(p) ==
  /\ menu = "closed"
  /\ \E b \in Shown(p) : b # focus /\ focus' = b
  /\ UNCHANGED <<width, value, menu>>
\* The person opens the menu from the press.
Open(p) ==
  /\ menu = "closed" /\ "press" \in Shown(p)
  /\ menu' = "open" /\ focus' = "menu"
  /\ UNCHANGED <<width, value>>
\* The person picks an option, in the group or the menu.
Pick(p) ==
  /\ "pick" \in Shown(p)
  /\ \E o \in Options : value' = o
  /\ menu' = "closed"
  /\ focus' = IF menu = "open" THEN "press" ELSE "pick"
  /\ UNCHANGED width
\* Escape or a click away closes the menu.
Dismiss(p) ==
  /\ "dismiss" \in Shown(p)
  /\ menu' = "closed" /\ focus' = "press"
  /\ UNCHANGED <<width, value>>

Next ==
  \/ Resize
  \/ \E p \in People : Focus(p) \/ Open(p) \/ Pick(p) \/ Dismiss(p)

S == INSTANCE Surface WITH Actions <- Buttons, Next <- Hint
EveryShownActionWorks == S!EveryShownActionWorks
NobodyIsStuck == S!NobodyIsStuck

\* The menu is open only while its press is in the bar.
MenuOnlyNarrow == menu = "open" => width = "narrow"
\* A closed choice shows exactly one of the group and the press.
ShownOnce == menu = "closed" => Shown("person") \in {{"pick"}, {"press"}}
\* Focus sits on a shown button, the open menu, or away; never on a press the
\* bar hides.
FocusShown ==
  /\ focus \in Focuses
  /\ (menu = "open") = (focus = "menu")
  /\ menu = "closed" /\ focus \in Buttons => focus \in Shown("person")
====
