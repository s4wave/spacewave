---- MODULE RootMoveQueue ----
\* The deferred admission root move of Adapter, as rootMoveQueue implements
\* it. Two admissions, a and b, both name one payload p that an outside stage
\* holds until it releases at any time, as an upload's stage does. Each may
\* have a candidate root that names p, and may also have written a World page
\* under that root to its writer's stage before Admit, as a write too large
\* for its buffer does (world.Candidate.Staged). The writer's stage ends with
\* the transaction's retirement, right after the turn, before the admission's
\* queued moves land.
\*
\* An admission's turn records the drop epoch and reads the held view (the
\* durable roots and the queued moves), then takes rootMtx twice: once to
\* queue its payload move, a strict move of p when the view lacks p, which
\* requires p held now and at landing, and once to queue its candidate move,
\* a non-strict move of its candidate root and staged page. An admission whose
\* view already holds p through another admission's queued move borrows that
\* hold and queues no move of p. rootMoveQueue.depend then ends the turn. The
\* lander, which never holds rootMtx, lands the oldest move, or drops every
\* queued move when a strict move's block is gone, failing their admissions;
\* it can run between any two steps of a turn. The durable step writes an
\* admission's state once none of its moves is queued and none was dropped.
\* The collector frees a block nothing holds, and a durable candidate holds
\* the blocks it names. A crash drops the queue and every stage.
\*
\* The fixes the code makes are switches, so the mutants show each is needed:
\* LandStaged lands a staged admission's moves in the turn, behind every
\* queued move, while its writer's stage still holds its page, Markers queues
\* an empty move for an admission that borrowed a queued hold so a drop fails
\* it too, EpochCheck fails an admission at depend when a drop made its view
\* stale, and BoundPush refuses a move whose owner began before the latest
\* drop, so a drop between the payload and candidate moves cannot leave the
\* candidate queued without the payload's hold.

EXTENDS Naturals, Sequences, FiniteSets

LandStaged == TRUE
Markers == TRUE
EpochCheck == TRUE
StrictPayload == TRUE
BoundPush == TRUE

Adm == {"a", "b"}
Page == [x \in Adm |-> "w" \o x]
Cand == [x \in Adm |-> "c" \o x]
Blocks == {"p"} \cup {Page[x] : x \in Adm}

VARIABLES phase, staged, cand, writer, ext, queue, durable, freed, epoch,
          seen, view
vars == <<phase, staged, cand, writer, ext, queue, durable, freed, epoch,
          seen, view>>

Need(x) == {"p"} \cup (IF staged[x] THEN {Page[x]} ELSE {})
Queued == UNION {queue[i].blocks : i \in 1..Len(queue)}
Owns(q, x) == \E i \in 1..Len(q) : q[i].owner = x
Held(b) == \/ b \in durable
           \/ b = "p" /\ ext
           \/ \E x \in Adm : b = Page[x] /\ writer[x]
           \/ \E x \in Adm : Cand[x] \in durable /\ b \in Need(x)

TypeOK == /\ phase \in [Adm -> {"writing", "read", "half", "pushed",
                                "queued", "written", "failed", "lost"}]
          /\ durable \subseteq Blocks \cup {Cand[x] : x \in Adm}
          /\ freed \subseteq Blocks
          /\ epoch \in Nat

Init == /\ phase = [x \in Adm |-> "writing"]
        /\ staged \in [Adm -> BOOLEAN]
        /\ cand \in [Adm -> BOOLEAN]
        /\ \A x \in Adm : staged[x] => cand[x]
        /\ writer = [x \in Adm |-> TRUE]
        /\ ext = TRUE
        /\ queue = <<>>
        /\ durable = {}
        /\ freed = {}
        /\ epoch = 0
        /\ seen = [x \in Adm |-> 0]
        /\ view = [x \in Adm |-> {}]

\* rootMoveQueue.begin records the drop epoch, then holdAdmitted reads the
\* held view.
ReadView(x) == /\ phase[x] = "writing"
               /\ phase' = [phase EXCEPT ![x] = "read"]
               /\ seen' = [seen EXCEPT ![x] = epoch]
               /\ view' = [view EXCEPT ![x] = durable \cup Queued]
               /\ UNCHANGED <<staged, cand, writer, ext, queue, durable, freed,
                              epoch>>

Fail(x) == /\ phase' = [phase EXCEPT ![x] = "failed"]
           /\ UNCHANGED <<staged, cand, writer, ext, queue, durable, freed,
                          epoch, seen, view>>

\* A drop empties the queue and starts a new drop epoch. Its caller fails the
\* admissions whose moves it dropped.
Drop == /\ queue' = <<>>
        /\ epoch' = epoch + 1
        /\ UNCHANGED <<staged, cand, writer, ext, durable, freed, seen, view>>

\* The payload move and the candidate move. A strict move refuses a block not
\* held now (ErrRootNotHeld).
PayloadMove(x) == IF "p" \notin view[x]
                  THEN <<[owner |-> x, blocks |-> {"p"}, strict |-> StrictPayload]>>
                  ELSE <<>>
CandMove(x) == IF cand[x]
               THEN <<[owner |-> x, strict |-> FALSE,
                       blocks |-> {Cand[x]} \cup (IF staged[x] THEN {Page[x]} ELSE {})]>>
               ELSE <<>>
Gone(q) == \E i \in 1..Len(q) : q[i].strict /\ q[i].blocks \cap freed # {}
Refused(x) == BoundPush /\ seen[x] # epoch

\* A staged admission's moves land at once, behind every queued move, as
\* moveRootsOn's landAll does, and a queued strict move whose block is gone
\* drops the queue and fails it.
LandTurn(x) ==
  LET own == PayloadMove(x) \o CandMove(x)
  IN /\ phase[x] = "read" /\ LandStaged /\ staged[x]
     /\ IF \/ Markers /\ EpochCheck /\ seen[x] # epoch
           \/ "p" \notin view[x] /\ ~Held("p")
        THEN Fail(x)
        ELSE IF Gone(queue)
        THEN /\ Drop
             /\ phase' = [y \in Adm |-> IF y = x \/ (Owns(queue, y) /\ phase[y] = "queued")
                                        THEN "failed" ELSE phase[y]]
        ELSE /\ phase' = [phase EXCEPT ![x] = "queued"]
             /\ durable' = durable \cup Queued
                           \cup UNION {own[i].blocks : i \in 1..Len(own)}
             /\ queue' = <<>>
             /\ UNCHANGED <<staged, cand, writer, ext, freed, epoch, seen, view>>

\* addPayloads under the first rootMtx scope: queueMove, then push.
PushPayload(x) ==
  /\ phase[x] = "read" /\ ~(LandStaged /\ staged[x])
  /\ IF \/ PayloadMove(x) # <<>> /\ Refused(x)
        \/ "p" \notin view[x] /\ ~Held("p")
     THEN Fail(x)
     ELSE /\ phase' = [phase EXCEPT ![x] = "half"]
          /\ queue' = queue \o PayloadMove(x)
          /\ UNCHANGED <<staged, cand, writer, ext, durable, freed, epoch,
                         seen, view>>

\* addRoot under the second rootMtx scope. A lander may have dropped the
\* payload move in between.
PushCand(x) ==
  /\ phase[x] = "half"
  /\ IF CandMove(x) # <<>> /\ Refused(x)
     THEN Fail(x)
     ELSE /\ phase' = [phase EXCEPT ![x] = "pushed"]
          /\ queue' = queue \o CandMove(x)
          /\ UNCHANGED <<staged, cand, writer, ext, durable, freed, epoch,
                         seen, view>>

\* rootMoveQueue.depend fails a turn a drop made stale, or queues a marker
\* behind the moves it saw.
Depend(x) ==
  /\ phase[x] = "pushed"
  /\ IF Markers /\ EpochCheck /\ seen[x] # epoch
     THEN Fail(x)
     ELSE /\ phase' = [phase EXCEPT ![x] = "queued"]
          /\ queue' = IF Markers /\ queue # <<>> /\ queue[Len(queue)].owner # x
                      THEN Append(queue, [owner |-> x, blocks |-> {}, strict |-> FALSE])
                      ELSE queue
          /\ UNCHANGED <<staged, cand, writer, ext, durable, freed, epoch,
                         seen, view>>

\* drainRetirement ends the writer's stage once the turn is over.
EndTurn(x) == /\ writer[x] /\ phase[x] \notin {"writing", "read", "half", "pushed"}
              /\ writer' = [writer EXCEPT ![x] = FALSE]
              /\ UNCHANGED <<phase, staged, cand, ext, queue, durable, freed,
                             epoch, seen, view>>

ReleaseExt == /\ ext
              /\ ext' = FALSE
              /\ UNCHANGED <<phase, staged, cand, writer, queue, durable, freed,
                             epoch, seen, view>>

\* landLocked lands the oldest move, or drops the whole queue (Drop) when a
\* strict move's block is gone.
Land == /\ queue # <<>>
        /\ IF Head(queue).strict /\ Head(queue).blocks \cap freed # {}
           THEN /\ Drop
                /\ phase' = [x \in Adm |-> IF Owns(queue, x) /\ phase[x] = "queued"
                                           THEN "failed" ELSE phase[x]]
           ELSE /\ durable' = durable \cup Head(queue).blocks
                /\ queue' = Tail(queue)
                /\ UNCHANGED <<phase, staged, cand, writer, ext, freed, epoch,
                               seen, view>>

\* opQueue.execute: landQueued, then queueOperation.
WriteState(x) == /\ phase[x] = "queued" /\ ~Owns(queue, x)
                 /\ phase' = [phase EXCEPT ![x] = "written"]
                 /\ UNCHANGED <<staged, cand, writer, ext, queue, durable, freed,
                                epoch, seen, view>>

Collect(b) == /\ b \notin freed /\ ~Held(b)
              /\ freed' = freed \cup {b}
              /\ UNCHANGED <<phase, staged, cand, writer, ext, queue, durable,
                             epoch, seen, view>>

Crash == /\ phase' = [x \in Adm |-> IF phase[x] \in {"written", "failed"}
                                    THEN phase[x] ELSE "lost"]
         /\ writer' = [x \in Adm |-> FALSE]
         /\ ext' = FALSE
         /\ queue' = <<>>
         /\ UNCHANGED <<staged, cand, durable, freed, epoch, seen, view>>

Next == \/ \E x \in Adm : \/ ReadView(x) \/ LandTurn(x) \/ PushPayload(x)
                          \/ PushCand(x) \/ Depend(x) \/ EndTurn(x)
                          \/ WriteState(x)
        \/ ReleaseExt \/ Land \/ Crash
        \/ \E b \in Blocks : Collect(b)
Spec == Init /\ [][Next]_vars

\* A durable state names only blocks the store still has.
DurableStateHolds == \A x \in Adm : phase[x] = "written" => Need(x) \cap freed = {}

\* A durable candidate root names only blocks the store still has.
DurableGraphWhole == \A x \in Adm : Cand[x] \in durable => Need(x) \cap freed = {}
====
