---- MODULE Adopt ----
\* How a Volume whose counts nobody trusts reaches format 10's exact counts
\* while writers keep committing: the online recount that adoption and the
\* format 9 upgrade run before frees may start.
\*
\* While the Volume is unadopted, Land stores lists and moves roots and keeps
\* no counts. AdoptStart begins a run, and AdoptClear deletes the stale counts;
\* it runs in slices, but Land keeps no counts until the mark, so one step
\* stands for them. AdoptMark takes the mark once every count is
\* clear and starts fresh logs. From the mark Land keeps counts as Liveness.tla
\* does, and also writes a born mark ('B') on each block it gives birth to and,
\* the first time it moves a root, the root's target at the mark ('A').
\* AdoptStep scans the next record in key order, roots first and then child
\* lists: it counts a root's target at the mark, or the children of a listed
\* block that has no born mark, so the scan counts the state at the mark and
\* the logs count each change after it. The scan's cursor commits with the
\* counts it covers. AdoptEnd flips the Volume to adopted once the cursor has
\* passed every record, queuing each listed block at zero. AdoptResume is a
\* process that dies mid-scan and a later one that takes the run over: it
\* continues after the cursor. AdoptCrash loses a run at any point, returning
\* the Volume to unadopted, and the next run clears the counts and the
\* cursor again.
\*
\* At adoption the counts are those Liveness.tla starts from: CountsExact and
\* HoldUntilFree hold, and every block at zero is queued. Frees and the
\* release log's netting wait for adoption, so this model has neither.
\*
\* Callers found from source: Adopt and its slices (db/block/gc/adopt.go,
\* adopt-records.go), Land (land.go) and the kvtx Volume that starts a run
\* when frees turn on (db/volume/common/kvtx/volume.go).

EXTENDS Naturals, Sequences, FiniteSets

CONSTANTS
  MaxTxns,    \* bounds the Land transactions, so the model is finite
  MaxCrashes  \* bounds the lost runs

None == "none"
Unset == "unset"
Blocks == {"a", "b", "c", "d"}
RootNames == {"main", "stg"}
Phases == {"off", "clear", "scan", "adopted"}

\* Stale is a count per block that matches no state: an orphan, one too many,
\* and two missing.
Stale == [a |-> 0, b |-> 1, c |-> 2, d |-> 0]

Kids(b) == [a |-> {"c"}, b |-> {"c"}, c |-> {"d"}, d |-> {}][b]

VARIABLES
  phase,     \* "off", "clear", "scan" or "adopted"
  listed,    \* blocks stored with their child list
  H,         \* each block's count: stale until cleared
  queued,    \* blocks at zero that hold a 'Q' entry
  roots,     \* each root's target, or None
  upLog,     \* U: the blocks of unnetted increments since the mark
  downLog,   \* D: the blocks of releases since the mark
  cleared,   \* whether this run cleared the stale counts
  bmark,     \* born marks: blocks born since the mark
  amark,     \* each root's target at the mark, Unset until it moves
  cursor,    \* the records the scan has counted, a prefix of Scan
  txns,      \* Land transactions so far
  crashes    \* runs lost so far
vars == <<phase, listed, H, queued, roots, upLog, downLog, cleared, bmark, amark, cursor, txns, crashes>>

RECURSIVE SeqOf(_)
SeqOf(S) ==
  IF S = {} THEN <<>>
  ELSE LET x == CHOOSE x \in S : TRUE IN <<x>> \o SeqOf(S \ {x})

\* Scan is the scan's records in key order: the root names, then the child
\* list keys of every block that may be listed.
Scan == <<[k |-> "root", n |-> "main"], [k |-> "root", n |-> "stg"],
          [k |-> "list", p |-> "a"], [k |-> "list", p |-> "b"],
          [k |-> "list", p |-> "c"], [k |-> "list", p |-> "d"]>>

Count(s, b) == Cardinality({i \in 1..Len(s) : s[i] = b})
Holders(b) ==
  Cardinality({n \in RootNames : roots[n] = b})
    + Cardinality({p \in listed : b \in Kids(p)})
Counting == phase \in {"scan", "adopted"}

Init ==
  /\ phase = "off"
  /\ listed = {"a", "c", "d"}
  /\ H = Stale
  /\ queued = {}
  /\ roots = [n \in RootNames |-> IF n = "main" THEN "a" ELSE None]
  /\ upLog = <<>>
  /\ downLog = <<>>
  /\ cleared = FALSE
  /\ bmark = {}
  /\ amark = [n \in RootNames |-> Unset]
  /\ cursor = 0
  /\ txns = 0
  /\ crashes = 0

\* Land stores batch and moves root n to t in one transaction; it keeps
\* counts and marks only from the mark on.
Land ==
  \E n \in RootNames, t \in Blocks \cup {None}, batch \in SUBSET Blocks :
    LET born == batch \ listed
        move == t # roots[n]
        edges == UNION {{<<p, k>> : k \in Kids(p) \ born} : p \in born}
        tUp == move /\ t # None /\ t \notin born
        ups == [i \in 1..Cardinality(edges) |-> SeqOf(edges)[i][2]]
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
    /\ roots' = [roots EXCEPT ![n] = t]
    /\ txns' = txns + 1
    /\ IF Counting
         THEN /\ H' = [b \in Blocks |-> IF b \in born THEN H[b] + birth(b) ELSE H[b]]
              /\ upLog' = upLog \o ups
              /\ downLog' = IF move /\ roots[n] # None
                              THEN Append(downLog, roots[n]) ELSE downLog
              /\ queued' = queued \ named
              /\ bmark' = bmark \cup born
              /\ amark' = IF move /\ amark[n] = Unset
                            THEN [amark EXCEPT ![n] = roots[n]] ELSE amark
         ELSE UNCHANGED <<H, upLog, downLog, queued, bmark, amark>>
    /\ UNCHANGED <<phase, cleared, cursor, crashes>>

AdoptStart ==
  /\ phase = "off"
  /\ phase' = "clear"
  /\ UNCHANGED <<listed, H, queued, roots, upLog, downLog, cleared, bmark, amark, cursor, txns, crashes>>

AdoptClear ==
  /\ phase = "clear"
  /\ ~cleared
  /\ H' = [b \in Blocks |-> 0]
  /\ cleared' = TRUE
  /\ cursor' = 0
  /\ UNCHANGED <<phase, listed, queued, roots, upLog, downLog, bmark, amark, txns, crashes>>

AdoptMark ==
  /\ phase = "clear"
  /\ cleared
  /\ phase' = "scan"
  /\ upLog' = <<>>
  /\ downLog' = <<>>
  /\ queued' = {}
  /\ bmark' = {}
  /\ amark' = [n \in RootNames |-> Unset]
  /\ UNCHANGED <<listed, H, roots, cleared, cursor, txns, crashes>>

AdoptStep ==
  /\ phase = "scan"
  /\ cursor < Len(Scan)
  /\ LET r == Scan[cursor + 1] IN
       IF r.k = "root"
         THEN LET v == IF amark[r.n] # Unset THEN amark[r.n] ELSE roots[r.n] IN
                H' = IF v = None THEN H ELSE [H EXCEPT ![v] = @ + 1]
         ELSE H' = IF r.p \notin listed \/ r.p \in bmark THEN H
                   ELSE [b \in Blocks |-> IF b \in Kids(r.p) THEN H[b] + 1 ELSE H[b]]
  /\ cursor' = cursor + 1
  /\ UNCHANGED <<phase, listed, queued, roots, upLog, downLog, cleared, bmark, amark, txns, crashes>>

AdoptEnd ==
  /\ phase = "scan"
  /\ cursor = Len(Scan)
  /\ phase' = "adopted"
  /\ queued' = {b \in listed : H[b] = 0}
  /\ UNCHANGED <<listed, H, roots, upLog, downLog, cleared, bmark, amark, cursor, txns, crashes>>

AdoptCrash ==
  /\ phase \in {"clear", "scan"}
  /\ crashes < MaxCrashes
  /\ phase' = "clear"
  /\ cleared' = FALSE
  /\ crashes' = crashes + 1
  /\ UNCHANGED <<listed, H, queued, roots, upLog, downLog, bmark, amark, cursor, txns>>

\* A process dies mid-scan and another takes the run over. The counts and the
\* cursor committed together, so it continues after the cursor.
AdoptResume ==
  /\ phase = "scan"
  /\ crashes < MaxCrashes
  /\ crashes' = crashes + 1
  /\ cursor' = cursor
  /\ UNCHANGED <<phase, listed, H, queued, roots, upLog, downLog, cleared, bmark, amark, txns>>

NetUps ==
  /\ Counting
  /\ upLog # <<>>
  /\ H' = [H EXCEPT ![Head(upLog)] = @ + 1]
  /\ queued' = queued \ {Head(upLog)}
  /\ upLog' = Tail(upLog)
  /\ UNCHANGED <<phase, listed, roots, downLog, cleared, bmark, amark, cursor, txns, crashes>>

Next ==
  \/ Land \/ AdoptStart \/ AdoptClear \/ AdoptMark \/ AdoptStep \/ AdoptEnd
  \/ AdoptCrash \/ AdoptResume \/ NetUps

Spec == Init /\ [][Next]_vars

\* Adopted counts, with the unnetted logs, equal the roots and listed parents
\* that name each block.
CountsExact ==
  phase = "adopted" =>
    \A b \in Blocks : H[b] + Count(upLog, b) = Holders(b) + Count(downLog, b)

\* Each child of a listed block counts at least that parent.
HoldUntilFree ==
  phase = "adopted" =>
    \A p \in listed : \A k \in Kids(p) : H[k] + Count(upLog, k) >= 1 + Count(downLog, k)

\* Every listed block that nothing holds is queued.
ZeroQueued ==
  phase = "adopted" =>
    \A b \in listed : H[b] = 0 /\ Count(upLog, b) = 0 => b \in queued
====
