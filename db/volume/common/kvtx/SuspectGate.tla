---------------------------- MODULE SuspectGate ----------------------------
(***************************************************************************)
(* The suspect gate of suspect-gate.go: a Volume's readers skip suspect    *)
(* records while the gate is closed, and only a list rebuild's Finish      *)
(* writes one.                                                             *)
(*                                                                         *)
(* epoch is even while the Volume may hold suspect records and odd while   *)
(* it proves it holds none. A Finish bumps it on entry, waits for the      *)
(* optimistic assumers that saw it closed, writes suspect records under    *)
(* the store's writer, and bumps it again on exit. A closer loads the      *)
(* epoch, reads the suspect count in a later snapshot, and moves the epoch *)
(* to the next odd value when the count is zero and no Finish runs. A      *)
(* writer holding the store's writer loads the epoch after it takes the    *)
(* writer and skips suspect reads while it is odd. z is whether any        *)
(* suspect record exists.                                                  *)
(*                                                                         *)
(* Each constant but Cancel switches off one ordering the code relies on;  *)
(* testdata/ holds a mutant config for each.                               *)
(***************************************************************************)
EXTENDS Naturals, FiniteSets

CONSTANTS
  Finishes,  \* the Finish calls
  Assumers,  \* the optimistic fills
  Closers,   \* the readers that may close the gate
  Writers,   \* the writers holding the store's writer
  Recheck,   \* assume checks the epoch again after registering
  RunCheck,  \* close requires that no Finish runs
  BumpFirst, \* a Finish opens the gate before it reads the assumers
  DecFirst,  \* release decrements before it reads the epoch
  Cancel     \* a waiting Finish may return when its context ends

VARIABLES
  epoch,    \* the gate's epoch
  running,  \* the Finish calls counted as running
  assumers, \* the registered assumers
  dopen,    \* a drained channel exists
  dgen,     \* the drained channels closed so far
  z,        \* a suspect record exists
  wlock,    \* the holder of the store's writer, or "none"
  fpc, fw,  \* each Finish's step and the channel it waits on
  apc, ae, re, \* each assumer's step, loaded epoch, and release's epoch
  cpc, ce,  \* each closer's step and loaded epoch
  wpc, we   \* each writer's step and loaded epoch

vars == <<epoch, running, assumers, dopen, dgen, z, wlock, fpc, fw,
          apc, ae, re, cpc, ce, wpc, we>>

Odd(n) == n % 2 = 1
Bumped(e) == IF Odd(e) THEN e + 1 ELSE e + 2

Init ==
  /\ epoch = 1
  /\ running = 0
  /\ assumers = 0
  /\ dopen = FALSE
  /\ dgen = 0
  /\ z = FALSE
  /\ wlock = "none"
  /\ fpc = [f \in Finishes |-> "idle"]
  /\ fw = [f \in Finishes |-> 0]
  /\ apc = [a \in Assumers |-> "idle"]
  /\ ae = [a \in Assumers |-> 0]
  /\ re = [a \in Assumers |-> 0]
  /\ cpc = [c \in Closers |-> "idle"]
  /\ ce = [c \in Closers |-> 0]
  /\ wpc = [w \in Writers |-> "idle"]
  /\ we = [w \in Writers |-> 0]

-----------------------------------------------------------------------------
(* Finish: marking, then writes, then the returned func.                    *)

FinishEnter(f) ==
  /\ fpc[f] = "idle"
  /\ running' = running + 1
  /\ epoch' = IF BumpFirst THEN Bumped(epoch) ELSE epoch
  /\ fpc' = [fpc EXCEPT ![f] = "check"]
  /\ UNCHANGED <<assumers, dopen, dgen, z, wlock, fw, apc, ae, re, cpc, ce, wpc, we>>

\* Under mu: proceed with no assumer, or wait on the drained channel.
FinishCheck(f) ==
  /\ fpc[f] = "check"
  /\ IF assumers = 0
       THEN /\ fpc' = [fpc EXCEPT ![f] = IF BumpFirst THEN "write" ELSE "open"]
            /\ UNCHANGED <<dopen, fw>>
       ELSE /\ fpc' = [fpc EXCEPT ![f] = "wait"]
            /\ dopen' = TRUE
            /\ fw' = [fw EXCEPT ![f] = dgen]
  /\ UNCHANGED <<epoch, running, assumers, dgen, z, wlock, apc, ae, re, cpc, ce, wpc, we>>

\* Without BumpFirst, the gate opens only after the assumers read zero.
FinishOpen(f) ==
  /\ fpc[f] = "open"
  /\ epoch' = Bumped(epoch)
  /\ fpc' = [fpc EXCEPT ![f] = "write"]
  /\ UNCHANGED <<running, assumers, dopen, dgen, z, wlock, fw, apc, ae, re, cpc, ce, wpc, we>>

FinishWake(f) ==
  /\ fpc[f] = "wait"
  /\ dgen > fw[f]
  /\ fpc' = [fpc EXCEPT ![f] = "check"]
  /\ UNCHANGED <<epoch, running, assumers, dopen, dgen, z, wlock, fw, apc, ae, re, cpc, ce, wpc, we>>

FinishCancel(f) ==
  /\ Cancel
  /\ fpc[f] = "wait"
  /\ epoch' = Bumped(epoch)
  /\ running' = running - 1
  /\ fpc' = [fpc EXCEPT ![f] = "done"]
  /\ UNCHANGED <<assumers, dopen, dgen, z, wlock, fw, apc, ae, re, cpc, ce, wpc, we>>

\* One Update of markSuspects or the clear, under the store's writer.
FinishWrite(f) ==
  /\ fpc[f] = "write"
  /\ wlock = "none"
  /\ z' \in BOOLEAN
  /\ UNCHANGED <<epoch, running, assumers, dopen, dgen, wlock, fpc, fw, apc, ae, re, cpc, ce, wpc, we>>

FinishEnd(f) ==
  /\ fpc[f] = "write"
  /\ epoch' = Bumped(epoch)
  /\ running' = running - 1
  /\ fpc' = [fpc EXCEPT ![f] = "done"]
  /\ UNCHANGED <<assumers, dopen, dgen, z, wlock, fw, apc, ae, re, cpc, ce, wpc, we>>

-----------------------------------------------------------------------------
(* An optimistic fill: assume, the attempt, release.                        *)

AssumeLoad(a) ==
  /\ apc[a] = "idle"
  /\ ae' = [ae EXCEPT ![a] = epoch]
  /\ apc' = [apc EXCEPT ![a] = IF Odd(epoch) THEN "loaded" ELSE "done"]
  /\ UNCHANGED <<epoch, running, assumers, dopen, dgen, z, wlock, fpc, fw, re, cpc, ce, wpc, we>>

AssumeRegister(a) ==
  /\ apc[a] = "loaded"
  /\ assumers' = assumers + 1
  /\ apc' = [apc EXCEPT ![a] = "reg"]
  /\ UNCHANGED <<epoch, running, dopen, dgen, z, wlock, fpc, fw, ae, re, cpc, ce, wpc, we>>

AssumeRecheck(a) ==
  /\ apc[a] = "reg"
  /\ apc' = [apc EXCEPT ![a] = IF Recheck /\ epoch # ae[a] THEN "rel" ELSE "valid"]
  /\ UNCHANGED <<epoch, running, assumers, dopen, dgen, z, wlock, fpc, fw, ae, re, cpc, ce, wpc, we>>

AssumeEnd(a) ==
  /\ apc[a] = "valid"
  /\ apc' = [apc EXCEPT ![a] = "rel"]
  /\ UNCHANGED <<epoch, running, assumers, dopen, dgen, z, wlock, fpc, fw, ae, re, cpc, ce, wpc, we>>

\* release: the decrement and the epoch load, in the order DecFirst sets.
ReleaseFirst(a) ==
  /\ apc[a] = "rel"
  /\ IF DecFirst
       THEN /\ assumers' = assumers - 1
            /\ UNCHANGED re
       ELSE /\ re' = [re EXCEPT ![a] = epoch]
            /\ UNCHANGED assumers
  /\ apc' = [apc EXCEPT ![a] = "rel2"]
  /\ UNCHANGED <<epoch, running, dopen, dgen, z, wlock, fpc, fw, ae, cpc, ce, wpc, we>>

ReleaseSecond(a) ==
  /\ apc[a] = "rel2"
  /\ LET n == IF DecFirst THEN assumers ELSE assumers - 1
         e == IF DecFirst THEN epoch ELSE re[a]
     IN /\ assumers' = n
        /\ apc' = [apc EXCEPT ![a] = IF n = 0 /\ ~Odd(e) THEN "wake" ELSE "done"]
  /\ UNCHANGED <<epoch, running, dopen, dgen, z, wlock, fpc, fw, ae, re, cpc, ce, wpc, we>>

\* Under mu: close the drained channel when one exists.
ReleaseWake(a) ==
  /\ apc[a] = "wake"
  /\ IF dopen
       THEN /\ dgen' = dgen + 1
            /\ dopen' = FALSE
       ELSE UNCHANGED <<dgen, dopen>>
  /\ apc' = [apc EXCEPT ![a] = "done"]
  /\ UNCHANGED <<epoch, running, assumers, z, wlock, fpc, fw, ae, re, cpc, ce, wpc, we>>

-----------------------------------------------------------------------------
(* close, shared by the closers and the writers' settle.                    *)

\* The running load and the compare-and-swap of close from e.
CloseRunning(e) == ~Odd(e) /\ (~RunCheck \/ running = 0)
CloseCAS(e) == epoch' = IF epoch = e THEN e + 1 ELSE epoch

CloserLoad(c) ==
  /\ cpc[c] = "idle"
  /\ ce' = [ce EXCEPT ![c] = epoch]
  /\ cpc' = [cpc EXCEPT ![c] = "snap"]
  /\ UNCHANGED <<epoch, running, assumers, dopen, dgen, z, wlock, fpc, fw, apc, ae, re, wpc, we>>

\* A snapshot opened after the load reads the count, and close loads running.
CloserSnap(c) ==
  /\ cpc[c] = "snap"
  /\ cpc' = [cpc EXCEPT ![c] = IF ~z THEN "run" ELSE "done"]
  /\ UNCHANGED <<epoch, running, assumers, dopen, dgen, z, wlock, fpc, fw, apc, ae, re, ce, wpc, we>>

CloserRun(c) ==
  /\ cpc[c] = "run"
  /\ cpc' = [cpc EXCEPT ![c] = IF CloseRunning(ce[c]) THEN "cas" ELSE "done"]
  /\ UNCHANGED <<epoch, running, assumers, dopen, dgen, z, wlock, fpc, fw, apc, ae, re, ce, wpc, we>>

CloserCAS(c) ==
  /\ cpc[c] = "cas"
  /\ CloseCAS(ce[c])
  /\ cpc' = [cpc EXCEPT ![c] = "done"]
  /\ UNCHANGED <<running, assumers, dopen, dgen, z, wlock, fpc, fw, apc, ae, re, ce, wpc, we>>

-----------------------------------------------------------------------------
(* A writer holding the store's writer, as withDirectOnce runs it.          *)

WriterLoad(w) ==
  /\ wpc[w] = "idle"
  /\ we' = [we EXCEPT ![w] = epoch]
  /\ wpc' = [wpc EXCEPT ![w] = "open"]
  /\ UNCHANGED <<epoch, running, assumers, dopen, dgen, z, wlock, fpc, fw, apc, ae, re, cpc, ce>>

\* Take the writer, then load the epoch again: skip suspect reads if closed.
WriterOpen(w) ==
  /\ wpc[w] = "open"
  /\ wlock = "none"
  /\ wlock' = w
  /\ wpc' = [wpc EXCEPT ![w] = IF Odd(epoch) THEN "skip" ELSE "read"]
  /\ UNCHANGED <<epoch, running, assumers, dopen, dgen, z, fpc, fw, apc, ae, re, cpc, ce, we>>

\* Commit, which may delete suspect records, and release the writer. A
\* commit that saw no suspect record goes on to close.
WriterCommit(w) ==
  /\ wpc[w] \in {"skip", "read"}
  /\ z' \in {z, FALSE}
  /\ wlock' = "none"
  /\ wpc' = [wpc EXCEPT ![w] = IF ~z' THEN "run" ELSE "done"]
  /\ UNCHANGED <<epoch, running, assumers, dopen, dgen, fpc, fw, apc, ae, re, cpc, ce, we>>

WriterRun(w) ==
  /\ wpc[w] = "run"
  /\ wpc' = [wpc EXCEPT ![w] = IF CloseRunning(we[w]) THEN "cas" ELSE "done"]
  /\ UNCHANGED <<epoch, running, assumers, dopen, dgen, z, wlock, fpc, fw, apc, ae, re, cpc, ce, we>>

WriterCAS(w) ==
  /\ wpc[w] = "cas"
  /\ CloseCAS(we[w])
  /\ wpc' = [wpc EXCEPT ![w] = "done"]
  /\ UNCHANGED <<running, assumers, dopen, dgen, z, wlock, fpc, fw, apc, ae, re, cpc, ce, we>>

-----------------------------------------------------------------------------

Done ==
  /\ \A f \in Finishes : fpc[f] = "done"
  /\ \A a \in Assumers : apc[a] = "done"
  /\ \A c \in Closers : cpc[c] = "done"
  /\ \A w \in Writers : wpc[w] = "done"
  /\ UNCHANGED vars

Next ==
  \/ \E f \in Finishes :
       FinishEnter(f) \/ FinishCheck(f) \/ FinishOpen(f) \/ FinishWake(f) \/ FinishCancel(f)
       \/ FinishWrite(f) \/ FinishEnd(f)
  \/ \E a \in Assumers :
       AssumeLoad(a) \/ AssumeRegister(a) \/ AssumeRecheck(a) \/ AssumeEnd(a)
       \/ ReleaseFirst(a) \/ ReleaseSecond(a) \/ ReleaseWake(a)
  \/ \E c \in Closers : CloserLoad(c) \/ CloserSnap(c) \/ CloserRun(c) \/ CloserCAS(c)
  \/ \E w \in Writers :
       WriterLoad(w) \/ WriterOpen(w) \/ WriterCommit(w) \/ WriterRun(w) \/ WriterCAS(w)
  \/ Done

Spec == Init /\ [][Next]_vars

-----------------------------------------------------------------------------
(* The gate's invariant: a reader skips suspect reads only while no suspect *)
(* record exists and no Finish may write one.                               *)

\* A closed gate proves no suspect record exists.
ClosedClean == Odd(epoch) => ~z

\* An assumer that skips suspect reads overlaps no Finish's writes.
AssumerSafe ==
  \A a \in Assumers : apc[a] = "valid" =>
    /\ ~z
    /\ \A f \in Finishes : fpc[f] # "write"

\* A writer that skips suspect reads sees none, and none appears while it
\* holds the writer.
WriterSafe == \A w \in Writers : wpc[w] = "skip" => ~z

\* running counts exactly the Finish calls between entry and return.
Counted == running = Cardinality({f \in Finishes : fpc[f] \in {"check", "wait", "open", "write"}})

=============================================================================
