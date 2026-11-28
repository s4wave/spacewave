---- MODULE RootMoveQueue ----
\* The deferred admission root move of Adapter, as rootMoveQueue implements
\* it. Two admissions take the writer turn in order, a then b, and b's
\* transaction builds on a's candidate while a is still pending (base). Each
\* may name one payload p that an outside stage holds until it releases at any
\* time, as an upload's stage does. Each may have a candidate World (kind):
\* none; kept in memory ("mem"), whose turn moves no root; spilled to its
\* overflow stage ("ovf"), which holds the spilled page until the admission
\* leaves the queue; or drained to its writer's stage before Admit ("staged"),
\* which holds the page until the transaction's retirement right after the
\* turn. A candidate's graph is its own page, p when it names p, and, for b
\* with share, the graph of a's candidate its pages reach.
\*
\* An admission's turn joins the queue, then takes rootMtx twice. Its payload
\* scope reads the held view (the durable roots and the queued moves) and
\* pushes a strict move of p when the view lacks p, which requires p held now
\* and at landing; an admission whose view holds p through a queued move
\* borrows that hold. Its candidate scope pushes a non-strict move of its
\* candidate root and own page. depend ends the turn behind every move the
\* turn saw, and a staged admission's moves land at once, while its writer's
\* stage still holds its page. The lander, which never holds rootMtx, lands
\* the oldest move, or drops every queued move when a strict move's block is
\* gone; it can run between any two steps of a turn. The durable step writes
\* an admission's state once none of its moves is queued, unless it failed.
\* The collector frees a block nothing holds, and a durable candidate holds
\* its graph. A crash drops the queue and every stage.
\*
\* Ownership: a drop fails every live owner, one that has joined and still has
\* a move or a turn in flight, in the same step that empties the queue, and an
\* owner whose base admission failed fails as it joins. push refuses a failed
\* owner, and every admission move, queued or landed in the turn, is a push,
\* so no landing stores a move whose holds a drop released.
\*
\* The rules are switches, so the mutants show each is needed: GatePush
\* refuses a failed owner's push (A1), StagedThroughQueue sends a staged
\* admission's moves through push instead of landing them directly (B1),
\* InheritBase fails an owner whose base failed (B2), FailLive fails the
\* owners mid-turn as well as those with queued moves, Markers holds an
\* owner's durable step behind the moves its turn saw, LandNow lands a staged
\* admission's moves before its writer's stage ends, and StrictPayload makes
\* the payload move require p held.

EXTENDS Naturals, Sequences, FiniteSets

GatePush == TRUE
StagedThroughQueue == TRUE
InheritBase == TRUE
FailLive == TRUE
Markers == TRUE
LandNow == TRUE
StrictPayload == TRUE


A == "a"
B == "b"
Admissions == {A, B}
Pred == [x \in Admissions |-> IF x = B THEN A ELSE "none"]
O == [x \in Admissions |-> "o" \o x]
W == [x \in Admissions |-> "w" \o x]
C == [x \in Admissions |-> "c" \o x]
Blocks == {"p"} \cup {O[x] : x \in Admissions} \cup {W[x] : x \in Admissions}
Kinds == {"none", "mem", "ovf", "staged"}
Turn == {"begun", "half", "pushed", "landing"}

\* landed is a ghost: it marks an owner once all its moves landed unfailed.
VARIABLES phase, kind, pay, share, base, reg, err, writer, ext, queue,
          durable, freed, landed
vars == <<phase, kind, pay, share, base, reg, err, writer, ext, queue,
          durable, freed, landed>>

Own(x) == CASE kind[x] = "ovf" -> {O[x]}
          [] kind[x] = "staged" -> {W[x]}
          [] OTHER -> {}
Pay(x) == IF pay[x] THEN {"p"} ELSE {}
GraphA == Own(A) \cup Pay(A)
Graph(x) == IF x = A THEN GraphA
            ELSE Own(B) \cup Pay(B) \cup (IF share /\ base[B] THEN GraphA ELSE {})
HasCand(x) == kind[x] \in {"ovf", "staged"}

Queued == UNION {queue[i].blocks : i \in 1..Len(queue)}
Owns(q, x) == \E i \in 1..Len(q) : q[i].owner = x
Live(x) == /\ reg[x]
           /\ \/ FailLive /\ phase[x] \in Turn
              \/ phase[x] \in {"queued", "landing"} /\ Owns(queue, x)
OvHeld(x) == /\ kind[x] = "ovf" /\ ~err[x]
             /\ \/ phase[x] \in {"idle"} \cup Turn
                \/ phase[x] = "queued" /\ Owns(queue, x)
Held(b) == \/ b \in durable
           \/ b = "p" /\ ext
           \/ \E x \in Admissions : b = W[x] /\ kind[x] = "staged" /\ writer[x]
           \/ \E x \in Admissions : b = O[x] /\ OvHeld(x)
           \/ \E x \in Admissions : C[x] \in durable /\ b \in Graph(x)

TypeOK == /\ phase \in [Admissions -> {"idle", "failed", "written", "lost",
                                       "queued"} \cup Turn]
          /\ durable \subseteq Blocks \cup {C[x] : x \in Admissions}
          /\ freed \subseteq Blocks

Init == /\ phase = [x \in Admissions |-> "idle"]
        /\ kind \in [Admissions -> Kinds]
        /\ pay \in [Admissions -> BOOLEAN]
        /\ share \in BOOLEAN
        /\ share => kind[B] # "none" /\ HasCand(A)
        /\ base = [x \in Admissions |-> FALSE]
        /\ reg = [x \in Admissions |-> FALSE]
        /\ err = [x \in Admissions |-> FALSE]
        /\ writer = [x \in Admissions |-> TRUE]
        /\ ext = TRUE
        /\ queue = <<>>
        /\ durable = {}
        /\ freed = {}
        /\ landed = [x \in Admissions |-> FALSE]

\* A drop empties the queue and fails every live owner in one step. Its
\* stages end with it (OvHeld).
Drop == /\ queue' = <<>>
        /\ err' = [y \in Admissions |-> err[y] \/ Live(y)]

Fail(x) == phase' = [phase EXCEPT ![x] = "failed"]

\* Land the queue, then store blocks, in the turn: moveRootsOn's landAll and
\* SetRoots. A strict SetRoots refuses a block not held.
LandIn(x, blocks, strict, next) ==
  IF \E i \in 1..Len(queue) : queue[i].strict /\ queue[i].blocks \cap freed # {}
  THEN /\ Drop /\ Fail(x)
       /\ UNCHANGED <<durable>>
  ELSE IF strict /\ \E b \in blocks : ~Held(b)
  THEN /\ Fail(x) /\ durable' = durable \cup Queued /\ queue' = <<>>
       /\ UNCHANGED <<err>>
  ELSE /\ phase' = [phase EXCEPT ![x] = next]
       /\ durable' = durable \cup Queued \cup blocks /\ queue' = <<>>
       /\ UNCHANGED <<err>>

Push(x, m, next) ==
  IF GatePush /\ m # <<>> /\ err[x]
  THEN Fail(x) /\ UNCHANGED <<queue>>
  ELSE phase' = [phase EXCEPT ![x] = next] /\ queue' = queue \o m

Direct(x) == kind[x] = "staged" /\ ~StagedThroughQueue

\* Begin takes the turn at the writer head: a's pending candidate while a is
\* undecided.
Begin(x) == /\ phase[x] = "idle"
            /\ x = B => phase[A] \in {"queued", "written", "failed"}
            /\ phase' = [phase EXCEPT ![x] = "begun"]
            /\ base' = [base EXCEPT ![x] = x = B /\ phase[A] \in {"queued", "written"}]
            /\ UNCHANGED <<kind, pay, share, reg, err, writer, ext, queue,
                           durable, freed>>

\* keepAdmitted joins the queue, failing at once when its base failed, then
\* the payload scope reads the view and pushes or borrows.
PayloadScope(x) ==
  LET inherited == InheritBase /\ base[x] /\ err[Pred[x]]
      need == pay[x] /\ "p" \notin durable \cup Queued
      m == IF need THEN <<[owner |-> x, blocks |-> {"p"}, strict |-> StrictPayload]>>
           ELSE <<>>
  IN /\ phase[x] = "begun"
     /\ reg' = [reg EXCEPT ![x] = TRUE]
     /\ UNCHANGED <<kind, pay, share, base, writer, ext, freed>>
     /\ IF inherited
        THEN Fail(x) /\ err' = [err EXCEPT ![x] = TRUE]
             /\ UNCHANGED <<queue, durable>>
        ELSE IF need /\ ~Held("p")
        THEN Fail(x) /\ UNCHANGED <<err, queue, durable>>
        ELSE IF Direct(x)
        THEN IF need THEN LandIn(x, {"p"}, StrictPayload, "half")
             ELSE phase' = [phase EXCEPT ![x] = "half"] /\ UNCHANGED <<err, queue, durable>>
        ELSE Push(x, m, "half") /\ UNCHANGED <<err, durable>>

\* The candidate scope: addRoot moves the pending root onto the candidate.
CandScope(x) ==
  LET m == IF HasCand(x)
           THEN <<[owner |-> x, blocks |-> {C[x]} \cup Own(x), strict |-> FALSE]>>
           ELSE <<>>
  IN /\ phase[x] = "half"
     /\ UNCHANGED <<kind, pay, share, base, reg, writer, ext, freed>>
     /\ IF Direct(x)
        THEN LandIn(x, {C[x]} \cup Own(x), FALSE, "pushed")
        ELSE Push(x, m, "pushed") /\ UNCHANGED <<err, durable>>

\* depend ends the turn behind every move it saw. A staged admission's moves
\* land next, still in the turn.
Depend(x) ==
  /\ phase[x] = "pushed"
  /\ UNCHANGED <<kind, pay, share, base, reg, err, writer, ext, durable, freed>>
  /\ IF err[x]
     THEN Fail(x) /\ UNCHANGED <<queue>>
     ELSE /\ phase' = [phase EXCEPT ![x] =
                         IF kind[x] = "staged" /\ StagedThroughQueue /\ LandNow
                         THEN "landing" ELSE "queued"]
          /\ queue' = IF Markers /\ queue # <<>> /\ queue[Len(queue)].owner # x
                      THEN Append(queue, [owner |-> x, blocks |-> {}, strict |-> FALSE])
                      ELSE queue

\* landFor in the turn: land through the owner's moves, or fail with the drop.
LandTurn(x) ==
  /\ phase[x] = "landing"
  /\ UNCHANGED <<kind, pay, share, base, reg, writer, ext, freed>>
  /\ IF err[x]
     THEN Fail(x) /\ UNCHANGED <<err, queue, durable>>
     ELSE LandIn(x, {}, FALSE, "queued")

\* drainRetirement ends the writer's stage once the turn is over.
EndTurn(x) == /\ writer[x] /\ phase[x] \notin {"idle"} \cup Turn
              /\ writer' = [writer EXCEPT ![x] = FALSE]
              /\ UNCHANGED <<phase, kind, pay, share, base, reg, err, ext,
                             queue, durable, freed>>

ReleaseExt == /\ ext
              /\ ext' = FALSE
              /\ UNCHANGED <<phase, kind, pay, share, base, reg, err, writer,
                             queue, durable, freed>>

\* landLocked lands the oldest move, or drops the whole queue when a strict
\* move's block is gone.
Land == /\ queue # <<>>
        /\ UNCHANGED <<phase, kind, pay, share, base, reg, writer, ext, freed>>
        /\ IF Head(queue).strict /\ Head(queue).blocks \cap freed # {}
           THEN Drop /\ UNCHANGED <<durable>>
           ELSE /\ durable' = durable \cup Head(queue).blocks
                /\ queue' = Tail(queue)
                /\ UNCHANGED <<err>>

\* opQueue.execute: landQueued, then queueOperation, which a drop fails.
WriteState(x) == /\ phase[x] = "queued" /\ ~Owns(queue, x)
                 /\ phase' = [phase EXCEPT ![x] = IF err[x] THEN "failed" ELSE "written"]
                 /\ UNCHANGED <<kind, pay, share, base, reg, err, writer, ext,
                                queue, durable, freed>>

Collect(b) == /\ b \notin freed /\ ~Held(b)
              /\ freed' = freed \cup {b}
              /\ UNCHANGED <<phase, kind, pay, share, base, reg, err, writer,
                             ext, queue, durable>>

Crash == /\ phase' = [x \in Admissions |-> IF phase[x] \in {"written", "failed"}
                                           THEN phase[x] ELSE "lost"]
         /\ writer' = [x \in Admissions |-> FALSE]
         /\ ext' = FALSE
         /\ queue' = <<>>
         /\ UNCHANGED <<kind, pay, share, base, reg, err, durable, freed>>

Step == \/ \E x \in Admissions : \/ Begin(x) \/ PayloadScope(x) \/ CandScope(x)
                                 \/ Depend(x) \/ LandTurn(x) \/ EndTurn(x)
                                 \/ WriteState(x)
        \/ ReleaseExt \/ Land \/ Crash
        \/ \E b \in Blocks : Collect(b)
Landed(x) == phase[x] = "queued" /\ ~Owns(queue, x) /\ ~err[x]
Next == /\ Step
        /\ landed' = [x \in Admissions |-> landed[x] \/ Landed(x)]
Spec == Init /\ [][Next]_vars

\* A durable state names only blocks the store still has.
DurableStateHolds == \A x \in Admissions : phase[x] = "written" => Graph(x) \cap freed = {}

\* A durable candidate root names only blocks the store still has.
DurableGraphWhole == \A x \in Admissions : C[x] \in durable => Graph(x) \cap freed = {}

\* A drop never fails an owner whose moves all landed: it relies only on
\* durable roots, so its state write proceeds.
LandedKept == \A x \in Admissions : landed[x] => ~err[x]
====
