---- MODULE SpaceWorldRegen ----
\* Regeneration of a Space World through its agreed order: the contract of
\* RegenerateWorldOp's apply in core/space/world/space-regen.go. A Space World
\* has no single writer, so the regeneration is an ordered operation that
\* every replica applies to the World before it, as WorldRegen.tla's local
\* Regenerator does for a World on one Authority.
\*
\* The lifecycle, by the reserved retained roots the checkpoint carries:
\* - idle: neither world-regen/base nor world-regen/built is held.
\* - switched: a RegenerateWorldOp to the new format applied. It first adds
\*   the carried quads to the World before it, in that World's format, and
\*   holds the result as base; then it builds base in the new format, holds
\*   the build as built, and installs it as the head.
\* - switched back: a RegenerateWorldOp back to the old format applied while
\*   base was held. The head is base carried forward by the new format's
\*   changes since built; both names are released.
\* - released: an ordered release named the held base and built after one
\*   device's comparison of the two passed. The head stays in the new
\*   format. The release names both roots and applies only while both are
\*   held: one ordered before a switch back and a second regeneration must
\*   not free that regeneration's base, which no comparison passed, and the
\*   second base can equal the first when the World did not change.
\*
\* base holds the carried quads, not the World before the operation, because
\* a switch back from that World loses a write: a write after the operation
\* that sets a carried quad changes nothing in the new format, so the diff
\* since built cannot carry it back. The first version of this model held the
\* World before and found that trace.
\*
\* A World's content is a set of items, object records and quads alike. A
\* write adds or deletes one item. Carry is the set the operation carries,
\* which may hold items the World already holds.
\*
\* Actors:
\* - Proposers: modeled. Any device appends any operation to the order at any
\*   time, so a write, a regeneration, a switch back or a release can follow
\*   every state of every replica.
\* - Replicas: modeled, two. Each applies the order in sequence. Applying a
\*   regeneration first builds, then commits the result with the replay
\*   cursor in one step; a crash during the build loses only the build.
\* - The comparison: modeled as the guard on proposing a release. It reads a
\*   replica's held roots off the replay path and passes only when built
\*   holds exactly what base holds; the release names both roots.
\* - A full disk: modeled. A replica whose disk is below the build's floor
\*   cannot build, and one whose disk fills partway through the build loses
\*   the build; either waits at the operation and never refuses it.
\* - A malformed operation: modeled. A quad list that does not decode, or a
\*   base whose graph does not decode, is a property of the operation and the
\*   World before it, so every replica meets it alike and refuses the
\*   operation instead of waiting at it forever.
\* - The collector: modeled. Replay may apply operations past the installed
\*   head, as behind a missing operation, and before the head installs a
\*   cursor save may move the floor past the position and release the pages
\*   that held base, or a checkpoint may cover the operation. The apply holds
\*   every root a replay World retains under the retained root before it
\*   appends the position, so the collector never frees an uninstalled base.
\* - Checkpoints, the adopt path and older builds: left out. Each installs a
\*   state another replica computed by the same apply, which Agree covers.
\*
\* With Faults, a regeneration may carry a defect that skips one item of the
\* build. The defect is in the code every replica runs, so it is part of the
\* operation's effect and every replica skips the same item. The comparison
\* must then refuse the release, and the switch back must restore the item.

EXTENDS Naturals, Sequences, FiniteSets

CONSTANTS Replicas, N, Carry, MaxOps, MaxCrashes, Faults

Items == 1..N
NoRoot == [fmt |-> "none", items |-> {}]
\* An operation's verdict on a replica: applied or refused.
Verdicts == {"applied", "refused"}

\* Apply moves set by the change from one content to another. The switch back
\* applies it leniently: a delete of an item set lacks, or an add of one it
\* holds, changes nothing. After a faulty build skipped an item, a write of
\* that item is such a change.
Apply(set, from, to) == (set \ (from \ to)) \cup (to \ from)

Ops ==
  [t: {"write"}, i: Items, add: BOOLEAN]
    \cup [t: {"regen"}, skip: 0..N, bad: BOOLEAN]
    \cup [t: {"back"}]
    \cup [t: {"release"}, base: SUBSET Items, built: SUBSET Items]

VARIABLES
  log,      \* the agreed order
  cur,      \* per replica: operations applied
  world,    \* per replica: the head's format and content
  base,     \* per replica: world-regen/base
  built,    \* per replica: world-regen/built
  verdict,  \* per replica: the verdict of each applied operation
  phase,    \* per replica: "idle", "building" or "down"
  pending,  \* per replica: an applied regeneration not yet installed as head
  full,     \* per replica: whether its disk is below the floor
  crashes,  \* crashes and disk fills so far
  exp,      \* ghost per replica: the content the operations define
  faulted   \* ghost: whether a faulty regeneration was ordered

vars == <<log, cur, world, base, built, verdict, phase, pending, full,
          crashes, exp, faulted>>

Init ==
  /\ log = <<>>
  /\ cur = [r \in Replicas |-> 0]
  /\ world = [r \in Replicas |-> [fmt |-> "old", items |-> {}]]
  /\ base = [r \in Replicas |-> NoRoot]
  /\ built = [r \in Replicas |-> NoRoot]
  /\ verdict = [r \in Replicas |-> <<>>]
  /\ phase = [r \in Replicas |-> "idle"]
  /\ pending = [r \in Replicas |-> FALSE]
  /\ full = [r \in Replicas |-> FALSE]
  /\ crashes = 0
  /\ exp = [r \in Replicas |-> {}]
  /\ faulted = FALSE

----
\* Proposers.

Propose(op) ==
  /\ Len(log) < MaxOps
  /\ log' = Append(log, op)
  /\ UNCHANGED <<cur, world, base, built, verdict, phase, pending, full, crashes,
                 exp>>

ProposeWrite ==
  \E i \in Items, add \in BOOLEAN :
    /\ Propose([t |-> "write", i |-> i, add |-> add])
    /\ UNCHANGED faulted

ProposeRegen ==
  \E skip \in (IF Faults THEN 0..N ELSE {0}), bad \in BOOLEAN :
    /\ Propose([t |-> "regen", skip |-> skip, bad |-> bad])
    /\ faulted' = (faulted \/ (skip # 0 /\ ~bad))

ProposeBack ==
  /\ Propose([t |-> "back"])
  /\ UNCHANGED faulted

\* The comparison runs on one replica off the replay path and passes only
\* when built holds base plus the carried quads; the release names base.
ProposeRelease ==
  \E r \in Replicas :
    /\ phase[r] # "down"
    /\ base[r] # NoRoot
    /\ built[r].items = base[r].items
    /\ Propose([t |-> "release", base |-> base[r].items, built |-> built[r].items])
    /\ UNCHANGED faulted

----
\* Replicas.

Next1(r) == log[cur[r] + 1]

\* Record the operation's verdict and move past it.
Done(r, v) ==
  /\ cur' = [cur EXCEPT ![r] = @ + 1]
  /\ verdict' = [verdict EXCEPT ![r] = Append(@, v)]

\* Apply a write: the head changes; the held roots stay as they were.
ApplyWrite(r) ==
  LET op == Next1(r)
      set(s) == IF op.add THEN s \cup {op.i} ELSE s \ {op.i}
  IN
  /\ phase[r] = "idle"
  /\ cur[r] < Len(log)
  /\ op.t = "write"
  /\ world' = [world EXCEPT ![r].items = set(@)]
  /\ exp' = [exp EXCEPT ![r] = set(@)]
  /\ Done(r, "applied")
  /\ UNCHANGED <<log, base, built, phase, pending, full, crashes, faulted>>

\* Whether replica r refuses the next operation, by the World and the held
\* roots alone, so every replica computes the same verdict.
RegenRefused(r) == world[r].fmt # "old" \/ base[r] # NoRoot
BackRefused(r) == world[r].fmt # "new" \/ base[r] = NoRoot

\* Refuse an operation that does not apply to this World, or a malformed
\* regeneration, which every replica meets alike.
Refuse(r) ==
  LET op == Next1(r) IN
  /\ phase[r] = "idle"
  /\ cur[r] < Len(log)
  /\ \/ op.t = "regen" /\ (RegenRefused(r) \/ op.bad)
     \/ op.t = "back" /\ BackRefused(r)
  /\ Done(r, "refused")
  /\ UNCHANGED <<log, world, base, built, phase, pending, full, crashes,
                 exp, faulted>>

\* Start the build. A replica whose disk is below the floor waits here: the
\* failure is local, so it is a replay gap and never a refusal.
StartBuild(r) ==
  LET op == Next1(r) IN
  /\ phase[r] = "idle"
  /\ cur[r] < Len(log)
  /\ op.t = "regen"
  /\ ~RegenRefused(r)
  /\ ~op.bad
  /\ ~full[r]
  /\ phase' = [phase EXCEPT ![r] = "building"]
  /\ UNCHANGED <<log, cur, world, base, built, verdict, pending, full,
                 crashes, exp, faulted>>

\* Commit the build with the replay cursor: base is the World before it plus
\* the carried quads, and the new-format head is the build of base, less the
\* item a defect skips; base and built are retained in the same step.
FinishBuild(r) ==
  LET op == Next1(r)
      old == world[r].items \cup Carry
      new == old \ {op.skip}
  IN
  /\ phase[r] = "building"
  /\ ~full[r]
  /\ world' = [world EXCEPT ![r] = [fmt |-> "new", items |-> new]]
  /\ base' = [base EXCEPT ![r] = [fmt |-> "old", items |-> old]]
  /\ built' = [built EXCEPT ![r] = [fmt |-> "new", items |-> new]]
  /\ exp' = [exp EXCEPT ![r] = @ \cup Carry]
  /\ phase' = [phase EXCEPT ![r] = "idle"]
  /\ pending' = [pending EXCEPT ![r] = TRUE]
  /\ Done(r, "applied")
  /\ UNCHANGED <<log, full, crashes, faulted>>

\* A disk that fills partway through the build fails it: the replica writes
\* nothing and waits at the operation, a replay gap, until the disk frees.
FailBuild(r) ==
  /\ phase[r] = "building"
  /\ full[r]
  /\ phase' = [phase EXCEPT ![r] = "idle"]
  /\ UNCHANGED <<log, cur, world, base, built, verdict, pending, full,
                 crashes, exp, faulted>>

\* The applied state installs as the head.
Install(r) ==
  /\ pending[r]
  /\ pending' = [pending EXCEPT ![r] = FALSE]
  /\ UNCHANGED <<log, cur, world, base, built, verdict, phase, full,
                 crashes, exp, faulted>>

\* Whether replica r's base is held: the apply holds it under the retained
\* root, installed or not, until the base and the positions move past its
\* release.
Live(r) == TRUE

\* The collector frees a base it does not count as live. A freed base cannot
\* be read again, so a switch back from it waits forever.
Sweep(r) ==
  /\ base[r] \notin {NoRoot}
  /\ base[r].fmt = "old"
  /\ ~Live(r)
  /\ base' = [base EXCEPT ![r].fmt = "freed"]
  /\ UNCHANGED <<log, cur, world, built, verdict, phase, pending, full,
                 crashes, exp, faulted>>

\* Switch back: carry base forward by the new format's changes since built,
\* and release both names.
ApplyBack(r) ==
  /\ phase[r] = "idle"
  /\ cur[r] < Len(log)
  /\ Next1(r).t = "back"
  /\ ~BackRefused(r)
  /\ base[r].fmt = "old"
  /\ world' = [world EXCEPT ![r] =
       [fmt |-> "old", items |-> Apply(base[r].items, built[r].items, world[r].items)]]
  /\ base' = [base EXCEPT ![r] = NoRoot]
  /\ built' = [built EXCEPT ![r] = NoRoot]
  /\ Done(r, "applied")
  /\ UNCHANGED <<log, phase, pending, full, crashes, exp, faulted>>

\* Release the held roots when they are the ones the comparison named; a
\* stale release, ordered before a switch back and a new regeneration, is
\* refused so it cannot free a base no comparison passed.
ApplyRelease(r) ==
  LET op == Next1(r)
      ok == base[r] # NoRoot /\ base[r].items = op.base /\ built[r].items = op.built
  IN
  /\ phase[r] = "idle"
  /\ cur[r] < Len(log)
  /\ op.t = "release"
  /\ base' = [base EXCEPT ![r] = IF ok THEN NoRoot ELSE @]
  /\ built' = [built EXCEPT ![r] = IF ok THEN NoRoot ELSE @]
  /\ Done(r, IF ok THEN "applied" ELSE "refused")
  /\ UNCHANGED <<log, world, phase, pending, full, crashes, exp, faulted>>

\* A crash loses the build and nothing the replay cursor holds.
Crash(r) ==
  /\ crashes < MaxCrashes
  /\ phase[r] # "down"
  /\ phase' = [phase EXCEPT ![r] = "down"]
  /\ crashes' = crashes + 1
  /\ UNCHANGED <<log, cur, world, base, built, verdict, pending, full,
                 exp, faulted>>

Recover(r) ==
  /\ phase[r] = "down"
  /\ phase' = [phase EXCEPT ![r] = "idle"]
  /\ UNCHANGED <<log, cur, world, base, built, verdict, pending, full,
                 crashes, exp, faulted>>

\* The disk falls below the floor, then frees.
Fill(r) ==
  /\ crashes < MaxCrashes
  /\ ~full[r]
  /\ full' = [full EXCEPT ![r] = TRUE]
  /\ crashes' = crashes + 1
  /\ UNCHANGED <<log, cur, world, base, built, verdict, phase, pending,
                 exp, faulted>>

Free(r) ==
  /\ full[r]
  /\ full' = [full EXCEPT ![r] = FALSE]
  /\ UNCHANGED <<log, cur, world, base, built, verdict, phase, pending,
                 crashes, exp, faulted>>

Replay(r) ==
  \/ ApplyWrite(r) \/ Refuse(r) \/ StartBuild(r) \/ FinishBuild(r)
  \/ ApplyBack(r) \/ ApplyRelease(r) \/ FailBuild(r)

\* The order is complete and every replica applied it.
Terminated ==
  /\ Len(log) = MaxOps
  /\ \A r \in Replicas : cur[r] = Len(log)
  /\ UNCHANGED vars

Next ==
  \/ Terminated
  \/ ProposeWrite \/ ProposeRegen \/ ProposeBack \/ ProposeRelease
  \/ \E r \in Replicas :
       \/ Replay(r) \/ Crash(r) \/ Recover(r) \/ Fill(r) \/ Free(r)
       \/ Install(r) \/ Sweep(r)

Spec ==
  /\ Init /\ [][Next]_vars
  /\ \A r \in Replicas :
       /\ WF_vars(Replay(r)) /\ WF_vars(Recover(r)) /\ WF_vars(Free(r))
       /\ WF_vars(Install(r))

----
\* Properties.

TypeOK ==
  /\ cur \in [Replicas -> 0..MaxOps]
  /\ phase \in [Replicas -> {"idle", "building", "down"}]
  /\ pending \in [Replicas -> BOOLEAN]
  /\ \A r \in Replicas : cur[r] <= Len(log)

\* Replicas that applied the same operations hold the same World, roots and
\* verdicts: the result is a function of the World before and the order.
Agree ==
  \A r, s \in Replicas :
    cur[r] = cur[s] =>
      /\ world[r] = world[s]
      /\ base[r] = base[s]
      /\ built[r] = built[s]
      /\ verdict[r] = verdict[s]

\* Without a defect, every replica's head holds exactly what the operations
\* define: no item is lost or invented, through any switch and switch back.
NoLoss == ~faulted => \A r \in Replicas : world[r].items = exp[r]

\* Even with a defect, a World back in the old format and holding no
\* regeneration has lost nothing the operations define, and nothing a
\* regeneration skipped stays lost.
BackLosesNothing ==
  \A r \in Replicas :
    (world[r].fmt = "old" /\ base[r] = NoRoot) => exp[r] \subseteq world[r].items

\* While held, built is the head the regeneration installed or a release
\* has not yet come: base and built are held together.
HeldTogether == \A r \in Replicas : (base[r] = NoRoot) = (built[r] = NoRoot)

\* A new-format World whose base was released passed the comparison, so it
\* holds what the operations define even with a defect ordered.
ReleasedOnlyEqual ==
  \A r \in Replicas :
    (world[r].fmt = "new" /\ base[r] = NoRoot) => world[r].items = exp[r]

\* Every replica applies the whole order: a full disk or a crash delays a
\* replica, and nothing stops it.
Finishes == <>[](\A r \in Replicas : cur[r] = Len(log))

====
