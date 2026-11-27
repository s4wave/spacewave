---- MODULE ReplayUnknownSettle ----
\* Settling a write whose commit outcome is unknown, across the restart of the
\* local mount that wrote it.
\*
\* One device's client writes operations 1..N, pipelined: it admits each on
\* the rows it believes applied (base), and the serial queue commits them in
\* admission order. Each commit is a compare-and-set of the mount's token
\* against storage's. It stores the operation, fails its compare after an
\* outside replace (Ext), or ends unknown: the state either landed or was
\* lost, and the mount cannot read which.
\*
\* An unknown or a failed compare makes the mount call Reload, whose restart
\* a backoff may postpone (run = "postponed"). From that call the provider's
\* fence (fenced) and the adapter's Reloading state (qstop) refuse every
\* later commit of the old run, so its client learns ErrEngineRetired and
\* drops the refused write's rows. The restart loads storage. The new adapter
\* settles the unknown write from what it loaded (Settle), and the client,
\* which held its writes since the unknown, keeps the write's rows when it
\* applied and drops them otherwise.
\*
\* The settle set (kept) holds the unknown write's entry from the unknown
\* until WaitSettled reads its answer. It lives in the Space body's World
\* factory, which the space sobject controller keeps across every mount run,
\* so the restart does not drop it (KeptOnRestart). The old adapter tells the
\* entry its outcome went unknown (Tell) when its queue sees the error, which
\* nothing orders before the restart: the restart runs inside the reload the
\* unknown called. The new adapter takes the entry for its replay (adopted)
\* when it opens, and again whenever a told entry appears (Adopt), so a Tell
\* after its open is still settled (LateAdopt).
\*
\* The invariants say the settle answers what storage holds, the settled head
\* is the state at the unknown plus at most the unknown write, a held client's
\* write stays settleable, no stored operation was built on rows storage
\* lacks, and an idle client's rows match storage.
EXTENDS Naturals, Sequences, FiniteSets

N == 4
Ops == 1..N
Runs == {"live", "postponed", "new"}
Results == {"none", "applied", "lost"}

VARIABLES st, tok, mtok, mview, run, fenced, qstop, inflight, rows, base,
          unk, snap, result, shead, next, extUsed, kept, told, adopted
vars == <<st, tok, mtok, mview, run, fenced, qstop, inflight, rows, base,
          unk, snap, result, shead, next, extUsed, kept, told, adopted>>

\* Range is the set of elements of a sequence.
Range(s) == {s[i] : i \in DOMAIN s}

\* CommitGuard is what lets the old run's queue commit: neither the
\* provider's fence nor the adapter's Reloading state has refused it.
CommitGuard == ~fenced /\ ~qstop

\* Held is the client waiting for WaitSettled before it writes again.
Held == unk # 0 /\ result = "none"

\* SettleReady and SettleView are when and from what the settle answers: an
\* adapter on the restarted mount that adopted the told entry, from the
\* storage it loaded.
SettleReady == run = "new" /\ told /\ adopted
SettleView == st

\* KeptOnRestart is whether the settle set still holds its entries after the
\* restart: the factory that holds it outlives the mount run.
KeptOnRestart == kept

\* LateAdopt is whether a live adapter adopts an entry told after it opened.
LateAdopt == TRUE

TypeOK ==
    /\ st \subseteq Ops
    /\ tok \in Nat /\ mtok \in Nat
    /\ mview \subseteq Ops
    /\ run \in Runs
    /\ fenced \in BOOLEAN /\ qstop \in BOOLEAN
    /\ Range(inflight) \subseteq Ops
    /\ rows \subseteq Ops
    /\ base \in [Ops -> SUBSET Ops]
    /\ unk \in 0..N
    /\ snap \subseteq Ops
    /\ result \in Results
    /\ shead \subseteq Ops
    /\ next \in 1..(N + 1)
    /\ extUsed \in BOOLEAN
    /\ kept \in BOOLEAN
    /\ told \in BOOLEAN /\ adopted \in BOOLEAN

Init ==
    /\ st = {} /\ tok = 0 /\ mtok = 0 /\ mview = {}
    /\ run = "live" /\ fenced = FALSE /\ qstop = FALSE
    /\ inflight = <<>> /\ rows = {} /\ base = [o \in Ops |-> {}]
    /\ unk = 0 /\ snap = {} /\ result = "none" /\ shead = {}
    /\ next = 1 /\ extUsed = FALSE /\ kept = FALSE
    /\ told = FALSE /\ adopted = FALSE

\* Admit takes the client's next write on the rows it believes applied, while
\* the adapter admits writes and the client holds none for a settle.
Admit ==
    /\ next <= N
    /\ run \in {"live", "new"}
    /\ ~qstop
    /\ ~Held
    /\ inflight' = Append(inflight, next)
    /\ base' = [base EXCEPT ![next] = rows]
    /\ rows' = rows \cup {next}
    /\ next' = next + 1
    /\ UNCHANGED <<st, tok, mtok, mview, run, fenced, qstop, unk, snap, result, shead, extUsed, kept, told, adopted>>

\* Ok commits the oldest queued write over the token the mount holds.
Ok ==
    /\ inflight # <<>>
    /\ CommitGuard
    /\ mtok = tok
    /\ LET o == Head(inflight) IN
        /\ st' = st \cup {o}
        /\ tok' = tok + 1 /\ mtok' = tok + 1
        /\ mview' = mview \cup {o}
    /\ inflight' = Tail(inflight)
    /\ UNCHANGED <<run, fenced, qstop, rows, base, unk, snap, result, shead, next, extUsed, kept, told, adopted>>

\* Changed is a failed compare: storage holds another state, so the mount
\* reloads and nothing was written. The client drops the write's rows.
Changed ==
    /\ inflight # <<>>
    /\ CommitGuard
    /\ mtok # tok
    /\ run' = "postponed" /\ fenced' = TRUE
    /\ rows' = rows \ {Head(inflight)}
    /\ inflight' = Tail(inflight)
    /\ UNCHANGED <<st, tok, mtok, mview, qstop, base, unk, snap, result, shead, next, extUsed, kept, told, adopted>>

\* Unknown ends the oldest write unknown: storage holds it (landed) or not.
\* The mount fences its writes and the adapter stops, both at once. The
\* client keeps the write's rows and holds for the settle.
Unknown(landed) ==
    /\ inflight # <<>>
    /\ CommitGuard
    /\ mtok = tok
    /\ unk = 0
    /\ LET o == Head(inflight) IN
        /\ unk' = o
        /\ st' = IF landed THEN st \cup {o} ELSE st
        /\ tok' = IF landed THEN tok + 1 ELSE tok
    /\ snap' = st
    /\ kept' = TRUE
    /\ run' = "postponed" /\ fenced' = TRUE /\ qstop' = TRUE
    /\ inflight' = Tail(inflight)
    /\ UNCHANGED <<mtok, mview, rows, base, result, shead, next, extUsed, told, adopted>>

\* Refuse ends the oldest write of a refused run unsent, with
\* ErrEngineRetired. The client drops its rows and writes it again later.
Refuse ==
    /\ inflight # <<>>
    /\ ~CommitGuard
    /\ rows' = rows \ {Head(inflight)}
    /\ inflight' = Tail(inflight)
    /\ UNCHANGED <<st, tok, mtok, mview, run, fenced, qstop, base, unk, snap, result, shead, next, extUsed, kept, told, adopted>>

\* Ext replaces the stored state with an equal one under a new token, as
\* ReplaceSharedObjectState may.
Ext ==
    /\ ~extUsed
    /\ tok' = tok + 1 /\ extUsed' = TRUE
    /\ UNCHANGED <<st, mtok, mview, run, fenced, qstop, inflight, rows, base, unk, snap, result, shead, next, kept, told, adopted>>

\* Restart is the postponed restart firing: the new mount loads storage, the
\* writes the old run left queued end unsent, and the new adapter adopts the
\* entry when the old one has told it.
Restart ==
    /\ run = "postponed"
    /\ run' = "new"
    /\ mtok' = tok /\ mview' = st
    /\ fenced' = FALSE /\ qstop' = FALSE
    /\ rows' = rows \ Range(inflight)
    /\ inflight' = <<>>
    /\ kept' = KeptOnRestart
    /\ adopted' = told
    /\ UNCHANGED <<st, tok, base, unk, snap, result, shead, next, extUsed, told>>

\* Tell is the old adapter registering the unknown write's outcome, before or
\* after the restart. The client learns the unknown only then.
Tell ==
    /\ unk # 0
    /\ ~told
    /\ told' = TRUE
    /\ UNCHANGED <<st, tok, mtok, mview, run, fenced, qstop, inflight, rows, base, unk, snap, result, shead, next, extUsed, kept, adopted>>

\* Adopt is the live adapter taking an entry told after it opened.
Adopt ==
    /\ LateAdopt
    /\ run = "new"
    /\ told /\ ~adopted
    /\ adopted' = TRUE
    /\ UNCHANGED <<st, tok, mtok, mview, run, fenced, qstop, inflight, rows, base, unk, snap, result, shead, next, extUsed, kept, told>>

\* Settle answers WaitSettled from the write's entry: whether the unknown
\* write applied, at the settling adapter's head, and removes the entry. The
\* client keeps or drops its rows.
Settle ==
    /\ Held
    /\ SettleReady
    /\ kept
    /\ kept' = FALSE
    /\ result' = IF unk \in SettleView THEN "applied" ELSE "lost"
    /\ shead' = SettleView
    /\ rows' = IF unk \in SettleView THEN rows ELSE rows \ {unk}
    /\ UNCHANGED <<st, tok, mtok, mview, run, fenced, qstop, inflight, base, unk, snap, next, extUsed, told, adopted>>

Next ==
    \/ Admit \/ Ok \/ Changed \/ Unknown(TRUE) \/ Unknown(FALSE)
    \/ Refuse \/ Ext \/ Restart \/ Tell \/ Adopt \/ Settle

\* The restart, the old adapter's Tell, the adoption and the settle each
\* happen once enabled.
Spec == Init /\ [][Next]_vars
    /\ WF_vars(Restart) /\ WF_vars(Tell) /\ WF_vars(Adopt) /\ WF_vars(Settle)

\* SettleCorrect: the settle says applied exactly when storage holds the write.
SettleCorrect == result # "none" => ((result = "applied") <=> (unk \in st))

\* SettleHead: the settled head is the state at the unknown, with the unknown
\* write when it applied. No write of the old run committed in between.
SettleHead == result # "none" =>
    shead = snap \cup (IF result = "applied" THEN {unk} ELSE {})

\* SettleKept: a client held for a settle has an entry to settle, so
\* WaitSettled neither hangs nor returns ErrNoSettle across the restart.
SettleKept == Held => kept

\* BasesHeld: no stored operation was built on rows storage lacks.
BasesHeld == \A o \in st : base[o] \subseteq st

\* Answered: a client held for a settle is answered, so WaitSettled does not
\* hang until the next reload.
Answered == Held ~> ~Held

\* RowsMatch: an idle client holding no settle has exactly storage's rows.
RowsMatch == (inflight = <<>> /\ ~Held) => rows = st
====
