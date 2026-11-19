---- MODULE SpaceWorldRegen ----
\* Regeneration of a Space World through its agreed order: the contract of
\* RegenerateWorldOp's apply in core/space/world/space-regen.go. A Space World
\* has no single writer, so the regeneration is ordered, and every replica
\* applies it to the World before it, as WorldRegen.tla's local Regenerator
\* does for a World on one Authority. A build of the whole World under the
\* replay turn stalls every write behind it, so the regeneration is split as
\* the Regenerator splits it: pin, build off the path, switch by a bounded
\* diff.
\*
\* The lifecycle, by the reserved retained roots the checkpoint carries:
\* - idle: no root is held.
\* - pinned: a RegenerateWorldOp naming no base applied. It holds the World
\*   before it as world-regen/pin, the pin, which already holds the carried
\*   quads. The head stays in the old format. A second pin while only a pin
\*   is held moves it to the newer World.
\* - building: off the replay path, each replica builds the pin in the new
\*   format into a prebuilt root it keeps locally and does not persist. A
\*   crash loses the prebuilt root, and the build resumes in the background
\*   after the restart while replay goes on.
\* - switched: a RegenerateWorldOp naming the pin applied. It counts the quad
\*   changes from the pin to the head and refuses past the bound, a function
\*   of the World and the order that every replica computes alike. Within the
\*   bound it carries the prebuilt root forward by those changes and installs
\*   the result as the head; a replica with no prebuilt root for the pin
\*   builds it inline by the same steps, build then diff, so its result is
\*   the same even when the build has a defect. The head before the switch is
\*   then held as base and the installed head as built, so the comparison and
\*   the switch back keep their meaning.
\* - switched back: a RegenerateWorldOp back to the old format applied while
\*   base was held. The head is base carried forward by the new format's
\*   changes since built; both names are released.
\* - released: an ordered release named the held base and built after one
\*   device's comparison of the two passed. The head stays in the new
\*   format. The release names both roots and applies only while both are
\*   held: one ordered before a switch back and a second regeneration must
\*   not free that regeneration's base, which no comparison passed.
\*
\* The carried quads reach the World as ordinary writes, each bounded, that
\* the engine orders before the pin, so no step that carries them holds
\* replay longer than one write. The pin then carries nothing: a delete
\* ordered between two of those writes, or between them and the pin, stands
\* as any write does, and the switch's diff is what it would be had the pin
\* carried them.
\*
\* A World's content is a set of items, object records and quads alike. A
\* write adds or deletes one item. Carry is the set the engine writes before
\* the pin, which may hold items the World already holds. Bound is the
\* largest count of item changes a switch applies.
\*
\* Actors:
\* - Proposers: modeled. Any device appends any operation to the order at any
\*   time, so a write, a carried quad's write, a pin, a switch, a switch back
\*   or a release can follow every state of every replica. A switch names a
\*   pin some replica held.
\* - Replicas: modeled, two. Each applies the order in sequence. An inline
\*   build commits its result with the replay cursor in one step; a crash
\*   during it loses only the build.
\* - The background builder: modeled, one per replica. It builds the pin
\*   while replay goes on, and never holds replay up.
\* - The comparison: modeled as the guard on proposing a release. It reads a
\*   replica's held roots off the replay path and passes only when built
\*   holds exactly what base holds; the release names both roots.
\* - A full disk: modeled. A replica whose disk is below the build's floor
\*   cannot build, and one whose disk fills partway through an inline build
\*   loses the build; either waits at the operation and never refuses it.
\* - A malformed operation: modeled. A quad list that does not decode is a
\*   property of the operation, so every replica refuses it alike.
\* - The collector: modeled. The apply holds the pin and the switch's roots
\*   under the retained root before it appends the position, so the
\*   collector never frees a pin or base an uninstalled head needs.
\* - Checkpoints, the adopt path and older builds: left out. Each installs a
\*   state another replica computed by the same apply, which Agree covers.
\*
\* With Faults, a pin may carry a defect that skips one item of the build.
\* The defect is in the code every replica runs, so it is part of the
\* operation's effect and every replica skips the same item. The comparison
\* must then refuse the release, and the switch back must restore the item.

EXTENDS Naturals, Sequences, FiniteSets

CONSTANTS Replicas, N, Carry, Bound, MaxOps, MaxCrashes, Faults

Items == 1..N
NoRoot == [fmt |-> "none", items |-> {}]
NoPin == [fmt |-> "none", items |-> {}, skip |-> 0]
NoBuild == [ok |-> FALSE, of |-> NoPin, items |-> {}]

\* Apply moves set by the change from one content to another. The switch back
\* applies it leniently: a delete of an item set lacks, or an add of one it
\* holds, changes nothing. After a faulty build skipped an item, a write of
\* that item is such a change.
Apply(set, from, to) == (set \ (from \ to)) \cup (to \ from)

\* The item changes from one content to another, as the switch counts them.
Diff(from, to) == (from \ to) \cup (to \ from)

Ops ==
  [t: {"write"}, i: Items, add: BOOLEAN]
    \cup [t: {"pin"}, skip: 0..N, bad: BOOLEAN]
    \cup [t: {"switch"}, base: SUBSET Items]
    \cup [t: {"back"}]
    \cup [t: {"release"}, base: SUBSET Items, built: SUBSET Items]

VARIABLES
  log,      \* the agreed order
  cur,      \* per replica: operations applied
  world,    \* per replica: the head's format and content
  pin,      \* per replica: world-regen/base while only the pin is held
  prebuilt, \* per replica: the local build of the pin, not persisted
  base,     \* per replica: world-regen/base after the switch
  built,    \* per replica: world-regen/built
  verdict,  \* per replica: the verdict of each applied operation
  phase,    \* per replica: "idle", "building" or "down"
  pending,  \* per replica: an applied state not yet installed as head
  full,     \* per replica: whether its disk is below the floor
  crashes,  \* crashes and disk fills so far
  stall,    \* ghost per replica: the most item changes a switch applied
  exp,      \* ghost per replica: the content the operations define
  faulted   \* ghost: whether a faulty pin was ordered

vars == <<log, cur, world, pin, prebuilt, base, built, verdict, phase,
          pending, full, crashes, stall, exp, faulted>>

Init ==
  /\ log = <<>>
  /\ cur = [r \in Replicas |-> 0]
  /\ world = [r \in Replicas |-> [fmt |-> "old", items |-> {}]]
  /\ pin = [r \in Replicas |-> NoPin]
  /\ prebuilt = [r \in Replicas |-> NoBuild]
  /\ base = [r \in Replicas |-> NoRoot]
  /\ built = [r \in Replicas |-> NoRoot]
  /\ verdict = [r \in Replicas |-> <<>>]
  /\ phase = [r \in Replicas |-> "idle"]
  /\ pending = [r \in Replicas |-> FALSE]
  /\ full = [r \in Replicas |-> FALSE]
  /\ crashes = 0
  /\ stall = [r \in Replicas |-> 0]
  /\ exp = [r \in Replicas |-> {}]
  /\ faulted = FALSE

----
\* Proposers.

Propose(op) ==
  /\ Len(log) < MaxOps
  /\ log' = Append(log, op)
  /\ UNCHANGED <<cur, world, pin, prebuilt, base, built, verdict, phase,
                 pending, full, crashes, stall, exp>>

ProposeWrite ==
  \E i \in Items, add \in BOOLEAN :
    /\ Propose([t |-> "write", i |-> i, add |-> add])
    /\ UNCHANGED faulted

\* The engine writes a carried quad as an ordinary write before the pin.
ProposeCarry ==
  \E i \in Carry :
    /\ Propose([t |-> "write", i |-> i, add |-> TRUE])
    /\ UNCHANGED faulted

ProposePin ==
  \E skip \in (IF Faults THEN 0..N ELSE {0}), bad \in BOOLEAN :
    /\ Propose([t |-> "pin", skip |-> skip, bad |-> bad])
    /\ faulted' = (faulted \/ (skip # 0 /\ ~bad))

\* The driver names the pin it saw applied on its own replica.
ProposeSwitch ==
  \E r \in Replicas :
    /\ pin[r] # NoPin
    /\ Propose([t |-> "switch", base |-> pin[r].items])
    /\ UNCHANGED faulted

ProposeBack ==
  /\ Propose([t |-> "back"])
  /\ UNCHANGED faulted

\* The comparison runs on one replica off the replay path and passes only
\* when built holds what base holds; the release names both.
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

\* Whether replica r is at an operation of type t.
At(r, t) ==
  /\ phase[r] = "idle"
  /\ cur[r] < Len(log)
  /\ Next1(r).t = t

\* Record the operation's verdict and move past it.
Done(r, v) ==
  /\ cur' = [cur EXCEPT ![r] = @ + 1]
  /\ verdict' = [verdict EXCEPT ![r] = Append(@, v)]

\* The build of a pin: the pin less the item a defect skips.
Build(p) == p.items \ {p.skip}

\* Whether replica r holds a prebuilt root of its current pin.
Ready(r) == prebuilt[r].ok /\ prebuilt[r].of = pin[r]

\* Apply a write: the head changes; the held roots stay as they were.
ApplyWrite(r) ==
  LET op == Next1(r)
      set(s) == IF op.add THEN s \cup {op.i} ELSE s \ {op.i}
  IN
  /\ At(r, "write")
  /\ world' = [world EXCEPT ![r].items = set(@)]
  /\ exp' = [exp EXCEPT ![r] = set(@)]
  /\ Done(r, "applied")
  /\ UNCHANGED <<log, pin, prebuilt, base, built, phase, pending, full,
                 crashes, stall, faulted>>

\* Whether replica r refuses the next operation, by the World, the held roots
\* and the order alone, so every replica computes the same verdict.
PinRefused(r) == world[r].fmt # "old" \/ base[r] # NoRoot
SwitchRefused(r) ==
  \/ world[r].fmt # "old"
  \/ base[r] # NoRoot
  \/ pin[r] = NoPin
  \/ pin[r].items # Next1(r).base
  \/ Cardinality(Diff(pin[r].items, world[r].items)) > Bound
BackRefused(r) == world[r].fmt # "new" \/ base[r] = NoRoot

\* Refuse an operation that does not apply to this World, or a malformed
\* pin, which every replica meets alike.
Refuse(r) ==
  LET op == Next1(r) IN
  /\ phase[r] = "idle"
  /\ cur[r] < Len(log)
  /\ \/ op.t = "pin" /\ (PinRefused(r) \/ op.bad)
     \/ op.t = "switch" /\ SwitchRefused(r)
     \/ op.t = "back" /\ BackRefused(r)
  /\ Done(r, "refused")
  /\ UNCHANGED <<log, world, pin, prebuilt, base, built, phase, pending, full,
                 crashes, stall, exp, faulted>>

\* Pin the World before, in the old format; a pin while only a pin is held
\* moves it. The World and what the operations define stay as they were.
ApplyPin(r) ==
  LET op == Next1(r) IN
  /\ At(r, "pin")
  /\ ~PinRefused(r)
  /\ ~op.bad
  /\ pin' = [pin EXCEPT ![r] = [fmt |-> "old", items |-> world[r].items, skip |-> op.skip]]
  /\ pending' = [pending EXCEPT ![r] = TRUE]
  /\ Done(r, "applied")
  /\ UNCHANGED <<log, world, prebuilt, base, built, phase, full, crashes,
                 stall, exp, faulted>>

\* The background builder builds the current pin off the replay path. It
\* reads only the pin, so replay goes on beside it.
BuildOffPath(r) ==
  /\ phase[r] # "down"
  /\ ~full[r]
  /\ pin[r].fmt = "old"
  /\ ~Ready(r)
  /\ prebuilt' = [prebuilt EXCEPT ![r] =
       [ok |-> TRUE, of |-> pin[r], items |-> Build(pin[r])]]
  /\ UNCHANGED <<log, cur, world, pin, base, built, verdict, phase, pending,
                 full, crashes, stall, exp, faulted>>

\* Switch to img, the build of the pin: carry it forward by the changes from
\* the pin to the head, install it, and hold the head before as base and the
\* installed head as built.
SwitchTo(r, img) ==
  LET new == Apply(img, pin[r].items, world[r].items) IN
  /\ world' = [world EXCEPT ![r] = [fmt |-> "new", items |-> new]]
  /\ base' = [base EXCEPT ![r] = [fmt |-> "old", items |-> world[r].items]]
  /\ built' = [built EXCEPT ![r] = [fmt |-> "new", items |-> new]]
  /\ pin' = [pin EXCEPT ![r] = NoPin]
  /\ prebuilt' = [prebuilt EXCEPT ![r] = NoBuild]
  /\ stall' = [stall EXCEPT ![r] =
       IF Cardinality(Diff(pin[r].items, world[r].items)) > @
       THEN Cardinality(Diff(pin[r].items, world[r].items)) ELSE @]
  /\ pending' = [pending EXCEPT ![r] = TRUE]
  /\ phase' = [phase EXCEPT ![r] = "idle"]
  /\ Done(r, "applied")

\* Switch from the prebuilt root: only the bounded diff runs on the path.
ApplySwitch(r) ==
  /\ At(r, "switch")
  /\ ~SwitchRefused(r)
  /\ pin[r].fmt = "old"
  /\ Ready(r)
  /\ SwitchTo(r, prebuilt[r].items)
  /\ UNCHANGED <<log, full, crashes, exp, faulted>>

\* A replica with no prebuilt root of the pin builds it inline. A replica
\* whose disk is below the floor waits here: the failure is local, so it is
\* a replay gap and never a refusal.
StartInline(r) ==
  /\ At(r, "switch")
  /\ ~SwitchRefused(r)
  /\ pin[r].fmt = "old"
  /\ ~Ready(r)
  /\ ~full[r]
  /\ phase' = [phase EXCEPT ![r] = "building"]
  /\ UNCHANGED <<log, cur, world, pin, prebuilt, base, built, verdict,
                 pending, full, crashes, stall, exp, faulted>>

\* The inline build runs the same steps as the background one, build then
\* diff, and commits with the replay cursor.
FinishInline(r) ==
  /\ phase[r] = "building"
  /\ ~full[r]
  /\ SwitchTo(r, Build(pin[r]))
  /\ UNCHANGED <<log, full, crashes, exp, faulted>>

\* A disk that fills partway through the inline build fails it: the replica
\* writes nothing and waits at the operation, a replay gap, until it frees.
FailInline(r) ==
  /\ phase[r] = "building"
  /\ full[r]
  /\ phase' = [phase EXCEPT ![r] = "idle"]
  /\ UNCHANGED <<log, cur, world, pin, prebuilt, base, built, verdict,
                 pending, full, crashes, stall, exp, faulted>>

\* The applied state installs as the head.
Install(r) ==
  /\ pending[r]
  /\ pending' = [pending EXCEPT ![r] = FALSE]
  /\ UNCHANGED <<log, cur, world, pin, prebuilt, base, built, verdict, phase,
                 full, crashes, stall, exp, faulted>>

\* Whether replica r's held roots are live: the apply holds the pin, base
\* and built under the retained root, installed or not, until the positions
\* move past their release.
Live(r) == TRUE

\* The collector frees a root it does not count as live. A freed root cannot
\* be read again, so a switch or switch back from it waits forever.
Sweep(r) ==
  /\ ~Live(r)
  /\ \/ /\ base[r].fmt = "old"
        /\ base' = [base EXCEPT ![r].fmt = "freed"]
        /\ UNCHANGED pin
     \/ /\ pin[r].fmt = "old"
        /\ pin' = [pin EXCEPT ![r].fmt = "freed"]
        /\ UNCHANGED base
  /\ UNCHANGED <<log, cur, world, prebuilt, built, verdict, phase, pending,
                 full, crashes, stall, exp, faulted>>

\* Switch back: carry base forward by the new format's changes since built,
\* and release both names.
ApplyBack(r) ==
  /\ At(r, "back")
  /\ ~BackRefused(r)
  /\ base[r].fmt = "old"
  /\ world' = [world EXCEPT ![r] =
       [fmt |-> "old", items |-> Apply(base[r].items, built[r].items, world[r].items)]]
  /\ base' = [base EXCEPT ![r] = NoRoot]
  /\ built' = [built EXCEPT ![r] = NoRoot]
  /\ Done(r, "applied")
  /\ UNCHANGED <<log, pin, prebuilt, phase, pending, full, crashes, stall,
                 exp, faulted>>

\* Release the held roots when they are the ones the comparison named; a
\* stale release, ordered after a switch back and a new regeneration, is
\* refused so it cannot free a base no comparison passed.
ApplyRelease(r) ==
  LET op == Next1(r)
      ok == base[r] # NoRoot /\ base[r].items = op.base /\ built[r].items = op.built
  IN
  /\ At(r, "release")
  /\ base' = [base EXCEPT ![r] = IF ok THEN NoRoot ELSE @]
  /\ built' = [built EXCEPT ![r] = IF ok THEN NoRoot ELSE @]
  /\ Done(r, IF ok THEN "applied" ELSE "refused")
  /\ UNCHANGED <<log, world, pin, prebuilt, phase, pending, full, crashes,
                 stall, exp, faulted>>

\* A crash loses the inline build and the prebuilt root, and nothing the
\* replay cursor holds.
Crash(r) ==
  /\ crashes < MaxCrashes
  /\ phase[r] # "down"
  /\ phase' = [phase EXCEPT ![r] = "down"]
  /\ prebuilt' = [prebuilt EXCEPT ![r] = NoBuild]
  /\ crashes' = crashes + 1
  /\ UNCHANGED <<log, cur, world, pin, base, built, verdict, pending, full,
                 stall, exp, faulted>>

Recover(r) ==
  /\ phase[r] = "down"
  /\ phase' = [phase EXCEPT ![r] = "idle"]
  /\ UNCHANGED <<log, cur, world, pin, prebuilt, base, built, verdict,
                 pending, full, crashes, stall, exp, faulted>>

\* The disk falls below the floor, then frees.
Fill(r) ==
  /\ crashes < MaxCrashes
  /\ ~full[r]
  /\ full' = [full EXCEPT ![r] = TRUE]
  /\ crashes' = crashes + 1
  /\ UNCHANGED <<log, cur, world, pin, prebuilt, base, built, verdict, phase,
                 pending, stall, exp, faulted>>

Free(r) ==
  /\ full[r]
  /\ full' = [full EXCEPT ![r] = FALSE]
  /\ UNCHANGED <<log, cur, world, pin, prebuilt, base, built, verdict, phase,
                 pending, crashes, stall, exp, faulted>>

Replay(r) ==
  \/ ApplyWrite(r) \/ Refuse(r) \/ ApplyPin(r) \/ ApplySwitch(r)
  \/ StartInline(r) \/ FinishInline(r) \/ FailInline(r)
  \/ ApplyBack(r) \/ ApplyRelease(r)

\* The order is complete and every replica applied it.
Terminated ==
  /\ Len(log) = MaxOps
  /\ \A r \in Replicas : cur[r] = Len(log)
  /\ UNCHANGED vars

Next ==
  \/ Terminated
  \/ ProposeWrite \/ ProposeCarry \/ ProposePin \/ ProposeSwitch
  \/ ProposeBack
  \/ ProposeRelease
  \/ \E r \in Replicas :
       \/ Replay(r) \/ BuildOffPath(r) \/ Crash(r) \/ Recover(r) \/ Fill(r)
       \/ Free(r) \/ Install(r) \/ Sweep(r)

\* A state constraint for the stale configuration: the order carries no
\* writes but the carried quads' and no malformed pin, so seven operations
\* reach a release ordered after a switch back and a second regeneration.
RegenOnly ==
  \A i \in 1..Len(log) :
    /\ log[i].t = "write" => log[i].i \in Carry /\ log[i].add
    /\ log[i].t = "pin" => ~log[i].bad

Spec ==
  /\ Init /\ [][Next]_vars
  /\ \A r \in Replicas :
       /\ WF_vars(Replay(r)) /\ WF_vars(Recover(r)) /\ WF_vars(Free(r))
       /\ WF_vars(Install(r)) /\ WF_vars(BuildOffPath(r))

----
\* Properties.

TypeOK ==
  /\ cur \in [Replicas -> 0..MaxOps]
  /\ phase \in [Replicas -> {"idle", "building", "down"}]
  /\ pending \in [Replicas -> BOOLEAN]
  /\ \A r \in Replicas : cur[r] <= Len(log)

\* Replicas that applied the same operations hold the same World, roots and
\* verdicts: the result is a function of the World before and the order,
\* whether a replica switched from its prebuilt root or built inline.
Agree ==
  \A r, s \in Replicas :
    cur[r] = cur[s] =>
      /\ world[r] = world[s]
      /\ pin[r] = pin[s]
      /\ base[r] = base[s]
      /\ built[r] = built[s]
      /\ verdict[r] = verdict[s]

\* Without a defect, every replica's head holds exactly what the operations
\* define: no item is lost or invented, through any switch and switch back.
NoLoss == ~faulted => \A r \in Replicas : world[r].items = exp[r]

\* Even with a defect, a World back in the old format and holding no
\* regeneration has lost nothing the operations define.
BackLosesNothing ==
  \A r \in Replicas :
    (world[r].fmt = "old" /\ base[r] = NoRoot) => exp[r] \subseteq world[r].items

\* base and built are held together, and never beside a pin.
HeldTogether ==
  \A r \in Replicas :
    /\ (base[r] = NoRoot) = (built[r] = NoRoot)
    /\ base[r] # NoRoot => pin[r] = NoPin

\* A new-format World whose base was released passed the comparison, so it
\* holds what the operations define even with a defect ordered.
ReleasedOnlyEqual ==
  \A r \in Replicas :
    (world[r].fmt = "new" /\ base[r] = NoRoot) => world[r].items = exp[r]

\* No switch applies more item changes than the bound on the replay path.
WithinBound == \A r \in Replicas : stall[r] <= Bound

\* A pending or lost build never holds replay up: a replica at a write can
\* apply it whatever its builder is doing.
ReplayUnblocked ==
  \A r \in Replicas : At(r, "write") => ENABLED ApplyWrite(r)

\* Every replica applies the whole order: a full disk or a crash delays a
\* replica, and nothing stops it.
Finishes == <>[](\A r \in Replicas : cur[r] = Len(log))

====
