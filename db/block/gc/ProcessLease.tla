---- MODULE ProcessLease ----
\* Format 11's process lease: one process holds its reader pins and stage
\* roots under one lease, renews the lease and not each root, and the
\* collector releases every root of a lease that lapsed through an index of
\* the roots by lease, one root per transaction at worst.
\*
\* A lease record is "none" before it starts, then "live", "lapsed" once its
\* expiry passes without renewal, "released" once the collector marks it in
\* the transaction of its first release, and "ended" once the collector
\* deletes its record in a slice that finds no root under it. Lapse may fire
\* at any time, standing for a stall of any length. Write lands a root on the
\* live process lease, as Land's lease check allows; Drop releases one, as a
\* reader or stage does. Recover is the renewal that finds its lease not live:
\* it starts a fresh lease, moves the roots indexed under the old one, holds
\* again each released root whose block still has holders, and loses the
\* stage of each one whose block lost them. Net drops the holders of a block
\* no root holds, as netting a release past the grace period does.
\*
\* Several renewers may run renewRoots at once: the ticker, a collection, a
\* writer whose lease ran short and each opening stage. A renewal commits
\* its lease (Recover or Renew) and publishes it to the process only after
\* the commit (Publish), and the renew lock runs one renewal at a time, from
\* its snapshot through its commit to its publication. Without the lock a
\* second renewer reads the old published lease, starts another, and leaves
\* the first's roots to lapse while the process holds them.
\*
\* Callers found from source: Records' leases.go and Land's lease check
\* (db/block/gc), the collector's sweepLeases (collector.go), and the Volume's
\* renewRoots, withProcessLease and rehold (db/volume/common/kvtx/lease.go),
\* under which pinRoot (pins.go) and the stage puts (stage.go) write. Cache
\* fills hold their roots under leases they never renew, which this model
\* leaves out: Land's same check covers them.

EXTENDS Naturals, FiniteSets

CONSTANTS
  Roots,     \* the root names the process may hold
  MaxLeases, \* the leases the process may start, so the model is finite
  Renewers,  \* the callers that may run renewRoots at once
  NoRenewer  \* the renew lock's holder when it is free

NoLease == 0
Leases == 1..MaxLeases

VARIABLES
  lease,   \* the record of each lease
  lapsed,  \* the leases whose expiry ever passed without renewal
  proc,    \* the process lease, NoLease before the first
  next,    \* the next fresh lease ID
  rootOf,  \* the lease each stored root names, NoLease when not stored
  index,   \* the lease index records, pairs <<lease, root>>
  inMem,   \* the roots the process's pins and stages hold in memory
  alive,   \* the roots whose block is stored with holders
  lost,    \* the roots whose stage the process lost
  pend,    \* the lease each renewer committed and has not published yet
  holder,  \* the renewer holding the renew lock, or NoRenewer
  bad      \* the collector released a root the process holds in memory
           \* while the published lease was live

vars == <<lease, lapsed, proc, next, rootOf, index, inMem, alive, lost, pend, holder, bad>>

Pending == {pend[w] : w \in Renewers} \ {NoLease}

States == {"none", "live", "lapsed", "released", "ended"}

TypeOK ==
  /\ lease \in [Leases -> States]
  /\ lapsed \subseteq Leases
  /\ proc \in Leases \cup {NoLease}
  /\ next \in 1..MaxLeases + 1
  /\ rootOf \in [Roots -> Leases \cup {NoLease}]
  /\ index \subseteq Leases \X Roots
  /\ inMem \subseteq Roots
  /\ alive \subseteq Roots
  /\ lost \subseteq Roots
  /\ pend \in [Renewers -> Leases \cup {NoLease}]
  /\ holder \in Renewers \cup {NoRenewer}
  /\ bad \in BOOLEAN

Init ==
  /\ lease = [l \in Leases |-> "none"]
  /\ lapsed = {}
  /\ proc = NoLease
  /\ next = 1
  /\ rootOf = [r \in Roots |-> NoLease]
  /\ index = {}
  /\ inMem = {}
  /\ alive = {}
  /\ lost = {}
  /\ pend = [w \in Renewers |-> NoLease]
  /\ holder = NoRenewer
  /\ bad = FALSE

Live(l) == l # NoLease /\ lease[l] = "live"

\* Recover starts the first lease too: the first stage or pin renews before
\* it writes, as withProcessLease and OpenStage do. The renewer takes the
\* renew lock, reads the published lease in its transaction, and commits a
\* new one, which the process does not use until Publish.
Recover(w) ==
  /\ holder = NoRenewer
  /\ pend[w] = NoLease
  /\ ~Live(proc)
  /\ next <= MaxLeases
  /\ LET id     == next
         moved  == {r \in Roots : proc # NoLease /\ rootOf[r] = proc}
         rehold == {r \in inMem \ moved : rootOf[r] = NoLease /\ r \in alive}
         gone   == {r \in inMem \ moved : rootOf[r] = NoLease /\ r \notin alive}
         keep   == moved \cup rehold
     IN /\ lease' = [lease EXCEPT ![id] = "live"]
        /\ pend' = [pend EXCEPT ![w] = id]
        /\ holder' = w
        /\ next' = next + 1
        /\ rootOf' = [r \in Roots |-> IF r \in keep THEN id ELSE rootOf[r]]
        /\ index' = {e \in index : e[2] \notin keep} \cup {<<id, r>> : r \in keep}
        /\ inMem' = inMem \ gone
        /\ lost' = lost \cup gone
        /\ UNCHANGED <<lapsed, alive, proc, bad>>

\* Publish is renewRoots setting the process lease after its commit, then
\* freeing the renew lock.
Publish(w) ==
  /\ pend[w] # NoLease
  /\ proc' = pend[w]
  /\ pend' = [pend EXCEPT ![w] = NoLease]
  /\ holder' = IF holder = w THEN NoRenewer ELSE holder
  /\ UNCHANGED <<lease, lapsed, next, rootOf, index, inMem, alive, lost, bad>>

\* Renew moves the expiry of the live process lease forward under the renew
\* lock, publishing nothing new. RenewLease renews only a live lease.
Renew ==
  /\ holder = NoRenewer
  /\ proc # NoLease
  /\ lease[proc] \in {"live"}
  /\ lease' = [lease EXCEPT ![proc] = "live"]
  /\ UNCHANGED <<lapsed, proc, next, rootOf, index, inMem, alive, lost, pend, holder, bad>>

Lapse(l) ==
  /\ lease[l] = "live"
  /\ lease' = [lease EXCEPT ![l] = "lapsed"]
  /\ lapsed' = lapsed \cup {l}
  /\ UNCHANGED <<proc, next, rootOf, index, inMem, alive, lost, pend, holder, bad>>

\* Write lands a new root on the process lease while it is live.
Write(r) ==
  /\ r \notin inMem
  /\ rootOf[r] = NoLease
  /\ Live(proc)
  /\ rootOf' = [rootOf EXCEPT ![r] = proc]
  /\ index' = index \cup {<<proc, r>>}
  /\ inMem' = inMem \cup {r}
  /\ alive' = alive \cup {r}
  /\ lost' = lost \ {r}
  /\ UNCHANGED <<lease, lapsed, proc, next, pend, holder, bad>>

\* Drop releases a root the process holds, whatever its lease.
Drop(r) ==
  /\ r \in inMem
  /\ rootOf' = [rootOf EXCEPT ![r] = NoLease]
  /\ index' = {e \in index : e[2] # r}
  /\ inMem' = inMem \ {r}
  /\ UNCHANGED <<lease, lapsed, proc, next, alive, lost, pend, holder, bad>>

\* CollectRelease releases one root of a lease that is not live and marks
\* the lease released in the same transaction. A stale index record, whose
\* root names another lease, is only deleted.
CollectRelease(l, r) ==
  /\ lease[l] \in {"lapsed", "released"}
  /\ <<l, r>> \in index
  /\ lease' = [lease EXCEPT ![l] = "released"]
  /\ index' = index \ {<<l, r>>}
  /\ rootOf' = IF rootOf[r] = l THEN [rootOf EXCEPT ![r] = NoLease] ELSE rootOf
  /\ bad' = (bad \/ (rootOf[r] = l /\ r \in inMem /\ Live(proc)))
  /\ UNCHANGED <<lapsed, proc, next, inMem, alive, lost, pend, holder>>

\* CollectEnd deletes the record of a lease that is not live and holds no
\* root.
CollectEnd(l) ==
  /\ lease[l] \in {"lapsed", "released"}
  /\ \A e \in index : e[1] # l
  /\ lease' = [lease EXCEPT ![l] = "ended"]
  /\ UNCHANGED <<lapsed, proc, next, rootOf, index, inMem, alive, lost, pend, holder, bad>>

\* Net drops the holders of a block no root holds.
Net(r) ==
  /\ rootOf[r] = NoLease
  /\ r \in alive
  /\ alive' = alive \ {r}
  /\ UNCHANGED <<lease, lapsed, proc, next, rootOf, index, inMem, lost, pend, holder, bad>>

Next ==
  \/ Renew
  \/ \E w \in Renewers : Recover(w) \/ Publish(w)
  \/ \E l \in Leases : Lapse(l) \/ CollectEnd(l)
  \/ \E l \in Leases, r \in Roots : CollectRelease(l, r)
  \/ \E r \in Roots : Write(r) \/ Drop(r) \/ Net(r)

Spec == Init /\ [][Next]_vars

\* LiveHolds: while the process lease is live, every root the process holds
\* in memory is stored under it, so no collector releases one.
LiveHolds == Live(proc) => \A r \in inMem : rootOf[r] = proc

\* ReleasedLapsed: the collector releases or ends only a lease that lapsed.
ReleasedLapsed == \A l \in Leases : lease[l] \in {"released", "ended"} => l \in lapsed

\* IndexExact: the index records exactly the roots stored under each lease,
\* so each root of a released lease is released once.
IndexExact == \A l \in Leases, r \in Roots : (<<l, r>> \in index) <=> (rootOf[r] = l)

\* RootsOnRecords: a stored root names a lease whose record exists.
RootsOnRecords == \A r \in Roots : rootOf[r] # NoLease => lease[rootOf[r]] \in {"live", "lapsed", "released"}

\* StoredAlive: a stored root's block has holders.
StoredAlive == \A r \in Roots : rootOf[r] # NoLease => r \in alive

\* NoReleaseUnderLiveProcess: the collector never releases a root the
\* process holds in memory while the published lease is live, where no
\* recovery would hold it again.
NoReleaseUnderLiveProcess == ~bad

\* InMemUnderOwnLease: a root the process holds in memory is stored only
\* under the published lease or one a renewer committed and will publish,
\* unless the collector released it.
InMemUnderOwnLease == \A r \in inMem : rootOf[r] \in {NoLease, proc} \cup Pending
====
