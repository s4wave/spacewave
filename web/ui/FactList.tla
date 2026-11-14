---- MODULE FactList ----
\* A fact row: one line of label, value and the press that acts on the value,
\* in a fact list that reads its own width. The value never wraps. When it
\* does not fit, an id keeps its head, an ellipsis and its tail, and the
\* ellipsis shows wherever the value is cut; below 13rem of the list's width
\* the label sits over the value. The copy press copies the whole value in
\* every state and says whether the copy worked.
\*
\* Copy is CopyButton's: idle, then copied or failed for two seconds, then
\* idle again. Another writer can change the value at any time.

People == {"person"}
Widths == {"roomy", "tight", "stacked"}
Lengths == {"short", "long"}
Heads == {"short", "long"}
Buttons == {"copy"}

VARIABLES
  width,   \* the fact list's own width class
  len,     \* whether the value is short or long for that width
  head,    \* whether its head is longer than the part it always keeps
  copy,    \* the copy press: idle, copied or failed
  clip     \* what the last good copy put on the clipboard, "" for none
vars == <<width, len, head, copy, clip>>

\* Fits holds when the whole value shows: a short value always fits, and a
\* long one fits only in a roomy list.
Fits == len = "short" \/ width = "roomy"

\* Mark is where a cut value draws its ellipsis: a long head truncates to its
\* kept part and draws it there, beside the tail; a short head never
\* truncates, so the tail draws it at its own start. A whole value has none.
Mark ==
  IF Fits THEN "none"
  ELSE IF head = "long" THEN "head end" ELSE "tail start"

\* Shown is what the value cell draws.
Shown(p) == {"copy"}

\* CopyButton accepts a press in every state.
Allowed(p, b) == b = "copy"

Hint(p) == ""

Init ==
  /\ width = "roomy" /\ len = "short" /\ head = "long"
  /\ copy = "idle" /\ clip = ""

\* The person resizes the pane, or the layout moves the list.
Resize ==
  /\ \E w \in Widths : w # width /\ width' = w
  /\ UNCHANGED <<len, head, copy, clip>>

\* Another writer changes the value.
Changed ==
  /\ \E l \in Lengths, h \in Heads :
       <<l, h>> # <<len, head>> /\ len' = l /\ head' = h
  /\ UNCHANGED <<width, copy, clip>>

\* The clipboard takes the whole value, whatever part of it shows.
Copied(p) ==
  /\ "copy" \in Shown(p)
  /\ copy' = "copied" /\ clip' = "whole value"
  /\ UNCHANGED <<width, len, head>>

\* The clipboard refuses, such as without permission or focus.
Failed(p) ==
  /\ "copy" \in Shown(p)
  /\ copy' = "failed"
  /\ UNCHANGED <<width, len, head, clip>>

\* Two seconds pass and the press returns to idle.
Reset ==
  /\ copy # "idle" /\ copy' = "idle"
  /\ UNCHANGED <<width, len, head, clip>>

Next ==
  \/ Resize \/ Changed \/ Reset
  \/ \E p \in People : Copied(p) \/ Failed(p)

S == INSTANCE Surface WITH Actions <- Buttons, Next <- Hint
EveryShownActionWorks == S!EveryShownActionWorks
NobodyIsStuck == S!NobodyIsStuck

\* Shortening happens only when the value does not fit.
ShortenedOnlyWhenTooLong == ~Fits => len = "long"
\* A shortened value always shows an ellipsis where it is cut, so it never
\* looks whole, whatever the length of its head.
ShortenedShowsEllipsis == Fits <=> Mark = "none"
\* A copy, once made, holds a whole value, never the part that showed.
CopyIsWhole == copy = "copied" => clip = "whole value"
====
