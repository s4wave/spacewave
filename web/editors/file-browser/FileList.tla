---- MODULE FileList ----
EXTENDS Naturals
\* The file list: a header of sortable columns over rows a person selects with
\* the pointer or the keyboard. The list reads its own width, not the window's,
\* because it sits in a pane the person resizes. Its columns follow that width:
\*
\*   wide    (24rem and wider):      Name, Date Modified, Size
\*   medium  (15.5rem up to 24rem):  Name, Size
\*   narrow  (under 15.5rem):        Name
\*
\* Date leaves first, then Size, so the name is the last thing to lose width.
\* Each row keeps its More press in every width: it is the only menu target on
\* a touch screen. Selection and focus are the List's, from listReducer; the
\* keys here are the ones List handles, each one SELECT_ITEM action.

People == {"person"}
Widths == {"wide", "medium", "narrow"}
Sorts == {"name", "date", "size"}
Rows == 2  \* the most rows a list holds in this model
Buttons == {"sortName", "sortDate", "sortSize", "more",
            "home", "end", "shiftHome", "shiftEnd", "modHome", "modEnd"}

VARIABLES
  width,    \* the list's own width class
  n,        \* how many rows the list holds; another writer changes it
  sort,     \* the column the rows are sorted by
  sel,      \* the selected rows
  focus,    \* the focused row, 0 for none
  anchor    \* where a Shift range starts, 0 for none
vars == <<width, n, sort, sel, focus, anchor>>

Indices == 1..n

\* Columns are the column headers and cells the list draws at its width.
Columns ==
  CASE width = "wide" -> {"name", "date", "size"}
    [] width = "medium" -> {"name", "size"}
    [] width = "narrow" -> {"name"}

SortButton == [name |-> "sortName", date |-> "sortDate", size |-> "sortSize"]

\* Wide: the three column headers, rows with their date and size.
\* Medium: the date header and cells are gone; Size keeps the right edge.
\* Narrow: names only, each with its More press.
\* Empty: the headers over "This folder is empty"; no row keys apply.
\* A sort by a column the width hides shows "by date" or "by size" and its
\* chevron beside Name, until that column shows again.
Shown(p) ==
  LET sorts == {"sortName"}
                 \cup (IF "date" \in Columns THEN {"sortDate"} ELSE {})
                 \cup (IF "size" \in Columns THEN {"sortSize"} ELSE {})
      keys == IF n = 0 THEN {}
              ELSE {"more", "home", "end", "shiftHome", "shiftEnd",
                    "modHome", "modEnd"}
  IN sorts \cup keys

\* The List reducer acts on any row that exists; a sort works on any column.
Allowed(p, b) ==
  \/ b \in {"sortName", "sortDate", "sortSize"}
  \/ b \in {"more", "home", "end", "shiftHome", "shiftEnd", "modHome", "modEnd"}
     /\ n > 0

Hint(p) == IF n = 0 THEN "This folder is empty" ELSE ""

Init ==
  /\ width = "wide"
  /\ n = Rows
  /\ sort = "name"
  /\ sel = {} /\ focus = 0 /\ anchor = 0

\* The person resizes the pane, or the layout moves the list to another one.
Resize ==
  /\ \E w \in Widths : w # width /\ width' = w
  /\ UNCHANGED <<n, sort, sel, focus, anchor>>

\* Sort by a column header the width shows.
Sort(p) ==
  /\ \E s \in Columns :
       /\ SortButton[s] \in Shown(p)
       /\ sort' = s
  /\ UNCHANGED <<width, n, sel, focus, anchor>>

\* Range is every row between two indices, inclusive.
Range(a, b) == IF a <= b THEN a..b ELSE b..a

\* Home and End select the first or last row; with Shift they extend the range
\* from the anchor; with Ctrl or Cmd they move focus and leave the selection.
Select(to) == sel' = {to} /\ focus' = to /\ anchor' = to
Extend(to) ==
  LET from == IF anchor # 0 THEN anchor ELSE IF focus # 0 THEN focus ELSE to
  IN sel' = Range(from, to) /\ focus' = to /\ anchor' = from
FocusOnly(to) == focus' = to /\ UNCHANGED <<sel, anchor>>

Home(p) == "home" \in Shown(p) /\ Select(1) /\ UNCHANGED <<width, n, sort>>
End(p) == "end" \in Shown(p) /\ Select(n) /\ UNCHANGED <<width, n, sort>>
ShiftHome(p) ==
  "shiftHome" \in Shown(p) /\ Extend(1) /\ UNCHANGED <<width, n, sort>>
ShiftEnd(p) ==
  "shiftEnd" \in Shown(p) /\ Extend(n) /\ UNCHANGED <<width, n, sort>>
ModHome(p) ==
  "modHome" \in Shown(p) /\ FocusOnly(1) /\ UNCHANGED <<width, n, sort>>
ModEnd(p) ==
  "modEnd" \in Shown(p) /\ FocusOnly(n) /\ UNCHANGED <<width, n, sort>>

\* More opens the row's menu and selects the row it belongs to.
More(p) ==
  /\ "more" \in Shown(p)
  /\ \E i \in Indices : sel' = IF i \in sel THEN sel ELSE {i}
  /\ UNCHANGED <<width, n, sort, focus, anchor>>

\* Another writer adds or removes files. The List keeps selection by id and
\* translates indices; here rows that no longer exist leave the selection.
Changed ==
  /\ \E m \in 0..Rows : m # n /\ n' = m
  /\ sel' = {i \in sel : i <= n'}
  /\ focus' = IF focus <= n' THEN focus ELSE 0
  /\ anchor' = IF anchor <= n' THEN anchor ELSE 0
  /\ UNCHANGED <<width, sort>>

Next ==
  \/ Resize \/ Changed
  \/ \E p \in People :
       \/ Sort(p) \/ More(p)
       \/ Home(p) \/ End(p) \/ ShiftHome(p) \/ ShiftEnd(p)
       \/ ModHome(p) \/ ModEnd(p)

S == INSTANCE Surface WITH Actions <- Buttons, Next <- Hint
EveryShownActionWorks == S!EveryShownActionWorks
NobodyIsStuck == S!NobodyIsStuck

\* The name shows at every width.
NameAlwaysShows == "name" \in Columns
\* Date leaves before Size: a list that shows Date shows Size.
DateLeavesFirst == "date" \in Columns => "size" \in Columns
\* Focus and selection only name rows that exist.
SelectionInRows == sel \subseteq Indices /\ focus \in 0..n /\ anchor \in 0..n
====
