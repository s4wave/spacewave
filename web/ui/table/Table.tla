---- MODULE Table ----
EXTENDS Naturals
\* A table of records: a header of sortable columns over List rows a person
\* selects with the pointer or the keyboard. The table reads its own width, not
\* the window's, because it sits in a pane the person resizes. Each column has
\* a priority and a least width; in priority order, a column shows from the
\* first width step (16, 22, 28 or 40rem) that holds it and every column before
\* it. The model holds one of each kind of column:
\*
\*   name   the identifying column, at every width
\*   state  priority 1, from 16rem
\*   owner  priority 2, from 28rem
\*   note   priority 3, too wide for 40rem, so it never shows as a column
\*
\* A column the width sheds stays reachable: the one selected row's shed
\* values show as facts under the rows, and leave as their columns return. A sort by
\* a shed column reads "by state" beside the name until that column shows.
\* Selection and focus are the List's, from listReducer; the keys here are the
\* ones List handles, each one SELECT_ITEM action.

People == {"person"}
Widths == {0, 16, 22, 28, 40}
Cols == {"name", "state", "owner", "note"}
Rows == 2  \* the most rows a table holds in this model
Buttons == {"sortName", "sortState", "sortOwner",
            "home", "end", "shiftHome", "shiftEnd", "modHome", "modEnd"}

\* Step is the least width each column shows from; 99 is never.
Step == [name |-> 0, state |-> 16, owner |-> 28, note |-> 99]
Priority == [name |-> 0, state |-> 1, owner |-> 2, note |-> 3]

VARIABLES
  width,    \* the largest step the table's own width reaches
  n,        \* how many rows the table holds; another writer changes it
  sort,     \* the column the rows are sorted by
  sel,      \* the selected rows
  focus,    \* the focused row, 0 for none
  anchor    \* where a Shift range starts, 0 for none
vars == <<width, n, sort, sel, focus, anchor>>

Indices == 1..n

\* Columns are the column headers and cells the table draws at its width.
Columns == {c \in Cols : Step[c] <= width}

\* Shed is every column the width hides.
Shed == Cols \ Columns

\* OneSelected holds while exactly one row is selected.
OneSelected == \E i \in Indices : sel = {i}

\* Facts are the shed values the table shows under the rows: the selected
\* row's, while exactly one is.
Facts == IF OneSelected THEN Shed ELSE {}

\* SortLabel is the shed column a header beside the name names, or "" when
\* the sorted column shows.
SortLabel == IF sort \in Shed THEN sort ELSE ""

SortButton == [name |-> "sortName", state |-> "sortState",
               owner |-> "sortOwner"]

\* Every width: the shown column headers, rows with their shown cells.
\* Narrow: names only, with a sort by a shed column named beside Name.
\* Empty: the headers over the placeholder; no row keys apply.
Shown(p) ==
  LET sorts == {SortButton[c] : c \in Columns \ {"note"}}
      keys == IF n = 0 THEN {}
              ELSE {"home", "end", "shiftHome", "shiftEnd",
                    "modHome", "modEnd"}
  IN sorts \cup keys

\* The List reducer acts on any row that exists; a sort works on any column.
Allowed(p, b) ==
  \/ b \in {"sortName", "sortState", "sortOwner"}
  \/ b \in {"home", "end", "shiftHome", "shiftEnd", "modHome", "modEnd"}
     /\ n > 0

Hint(p) == IF n = 0 THEN "No rows" ELSE ""

Init ==
  /\ width \in Widths
  /\ n = Rows
  /\ sort = "name"
  /\ sel = {} /\ focus = 0 /\ anchor = 0

\* The person resizes the pane, or the layout moves the table to another one.
Resize ==
  /\ \E w \in Widths : w # width /\ width' = w
  /\ UNCHANGED <<n, sort, sel, focus, anchor>>

\* Sort by a column header the width shows; the sort stays when it sheds.
Sort(p) ==
  /\ \E c \in Columns \ {"note"} :
       /\ SortButton[c] \in Shown(p)
       /\ sort' = c
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

\* Another writer adds or removes records. The List keeps selection by id and
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
       \/ Sort(p)
       \/ Home(p) \/ End(p) \/ ShiftHome(p) \/ ShiftEnd(p)
       \/ ModHome(p) \/ ModEnd(p)

S == INSTANCE Surface WITH Actions <- Buttons, Next <- Hint
EveryShownActionWorks == S!EveryShownActionWorks
NobodyIsStuck == S!NobodyIsStuck

\* The identifying column shows at every width.
NameAlwaysShows == "name" \in Columns
\* Columns shed in priority order: a shown column's betters all show.
ShedInPriorityOrder ==
  \A c, d \in Cols : c \in Columns /\ Priority[d] < Priority[c] => d \in Columns
\* The selected row's shed values stay reachable as facts.
ShedValuesReachable == OneSelected => Shed \subseteq Facts
\* A fact never repeats a column the row shows.
FactsOnlyShed == Facts \cap Columns = {}
\* A sort by a shed column is named beside the name.
ShedSortNamed == sort \notin Columns => SortLabel = sort
\* Focus and selection only name rows that exist.
SelectionInRows == sel \subseteq Indices /\ focus \in 0..n /\ anchor \in 0..n
====
