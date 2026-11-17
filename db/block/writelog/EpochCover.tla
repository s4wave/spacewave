------------------------------ MODULE EpochCover ------------------------------
EXTENDS Integers, Sequences, FiniteSets
\* The write log's epochs and resume marker across process runs, against the
\* pack sweep's cover of one cycle.
\*
\* One store and one log directory serve a run of processes, one at a time.
\* A process of the current build boots its store, attaches the log as the
\* store's observer (Attach, or not), takes the resume marker (Claim), turns
\* the log on and off (TurnOn and TurnOff are Set with options and with nil;
\* TurnOn is startEpoch), takes writes, and stops with Close and its
\* markResumable step (Close, Mark), or crashes at any point (Crash). A process
\* of another build (Boot("old")), such as the image a deploy rolls back to or
\* an offline tool, writes the store and never touches the log. A deploy that
\* changes the build removes the marker durably first, the operating rule for
\* every process that does not claim.
\*
\* Disk: each epoch's lines, the prefix of them that is durable, the marker
\* and the position it names. A crash keeps any prefix of an epoch's lines at
\* least as long as its durable prefix; a marker renamed or removed without a
\* directory sync may come back or vanish. A failed removal of the marker, or
\* of its directory sync, poisons the epoch it names (retireMarker,
\* poisonEpoch). Injected I/O failures are bounded by MaxFaults.
\*
\* The sweep: one cycle takes the walk position R from the open epoch
\* (Position), reads the backend's status, then pulls the log from R and moves
\* a pack only when the pull found no gap and the status showed the log on in
\* R's epoch (Pull is pullGap, LogBlocking and coverSince in one step).
\*
\* Left out: a marker that does not parse and a torn line, each of which reads
\* as one with no close line; the segment cap, rotation and TrimLog, since
\* the reader walks one epoch's lines in order and a rotation only adds a
\* durable boundary that SyncSeg covers; an append that fails, which breaks
\* the epoch as Missed does; the double load of on in record, which logs a
\* write that races startEpoch rather than dropping it; replacing one observer
\* with another, which production never does. The removal of other epochs'
\* segments in startEpoch is taken as durable; a removed epoch that came back
\* would still end without the marker the new epoch's Claim took. The configs
\* inject one I/O failure: a directory that refuses both the marker's removal
\* and the poison byte keeps a marker that a crash after a miss resumes past,
\* which no write to that directory can prevent.
\*
\* Contracts the model assumes of its callers: the store takes no write
\* before Claim (EarlyWrites relaxes this for the attach window, without a
\* crash inside it; the mutant that allows the crash shows why the contract
\* holds); the log is turned on only with its observer attached; no write
\* reaches the store between Close and Mark.

CONSTANTS
  MaxWrites,  \* writes the store accepts in the whole run
  MaxRuns,    \* processes booted, one at a time
  MaxEpochs,  \* epochs the log starts
  MaxFaults,  \* injected I/O failures
  MaxOffs,    \* times the knob turns the log off
  EarlyWrites \* the store may take writes between boot and Claim

Epochs == 1..MaxEpochs
CLOSE == 0
POISON == -1
EPOCH == -2 \* the epoch line that opens each segment startEpoch writes
NoPos == <<0, 0>>
MaxLen == MaxWrites + 2 * (MaxRuns + MaxOffs) + 1
Early == IF EarlyWrites THEN {"booted", "attached"} ELSE {}

VARIABLES
  \* The log directory.
  lines,     \* each epoch's lines: write ids, CLOSE, POISON and EPOCH
  syn,       \* each epoch's durable prefix length
  alive,     \* epochs whose segments exist
  marker,    \* "none", "durable", "volatile" (may vanish), "ghost" (may return)
  mPos,      \* the position the marker names: <<epoch, seq>>
  closeAcc,  \* writes the store had accepted at each epoch's last close line
  \* The store and its deploys.
  acc,       \* writes accepted, numbered 1..acc in order
  lastKind,  \* the build last deployed
  run,       \* processes booted
  nEpochs,   \* epochs started
  faults,    \* injected I/O failures so far
  offs,      \* knob-offs so far
  \* The running process.
  up, kind, stage,
  attached,  \* the log is the store's observer
  unseen,    \* the store took a write with no observer (observerSlot)
  claimed,   \* Claim took the directory's marker
  missed,    \* Log.missed
  cur,       \* the epoch with an open file; 0 when off
  closedPos, \* Log.closed
  resumable, \* Log.resumable
  unsynced,  \* Log.unsynced
  \* One sweep cycle.
  sw, rE, rS, rAcc, stOn, stE, pAcc, pulled, moved,
  \* History for the checks.
  torn,      \* epochs a crash left open or cut short
  resE,      \* the epoch this process's last TurnOn resumed, else 0
  bootAcc, bootFaults, \* acc and faults when the process booted
  cleanRun,  \* Close found the epoch closable with nothing missed
  prevClean, \* the previous process stopped cleanly
  carry,     \* this process follows a clean stop of the same build
  firstOn,   \* no TurnOn yet in this process
  wasted     \* a clean restart started a new epoch

diskv == <<lines, syn, alive, marker, mPos, closeAcc>>
storev == <<acc, lastKind, run, nEpochs, faults, offs>>
procv == <<up, kind, stage, attached, unseen, claimed, missed, cur, closedPos,
           resumable, unsynced>>
sweepv == <<sw, rE, rS, rAcc, stOn, stE, pAcc, pulled, moved>>
histv == <<torn, resE, bootAcc, bootFaults, cleanRun, prevClean, carry,
           firstOn, wasted>>
vars == <<diskv, storev, procv, sweepv, histv>>

\* The seq of line i of l: the writes up to it.
SeqAt(l, i) == Cardinality({j \in 1..i : l[j] > 0})
NWrites(l) == SeqAt(l, Len(l))
Last(l) == l[Len(l)]

\* closedAt: the epoch's last line is the close line at the position's seq.
ClosedIn(ls, al, p) ==
  /\ p[1] \in al
  /\ Len(ls[p[1]]) > 0
  /\ Last(ls[p[1]]) = CLOSE
  /\ NWrites(ls[p[1]]) = p[2]

\* The reader stops at a close line that ends the epoch, or that the poison
\* byte follows.
EndsClosed(l) ==
  \/ Len(l) > 0 /\ Last(l) = CLOSE
  \/ Len(l) > 1 /\ Last(l) = POISON /\ l[Len(l) - 1] = CLOSE

\* poisonEpoch: append a byte to epoch e's last segment and sync it.
PoisonLines(e) ==
  IF e \in alive THEN [lines EXCEPT ![e] = Append(@, POISON)] ELSE lines
PoisonSyn(e) ==
  IF e \in alive THEN [syn EXCEPT ![e] = Len(lines[e]) + 1] ELSE syn

\* retireMarker with the marker in state m0, naming epoch e: each outcome's
\* marker, lines, durable prefixes, whether it reports the removal durable,
\* and the faults it spends. One absent reads as removed (nil, true).
Retire(m0, e) ==
  IF m0 \in {"none", "ghost"}
  THEN {[m |-> m0, ls |-> lines, sy |-> syn, ok |-> TRUE, f |-> 0]}
  ELSE {[m |-> "none", ls |-> lines, sy |-> syn, ok |-> TRUE, f |-> 0]}
       \cup (IF faults < MaxFaults
             THEN {[m |-> x, ls |-> PoisonLines(e), sy |-> PoisonSyn(e), ok |-> TRUE, f |-> 1] :
                     x \in {m0, "ghost"}}
             ELSE {})
       \cup (IF faults + 1 < MaxFaults
             THEN {[m |-> x, ls |-> lines, sy |-> syn, ok |-> FALSE, f |-> 2] :
                     x \in {m0, "ghost"}}
             ELSE {})

Apply(r) ==
  /\ marker' = r.m /\ lines' = r.ls /\ syn' = r.sy /\ faults' = faults + r.f

\* Claim's takeMarker.
ClaimRetire == Retire(marker, mPos[1])

\* Whether pullGap reports a close line that ends the epoch.
ClosedGap == TRUE

\* Whether the backend's stop detaches the observer before Close.
DetachFirst == FALSE

\* A crash can strike anywhere but inside the early window it relaxes.
CrashOK == ~(kind = "new" /\ stage \in Early)

Init ==
  /\ lines = [e \in Epochs |-> <<>>] /\ syn = [e \in Epochs |-> 0]
  /\ alive = {} /\ marker = "none" /\ mPos = NoPos
  /\ closeAcc = [e \in Epochs |-> 0]
  /\ acc = 0 /\ lastKind = "new" /\ run = 0 /\ nEpochs = 0 /\ faults = 0
  /\ offs = 0
  /\ up = FALSE /\ kind = "new" /\ stage = "down"
  /\ attached = FALSE /\ unseen = FALSE /\ claimed = FALSE /\ missed = FALSE
  /\ cur = 0 /\ closedPos = NoPos /\ resumable = FALSE /\ unsynced = FALSE
  /\ sw = "idle" /\ rE = 0 /\ rS = 0 /\ rAcc = 0 /\ stOn = FALSE /\ stE = 0
  /\ pAcc = 0 /\ pulled = {} /\ moved = FALSE
  /\ torn = {} /\ resE = 0 /\ bootAcc = 0 /\ bootFaults = 0
  /\ cleanRun = FALSE /\ prevClean = FALSE /\ carry = FALSE
  /\ firstOn = FALSE /\ wasted = FALSE

\* A process of build k starts. A deploy that changes the build removes the
\* marker durably before it starts the image.
Boot(k) ==
  /\ ~up /\ run < MaxRuns
  /\ up' = TRUE /\ kind' = k /\ run' = run + 1 /\ stage' = "booted"
  /\ attached' = FALSE /\ unseen' = FALSE /\ claimed' = FALSE
  /\ missed' = FALSE /\ cur' = 0 /\ closedPos' = NoPos
  /\ resumable' = FALSE /\ unsynced' = FALSE
  /\ marker' = IF k # lastKind THEN "none" ELSE marker
  /\ lastKind' = k
  /\ carry' = (prevClean /\ k = lastKind) /\ prevClean' = FALSE
  /\ bootAcc' = acc /\ bootFaults' = faults /\ firstOn' = TRUE
  /\ cleanRun' = FALSE
  /\ UNCHANGED <<lines, syn, alive, mPos, closeAcc, acc, nEpochs, faults,
                 offs, sweepv, torn, resE, wasted>>

\* SetObserver(store, log): a write the store took with no observer counts
\* as missed.
Attach ==
  /\ up /\ kind = "new" /\ stage = "booted"
  /\ stage' = "attached" /\ attached' = TRUE
  /\ missed' = (missed \/ unseen)
  /\ UNCHANGED <<diskv, storev, up, kind, unseen, claimed, cur, closedPos,
                 resumable, unsynced, sweepv, histv>>

\* Claim: take the marker durably and hold its position; claimed with no
\* observer, resume is off.
Claim ==
  /\ up /\ kind = "new" /\ stage \in {"booted", "attached"}
  /\ stage' = "claimed" /\ claimed' = TRUE
  /\ \E r \in ClaimRetire :
       /\ Apply(r)
       /\ closedPos' = mPos
       /\ resumable' = (marker \in {"durable", "volatile"} /\ r.ok
                        /\ ClosedIn(r.ls, alive, mPos))
  /\ missed' = (missed \/ ~attached)
  /\ UNCHANGED <<alive, mPos, closeAcc, acc, lastKind, run, nEpochs, offs, up,
                 kind, attached, unseen, cur, unsynced, sweepv, histv>>

\* The store accepts write acc+1 and calls the observer: a line in the open
\* epoch, or a miss that retires the marker once.
Write ==
  /\ up /\ acc < MaxWrites
  /\ kind = "old" \/ stage = "claimed" \/ stage \in Early
  /\ acc' = acc + 1
  /\ CASE kind = "old" ->
            UNCHANGED <<lines, syn, marker, faults, unseen, missed>>
       [] ~attached ->
            /\ unseen' = TRUE
            /\ UNCHANGED <<lines, syn, marker, faults, missed>>
       [] cur # 0 ->
            /\ lines' = [lines EXCEPT ![cur] = Append(@, acc + 1)]
            /\ UNCHANGED <<syn, marker, faults, unseen, missed>>
       [] missed ->
            UNCHANGED <<lines, syn, marker, faults, unseen, missed>>
       [] OTHER ->
            /\ missed' = TRUE
            /\ IF claimed THEN \E r \in Retire(marker, mPos[1]) : Apply(r)
               ELSE UNCHANGED <<lines, syn, marker, faults>>
            /\ UNCHANGED unseen
  /\ UNCHANGED <<alive, mPos, closeAcc, lastKind, run, nEpochs, offs, up, kind,
                 stage,
                 attached, claimed, cur, closedPos, resumable, unsynced,
                 sweepv, histv>>

\* Set with a directory: startEpoch retires the marker again, then resumes
\* the claimed epoch when it closed there and nothing was missed, or starts a
\* new epoch and removes every other epoch's segments.
TurnOn ==
  /\ up /\ stage = "claimed" /\ attached /\ cur = 0
  /\ \E r \in Retire(marker, mPos[1]) :
       LET resume == resumable /\ r.ok /\ ~missed /\ ClosedIn(r.ls, alive, closedPos)
           e == IF resume THEN closedPos[1] ELSE nEpochs + 1
       IN /\ resume \/ nEpochs < MaxEpochs
          /\ marker' = r.m /\ syn' = r.sy /\ faults' = faults + r.f
          /\ lines' = [r.ls EXCEPT ![e] = Append(@, EPOCH)]
          /\ alive' = {e}
          /\ nEpochs' = IF resume THEN nEpochs ELSE e
          /\ cur' = e
          /\ resE' = IF resume THEN e ELSE 0
          /\ wasted' = (wasted \/ (~resume /\ firstOn /\ carry
                                   /\ acc = bootAcc /\ faults + r.f = bootFaults))
  /\ missed' = FALSE /\ resumable' = FALSE /\ firstOn' = FALSE
  /\ UNCHANGED <<mPos, closeAcc, acc, lastKind, run, offs, up, kind, stage,
                 attached,
                 unseen, claimed, closedPos, unsynced, sweepv, torn, bootAcc,
                 bootFaults, cleanRun, prevClean, carry>>

\* closeEpoch: the close line, and the position it names.
CloseEpoch ==
  /\ lines' = [lines EXCEPT ![cur] = Append(@, CLOSE)]
  /\ closedPos' = <<cur, NWrites(lines[cur])>>
  /\ closeAcc' = [closeAcc EXCEPT ![cur] = acc]
  /\ resumable' = TRUE /\ cur' = 0

\* Set(nil): close the epoch and leave no marker.
TurnOff ==
  /\ up /\ stage = "claimed" /\ cur # 0 /\ offs < MaxOffs
  /\ offs' = offs + 1
  /\ CloseEpoch
  /\ UNCHANGED <<syn, alive, marker, mPos, acc, lastKind, run, nEpochs, faults,
                 up, kind, stage, attached,
                 unseen, claimed, missed, unsynced, sweepv, histv>>

\* The backend stops: Close's closeEpoch, after the store took its last
\* write. A detach first calls Missed, which breaks the open epoch.
Close ==
  /\ up /\ kind = "new" /\ stage = "claimed"
  /\ stage' = "closing"
  /\ cleanRun' = (~missed /\ faults = bootFaults /\ (cur # 0 \/ resumable))
  /\ IF DetachFirst
     THEN /\ missed' = TRUE /\ cur' = 0
          /\ IF ~missed THEN \E r \in Retire(marker, mPos[1]) : Apply(r)
             ELSE UNCHANGED <<lines, syn, marker, faults>>
          /\ UNCHANGED <<closedPos, closeAcc, resumable>>
     ELSE /\ IF cur # 0 THEN CloseEpoch
             ELSE UNCHANGED <<lines, closedPos, closeAcc, resumable, cur>>
          /\ UNCHANGED <<missed, syn, marker, faults>>
  /\ UNCHANGED <<alive, mPos, acc, lastKind, run, nEpochs, offs, up, kind,
                 attached,
                 unseen, claimed, unsynced, sweepv, torn, resE, bootAcc,
                 bootFaults, prevClean, carry, firstOn, wasted>>

\* markResumable: retire the marker after a miss; otherwise wait for every
\* segment's sync and write the marker when the epoch still ends at its close
\* line. A failed write retires what it may have renamed.
Mark ==
  /\ up /\ stage = "closing"
  /\ stage' = "closed"
  /\ LET e == closedPos[1] IN
     \/ /\ missed
        /\ \E r \in Retire(marker, mPos[1]) : Apply(r)
        /\ UNCHANGED <<mPos, unsynced>>
     \/ /\ ~missed /\ ~resumable
        /\ UNCHANGED <<lines, syn, marker, mPos, faults, unsynced>>
     \/ /\ ~missed /\ resumable
        /\ syn' = [syn EXCEPT ![e] = Len(lines[e])]
        /\ IF unsynced \/ ~ClosedIn(lines, alive, closedPos)
           THEN UNCHANGED <<marker, mPos>>
           ELSE marker' = "durable" /\ mPos' = closedPos
        /\ UNCHANGED <<lines, faults, unsynced>>
     \/ /\ ~missed /\ resumable /\ faults < MaxFaults
        /\ unsynced' = TRUE /\ faults' = faults + 1
        /\ UNCHANGED <<lines, syn, marker, mPos>>
     \/ /\ ~missed /\ resumable /\ faults < MaxFaults
        /\ ~unsynced /\ ClosedIn(lines, alive, closedPos)
        /\ \/ /\ faults' = faults + 1
              /\ syn' = [syn EXCEPT ![e] = Len(lines[e])]
              /\ UNCHANGED <<lines, marker>>
           \/ \E r \in Retire("volatile", e) :
                /\ faults + 1 + r.f <= MaxFaults
                /\ marker' = r.m /\ lines' = r.ls
                /\ syn' = [r.sy EXCEPT ![e] = Len(r.ls[e])]
                /\ faults' = faults + 1 + r.f
        /\ mPos' = closedPos
        /\ UNCHANGED unsynced
  /\ UNCHANGED <<alive, closeAcc, acc, lastKind, run, nEpochs, offs, up, kind,
                 attached, unseen, claimed, missed, cur, closedPos, resumable,
                 sweepv, histv>>

\* A segment's background sync completes or fails.
SyncSeg(e) ==
  /\ up /\ kind = "new" /\ syn[e] < Len(lines[e])
  /\ \/ /\ syn' = [syn EXCEPT ![e] = Len(lines[e])]
        /\ UNCHANGED <<faults, unsynced>>
     \/ /\ faults < MaxFaults /\ faults' = faults + 1 /\ unsynced' = TRUE
        /\ UNCHANGED syn
  /\ UNCHANGED <<lines, alive, marker, mPos, closeAcc, acc, lastKind, run,
                 nEpochs, offs, up, kind, stage, attached, unseen, claimed, missed,
                 cur, closedPos, resumable, sweepv, histv>>

\* The process exits after Close; another build's process exits at will.
Exit ==
  /\ up /\ (stage = "closed" \/ kind = "old")
  /\ up' = FALSE /\ stage' = "down"
  /\ prevClean' = (kind = "new" /\ cleanRun /\ faults = bootFaults)
  /\ UNCHANGED <<diskv, storev, kind, attached, unseen, claimed, missed, cur,
                 closedPos, resumable, unsynced, sweepv, torn, resE, bootAcc,
                 bootFaults, cleanRun, carry, firstOn, wasted>>

\* A crash keeps each epoch's durable prefix and any part of the rest, and
\* settles each unsynced marker rename or removal either way.
Cuts == {f \in [Epochs -> 0..MaxLen] :
           \A e \in Epochs : syn[e] <= f[e] /\ f[e] <= Len(lines[e])}
AfterCrash(m) == IF m \in {"volatile", "ghost"} THEN {"none", "durable"} ELSE {m}

Crash ==
  /\ up /\ CrashOK
  /\ \E f \in Cuts :
       /\ lines' = [e \in Epochs |-> SubSeq(lines[e], 1, f[e])]
       /\ torn' = torn \cup {e \in Epochs : f[e] < Len(lines[e])}
                       \cup (IF cur # 0 THEN {cur} ELSE {})
  /\ marker' \in AfterCrash(marker)
  /\ up' = FALSE /\ stage' = "down" /\ cur' = 0 /\ prevClean' = FALSE
  /\ resE' = 0
  /\ UNCHANGED <<syn, alive, mPos, closeAcc, storev, kind, attached, unseen,
                 claimed, missed, closedPos, resumable, unsynced, sweepv,
                 bootAcc, bootFaults, cleanRun, carry, firstOn, wasted>>

\* The sweep takes R from the open epoch.
Walk ==
  /\ sw = "idle" /\ up /\ cur # 0
  /\ sw' = "walked" /\ rE' = cur /\ rS' = NWrites(lines[cur]) /\ rAcc' = acc
  /\ UNCHANGED <<diskv, storev, procv, stOn, stE, pAcc, pulled, moved, histv>>

\* It reads the backend's status: the log on, and its epoch.
Status ==
  /\ sw = "walked" /\ up
  /\ sw' = "status" /\ stOn' = (cur # 0) /\ stE' = cur
  /\ UNCHANGED <<diskv, storev, procv, rE, rS, rAcc, pAcc, pulled, moved, histv>>

\* It pulls R's epoch from R and moves a pack when nothing blocks: the epoch
\* exists, it reads open (live) rather than closed or ended, and the status
\* showed it on.
Pull ==
  /\ sw = "status"
  /\ LET l == lines[rE]
         live == up /\ cur = rE
         gap == \/ rE \notin alive
                \/ EndsClosed(l) /\ ClosedGap
                \/ ~EndsClosed(l) /\ ~live
     IN /\ pulled' = {l[i] : i \in {j \in 1..Len(l) : l[j] > 0 /\ SeqAt(l, j) > rS}}
        /\ moved' = (~gap /\ stOn /\ stE = rE)
  /\ sw' = "done" /\ pAcc' = acc
  /\ UNCHANGED <<diskv, storev, procv, rE, rS, rAcc, stOn, stE, histv>>

Done ==
  /\ ~up /\ run = MaxRuns
  /\ UNCHANGED vars

Next ==
  \/ \E k \in {"new", "old"} : Boot(k)
  \/ Attach \/ Claim \/ Write \/ TurnOn \/ TurnOff \/ Close \/ Mark \/ Exit
  \/ Crash
  \/ \E e \in Epochs : SyncSeg(e)
  \/ Walk \/ Status \/ Pull
  \/ Done

Spec == Init /\ [][Next]_vars

TypeOK ==
  /\ marker \in {"none", "durable", "volatile", "ghost"}
  /\ alive \subseteq Epochs
  /\ cur \in 0..MaxEpochs
  /\ faults \in 0..MaxFaults

\* I1: a pack moves only when the cycle's pull read every write the store
\* accepted after R.
Cover == moved => \A w \in (rAcc + 1)..pAcc : w \in pulled

\* I2: wherever a crash can strike, a marker that could survive it names an
\* epoch that is gone, poisoned durably, or durable through the close line it
\* names with no write accepted since.
DurableLines(e) == SubSeq(lines[e], 1, syn[e])
MarkerSound ==
  (marker # "none" /\ (up => CrashOK)) =>
    LET e == mPos[1] IN
      \/ e \notin alive
      \/ \E i \in 1..Len(DurableLines(e)) : DurableLines(e)[i] = POISON
      \/ /\ ClosedIn([lines EXCEPT ![e] = DurableLines(e)], alive, mPos)
         /\ acc = closeAcc[e]

\* I3: a resume continues only an epoch no crash left open or cut short.
FreshAfterCrash == resE # 0 => resE \notin torn

\* A clean stop and restart of the same build, with no write or fault
\* between, resumes.
ResumeWhenClean == ~wasted
=============================================================================
