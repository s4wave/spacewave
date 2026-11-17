---- MODULE AuditWalkTail ----
\* The streaming audit's copy of the journal past the cursor. AuditWalk.tla
\* reads each block's journal delta whole in a view; this model checks that
\* the walk computes that delta from its scratch copies and a live read.
\*
\* The journal is a sequence of records, each a sequence of pairs <<block,
\* sign>>. Netting applies one pair at a time: cursor is the record it is in
\* and off the pairs of that record already applied, as the collector's
\* Cursor and NetOffset. Writers append records. A check view copies the
\* records from the later of the walk's tail and the cursor, at most Bound of
\* them, into scratch, and sums the records past those live. It skips every
\* copy netting already applied: a record before the cursor, or a pair of the
\* cursor's record within its offset. Exact: the walk's delta equals the
\* journal's in every view.
EXTENDS Integers, Sequences, FiniteSets

CONSTANTS
  \* MaxRecs bounds the journal's length.
  MaxRecs,
  \* Bound is the records one check copies.
  Bound

Blocks == {"x", "y"}
Pairs == Blocks \X {1, -1}
Records == {<<p>> : p \in Pairs} \cup {<<p, q>> : p, q \in Pairs}

VARIABLES journal, cursor, off, tail, copies, wrong

vars == <<journal, cursor, off, tail, copies, wrong>>

\* Sum adds the signs of a set of <<seq, index, block, sign>> tuples.
RECURSIVE Sum(_)
Sum(S) == IF S = {} THEN 0
          ELSE LET t == CHOOSE u \in S : TRUE IN t[4] + Sum(S \ {t})

\* Tuples returns the pairs of records lo..hi as tuples.
Tuples(lo, hi) == UNION {{<<k, i, journal[k][i][1], journal[k][i][2]>> :
                            i \in 1..Len(journal[k])} : k \in lo..hi}

\* Unnetted reports whether netting has not yet applied tuple t.
Unnetted(t) == t[1] > cursor \/ (t[1] = cursor /\ t[2] > off)

\* TrueDelta is journalTail's sum for block b.
TrueDelta(b) == Sum({t \in Tuples(cursor, Len(journal)) : t[3] = b /\ Unnetted(t)})

Init ==
  /\ journal = <<>>
  /\ cursor = 1
  /\ off = 0
  /\ tail = 1
  /\ copies = {}
  /\ wrong = FALSE

Write(r) ==
  /\ Len(journal) < MaxRecs
  /\ journal' = Append(journal, r)
  /\ UNCHANGED <<cursor, off, tail, copies, wrong>>

\* Net applies the next pair, moving to the next record past the last.
Net ==
  /\ cursor <= Len(journal)
  /\ IF off + 1 = Len(journal[cursor])
     THEN (cursor' = cursor + 1 /\ off' = 0)
     ELSE (cursor' = cursor /\ off' = off + 1)
  /\ UNCHANGED <<journal, tail, copies, wrong>>

\* Check is one check view: copy, then sum the copies and the rest live.
Check ==
  LET start == IF tail > cursor THEN tail ELSE cursor
      last == IF start + Bound - 1 < Len(journal) THEN start + Bound - 1 ELSE Len(journal)
      new == copies \cup Tuples(start, last)
      walk(b) == Sum({t \in new : t[3] = b /\ Unnetted(t)})
                 + Sum({t \in Tuples(last + 1, Len(journal)) : t[3] = b})
  IN /\ copies' = new
     /\ tail' = last + 1
     /\ wrong' = (wrong \/ \E b \in Blocks : walk(b) # TrueDelta(b))
     /\ UNCHANGED <<journal, cursor, off>>

Next == (\E r \in Records : Write(r)) \/ Net \/ Check

Spec == Init /\ [][Next]_vars

Exact == ~wrong
====
