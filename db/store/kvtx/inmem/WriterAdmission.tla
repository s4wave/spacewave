---- MODULE WriterAdmission ----
\* Store admission composes the existing FIFO writer mutex with a broadcast
\* reader drain. A writer retains its turn through Commit or Discard. A
\* cancelled queued/draining writer drops both registration and any held turn.
EXTENDS Naturals, Sequences, FiniteSets
CONSTANTS FIFO, DropRegistration, DropTurn, GateReaders
Writers == {"a", "b", "c"}
Readers == {"r0", "r1"}
None == "none"
VARIABLES pc, queue, turn, activeReaders, pending, barged, readerBarged
vars == <<pc, queue, turn, activeReaders, pending, barged, readerBarged>>
Waiting == {w \in Writers : pc[w] \in {"registered", "queued", "turn"}}
First == IF queue = <<>> THEN None ELSE Head(queue)
Writing == IF turn = None THEN FALSE ELSE pc[turn] = "writing"
Init == /\ pc = [w \in Writers |-> "idle"]
        /\ queue = <<>> /\ turn = None
        /\ activeReaders = Readers /\ pending = {}
        /\ barged = FALSE /\ readerBarged = FALSE

Register(w) ==
  /\ (pc[w] = "idle" \/ (w = "a" /\ pc[w] = "done"))
  /\ pc' = [pc EXCEPT ![w] = "registered"]
  /\ pending' = pending \cup {w}
  /\ UNCHANGED <<queue, turn, activeReaders, barged, readerBarged>>

Enqueue(w) ==
  /\ pc[w] = "registered"
  /\ pc' = [pc EXCEPT ![w] = "queued"]
  /\ queue' = Append(queue, w)
  /\ UNCHANGED <<turn, activeReaders, pending, barged, readerBarged>>

TakeTurn(w) ==
  /\ turn = None /\ pc[w] = "queued"
  /\ (~FIFO \/ w = First)
  /\ pc' = [pc EXCEPT ![w] = "turn"]
  /\ turn' = w
  /\ queue' = SelectSeq(queue, LAMBDA v : v # w)
  /\ barged' = (barged \/ w # First)
  /\ UNCHANGED <<activeReaders, pending, readerBarged>>

BeginWrite(w) ==
  /\ pc[w] = "turn" /\ turn = w /\ activeReaders = {}
  /\ pc' = [pc EXCEPT ![w] = "writing"]
  /\ pending' = pending \ {w}
  /\ UNCHANGED <<queue, turn, activeReaders, barged, readerBarged>>

\* Commit publishes data before this release; Discard drops its private overlay.
Finish(w) ==
  /\ pc[w] = "writing" /\ turn = w
  /\ pc' = [pc EXCEPT ![w] = "done"]
  /\ turn' = None
  /\ UNCHANGED <<queue, activeReaders, pending, barged, readerBarged>>

Cancel(w) ==
  /\ w \in Waiting
  /\ pc' = [pc EXCEPT ![w] = "cancelled"]
  /\ queue' = SelectSeq(queue, LAMBDA v : v # w)
  /\ pending' = (IF DropRegistration THEN pending \ {w} ELSE pending)
  /\ turn' = (IF DropTurn /\ turn = w THEN None ELSE turn)
  /\ UNCHANGED <<activeReaders, barged, readerBarged>>

Read(r) ==
  /\ r \notin activeReaders
  /\ ~Writing
  /\ (~GateReaders \/ pending = {})
  /\ activeReaders' = activeReaders \cup {r}
  /\ readerBarged' = (readerBarged \/ pending # {})
  /\ UNCHANGED <<pc, queue, turn, pending, barged>>

ReadDone(r) ==
  /\ r \in activeReaders
  /\ activeReaders' = activeReaders \ {r}
  /\ UNCHANGED <<pc, queue, turn, pending, barged, readerBarged>>

Next == \/ \E w \in Writers : Register(w) \/ Enqueue(w) \/ TakeTurn(w)
                              \/ BeginWrite(w) \/ Finish(w) \/ Cancel(w)
        \/ \E r \in Readers : Read(r) \/ ReadDone(r)
Fair == /\ \A w \in Writers : /\ WF_vars(Enqueue(w)) /\ WF_vars(TakeTurn(w))
                              /\ WF_vars(BeginWrite(w)) /\ WF_vars(Finish(w))
        /\ \A r \in Readers : WF_vars(ReadDone(r))
Spec == Init /\ [][Next]_vars /\ Fair
RegistrationConserved == pending = Waiting
TurnValid == IF turn = None THEN TRUE ELSE pc[turn] \in {"turn", "writing"}
Exclusive == \A w \in Writers : pc[w] = "writing" => turn = w /\ activeReaders = {}
NoWriterBarge == ~barged
NoReaderBarge == ~readerBarged
WriterProgress == \A w \in Writers : pc[w] \in {"registered", "queued", "turn"} ~> pc[w] \in {"writing", "done", "cancelled"}
====
