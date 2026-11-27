---- MODULE ReplayExpectedWrite ----
\* Replay of one device's World writes, each carrying the write it expects to
\* be its author's last applied one.
\*
\* Device A writes operations 1..NA in order. The writer builds each one on a
\* base: either on its own previous operation, still pending (pipelined), or
\* on the head it last saw. Chain(i) is the set of A's operations the base
\* holds, and Exp(i) is the last of them, the write operation i expects.
\* The queue stores each operation or, after an unknown outcome, loses it.
\* A stored operation may still be refused by replay for its own reason (bad).
\*
\* Replay keeps, beside the World, each device's last applied write (rec).
\* It applies operation i only when rec[A] equals Exp(i), so a write whose
\* author's previous write was refused or lost is refused too.
\*
\* Device B writes one concurrent operation. In Merge order it sorts among
\* A's operations by hash, so it may land ahead of A operations that were
\* already accepted on an order without it. Replay may start from a
\* checkpoint at any position, which keeps the World and the records.
EXTENDS Naturals, Sequences, FiniteSets

NA == 3
AOps == 1..NA
Bop == NA + 1
Ops == 1..Bop
Dev == {"A", "B"}

VARIABLES base, seen, bad, landed, bpos
vars == <<base, seen, bad, landed, bpos>>

\* A state is the World, as the set of applied operations, and each device's
\* last applied write, 0 for none.
State == [w : SUBSET Ops, rec : [Dev -> 0..Bop]]
Empty == [w |-> {}, rec |-> [d \in Dev |-> 0]]

\* Record is the records after device d's write o applies.
Record(r, d, o) == [r EXCEPT ![d] = o]

\* Restore is the state replay resumes from at a checkpoint.
Restore(s) == s

\* AOrder is A's stored operations in their chain order.
AOrder == SelectSeq([j \in AOps |-> j], LAMBDA j : landed[j])

\* Final is the Merge order with B's operation placed after bpos of them.
Final ==
  LET k == IF bpos > Len(AOrder) THEN Len(AOrder) ELSE bpos IN
  SubSeq(AOrder, 1, k) \o <<Bop>> \o SubSeq(AOrder, k + 1, Len(AOrder))

RECURSIVE Replay(_, _), Chain(_), Exp(_), HeadState(_)

\* HeadState(i) is the head the writer saw before writing operation i: the
\* replay of A's stored operations up to seen[i].
HeadState(i) == Replay(SelectSeq(AOrder, LAMBDA j : j <= seen[i]), Empty)

Chain(i) ==
  IF base[i] = "pending" THEN Chain(i - 1) \cup {i - 1}
  ELSE HeadState(i).w \cap AOps

Exp(i) ==
  IF base[i] = "pending" THEN i - 1 ELSE HeadState(i).rec["A"]

\* Check is the replay's precondition on A's operation o in state s.
Check(s, o) == s.rec["A"] = Exp(o)

Apply(s, o) ==
  IF o = Bop THEN [w |-> s.w \cup {o}, rec |-> Record(s.rec, "B", o)]
  ELSE IF ~bad[o] /\ Check(s, o)
    THEN [w |-> s.w \cup {o}, rec |-> Record(s.rec, "A", o)]
    ELSE s

Replay(seq, s) ==
  IF seq = <<>> THEN s ELSE Replay(Tail(seq), Apply(s, Head(seq)))

Index(seq, o) == CHOOSE k \in 1..Len(seq) : seq[k] = o
Applied(seq, o) == o \in Replay(seq, Empty).w
Before(seq, o) == Replay(SubSeq(seq, 1, Index(seq, o) - 1), Empty).w \cap AOps

Stored == {i \in AOps : landed[i]}

Init ==
  /\ base \in [AOps -> {"pending", "head"}]
  /\ base[1] = "head"
  /\ seen \in [AOps -> 0..NA]
  /\ \A i \in AOps : seen[i] < i
  /\ bad \in [AOps -> BOOLEAN]
  /\ landed \in [AOps -> BOOLEAN]
  /\ bpos \in 0..NA

Next == UNCHANGED vars
Spec == Init /\ [][Next]_vars

TypeOK ==
  /\ Replay(Final, Empty) \in State
  /\ \A i \in AOps : Exp(i) \in 0..NA

\* RowsMatch: a stored operation applies exactly when replay finds before it
\* the A writes it was built on, and it is not refused for its own reason.
RowsMatch ==
  \A i \in Stored :
    Applied(Final, i) <=> (~bad[i] /\ Before(Final, i) = Chain(i))

\* NothingAcceptedLost: an operation accepted before B's concurrent write
\* arrived stays applied once that write sorts ahead of it.
NothingAcceptedLost ==
  \A i \in Stored : Applied(AOrder, i) => Applied(Final, i)

\* CheckpointAgrees: replay resumed from a checkpoint at any position reaches
\* the state a full replay reaches.
CheckpointAgrees ==
  \A k \in 0..Len(Final) :
    Replay(SubSeq(Final, k + 1, Len(Final)),
           Restore(Replay(SubSeq(Final, 1, k), Empty)))
      = Replay(Final, Empty)
====
