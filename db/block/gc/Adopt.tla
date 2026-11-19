---- MODULE Adopt ----
\* How a Volume whose counts nobody trusts reaches format 10's exact counts
\* while writers keep committing: the online recount that adoption and the
\* format 9 upgrade run before frees may start.
\*
\* While the Volume is unadopted, Land stores lists and moves roots and keeps
\* no counts. Claim begins a run, superseding any run that claimed or marked
\* the Volume, and drops the cursor. AdoptClear deletes the stale counts; it
\* runs in slices, but Land keeps no counts until the mark, so one step stands
\* for them. AdoptMark takes the mark once every count is clear, starts fresh
\* logs and writes the run's cursor, which exists exactly while the Volume is
\* adopting. From the mark Land keeps counts as Liveness.tla does, and while
\* adopting also writes a born mark ('B') on each block it gives birth to and,
\* the first time it moves a root, the root's target at the mark ('A').
\*
\* The run's two passes move the cursor. The count pass scans the next record
\* in key order, roots first and then child lists: it adds one to the raw
\* tally of a root's target at the mark, or of each child of a listed block
\* that has no born mark, so the scan counts the state at the mark and the
\* logs count each change after it. The settle pass walks the lists again and
\* writes the tally of each block with a list or a hole record as its holders, queuing a block at zero; a
\* raw tally n reads as n + 1 holders until then, since a missing settled
\* record means one holder. Born blocks Land writes settled, and both passes
\* skip them. Each step commits its records with the cursor's advance. The
\* last settle step marks the Volume adopted and drops the cursor, and the
\* run then deletes its marks while the Volume is adopted in its name.
\*
\* AdoptTakeover is a process that dies mid-run and a later one that takes
\* the run over at the cursor, restamping both under its own run. It accepts
\* only a cursor that names the run in progress: OldClaim is a build that
\* predates the cursor, which claims, clears and marks the Volume in a run of
\* its own, keeping the cursor's bytes unread, then dies. AdoptFail loses a
\* run, returning the Volume to unadopted under no claim. Reset is frees
\* turning on, which returns an adopted Volume to unadopted for a recount.
\*
\* A hole ("b") is a block a list names that the store lost with its list,
\* and that an audit recorded. The count gives it a tally from the lists that
\* name it, and the settle settles it with the lists, in key order, and
\* forgets a hole nothing names. The count reads no list for a hole, so its
\* children lack its holder. Land's put of a hole is a refill, not a birth:
\* it writes the list back with no born mark, and logs a holder for each
\* child where the children lack it, which while adopting is where the count
\* passed the hole's key, and otherwise is always.
\*
\* Lose is the store losing a block with its list while the Volume keeps no
\* counts, so the next recount reads no list for it and leaves its own
\* tally raw, one holder high. AuditHole is an audit after that adoption
\* recording the loss as a hole: it cannot tell whether a recount read the
\* lost list, so the refill gives the children the hole's holder. A loss
\* after a count, whose refill leaves the children one holder high until a
\* recount, and a rebirth, which needs the Sweeping generation, are left out.
\*
\* At adoption the counts are those Liveness.tla starts from: CountsExact and
\* HoldUntilFree hold, and every block at zero is queued. Frees and the down
\* log's netting wait for adoption, and the up log nets only once adopted, so
\* this model has no frees and nets only ups.
\*
\* Callers found from source: Adopt and its passes (db/block/gc/adopt.go),
\* the count encodings (records.go), Land (land.go), the net gate
\* (collector.go) and the kvtx Volume that starts a run when frees turn on
\* (db/volume/common/kvtx/volume.go).

EXTENDS Naturals, Sequences, FiniteSets

CONSTANTS
  MaxTxns,    \* bounds the Land transactions, so the model is finite
  MaxRuns,    \* bounds the runs claimed and taken over
  MaxFails    \* bounds the failed runs

None == "none"
Unset == "unset"
\* NoCursor is the absent cursor; no run is numbered 0.
NoCursor == [run |-> 0, pass |-> "none", at |-> 0]
Blocks == {"a", "b", "c", "d", "e"}
RootNames == {"main", "stg"}

\* Stale is a count per block that matches no state: an orphan, one too many,
\* and two missing.
Stale == [a |-> 0, b |-> 1, c |-> 2, d |-> 0, e |-> 0]

\* "e" is a block not yet stored, which a Land may give birth to.
Kids(b) == [a |-> {"b", "c"}, b |-> {"c"}, c |-> {"d"}, d |-> {}, e |-> {"c"}][b]

VARIABLES
  adoption,  \* "unadopted", "adopting" or "adopted"
  owner,     \* the run holding the Volume, 0 for none
  runs,      \* runs claimed or taken over so far, each named by its number
  listed,    \* blocks stored with their child list
  holes,     \* blocks a list names whose bytes and list the store lost
  H,         \* each block's count record: a raw tally while in raw
  raw,       \* blocks whose count record holds a raw tally
  queued,    \* blocks at zero that hold a 'Q' entry
  roots,     \* each root's target, or None
  upLog,     \* U: the blocks of unnetted increments since the mark
  downLog,   \* D: the blocks of releases since the mark
  cleared,   \* whether the claim's run cleared the stale counts
  bmark,     \* born marks: blocks born since the mark
  amark,     \* each root's target at the mark, Unset until it moves
  cursor,    \* NoCursor, or the run, pass and records passed of the run's passes
  cleanup,   \* runs that adopted and still owe the deletion of their marks
  txns,      \* Land transactions so far
  fails,     \* runs failed so far
  lost,      \* blocks the store lost with their lists that no audit recorded
  audited    \* holes AuditHole recorded: a ghost naming whose tally stays raw
vars == <<adoption, owner, runs, listed, holes, H, raw, queued, roots, upLog,
          downLog, cleared, bmark, amark, cursor, cleanup, txns, fails, lost, audited>>

RECURSIVE SeqOf(_)
SeqOf(S) ==
  IF S = {} THEN <<>>
  ELSE LET x == CHOOSE x \in S : TRUE IN <<x>> \o SeqOf(S \ {x})

\* Scan is the count pass's records in key order: the root names, then the
\* child list keys of every block that may be listed. Lists is the settle
\* pass's.
Scan == <<[k |-> "root", n |-> "main"], [k |-> "root", n |-> "stg"],
          [k |-> "list", p |-> "a"], [k |-> "list", p |-> "b"],
          [k |-> "list", p |-> "c"], [k |-> "list", p |-> "d"],
          [k |-> "list", p |-> "e"]>>
Lists == <<"a", "b", "c", "d", "e">>
ListAt(p) == CHOOSE i \in 1..Len(Scan) : Scan[i].k = "list" /\ Scan[i].p = p

Count(s, b) == Cardinality({i \in 1..Len(s) : s[i] = b})
Holders(b) ==
  Cardinality({n \in RootNames : roots[n] = b})
    + Cardinality({p \in listed : b \in Kids(p)})
\* Read is the holders a reader decodes from b's count record.
Read(b) == IF b \in raw THEN H[b] + 1 ELSE H[b]
Adopting == adoption = "adopting"
Counting == adoption \in {"adopting", "adopted"}
\* Counted is whether the run has counted p's list, as adoptCounted reads it.
Counted(p) ==
  /\ Adopting
  /\ \/ cursor.pass = "settle"
     \/ cursor.pass = "count" /\ cursor.at >= ListAt(p)
\* Owned is whether the cursor names the run holding the Volume.
Owned == Adopting /\ cursor.run = owner

Init ==
  /\ adoption = "unadopted"
  /\ owner = 0
  /\ runs = 0
  /\ listed = {"a", "c", "d"}
  /\ holes = {"b"}
  /\ H = Stale
  /\ raw = {}
  /\ queued = {}
  /\ roots = [n \in RootNames |-> IF n = "main" THEN "a" ELSE None]
  /\ upLog = <<>>
  /\ downLog = <<>>
  /\ cleared = FALSE
  /\ bmark = {}
  /\ amark = [n \in RootNames |-> Unset]
  /\ cursor = NoCursor
  /\ cleanup = {}
  /\ txns = 0
  /\ fails = 0
  /\ lost = {}
  /\ audited = {}

\* Land stores batch and moves root n to t in one transaction; it keeps
\* counts only from the mark on, and marks only while adopting. A hole in
\* the batch is refilled.
Land ==
  \E n \in RootNames, t \in Blocks \cup {None}, batch \in SUBSET (Blocks \ lost) :
    LET born == batch \ (listed \cup holes)
        refill == batch \cap holes
        move == t # roots[n]
        edges == UNION {{<<p, k>> : k \in Kids(p) \ born} : p \in born}
        tUp == move /\ t # None /\ t \notin born
        lacks(q) == ~Adopting \/ Counted(q)
        refillUps == UNION {{<<p, k>> : k \in Kids(p)} : p \in {q \in refill : lacks(q)}}
        ups == [i \in 1..Cardinality(edges) |-> SeqOf(edges)[i][2]]
                 \o [i \in 1..Cardinality(refillUps) |-> SeqOf(refillUps)[i][2]]
                 \o (IF tUp THEN <<t>> ELSE <<>>)
        named == {e[2] : e \in edges} \cup (IF tUp THEN {t} ELSE {})
        birth(b) == Cardinality({p \in born : b \in Kids(p)})
                      + (IF move /\ b = t THEN 1 ELSE 0)
    IN
    /\ txns < MaxTxns
    /\ n # "main" \/ t # None
    /\ move \/ batch # {}
    /\ t \in listed \cup batch \cup {None}
    /\ \A p \in born : Kids(p) \subseteq listed \cup batch
    /\ Counting => \A b \in born : (move /\ b = t) \/ \E p \in batch : b \in Kids(p)
    /\ listed' = listed \cup batch
    /\ holes' = holes \ batch
    /\ roots' = [roots EXCEPT ![n] = t]
    /\ txns' = txns + 1
    /\ IF Counting
         THEN /\ H' = [b \in Blocks |-> IF b \in born THEN H[b] + birth(b) ELSE H[b]]
              /\ raw' = raw \ born
              /\ upLog' = upLog \o ups
              /\ downLog' = IF move /\ roots[n] # None
                              THEN Append(downLog, roots[n]) ELSE downLog
              /\ queued' = queued \ named
         ELSE UNCHANGED <<H, raw, upLog, downLog, queued>>
    /\ IF Adopting
         THEN /\ bmark' = bmark \cup born
              /\ amark' = IF move /\ amark[n] = Unset
                            THEN [amark EXCEPT ![n] = roots[n]] ELSE amark
         ELSE UNCHANGED <<bmark, amark>>
    /\ UNCHANGED <<adoption, owner, runs, cleared, cursor, cleanup, fails, lost, audited>>

\* A run claims a Volume that is not adopted, superseding the run that held
\* it, and drops its cursor.
Claim ==
  /\ adoption # "adopted"
  /\ runs < MaxRuns
  /\ runs' = runs + 1
  /\ owner' = runs + 1
  /\ adoption' = "unadopted"
  /\ cleared' = FALSE
  /\ cursor' = NoCursor
  /\ UNCHANGED <<listed, holes, H, raw, queued, roots, upLog, downLog, bmark, amark, cleanup, txns, fails, lost, audited>>

AdoptClear ==
  /\ adoption = "unadopted"
  /\ owner # 0
  /\ ~cleared
  /\ H' = [b \in Blocks |-> 0]
  /\ raw' = Blocks
  /\ cleared' = TRUE
  /\ UNCHANGED <<adoption, owner, runs, listed, holes, queued, roots, upLog, downLog, bmark, amark, cursor, cleanup, txns, fails, lost, audited>>

AdoptMark ==
  /\ adoption = "unadopted"
  /\ owner # 0
  /\ cleared
  /\ adoption' = "adopting"
  /\ cursor' = [run |-> owner, pass |-> "count", at |-> 0]
  /\ upLog' = <<>>
  /\ downLog' = <<>>
  /\ queued' = {}
  /\ bmark' = {}
  /\ amark' = [n \in RootNames |-> Unset]
  /\ UNCHANGED <<owner, runs, listed, holes, H, raw, roots, cleared, cleanup, txns, fails, lost, audited>>

\* The run counts the next record and moves the cursor past it, in one
\* transaction.
AdoptCount ==
  /\ Owned
  /\ cursor.pass = "count"
  /\ cursor.at < Len(Scan)
  /\ LET r == Scan[cursor.at + 1] IN
       IF r.k = "root"
         THEN LET v == IF amark[r.n] # Unset THEN amark[r.n] ELSE roots[r.n] IN
                H' = IF v = None THEN H ELSE [H EXCEPT ![v] = @ + 1]
         ELSE H' = IF r.p \notin listed \/ r.p \in bmark THEN H
                   ELSE [b \in Blocks |-> IF b \in Kids(r.p) THEN H[b] + 1 ELSE H[b]]
  /\ cursor' = [cursor EXCEPT !.at = @ + 1]
  /\ UNCHANGED <<adoption, owner, runs, listed, holes, raw, queued, roots, upLog, downLog, cleared, bmark, amark, cleanup, txns, fails, lost, audited>>

AdoptCountEnd ==
  /\ Owned
  /\ cursor.pass = "count"
  /\ cursor.at = Len(Scan)
  /\ cursor' = [cursor EXCEPT !.pass = "settle", !.at = 0]
  /\ UNCHANGED <<adoption, owner, runs, listed, holes, H, raw, queued, roots, upLog, downLog, cleared, bmark, amark, cleanup, txns, fails, lost, audited>>

\* The run settles the block of the next list or hole record and moves the
\* cursor past it, in one transaction. A hole's tally is settled too, so a
\* refill behind the cursor finds its record settled; a hole named by
\* nothing is forgotten.
AdoptSettle ==
  /\ Owned
  /\ cursor.pass = "settle"
  /\ cursor.at < Len(Lists)
  /\ LET p == Lists[cursor.at + 1] IN
       CASE p \in bmark \/ p \notin listed \cup holes ->
              UNCHANGED <<raw, queued, holes>>
         [] p \in holes /\ H[p] = 0 ->
              /\ raw' = raw \ {p}
              /\ holes' = holes \ {p}
              /\ UNCHANGED queued
         [] OTHER ->
              /\ raw' = raw \ {p}
              /\ queued' = IF H[p] = 0 THEN queued \cup {p} ELSE queued
              /\ UNCHANGED holes
  /\ cursor' = [cursor EXCEPT !.at = @ + 1]
  /\ UNCHANGED <<adoption, owner, runs, listed, H, roots, upLog, downLog, cleared, bmark, amark, cleanup, txns, fails, lost, audited>>

AdoptEnd ==
  /\ Owned
  /\ cursor.pass = "settle"
  /\ cursor.at = Len(Lists)
  /\ adoption' = "adopted"
  /\ cursor' = NoCursor
  /\ cleanup' = cleanup \cup {owner}
  /\ UNCHANGED <<owner, runs, listed, holes, H, raw, queued, roots, upLog, downLog, cleared, bmark, amark, txns, fails, lost, audited>>

\* Run r deletes its marks while the Volume is adopted in its name, and
\* touches nothing once another run holds it.
AdoptCleanup ==
  \E r \in cleanup :
    /\ cleanup' = cleanup \ {r}
    /\ IF adoption = "adopted" /\ owner = r
         THEN /\ bmark' = {}
              /\ amark' = [n \in RootNames |-> Unset]
         ELSE UNCHANGED <<bmark, amark>>
    /\ UNCHANGED <<adoption, owner, runs, listed, holes, H, raw, queued, roots, upLog, downLog, cleared, cursor, txns, fails, lost, audited>>

\* A process dies mid-run and another takes the run over at the cursor,
\* restamping it, since the counts and the cursor committed together.
AdoptTakeover ==
  /\ Owned
  /\ runs < MaxRuns
  /\ runs' = runs + 1
  /\ owner' = runs + 1
  /\ cursor' = [cursor EXCEPT !.run = runs + 1]
  /\ UNCHANGED <<adoption, listed, holes, H, raw, queued, roots, upLog, downLog, cleared, bmark, amark, cleanup, txns, fails, lost, audited>>

\* A build that predates the cursor claims, clears and marks the Volume in a
\* run of its own and dies, leaving the cursor it never read.
OldClaim ==
  /\ Adopting
  /\ runs < MaxRuns
  /\ runs' = runs + 1
  /\ owner' = runs + 1
  /\ H' = [b \in Blocks |-> 0]
  /\ raw' = Blocks
  /\ upLog' = <<>>
  /\ downLog' = <<>>
  /\ queued' = {}
  /\ bmark' = {}
  /\ amark' = [n \in RootNames |-> Unset]
  /\ UNCHANGED <<adoption, listed, holes, roots, cleared, cursor, cleanup, txns, fails, lost, audited>>

\* A run fails, returning the Volume to unadopted under no claim.
AdoptFail ==
  /\ owner # 0
  /\ adoption # "adopted"
  /\ fails < MaxFails
  /\ fails' = fails + 1
  /\ adoption' = "unadopted"
  /\ owner' = 0
  /\ cleared' = FALSE
  /\ cursor' = NoCursor
  /\ UNCHANGED <<runs, listed, holes, H, raw, queued, roots, upLog, downLog, bmark, amark, cleanup, txns, lost, audited>>

\* The store loses a listed block with its list before the first run, while
\* the Volume keeps no counts, so every recount misses it.
Lose ==
  \E p \in listed :
    /\ runs = 0
    /\ lost = {}
    /\ listed' = listed \ {p}
    /\ lost' = {p}
    /\ UNCHANGED <<adoption, owner, runs, holes, H, raw, queued, roots, upLog, downLog, cleared, bmark, amark, cursor, cleanup, txns, fails, audited>>

\* An audit of the adopted Volume records a lost block a listed block names
\* as a hole. Its own count already reads at least its namers, so the audit
\* adds none.
AuditHole ==
  \E p \in lost :
    /\ adoption = "adopted"
    /\ \E q \in listed : p \in Kids(q)
    /\ lost' = lost \ {p}
    /\ holes' = holes \cup {p}
    /\ audited' = audited \cup {p}
    /\ UNCHANGED <<adoption, owner, runs, listed, H, raw, queued, roots, upLog, downLog, cleared, bmark, amark, cursor, cleanup, txns, fails>>

\* Frees turn on: an adopted Volume returns to unadopted for a recount.
Reset ==
  /\ adoption = "adopted"
  /\ runs < MaxRuns
  /\ adoption' = "unadopted"
  /\ owner' = 0
  /\ cleared' = FALSE
  /\ UNCHANGED <<runs, listed, holes, H, raw, queued, roots, upLog, downLog, bmark, amark, cursor, cleanup, txns, fails, lost, audited>>

NetUps ==
  /\ adoption = "adopted"
  /\ upLog # <<>>
  /\ H' = [H EXCEPT ![Head(upLog)] = @ + 1]
  /\ queued' = queued \ {Head(upLog)}
  /\ upLog' = Tail(upLog)
  /\ UNCHANGED <<adoption, owner, runs, listed, holes, raw, roots, downLog, cleared, bmark, amark, cursor, cleanup, txns, fails, lost, audited>>

Next ==
  \/ Land \/ Claim \/ AdoptClear \/ AdoptMark \/ AdoptCount \/ AdoptCountEnd
  \/ AdoptSettle \/ AdoptEnd \/ AdoptCleanup \/ AdoptTakeover \/ OldClaim
  \/ AdoptFail \/ Reset \/ NetUps \/ Lose \/ AuditHole

Spec == Init /\ [][Next]_vars

\* Adopted counts, with the unnetted logs, equal the roots and listed parents
\* that name each listed block, but for a refilled audited hole whose tally
\* the recount that missed it left raw, which reads one holder high until
\* the next recount.
CountsExact ==
  adoption = "adopted" =>
    \A b \in listed :
      Read(b) + Count(upLog, b)
        = Holders(b) + Count(downLog, b) + (IF b \in raw \cap audited THEN 1 ELSE 0)

\* Each child of a listed block counts at least that parent.
HoldUntilFree ==
  adoption = "adopted" =>
    \A p \in listed : \A k \in Kids(p) \cap listed :
      Read(k) + Count(upLog, k) >= 1 + Count(downLog, k)

\* Every listed block that nothing holds is queued.
ZeroQueued ==
  adoption = "adopted" =>
    \A b \in listed : Read(b) = 0 /\ Count(upLog, b) = 0 => b \in queued

\* The cursor exists exactly while the Volume is adopting.
CursorLife == (cursor # NoCursor) = Adopting
====
