---- MODULE ReplayRerun ----
\* One device's replay above an installed checkpoint, resuming from a saved
\* replay cursor that an older build may have written.
\*
\* An older build refused an operation whose handler was missing, and saved
\* that refusal in the cursor as a reason text, beside real refusals. The
\* current build waits at a missing handler instead, so a refusal it saves is
\* always a verdict on the operation and the World before it. A cursor
\* carries the version of the rule that decided its refusals. Loading a
\* cursor of an older version keeps every saved position except the
\* refusals, which lose their hash, so replay finds no saved position there
\* and runs the operation again: it applies, refuses again by the same
\* verdict, or waits for the handler. The rule never reads the reason: the
\* saved outcome here carries only whether the operation was refused.
\*
\* A position replay takes from the cursor must record the same operation
\* after the same World; a re-run that applies changes the World, so the
\* positions after it run again too.
\*
\* An operation every member refused (Member) carries its refusal in the
\* order itself, and replay refuses it before reading any saved position.
\*
\* Operations:
\*   1 a plugin operation that applies, with handler 1;
\*   2 applies only on a World without 1, as initializing a World does;
\*   3 a plugin operation its handler refuses, a real verdict, with handler 3;
\*   4 an operation every member refused;
\*   5 applies only on a World with 1.
EXTENDS Naturals, Sequences

CONSTANT MaxRestarts

N == 5
Ops == 1..N
Kind == <<"plug", "first", "verdict", "plug", "dep">>
Member == {4}
NeedsHandler == {1, 3}

\* Current is the version of the rule that waits at a missing handler.
Current == 1

\* Applies is whether operation i applies on World W, given its handler.
Applies(i, W) ==
  CASE Kind[i] = "plug" -> TRUE
    [] Kind[i] = "verdict" -> FALSE
    [] Kind[i] = "first" -> 1 \notin W
    [] Kind[i] = "dep" -> 1 \in W

\* A position records an operation, whether it was refused, and the World
\* before it. Op 0 is a refusal that lost its hash on load.
Pos == [op : 0..N, refused : BOOLEAN, before : SUBSET Ops]

WorldAfter(p) == IF p.refused THEN p.before ELSE p.before \cup {p.op}
WorldOf(s) == IF s = <<>> THEN {} ELSE WorldAfter(s[Len(s)])

\* Decide is a build's outcome for operation i on World W with handlers R:
\* the old build refuses at a missing handler, the current one waits.
Decide(i, W, R, old) ==
  IF i \in Member THEN "refused"
  ELSE IF i \in NeedsHandler \ R THEN (IF old THEN "refused" ELSE "wait")
  ELSE IF Applies(i, W) THEN "applied" ELSE "refused"

\* Run is the positions a build computes for operations 1..n with handlers R,
\* stopping at a wait.
RECURSIVE Run(_, _, _)
Run(R, old, n) ==
  IF n = 0 THEN <<>>
  ELSE LET s == Run(R, old, n - 1) IN
    IF Len(s) < n - 1 THEN s
    ELSE LET d == Decide(n, WorldOf(s), R, old) IN
      IF d = "wait" THEN s
      ELSE Append(s, [op |-> n, refused |-> d = "refused", before |-> WorldOf(s)])

\* Truth is the replay with every handler present.
Truth == Run(NeedsHandler, FALSE, N)

\* Load is the restored positions of cursor c.
Load(c) ==
  IF c.ver < Current
  THEN [i \in 1..Len(c.outs) |->
          IF c.outs[i].refused THEN [c.outs[i] EXCEPT !.op = 0] ELSE c.outs[i]]
  ELSE c.outs

MemberRefused(i) == i \in Member

VARIABLES cursor, restored, pos, phase, registered, restarts
vars == <<cursor, restored, pos, phase, registered, restarts>>

\* The saved cursor is a prefix either build computed with the handlers it had.
Init ==
  /\ \E R0 \in SUBSET NeedsHandler, old \in BOOLEAN, n \in 0..N :
       cursor = [ver |-> IF old THEN 0 ELSE Current, outs |-> Run(R0, old, n)]
  /\ registered \in SUBSET NeedsHandler
  /\ restored = Load(cursor)
  /\ pos = <<>>
  /\ phase = "running"
  /\ restarts = 0

\* NoHandler is the outcome at an operation whose handler is missing.
NoHandler(i, W) == phase' = "waiting" /\ UNCHANGED pos

\* Step decides the next operation, as run does.
Step ==
  /\ phase = "running"
  /\ LET i == Len(pos) + 1
         W == WorldOf(pos)
     IN IF i > N THEN /\ phase' = "caughtup"
                      /\ UNCHANGED pos
        ELSE IF MemberRefused(i) THEN
          /\ pos' = Append(pos, [op |-> i, refused |-> TRUE, before |-> W])
          /\ UNCHANGED phase
        ELSE IF i <= Len(restored) /\ restored[i].op = i /\ restored[i].before = W THEN
          /\ pos' = Append(pos, restored[i])
          /\ UNCHANGED phase
        ELSE IF i \in NeedsHandler \ registered THEN NoHandler(i, W)
        ELSE
          /\ pos' = Append(pos, [op |-> i, refused |-> ~Applies(i, W), before |-> W])
          /\ UNCHANGED phase
  /\ UNCHANGED <<cursor, restored, registered, restarts>>

\* Register adds a handler, which ends a wait.
Register ==
  /\ \E h \in NeedsHandler \ registered : registered' = registered \cup {h}
  /\ phase' = IF phase = "waiting" THEN "running" ELSE phase
  /\ UNCHANGED <<cursor, restored, pos, restarts>>

\* Save writes the positions replay holds under the current version.
Save ==
  /\ cursor' = [ver |-> Current, outs |-> pos]
  /\ cursor' # cursor
  /\ UNCHANGED <<restored, pos, phase, registered, restarts>>

\* Restart remounts the Space and loads the saved cursor.
Restart ==
  /\ restarts < MaxRestarts
  /\ restored' = Load(cursor)
  /\ pos' = <<>>
  /\ phase' = "running"
  /\ restarts' = restarts + 1
  /\ UNCHANGED <<cursor, registered>>

Next == Step \/ Register \/ Save \/ Restart
Spec == Init /\ [][Next]_vars
FairSpec == Spec /\ WF_vars(Step) /\ WF_vars(Register)

TypeOK ==
  /\ cursor.ver \in 0..Current /\ cursor.outs \in Seq(Pos)
  /\ restored \in Seq(Pos) /\ pos \in Seq(Pos)
  /\ phase \in {"running", "waiting", "caughtup"}
  /\ registered \subseteq NeedsHandler
  /\ restarts \in 0..MaxRestarts

Prefix(s) == Len(s) <= N /\ s = SubSeq(Truth, 1, Len(s))

\* Every position replay holds is the one a device with every handler
\* computes: no refusal for a missing handler survives, a real verdict stays,
\* and a member refusal stays.
PrefixTrue == Prefix(pos)

\* A cursor of the current version holds only such positions.
SavedTrue == cursor.ver = Current => Prefix(cursor.outs)

\* Replay at the end of the order holds the World every handler gives.
CaughtUpTrue == phase = "caughtup" => pos = Truth

\* Once every handler is registered, replay reaches the end of the order.
CatchesUp == (registered = NeedsHandler) ~> (phase = "caughtup")
====
