---- MODULE DeadlistLedger ----
\* Ledger adoption, reader watermarks, save generations and staged preparation.
\* An entry remains waiting through page receipts, pure encoding and bounded
\* stage writes. Resolve attaches only a ready result under the writer lock;
\* Advance stops at the first unresolved entry, even after adoption/pin release.
\* One symbolic waiting source represents From and its dropped-span closure.
\* The base hold and ready names transfer in the same atomic save, so rejected
\* or failed saves leave the prior durable hold and retryable ledger lease.
\*
\* A stage has a worker lease during I/O, a ledger lease until confirmation or
\* orphan/cancellation, and one lease per accepted naming save until its actual
\* receipt Done. Orphaning/ageout cancels the job; an in-flight atomic write may
\* still return under its worker lease. Prepared bytes transfer into durable
\* named ownership at LandSave; puts abstracts that transfer, not a record copy.
\* Stage token sets and sticky contract booleans are proof history, not Go fields.
\*
\* Carry/saved, SubmitAtomic and admission lease acquisition hold writeMtx, so
\* no ledger transition interleaves before Admit or Reject. Publication.After
\* rejects successors of failed saves. CancelEngine drops local ownership but
\* not admitted work. Restore waits for Accepted to drain, as executeWorld calls
\* lockSettled before replay.load; a late worker may return after the new entry
\* reuses its sequence, but entry/net identity and engine lifetime reject attach.
\*
\* The default retains the original one-record/three-save bound. Preparation
\* and chain configs add two records/one save with MaxPending one/two. Ageout
\* uses this smaller bound instead of the source's64. Negative configs under
\* testdata are checked explicitly, outside discovery. Pure encoding errors
\* may stutter; preparation I/O failure forfeits via move/lost and releases its
\* worker/ledger leases without propagating background I/O into a foreground save. Count funding and part contents belong to
\* IntervalCounts and DeadlistPreparation, not this resource-lifetime model.
\* Save IDs stay globally unique across one restart; Go's reset is equivalent
\* because old admissions drain first and callback identity is explicit here.
EXTENDS Naturals, FiniteSets, TLC
CONSTANTS ResetOnUnsave, ConfirmGeneration, CarryBlocks, MaxRecords, MaxSaves,
          ScanOrphans, MatchParent, BreakForfeit, RestoreAdopted, RestoreNext,
          MaxPending, AgeOut, ReadyBeforeName, HoldWaiting, ReceiptLease,
          KeepReceiptLease, MatchCompletion, BlockWaiting, KeepWorkerLease, ReleaseDone, ReleaseFailedJob, RequestFailedRecount, LoseReadyOrphans, LoseFailedNames
Records == 1..MaxRecords
Saves == 1..MaxSaves
None == "none"
VARIABLES status, carried, stored, durable, adopted, pin, gen, flight, snapshot, puts,
          asks, recount, recountGen, lostAt, covered, next, watermarkOK, parent,
          nameLost, savedCursor, driveCursor, landed, rebuilding, restarted, applied,
          used, seq, savedNext, savedAdopted, driveNext, driveAdopted, adoptOK, restoreOK, orphanOK
coreVars == <<status, carried, stored, durable, adopted, pin, gen, flight, snapshot, puts,
      asks, recount, recountGen, lostAt, covered, next, watermarkOK, parent,
      nameLost, savedCursor, driveCursor, landed, rebuilding, restarted, applied,
      used, seq, savedNext, savedAdopted, driveNext, driveAdopted, adoptOK, restoreOK, orphanOK>>
VARIABLES preparation, worker, cancelled, entryEngine, stage, acquired, released,
          receipt, receiptEngine, receiptAfter, admission, stopped, waitingOK, stageOK, leaseOK, completionOK, orphanLossOK, failedNameLossOK, knownStored, failedNameRecountOK
prepVars == <<preparation, worker, cancelled, entryEngine, stage, acquired, released,
              receipt, receiptEngine, receiptAfter, admission, stopped, waitingOK, stageOK, leaseOK, completionOK, orphanLossOK, failedNameLossOK, knownStored, failedNameRecountOK>>
vars == <<coreVars, prepVars>>
SaveToken(g) == ToString(g)
Tokens == {"ledger", "job"} \cup {SaveToken(g) : g \in Saves}
Pending == {r \in Records : status[r] \in {"waiting", "pending", "forfeited"}}
Oldest == IF Pending = {} THEN 0 ELSE
  CHOOSE r \in Pending : \A q \in Pending : seq[r] <= seq[q]
Evicted == IF AgeOut /\ Cardinality(Pending) >= MaxPending THEN {Oldest} ELSE {}
Accepted == {g \in Saves : receipt[g] = "accepted"}
ReceiptTokens == {SaveToken(g) : g \in Accepted}
\* puts is the immutable stage-resource set captured before admission. It
\* remains needed through Done even if an earlier name becomes durable.
NewNames(g) == puts[g]
Waiting == {r \in Records : status[r] = "waiting"}
\* One symbolic source bundles From and the dropped-span closure for an
\* unresolved entry. Every saved base hold includes them until ready naming;
\* a failed save leaves the previous durable hold intact.
WaitingSourcesForSave == IF HoldWaiting THEN Waiting ELSE {}
Owned(r) == stage[r] # {} \/ r \in stored
LastFlight == IF flight = {} THEN 0 ELSE CHOOSE g \in flight : \A h \in flight : h <= g
DropLedger(rs) ==
  /\ stage' = [r \in Records |-> IF r \in rs THEN stage[r] \ {"ledger"} ELSE stage[r]]
  /\ released' = [r \in Records |-> IF r \in rs THEN released[r] \cup (stage[r] \cap {"ledger"}) ELSE released[r]]
DropReceipt(g) ==
  /\ stage' = [r \in Records |-> IF ReleaseDone THEN stage[r] \ {SaveToken(g)} ELSE stage[r]]
  /\ released' = [r \in Records |-> IF ReleaseDone THEN released[r] \cup (stage[r] \cap {SaveToken(g)}) ELSE released[r]]
InitPrep ==
  /\ preparation = [r \in Records |-> "none"]
  /\ worker = [r \in Records |-> "idle"] /\ cancelled = {}
  /\ entryEngine = [r \in Records |-> FALSE]
  /\ stage = [r \in Records |-> {}]
  /\ acquired = [r \in Records |-> {}] /\ released = [r \in Records |-> {}]
  /\ receipt = [g \in Saves |-> "none"]
  /\ receiptEngine = [g \in Saves |-> FALSE]
  /\ receiptAfter = [g \in Saves |-> 0] /\ admission = 0 /\ stopped = FALSE
  /\ waitingOK = TRUE /\ stageOK = TRUE /\ leaseOK = TRUE /\ completionOK = TRUE /\ orphanLossOK = TRUE /\ failedNameLossOK = TRUE /\ knownStored = {} /\ failedNameRecountOK = TRUE
InitLedger ==
  /\ status = [r \in Records |-> "absent"]
  /\ carried = [r \in Records |-> 0] /\ stored = {}
  /\ durable = [r \in Records |-> None]
  /\ adopted = 0 /\ pin = 0 /\ gen = 0 /\ flight = {}
  /\ snapshot = [g \in Saves |-> [r \in Records |-> None]]
  /\ puts = [g \in Saves |-> {}] /\ asks = [g \in Saves |-> 0]
  /\ recount = FALSE /\ recountGen = 0 /\ lostAt = 0 /\ covered = 0
  /\ next = 1 /\ watermarkOK = TRUE
  /\ parent = [r \in Records |-> 0] /\ nameLost = [r \in Records |-> 0]
  /\ savedCursor = [g \in Saves |-> [r \in Records |-> "absent"]]
  /\ driveCursor = [r \in Records |-> "absent"]
  /\ landed = {} /\ rebuilding = FALSE /\ restarted = FALSE /\ applied = {}
  /\ used = {} /\ seq = [r \in Records |-> 0]
  /\ savedNext = [g \in Saves |-> 1] /\ savedAdopted = [g \in Saves |-> 0]
  /\ driveNext = 1 /\ driveAdopted = 0
  /\ adoptOK = TRUE /\ restoreOK = TRUE /\ orphanOK = TRUE
Init == InitLedger /\ InitPrep
\* Operational loss detection mirrors move(orphan), including ready records
\* neither a durable name nor an admitted save carries yet.
LosesRecord(r) == status[r] = "waiting" \/
  (LoseReadyOrphans /\ status[r] = "pending" /\ r \notin knownStored /\ carried[r] = 0)
\* Independent oracle derives required loss from prestate ownership and the
\* actual orphaned result, not LosesRecord or the operational orphan set.
RequiredOrphanLoss == \E r \in Records :
  /\ status'[r] = "orphaned" /\ status[r] \in {"waiting", "pending"}
  /\ (status[r] = "waiting" \/ (r \notin knownStored /\ carried[r] = 0))
OrphanLossKept == ~RequiredOrphanLoss \/
  (recount' /\ recountGen' = 0 /\ lostAt' > lostAt)
\* knownStored is the source entry.stored bit, unlike stored (durable bytes).
\* Resolve attaches record before its first name; immediate orphaning clears
\* an uncarried attachment, while an admitted name keeps it through failure.
OrphanPreparation(r) ==
  IF status[r] = "waiting" \/ (r \notin knownStored /\ carried[r] = 0)
  THEN "none" ELSE preparation[r]
NeedsFailedNameLoss == \E r \in Records :
  status[r] = "orphaned" /\ preparation[r] = "attached" /\ r \notin knownStored
\* The source caller invokes unsave on a failed receipt. Keep this observable
\* recount oracle separate from the stronger per-call loss-history oracle, so
\* an omission trace cannot be a premature abstract unsave or idempotent call.
FailedNameRecountKept ==
  (NeedsFailedNameLoss /\ (\E g \in flight : receipt[g] = "failure")) =>
    (recount' /\ recountGen' = 0)
FailedNameLossKept == ~NeedsFailedNameLoss \/
  (recount' /\ recountGen' = 0 /\ lostAt' > lostAt)
NameLedger(q, net) ==
  /\ Cardinality(used) < MaxRecords /\ q \in used \cup {0}
  /\ (~net \/ (\E r \in Evicted : LosesRecord(r))) => lostAt < 2
  /\ status' = [r \in Records |-> IF r = Cardinality(used) + 1 THEN IF net THEN "waiting" ELSE "forfeited"
        ELSE IF r \in Evicted THEN "orphaned" ELSE status[r]] /\ next' = next + 1
  /\ carried' = [r \in Records |-> IF r \in Evicted THEN 0 ELSE carried[r]]
  /\ used' = used \cup {Cardinality(used) + 1}
  /\ seq' = [seq EXCEPT ![Cardinality(used) + 1] = next]
  /\ parent' = [parent EXCEPT ![Cardinality(used) + 1] = q]
  /\ nameLost' = [nameLost EXCEPT ![Cardinality(used) + 1] = lostAt]
  /\ LET loses == ~net \/ (\E r \in Evicted : LosesRecord(r))
     IN /\ recount' = IF loses THEN TRUE ELSE recount
        /\ recountGen' = IF loses THEN 0 ELSE recountGen
        /\ lostAt' = IF loses THEN lostAt + 1 ELSE lostAt
  /\ UNCHANGED <<stored, durable, adopted, pin, gen, flight, snapshot, puts, asks,
      covered, watermarkOK, savedCursor, driveCursor, landed, rebuilding,
      restarted, applied, savedNext, savedAdopted, driveNext, driveAdopted, adoptOK, restoreOK, orphanOK>>
ResolveLedger(r) ==
  /\ status[r] = "waiting"
  /\ status' = [status EXCEPT ![r] = "pending"]
  /\ UNCHANGED <<carried, stored, durable, adopted, pin, gen, flight, snapshot, puts, asks,
      recount, recountGen, lostAt, covered, next, watermarkOK, parent, nameLost,
      savedCursor, driveCursor, landed, rebuilding, restarted, applied, used, seq,
      savedNext, savedAdopted, driveNext, driveAdopted, adoptOK, restoreOK, orphanOK>>
\* One lost event may follow an earlier carried recount: older confirmation
\* must not clear the newer request. Two events suffice for that race.
Lost ==
  /\ lostAt < 2
  /\ lostAt' = lostAt + 1 /\ recount' = TRUE /\ recountGen' = 0
  /\ UNCHANGED <<status, carried, stored, durable, adopted, pin, gen, flight, snapshot, puts,
      asks, covered, next, watermarkOK, parent, nameLost, savedCursor,
      driveCursor, landed, rebuilding, restarted, applied, used, seq, savedNext,
      savedAdopted, driveNext, driveAdopted, adoptOK, restoreOK, orphanOK>>
Prev(r) == LET ps == {q \in Pending : seq[q] < seq[r]}
           IN IF ps = {} THEN 0 ELSE CHOOSE q \in ps : \A x \in ps : seq[x] <= seq[q]
RECURSIVE Back(_)
Back(r) == {r} \cup IF Prev(r) # 0 /\ (BreakForfeit => status[r] # "forfeited")
                   /\ (MatchParent => parent[r] = Prev(r))
                   THEN Back(Prev(r)) ELSE {}
\* Contract oracle: every link in the pending sequence range must match.
\* It does not call Back or use the operational orphan set.
OnChain(q, r) == q \in Pending /\ seq[q] <= seq[r] /\
  \A m \in Pending : (seq[q] < seq[m] /\ seq[m] <= seq[r]) =>
    status[m] # "forfeited" /\ parent[m] = Prev(m)
AdoptLedger(r) ==
  /\ r \in Pending /\ seq[r] > adopted
  /\ LET chain == Back(r)
         first == CHOOSE q \in chain : \A x \in chain : seq[q] <= seq[x]
         orphan == IF ScanOrphans
                   THEN {q \in Pending : seq[q] < seq[first] /\ seq[q] > adopted}
                   ELSE {}
         loses == \E q \in orphan : LosesRecord(q)
     IN /\ loses => lostAt < 2
        /\ adopted' = seq[r]
        /\ status' = [q \in Records |-> IF q \in orphan THEN "orphaned" ELSE status[q]]
        /\ carried' = [q \in Records |-> IF q \in orphan THEN 0 ELSE carried[q]]
        /\ recount' = IF loses THEN TRUE ELSE recount
        /\ recountGen' = IF loses THEN 0 ELSE recountGen
        /\ lostAt' = IF loses THEN lostAt + 1 ELSE lostAt
  /\ adoptOK' = (adoptOK /\ adopted' = seq[r] /\ \A q \in Pending : seq[q] <= seq[r] =>
       ((seq[q] <= adopted \/ OnChain(q, r)) <=> status'[q] # "orphaned"))
  /\ UNCHANGED <<stored, durable, pin, gen, flight, snapshot, puts, asks, covered, next,
      watermarkOK, parent, nameLost, savedCursor, driveCursor, landed,
      rebuilding, restarted, applied, used, seq, savedNext, savedAdopted,
      driveNext, driveAdopted, restoreOK, orphanOK>>
\* The oldest reader is pinned at epoch zero; releasing it opens the watermark.
\* A later epoch changes only the numeric bound, checked by DueWatermark.
ReleasePin ==
  /\ pin = 0 /\ pin' = 3
  /\ UNCHANGED <<status, carried, stored, durable, adopted, gen, flight, snapshot, puts,
      asks, recount, recountGen, lostAt, covered, next, watermarkOK, parent,
      nameLost, savedCursor, driveCursor, landed, rebuilding, restarted, applied,
      used, seq, savedNext, savedAdopted, driveNext, driveAdopted, adoptOK, restoreOK, orphanOK>>
\* Go advance holds the writer lock across the eligible pending prefix.
\* No caller can name, save or adopt between its individual moves.
AdvanceLedger ==
  LET watermark == IF adopted < pin THEN adopted ELSE pin
      moved == {r \in Pending : (BlockWaiting => status[r] # "waiting") /\ seq[r] <= watermark /\
                 \A q \in Pending : seq[q] < seq[r] => (BlockWaiting => status[q] # "waiting")}
  IN /\ moved # {}
     /\ status' = [r \in Records |-> IF r \notin moved THEN status[r]
                    ELSE IF status[r] = "forfeited" THEN "absent" ELSE "due"]
     /\ carried' = [r \in Records |-> IF r \in moved THEN 0 ELSE carried[r]]
     /\ watermarkOK' = (watermarkOK /\ \A r \in moved : seq[r] <= adopted /\ seq[r] <= pin)
  /\ UNCHANGED <<stored, durable, adopted, pin, gen, flight, snapshot, puts, asks, recount,
      recountGen, lostAt, covered, next, parent, nameLost, savedCursor,
      driveCursor, landed, rebuilding, restarted, applied, used, seq, savedNext,
      savedAdopted, driveNext, driveAdopted, adoptOK, restoreOK, orphanOK>>
\* carry/saved snapshot precisely the entries not carried by an earlier save.
CarrySavedLedger ==
  /\ gen < MaxSaves /\ Cardinality(flight) < 2
  /\ LET g == gen + 1
         selected == {r \in Records : carried[r] = 0 /\
                       status[r] \in {"pending", "due", "orphaned"}}
     IN /\ snapshot' = [snapshot EXCEPT ![g] = [r \in Records |->
                    IF r \in selected THEN status[r] ELSE None]]
        /\ puts' = [puts EXCEPT ![g] = IF CarryBlocks
                   THEN {r \in selected : "ledger" \in stage[r] /\ status[r] # "orphaned"}
                   ELSE {}]
        /\ savedNext' = [savedNext EXCEPT ![g] = next]
        /\ savedAdopted' = [savedAdopted EXCEPT ![g] = adopted]
        /\ savedCursor' = [savedCursor EXCEPT ![g] = [r \in Records |->
             IF status[r] = "waiting" THEN "forfeited"
             ELSE IF status[r] \in {"pending", "forfeited"} THEN status[r] ELSE "absent"]]
        /\ asks' = [asks EXCEPT ![g] = IF recount /\ recountGen = 0
                     THEN lostAt ELSE 0]
        /\ recountGen' = IF recount /\ recountGen = 0 THEN g ELSE recountGen
        /\ carried' = [r \in Records |-> IF r \in selected THEN g ELSE carried[r]]
        /\ flight' = flight \cup {g} /\ gen' = g
  /\ UNCHANGED <<status, stored, durable, adopted, pin, recount, lostAt, covered, next,
      watermarkOK, parent, nameLost, driveCursor, landed, rebuilding, restarted,
      applied, used, seq, driveNext, driveAdopted, adoptOK, restoreOK, orphanOK>>
\* Landed, completed or discarded save payloads are forgotten: the engine no longer
\* reads their snapshots. This removes history that cannot change a future
\* transition, keeping the three-record model tractable.
\* A durable save stores its puts before its names. Saves land in order.
LandSaveLedger(g) ==
  /\ g \in Accepted
  /\ \A h \in Accepted : g <= h
  /\ stored' = stored \cup puts[g]
  /\ driveCursor' = savedCursor[g]
  /\ driveNext' = savedNext[g] /\ driveAdopted' = savedAdopted[g]
  /\ durable' = [r \in Records |->
       IF snapshot[g][r] = None THEN durable[r]
       ELSE IF snapshot[g][r] = "orphaned" THEN None ELSE snapshot[g][r]]
  /\ covered' = IF asks[g] > covered THEN asks[g] ELSE covered
  /\ landed' = landed \cup {g}
  /\ rebuilding' = (rebuilding \/ asks[g] > 0)
  /\ snapshot' = [snapshot EXCEPT ![g] = [r \in Records |-> None]]
  /\ puts' = [puts EXCEPT ![g] = {}]
  /\ savedCursor' = [savedCursor EXCEPT ![g] = [r \in Records |-> "absent"]]
  /\ savedNext' = [savedNext EXCEPT ![g] = 1]
  /\ savedAdopted' = [savedAdopted EXCEPT ![g] = 0]
  /\ UNCHANGED <<status, carried, adopted, pin, gen, flight, asks, recount,
      recountGen, lostAt, next, watermarkOK, parent, nameLost, restarted, applied, used, seq, adoptOK, restoreOK, orphanOK>>
ConfirmLedger(g) ==
  /\ g \in flight \cap landed /\ \A h \in flight : g <= h
  /\ LET clear == recountGen # 0 /\
           (IF ConfirmGeneration THEN recountGen <= g ELSE TRUE)
     IN /\ recount' = IF clear THEN FALSE ELSE recount
        /\ recountGen' = IF clear THEN 0 ELSE recountGen
  /\ orphanOK' = (orphanOK /\ \A r \in Records :
       (carried[r] = g /\ status[r] = "orphaned") => durable[r] = None)
  /\ flight' = flight \ {g}
  /\ landed' = landed \ {g}
  /\ status' = [r \in Records |->
       IF carried[r] = g /\ status[r] \in {"due", "orphaned"}
       THEN "absent" ELSE status[r]]
  /\ carried' = [r \in Records |->
       IF carried[r] = g THEN IF status[r] = "pending" THEN MaxSaves + 1 ELSE 0
       ELSE carried[r]]
  /\ snapshot' = [snapshot EXCEPT ![g] = [r \in Records |-> None]]
  /\ puts' = [puts EXCEPT ![g] = {}]
  /\ asks' = [asks EXCEPT ![g] = 0]
  /\ savedCursor' = [savedCursor EXCEPT ![g] = [r \in Records |-> "absent"]]
  /\ savedNext' = [savedNext EXCEPT ![g] = 1]
  /\ savedAdopted' = [savedAdopted EXCEPT ![g] = 0]
  /\ UNCHANGED <<adopted, pin, gen, lostAt, next,
      watermarkOK, parent, nameLost, stored, durable, driveCursor,
      covered, rebuilding, restarted, applied, used, seq, driveNext, driveAdopted, adoptOK, restoreOK>>
\* Settled pending carry markers are represented by MaxSaves+1: only their
\* nonzero status matters until move/unsave resets them. Confirm compacts due
\* and orphaned entries, as Go does. Admission confirms a successful prefix
\* before unsave discards the failed suffix; a landed success cannot fail.
\* Failure discards every unconfirmed save, and resets their carry markers.
UnsaveLedger ==
  /\ flight # {} /\ flight \cap landed = {}
  /\ (LoseFailedNames /\ NeedsFailedNameLoss) => lostAt < 2
  /\ recount' = IF LoseFailedNames /\ NeedsFailedNameLoss THEN TRUE ELSE recount
  /\ lostAt' = IF LoseFailedNames /\ NeedsFailedNameLoss THEN lostAt + 1 ELSE lostAt
  /\ flight' = {} /\ landed' = {} /\ carried' = [r \in Records |-> 0]
  /\ recountGen' = IF ResetOnUnsave THEN 0 ELSE recountGen
  /\ snapshot' = [g \in Saves |-> IF g \in Accepted THEN snapshot[g] ELSE [r \in Records |-> None]]
  /\ puts' = [g \in Saves |-> IF g \in Accepted THEN puts[g] ELSE {}]
  /\ asks' = [g \in Saves |-> IF g \in Accepted THEN asks[g] ELSE 0]
  /\ savedCursor' = [g \in Saves |-> IF g \in Accepted THEN savedCursor[g] ELSE [r \in Records |-> "absent"]]
  /\ savedNext' = [g \in Saves |-> IF g \in Accepted THEN savedNext[g] ELSE 1]
  /\ savedAdopted' = [g \in Saves |-> IF g \in Accepted THEN savedAdopted[g] ELSE 0]
  /\ UNCHANGED <<status, stored, durable, adopted, pin, gen,
      covered, next, watermarkOK, parent, nameLost, driveCursor, rebuilding, restarted, applied, used, seq, driveNext, driveAdopted, adoptOK, restoreOK, orphanOK>>
\* Restart restores the saved cursor: an unresolved record becomes forfeited.
\* Names and cursor are separate durable state; a pending cursor with no
\* record has no blocks to carry and its deaths leak. Sequence numbers restore with the cursor and may be reused; unique model
\* IDs distinguish new record blocks from older blocks with the same number.
\* Accepted receipt snapshots and their stage leases survive this engine reset.
RestoreDeadlistsLedger ==
  /\ ~restarted /\ lostAt < 2
  /\ flight' = {} /\ landed' = {} /\ restarted' = TRUE /\ carried' = [r \in Records |-> 0]
  /\ status' = driveCursor /\ next' = IF RestoreNext THEN driveNext ELSE next
  /\ adopted' = IF RestoreAdopted THEN driveAdopted ELSE adopted
  /\ pin' = 3
  /\ recount' = TRUE /\ recountGen' = 0 /\ lostAt' = lostAt + 1
  /\ snapshot' = [g \in Saves |-> IF g \in Accepted THEN snapshot[g] ELSE [r \in Records |-> None]]
  /\ puts' = [g \in Saves |-> IF g \in Accepted THEN puts[g] ELSE {}]
  /\ asks' = [g \in Saves |-> IF g \in Accepted THEN asks[g] ELSE 0]
  /\ savedCursor' = [g \in Saves |-> IF g \in Accepted THEN savedCursor[g] ELSE [r \in Records |-> "absent"]]
  /\ savedNext' = [g \in Saves |-> IF g \in Accepted THEN savedNext[g] ELSE 1]
  /\ savedAdopted' = [g \in Saves |-> IF g \in Accepted THEN savedAdopted[g] ELSE 0]
  /\ restoreOK' = (restoreOK /\ next' = driveNext /\ adopted' = driveAdopted)
  /\ UNCHANGED <<stored, durable, gen, covered, watermarkOK, parent,
      nameLost, driveCursor, rebuilding, applied, used, seq,
      driveNext, driveAdopted, adoptOK, orphanOK>>
\* Completing the store's recount is independent of confirming its receipt.
FinishRecount ==
  /\ rebuilding /\ rebuilding' = FALSE
  /\ UNCHANGED <<status, carried, stored, durable, adopted, pin, gen, flight, snapshot, puts,
      asks, recount, recountGen, lostAt, covered, next, watermarkOK, parent,
      nameLost, savedCursor, driveCursor, landed, restarted, applied, used, seq,
      savedNext, savedAdopted, driveNext, driveAdopted, adoptOK, restoreOK, orphanOK>>
\* Apply represents precise history application after recount. Source also
\* retires uncertain history during an incomplete recount; that collector
\* progress is outside this ledger-resource proof. Stage ownership ends on
\* receipt Done, confirmation or abandonment independently of that retirement.
Apply(r) ==
  /\ durable[r] = "due" /\ ~rebuilding /\ r \notin applied
  /\ applied' = applied \cup {r}
  /\ durable' = [durable EXCEPT ![r] = None]
  /\ UNCHANGED <<status, carried, stored, adopted, pin, gen, flight, snapshot, puts, asks,
      recount, recountGen, lostAt, covered, next, watermarkOK, parent, nameLost,
      savedCursor, driveCursor, landed, rebuilding, restarted, used, seq,
      savedNext, savedAdopted, driveNext, driveAdopted, adoptOK, restoreOK, orphanOK>>
\* Pure encoding starts only after all page receipts are ready. Encoding may
\* finish after orphaning or an engine reset; it cannot attach to another net.
StartEncoding(r) ==
  /\ r \in Waiting /\ preparation[r] = "receipts" /\ worker[r] = "idle"
  /\ worker' = [worker EXCEPT ![r] = "encoding"]
  /\ preparation' = [preparation EXCEPT ![r] = "encoding"]
  /\ UNCHANGED <<failedNameRecountOK, knownStored, failedNameLossOK, orphanLossOK, coreVars, cancelled, entryEngine, stage, acquired, released,
      receipt, receiptEngine, receiptAfter, admission, stopped, waitingOK, stageOK, leaseOK, completionOK>>
Encoded(r) ==
  /\ worker[r] = "encoding"
  /\ LET live == r \notin cancelled /\ r \in Waiting /\ entryEngine[r] = restarted
     IN /\ worker' = [worker EXCEPT ![r] = IF live THEN "preparing" ELSE "done"]
        /\ preparation' = [preparation EXCEPT ![r] = IF live THEN "preparing" ELSE preparation[r]]
        /\ stage' = [stage EXCEPT ![r] = IF live THEN {"job", "ledger"} ELSE stage[r]]
        /\ acquired' = [acquired EXCEPT ![r] = IF live THEN {"job", "ledger"} ELSE acquired[r]]
  /\ UNCHANGED <<failedNameRecountOK, knownStored, failedNameLossOK, orphanLossOK, coreVars, cancelled, entryEngine, released, receipt, receiptEngine, receiptAfter,
      admission, stopped, waitingOK, stageOK, leaseOK, completionOK>>
\* One bounded atomic preparation write may return after cancellation. Only
\* its own job lease ends here; an accepted save has a separate token.
Prepared(r, target) ==
  /\ worker[r] = "preparing"
  /\ target \in Waiting \cup {r}
  /\ (target = r \/ seq[target] = seq[r])
  /\ (MatchCompletion => target = r)
  /\ LET live == r \notin cancelled /\ r \in Waiting /\ entryEngine[r] = restarted
         attach == IF MatchCompletion THEN live ELSE
           (live /\ target = r) \/ (target # r /\ target \in Waiting /\
             entryEngine[target] = restarted /\ ~stopped)
     IN /\ preparation' = [preparation EXCEPT ![target] = IF attach THEN "ready" ELSE preparation[target]]
        /\ completionOK' = (completionOK /\ (attach => live /\ target = r))
  /\ worker' = [worker EXCEPT ![r] = "done"]
  /\ stage' = [stage EXCEPT ![r] = @ \ {"job"}]
  /\ released' = [released EXCEPT ![r] = @ \cup {"job"}]
  /\ leaseOK' = (leaseOK /\ "job" \in stage[r] /\ "job" \notin released[r])
  /\ UNCHANGED <<failedNameRecountOK, knownStored, failedNameLossOK, orphanLossOK, coreVars, cancelled, entryEngine, acquired, receipt, receiptEngine, receiptAfter,
      admission, stopped, waitingOK, stageOK>>
\* Actual preparation I/O failure forfeits the current entry through move/lost.
\* If orphan/cancel already won, only this worker lease remains to release.
PreparationFailed(r) ==
  /\ worker[r] = "preparing"
  /\ LET live == r \in Waiting /\ r \notin cancelled /\ entryEngine[r] = restarted /\ ~stopped
         drops == IF ReleaseFailedJob THEN {"job", "ledger"} ELSE {}
     IN /\ live => lostAt < 2
        /\ status' = [status EXCEPT ![r] = IF live THEN "orphaned" ELSE status[r]]
        /\ carried' = [carried EXCEPT ![r] = IF live THEN 0 ELSE carried[r]]
        /\ recount' = IF live /\ RequestFailedRecount THEN TRUE ELSE recount
        /\ recountGen' = IF live /\ RequestFailedRecount THEN 0 ELSE recountGen
        /\ lostAt' = IF live THEN lostAt + 1 ELSE lostAt
        /\ cancelled' = IF live THEN cancelled \cup {r} ELSE cancelled
        /\ preparation' = [preparation EXCEPT ![r] = IF live THEN "failed" ELSE preparation[r]]
        /\ stage' = [stage EXCEPT ![r] = @ \ drops]
        /\ released' = [released EXCEPT ![r] = @ \cup (stage[r] \cap drops)]
  /\ worker' = [worker EXCEPT ![r] = "done"]
  /\ leaseOK' = (leaseOK /\ "job" \in stage[r] /\ "job" \notin released[r])
  /\ orphanLossOK' = (orphanLossOK /\ OrphanLossKept)
  /\ UNCHANGED <<failedNameRecountOK, knownStored, failedNameLossOK, stored, durable, adopted, pin, gen, flight, snapshot, puts, asks, covered, next, watermarkOK, parent, nameLost, savedCursor, driveCursor, landed, rebuilding, restarted, applied, used, seq, savedNext, savedAdopted, driveNext, driveAdopted, adoptOK, restoreOK, orphanOK>>
  /\ UNCHANGED <<entryEngine, acquired, receipt, receiptEngine, receiptAfter,
      admission, stopped, waitingOK, stageOK, completionOK>>
Name(q, net) ==
  /\ NameLedger(q, net)
  /\ preparation' = [r \in Records |-> IF r = Cardinality(used)+1 THEN IF net THEN "receipts" ELSE "none"
       ELSE IF r \in Evicted THEN OrphanPreparation(r) ELSE preparation[r]]
  /\ entryEngine' = [entryEngine EXCEPT ![Cardinality(used)+1] = restarted]
  /\ cancelled' = cancelled \cup Evicted
  /\ DropLedger(Evicted)
  /\ orphanLossOK' = (orphanLossOK /\ OrphanLossKept)
  /\ UNCHANGED <<failedNameRecountOK, knownStored, failedNameLossOK, worker, acquired, receipt, receiptEngine, receiptAfter, admission, stopped,
      waitingOK, stageOK, leaseOK, completionOK>>
Resolve(r) ==
  /\ ResolveLedger(r)
  /\ preparation' = [preparation EXCEPT ![r] = "attached"]
  /\ (ReadyBeforeName => preparation[r] = "ready")
  /\ stageOK' = (stageOK /\ preparation[r] = "ready" /\ Owned(r))
  /\ UNCHANGED <<failedNameRecountOK, knownStored, failedNameLossOK, orphanLossOK, worker, cancelled, entryEngine, stage, acquired,
      released, receipt, receiptEngine, receiptAfter, admission, stopped, waitingOK, leaseOK, completionOK>>
Adopt(r) ==
  /\ AdoptLedger(r)
  /\ preparation' = [q \in Records |-> IF status'[q] = "orphaned" /\ status[q] # "orphaned"
                                    THEN OrphanPreparation(q) ELSE preparation[q]]
  /\ LET orphan == {q \in Records : status'[q] = "orphaned" /\ status[q] # "orphaned"}
     IN /\ cancelled' = cancelled \cup orphan /\ DropLedger(orphan)
  /\ orphanLossOK' = (orphanLossOK /\ OrphanLossKept)
  /\ UNCHANGED <<failedNameRecountOK, knownStored, failedNameLossOK,  worker, entryEngine, acquired, receipt, receiptEngine, receiptAfter,
      admission, stopped, waitingOK, stageOK, leaseOK, completionOK>>
Advance == AdvanceLedger /\ UNCHANGED prepVars
\* Snapshot, SubmitAtomic and lease acquisition share the writer critical
\* section. No adoption/ageout/resolve may interleave before Admit or Reject.
CarrySaved ==
  /\ CarrySavedLedger
  /\ admission' = gen + 1
  /\ receiptEngine' = [receiptEngine EXCEPT ![gen+1] = restarted]
  /\ receiptAfter' = [receiptAfter EXCEPT ![gen+1] = LastFlight]
  /\ waitingOK' = (waitingOK /\ Waiting \subseteq WaitingSourcesForSave)
  /\ UNCHANGED <<failedNameRecountOK, knownStored, failedNameLossOK, orphanLossOK, stopped, preparation, worker, cancelled, entryEngine, stage, acquired,
      released, receipt, stageOK, leaseOK, completionOK>>
Admit ==
  /\ admission # 0
  /\ LET g == admission
         refs == IF ReceiptLease THEN NewNames(g) ELSE {}
     IN /\ receipt' = [receipt EXCEPT ![g] = "accepted"]
        /\ stage' = [r \in Records |-> IF r \in refs THEN stage[r] \cup {SaveToken(g)} ELSE stage[r]]
        /\ acquired' = [r \in Records |-> IF r \in refs THEN acquired[r] \cup {SaveToken(g)} ELSE acquired[r]]
        /\ stageOK' = (stageOK /\ \A r \in NewNames(g) : preparation[r] = "attached" /\ Owned(r))
  /\ admission' = 0
  /\ UNCHANGED <<failedNameRecountOK, knownStored, failedNameLossOK, orphanLossOK, stopped, coreVars, preparation, worker, cancelled, entryEngine, released,
      receiptEngine, receiptAfter, waitingOK, leaseOK, completionOK>>
\* Submission rejection owns no receipt lease. A failed accepted publication
\* signals Done before failure handling resets the engine's carry markers.
Reject ==
  /\ admission # 0 /\ UnsaveLedger
  /\ admission' = 0
  /\ failedNameLossOK' = (failedNameLossOK /\ FailedNameLossKept)
  /\ failedNameRecountOK' = (failedNameRecountOK /\ FailedNameRecountKept)
  /\ UNCHANGED <<knownStored, orphanLossOK, stopped, preparation, worker, cancelled, entryEngine, stage, acquired, released,
      receipt, receiptEngine, receiptAfter, waitingOK, stageOK, leaseOK, completionOK>>
FailReceipt(g) ==
  /\ g \in Accepted /\ \A h \in Accepted : g <= h
  /\ receipt' = [receipt EXCEPT ![g] = "failure"]
  /\ DropReceipt(g)
  /\ UNCHANGED <<failedNameRecountOK, knownStored, failedNameLossOK, orphanLossOK, coreVars, preparation, worker, cancelled, entryEngine, acquired,
      receiptEngine, receiptAfter, admission, stopped, waitingOK, stageOK, leaseOK, completionOK>>
LandSave(g) ==
  /\ LandSaveLedger(g)
  /\ IF receiptAfter[g] = 0 THEN TRUE ELSE receipt[receiptAfter[g]] = "success"
  /\ receipt' = [receipt EXCEPT ![g] = "success"]
  /\ DropReceipt(g)
  /\ stageOK' = (stageOK /\ \A r \in NewNames(g) : SaveToken(g) \in stage[r] /\ Owned(r))
  /\ UNCHANGED <<failedNameRecountOK, knownStored, failedNameLossOK, orphanLossOK, preparation, worker, cancelled, entryEngine, acquired, receiptEngine, receiptAfter,
      admission, stopped, waitingOK, leaseOK, completionOK>>
Confirm(g) ==
  /\ ConfirmLedger(g) /\ receipt[g] = "success" /\ receiptEngine[g] = restarted
  /\ DropLedger({r \in Records : carried[r] = g})
  /\ knownStored' = knownStored \cup {r \in Records : carried[r] = g /\ status[r] = "pending"}
  /\ UNCHANGED <<failedNameRecountOK, failedNameLossOK, orphanLossOK, preparation, worker, cancelled, entryEngine, acquired, receipt,
      receiptEngine, receiptAfter, admission, stopped, waitingOK, stageOK, leaseOK, completionOK>>
Unsave ==
  /\ UnsaveLedger
  /\ stage' = [r \in Records |-> IF KeepReceiptLease THEN stage[r] ELSE stage[r] \ ReceiptTokens]
  /\ released' = [r \in Records |-> IF KeepReceiptLease THEN released[r] ELSE released[r] \cup (stage[r] \cap ReceiptTokens)]
  /\ failedNameLossOK' = (failedNameLossOK /\ FailedNameLossKept)
  /\ failedNameRecountOK' = (failedNameRecountOK /\ FailedNameRecountKept)
  /\ UNCHANGED <<knownStored, orphanLossOK, preparation, worker, cancelled, entryEngine, acquired, receipt,
      receiptEngine, receiptAfter, admission, stopped, waitingOK, stageOK, leaseOK, completionOK>>
RestoreDeadlists ==
  /\ stopped /\ Accepted = {} /\ RestoreDeadlistsLedger
  /\ stopped' = FALSE
  /\ preparation' = [r \in Records |-> IF driveCursor[r] = "pending" THEN "attached" ELSE "none"]
  /\ cancelled' = cancelled \cup used
  /\ stage' = [r \in Records |-> IF KeepReceiptLease THEN stage[r] \ {"ledger"}
                               ELSE stage[r] \ ({"ledger"} \cup ReceiptTokens)]
  /\ released' = [r \in Records |-> released[r] \cup
       (stage[r] \cap (IF KeepReceiptLease THEN {"ledger"} ELSE {"ledger"} \cup ReceiptTokens))]
  /\ knownStored' = {r \in Records : driveCursor[r] = "pending"}
  /\ UNCHANGED <<failedNameRecountOK, failedNameLossOK, orphanLossOK, worker, entryEngine, acquired, receipt, receiptEngine, receiptAfter, admission,
      waitingOK, stageOK, leaseOK, completionOK>>
\* Restart does not let old callbacks write new entries, even if sequence
\* numbers repeat. Accepted durable effects still occur in publication order.
CancelEngine ==
  /\ ~stopped /\ stopped' = TRUE
  /\ cancelled' = cancelled \cup used
  /\ stage' = [r \in Records |-> IF KeepReceiptLease THEN stage[r] \ (IF KeepWorkerLease THEN {"ledger"} ELSE {"ledger", "job"})
                               ELSE stage[r] \ ({"ledger"} \cup ReceiptTokens)]
  /\ released' = [r \in Records |-> released[r] \cup
       (stage[r] \cap ((IF KeepReceiptLease THEN {} ELSE ReceiptTokens) \cup (IF KeepWorkerLease THEN {"ledger"} ELSE {"ledger", "job"})))]
  /\ UNCHANGED <<failedNameRecountOK, knownStored, failedNameLossOK, orphanLossOK, coreVars, preparation, worker, entryEngine, acquired, receipt,
      receiptEngine, receiptAfter, admission, waitingOK, stageOK, leaseOK, completionOK>>
IdlePrep == UNCHANGED prepVars
Next ==
  \/ (admission = 0 /\
      (\/ (~stopped /\
           (\/ (\E q \in 0..MaxRecords, net \in BOOLEAN : Name(q, net))
            \/ (\E r \in Records : Resolve(r) \/ Adopt(r) \/ StartEncoding(r))
            \/ Advance \/ CarrySaved \/ Unsave
            \/ (\E g \in Saves : Confirm(g))
            \/ ((Lost \/ ReleasePin \/ FinishRecount \/ (\E r \in Records : Apply(r))) /\ IdlePrep)))
       \/ CancelEngine \/ RestoreDeadlists
       \/ (\E r \in Records : Encoded(r) \/ PreparationFailed(r) \/ (\E t \in Records : Prepared(r, t)))
       \/ (\E g \in Saves : LandSave(g) \/ FailReceipt(g))))
  \/ Admit \/ Reject \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars
FailedNameRecount == failedNameRecountOK
FailedNameLossRequested == failedNameLossOK
OrphanLossRequested == orphanLossOK
PreparationSafe == stageOK /\ completionOK
WaitingSources == waitingOK
StageConserved == leaseOK /\ \A r \in Records :
  /\ stage[r] = acquired[r] \ released[r]
  /\ released[r] \subseteq acquired[r]
  /\ stage[r] \subseteq Tokens
PreparingOwned == \A r \in Records : worker[r] = "preparing" => "job" \in stage[r]
AcceptedRetained == \A g \in Accepted : \A r \in NewNames(g) : SaveToken(g) \in stage[r]
DetachedLedgerReleased == \A r \in Records :
  (status[r] \in {"absent", "orphaned"} \/ r \in cancelled) => "ledger" \notin stage[r]
PendingBounded == Cardinality(Pending) <= MaxPending
ReadyBeforeDue == \A r \in Records : status[r] = "due" => preparation[r] = "attached"
FinishedJobReleased == \A r \in Records : worker[r] # "preparing" => "job" \notin stage[r]
SettledReceiptReleased == \A g \in Saves \ Accepted : \A r \in Records : SaveToken(g) \notin stage[r]

DurableNamesStored == {r \in Records : durable[r] # None} \subseteq stored
DueWatermark == watermarkOK
AdoptionContract == adoptOK
RestoredCounters == restoreOK
DueAdoption == \A r \in Records : durable[r] = "due" => seq[r] <= driveAdopted

\* Request responsibility cannot disappear between lost and a durable save.
RecountCarried == lostAt > covered =>
  recount /\ (recountGen = 0 \/
    (recountGen \in flight /\ asks[recountGen] >= lostAt))
\* When an orphan's removal is what the durable save carries, no name survives.
RecountBeforeDue == \A r \in Records : durable[r] = "due" => covered >= nameLost[r]
RecountBeforeApply == \A r \in applied : covered >= nameLost[r]
OrphanReleased == orphanOK
====
