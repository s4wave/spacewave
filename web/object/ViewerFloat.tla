---- MODULE ViewerFloat ----
\* The floating controls of a viewer whose content is the object: an image,
\* a video or a PDF page fills the pane and its presses float over it. Over
\* still content the floats always show. Over moving content, a video that
\* plays, they leave once the pointer has gone still or away, and come back
\* on the next pointer move or key press. They never leave while focus is
\* inside them, or while the media failed, since the failure's next step lives
\* in them. Content that starts to move counts as a touch, so a video that
\* plays with nobody at the stage keeps its floats for one idle window.
\*
\* Keyboard: a hidden float stays in the tab order, faded out rather than
\* removed, so Tab into it brings every float back; a key pressed in the
\* stage does the same. Leaving the floats with Tab or a click lets them
\* fade again once the content moves and the pointer is still.

People == {"person"}

Buttons == {"press"}

\* How a float hides: "faded" keeps it in the tab order, "removed" would take
\* it out, and the keyboard could not bring it back.
HiddenStyle == "faded"

VARIABLES
  media,   \* "still", "moving" (a video playing) or "failed"
  active,  \* the pointer moved or a key went down within the idle window
  focus    \* "inside" the floats or "outside"
vars == <<media, active, focus>>

\* The floats show unless the content moves, nobody touched it lately, focus
\* is elsewhere.
Visible ==
  \/ media # "moving"
  \/ active
  \/ focus = "inside"

\* A shown float offers its press; a hidden one is faded.
Shown(p) == IF Visible THEN Buttons ELSE {}

Allowed(p, b) == b \in Buttons

\* While the floats are hidden, moving the pointer or pressing a key brings
\* them back.
Hint(p) == IF Visible THEN "" ELSE "move the pointer or press a key"

Init ==
  /\ media \in {"still", "moving"}
  /\ active = TRUE
  /\ focus = "outside"

\* The video starts or stops playing, or the media fails. Playing, however
\* it began, starts an idle window as a touch does.
Play ==
  /\ media = "still"
  /\ media' = "moving"
  /\ active' = TRUE
  /\ UNCHANGED focus
Pause ==
  /\ media = "moving"
  /\ media' = "still"
  /\ UNCHANGED <<active, focus>>
Fail ==
  /\ media # "failed"
  /\ media' = "failed"
  /\ UNCHANGED <<active, focus>>

\* The pointer moves over the stage, or a key goes down in it.
Touch(p) ==
  /\ active' = TRUE
  /\ UNCHANGED <<media, focus>>

\* The idle window ends with the pointer still, or the pointer leaves.
Idle ==
  /\ active
  /\ active' = FALSE
  /\ UNCHANGED <<media, focus>>

\* Tab reaches a float; a hidden one is still in the tab order.
FocusIn(p) ==
  /\ HiddenStyle = "faded" \/ Visible
  /\ focus = "outside"
  /\ focus' = "inside"
  /\ UNCHANGED <<media, active>>

\* Tab or a click takes focus out of the floats.
FocusOut(p) ==
  /\ focus = "inside"
  /\ focus' = "outside"
  /\ UNCHANGED <<media, active>>

\* A shown press acts and leaves the floats as they were.
Press(p) ==
  /\ "press" \in Shown(p)
  /\ UNCHANGED vars

Next ==
  \/ Play \/ Pause \/ Fail \/ Idle
  \/ \E p \in People :
       Touch(p) \/ FocusIn(p) \/ FocusOut(p) \/ Press(p)

S == INSTANCE Surface WITH Actions <- Buttons, Next <- Hint
EveryShownActionWorks == S!EveryShownActionWorks
NobodyIsStuck == S!NobodyIsStuck

\* Floats never leave while focus is inside them.
FocusedShown == focus = "inside" => Visible
\* Still content and failed media keep their floats.
StillShown == media # "moving" => Visible
\* While the floats are hidden, a keyboard user can bring them back.
KeyboardReturns == ~Visible => ENABLED FocusIn("person")
====
