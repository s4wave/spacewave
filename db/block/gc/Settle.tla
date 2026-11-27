---- MODULE Settle ----
\* The settle of an old generation, which retires itself.
\*
\* A Volume whose child lists came from a store format before 7 may hold a
\* list that lacks a child, so a count drawn from it misses a holder. Under
\* hold until free a block's count is exact against the lists, as
\* Liveness.tla proves, so the settle's whole question is which lists to
\* trust. The generation moves Open, Sweeping, Settled, and a copied write
\* of a block a Settled Volume lacks moves it back to Open.
\*
\* While Open nothing is deleted: the free parks each block at zero, and a
\* parked block reads absent. A park is a free that keeps the bytes and the
\* list: it releases the block's holds, so a parked block holds nothing and the
\* free cascades down an unreachable subtree while Open, as it does after. Only
\* a rebirth, a copied write or a recount makes the list hold again: an unpark
\* gives no ups and owes a recount, as a rewrite that gains a child does, which
\* holds the walk's pops and the flip until it runs, and the sweep deletes a
\* parked block without giving its downs again. Seeding pushes each unverified
\* block a root names onto the front; the walk decodes a front block, which
\* verifies its list, and pushes its unverified children. Land pushes the
\* unverified blocks it holds, and a put of a stored block while Open verifies
\* its list. The run names every unverified block and every parked one. A parked
\* block that gains a holder, as a recount after a list gained a child gives it,
\* leaves the park. The flip finds the run complete, the roots seeded, the front
\* empty and no parked block held; every block the run names is then
\* unreachable, and becomes unmarked. The sweep deletes each unmarked block, and
\* a put of one gives birth to it again from its content. Once none is left the
\* run goes and the generation is Settled, where every stored list was written
\* from content, so no settle record or rule remains.
\*
\* A format upgrade recounts and keeps the generation and the run, so an
\* upgrade that lands mid-Sweeping finishes its sweep first. The recount
\* drops every park before it counts, so every stored list holds after it.
\*
\* A Volume may lack a block its lists name, as a peer's cache lacks what it
\* never read. Such a block has no list, so no count holds what lies below
\* it, and a stored block reached only through it would read unreachable.
\* The front therefore holds every child a list from content names that has
\* no list from content, stored or not. The walk decodes an absent block
\* through the full read chain, peers included, and writes its list without
\* its bytes, as an evicted block keeps its list; the list then holds the
\* stored blocks below. An absent block no read resolves stays on the front
\* while anything holds it, and so holds the flip.
\*
\* A put whose writer did not decode the block, such as a copy from another
\* store, carries the source's list, which may be cut. Such a list is no
\* proof, so the put never marks it. While Open it lands as an old block:
\* not stored, it is stored with the unverified list and enters the run;
\* parked, it leaves the park with its list and returns to the front; a
\* marked one takes the put as a put from content does. While Settled, a
\* block not stored lands the same way and reopens the generation: Settled
\* holds no run, front or park, and every list is marked, so the run is
\* complete and the roots seeded as they stand. While Sweeping it is refused
\* unless the block's list is marked. short holds the blocks whose marked
\* list is a copied one, which only a defect writes.
\*
\* A reopen differs from the first Open in one way: the sweep may already
\* have freed a block that the copied list lacks, as k when h arrives cut.
\* LiveStored then fails until the walk decodes the copied block and reads
\* the child back through a peer, so a reopened generation is checked with
\* SettledLive. With no peer holding the child, Lost, the child stays on the
\* front while held and the generation never settles again.
\*
\* An operator may write such a block off: off holds the absent blocks
\* whose recorded hole carries the write-off. The write-off takes the hole
\* off the front, no push returns it, and it no longer holds the flip, so
\* the sweep takes what the Volume stores only below it, which no count
\* holds. The hole record stays, so
\* a later put from content still lists the block from that content and
\* holds its children, and a copied put is still refused. Held is what the
\* root reaches without passing through a written-off hole, and the
\* invariants speak of Held.
\*
\* The spec does not represent three things. Holes nobody wrote off: an
\* audit's recorded hole, which a copied put refuses and only a put from
\* content refills; FlipProof.tla holds the flip on them.
\* Fills: a fill drops each copied entry of a block the records do not
\* list, which changes no record, so it is no action here. And the children
\* a CopyWrite names: the spec requires each present, where Land refuses
\* only a hidden one and lets an absent one join later, as a born block's
\* child does; a copied list's children hold as Land's holdChildren counts
\* them, which Liveness.tla covers. Put likewise requires its children
\* present, so while Settled no stored list names a block the Volume lacks,
\* and the front entry of a reopening copied write changes nothing here:
\* any later holder pushes the block itself. Land lets a parent land
\* before its child, and that entry then holds the flip until the walk
\* decodes the copied block; the Volume's run-through covers it.
\*
\* Counts here are derived from the stored lists, which hold until free
\* makes exact; Liveness.tla models how the logs keep them. Callers found
\* from source: the settle slices (db/block/gc/settle.go), the free and its
\* park (collector.go, records.go), Land (land.go) and the format upgrade
\* (db/volume/common/kvtx/format.go).
\*
\* No single config checks every dimension at once inside the checker's
\* memory, so each bounds the dimensions its interleavings do not need. Txns
\* is MaxTxns, Up is Upgrade; checks are S (LiveStored, HeldPresent,
\* ReachedShown, SelfRetiring), R (SettledLive, HeldPresent, SelfRetiring)
\* and L (EventuallySettled).
\*
\*   config                  Txns Up Lost  Extra Checks Interleavings
\*   Settle.cfg              2    no {}    g, n  S, L   the settle of g and n
\*                                                      beside the root's walk
\*   Settle-upgrade.cfg      2    on {}    -     S, L   an upgrade while a
\*                                                      rewrite's or an
\*                                                      unpark's recount is owed
\*   Settle-upgrade-g.cfg    2    on {}    g     S, L   an upgrade beside g
\*   Settle-upgrade-n.cfg    2    on {}    n     S, L   an upgrade beside n's
\*                                                      writes
\*   Settle-one.cfg          1    on {}    g, n  S, L   an upgrade beside g and
\*                                                      n at once
\*   Settle-lost.cfg         2    on h     g, n  S      every block but h
\*                                                      resolved, the flip held
\*   Settle-off.cfg          2    no h     -     S, L   h written off, then
\*                                                      put again from content
\*   Settle-reopen-g.cfg     3    no {}    g     R, L   a reopen beside g
\*   Settle-reopen-n.cfg     3    no {}    n     R, L   a reopen beside n's
\*                                                      writes
\*   Settle-reopen-lost.cfg  3    no h, k  g, n  R      a reopen whose lost
\*                                                      child holds the flip
\*
\* Every pair of dimensions runs together in some config, and each config
\* now carries the rewritten lists' gained children. Two whole combinations
\* run in none: an upgrade beside both g and n over two transactions with h
\* resolved, which Settle-one.cfg checks over one transaction and
\* Settle-lost.cfg with h lost, and a reopen beside both with h resolved,
\* which Settle-reopen-lost.cfg checks with h and k lost. Each mutant names
\* the smallest config that kills it by the invariant that states its
\* damage.

EXTENDS Integers, FiniteSets

CONSTANTS
  MaxTxns,  \* bounds the Land transactions, so the model is finite
  Upgrade,  \* whether the format upgrade may run
  Lost,     \* the blocks no read resolves once the Volume lacks them
  WriteOffs,\* the absent blocks an operator may write off
  Extra     \* the optional blocks the Volume has: g, n, both or neither

\* r is the root's block, whose old list was cut and lacks h; a's was cut and
\* lacks b; g is an old, unreachable block whose list was cut too; n is a block
\* a writer may write; h is a block the Volume lacks, and k a stored block only
\* h names. k holds b, so a k unparked by h's walk and freed twice would owe
\* b a debt its holders do not.
Blocks == {"r", "a", "b", "h", "k"} \cup Extra
True == [r |-> {"a", "h"}, a |-> {"b"}, b |-> {}, g |-> {"b"}, n |-> {"a"},
         h |-> {"k"}, k |-> {"b"}]
Cut == [r |-> {"a"}, a |-> {}, b |-> {}, g |-> {}, n |-> {"a"},
        h |-> {}, k |-> {"b"}]

VARIABLES
  gen,       \* "open", "sweeping" or "settled"
  stored,    \* blocks with a child list, bytes or not
  verified,  \* stored blocks whose list is marked
  short,     \* verified blocks whose marked list is a copied, cut one
  front,     \* blocks the walk still has to decode
  seeded,    \* the roots' blocks were pushed onto the front
  run,       \* the unmarked run: unverified and parked blocks
  complete,  \* the run names every unverified block
  parked,    \* blocks the free parked while Open
  released,  \* parked or unmarked blocks whose park released their holds
  debt,      \* downs given beyond the lists, which only a defect gives
  unmarked,  \* run blocks the flip unmarked and the sweep has not deleted
  root,      \* the block the root names
  fmt,       \* the store format
  owed,      \* an unpark of a released list or a rewrite owes a recount
  gained,    \* <<p, c>>: a rewrite gave p's list c without c's holder
  txns,      \* Land transactions so far
  off        \* absent blocks whose recorded hole an operator wrote off
vars == <<gen, stored, verified, short, front, seeded, run, complete, parked, released, debt, unmarked, root, fmt, owed, gained, txns, off>>

List(b) == IF b \in verified \ short THEN True[b] ELSE Cut[b]

\* The holders of b: the root and each stored list that names it, except a
\* list its park released and a list whose rewrite gave it b without its
\* holder, less any down a defect gave twice.
Count(b) ==
  LET n == (IF root = b THEN 1 ELSE 0)
             + Cardinality({p \in stored \ released : b \in List(p) /\ <<p, b>> \notin gained})
             - debt[b]
  IN IF n < 0 THEN 0 ELSE n

\* A hidden block reads absent, and a Land refuses a hold onto it.
Hidden(b) == b \in parked \/ b \in unmarked

Present(b) == b \in stored /\ ~Hidden(b)

Unverified(S) == {b \in S : b \in stored /\ b \notin verified}

\* The blocks of S the walk still owes a decode: those without a list from
\* content, whether or not the Volume stores them. It owes a written-off
\* hole none, so no push returns one to the front.
Owed(S) == {b \in S : b \notin verified /\ b \notin off}

RECURSIVE Reach(_)
Reach(S) ==
  LET next == S \cup UNION {True[b] : b \in S}
  IN IF next = S THEN S ELSE Reach(next)

\* The blocks the root holds: Reach, stopped at each written-off hole, whose
\* children no record names.
RECURSIVE Held(_)
Held(S) ==
  LET next == S \cup UNION {True[b] : b \in S \ off}
  IN IF next = S THEN S ELSE Held(next)

Init ==
  /\ off = {}
  /\ gen = "open"
  /\ stored = {"r", "a", "b", "k"} \cup (Extra \cap {"g"})
  /\ verified = {}
  /\ short = {}
  /\ front = {}
  /\ seeded = FALSE
  /\ run = {}
  /\ complete = FALSE
  /\ parked = {}
  /\ released = {}
  /\ debt = [b \in Blocks |-> 0]
  /\ unmarked = {}
  /\ root = "r"
  /\ fmt = 9
  /\ owed = FALSE
  /\ gained = {}
  /\ txns = 0

\* Verify gives the blocks V lists from content, takes them off the front
\* and out of the run unless parked, and, while Open, pushes the children
\* it still owes. A stored list it rewrites gains each child its cut list
\* lacked without the holder the list now gives it, as the rebuild and
\* markStored write a list, so the counts owe a recount; a list it writes
\* for a block not stored is new, and its holders are counted.
Verify(V) ==
  /\ LET g == IF gen = "open"
               THEN {<<p, c>> \in ((V \cap stored) \ verified) \X Blocks : c \in True[p] \ Cut[p]}
               ELSE {}
     IN /\ gained' = gained \cup g
        /\ owed' = (owed \/ g # {})
  /\ verified' = verified \cup V
  /\ short' = short \ V
  /\ front' = (front \ V)
               \cup (IF gen = "open" THEN Owed(UNION {True[b] : b \in V}) \ V ELSE {})
  /\ run' = run \ (V \ parked)

\* Put writes b from its content. A block without a list, or a hidden one,
\* is born again with every child present, and its list holds them as a
\* fresh put's does, gained children included; a stored one is verified
\* while Open and otherwise keeps its list. A put onto a written-off hole
\* is such a birth, and ends the hole.
Put(b) ==
  /\ txns < MaxTxns
  /\ off' = off \ {b}
  /\ \A k \in True[b] : Present(k)
  /\ IF Present(b)
       THEN /\ gen = "open"
            /\ b \notin verified
            /\ Verify({b})
            /\ UNCHANGED <<stored, parked, released, unmarked>>
       ELSE /\ stored' = stored \cup {b}
            /\ verified' = verified \cup {b}
            /\ short' = short \ {b}
            /\ front' = (front \ {b})
                         \cup (IF gen = "open" THEN Owed(True[b]) ELSE {})
            /\ parked' = parked \ {b}
            /\ unmarked' = unmarked \ {b}
            /\ released' = released \ {b}
            /\ run' = IF gen = "sweeping" THEN run ELSE run \ {b}
            /\ gained' = {x \in gained : x[1] # b}
            /\ UNCHANGED owed
  /\ txns' = txns + 1
  /\ UNCHANGED <<gen, seeded, complete, root, fmt, debt>>

\* Move moves the root onto a present block, pushing it while Open when its
\* list is unverified.
Move(t) ==
  /\ txns < MaxTxns
  /\ t # root
  /\ Present(t)
  /\ root' = t
  /\ front' = IF gen = "open" THEN front \cup Owed({t}) ELSE front
  /\ txns' = txns + 1
  /\ UNCHANGED <<gen, stored, verified, short, seeded, run, complete, parked, released, debt, unmarked, fmt, owed, gained>>
  /\ UNCHANGED off

\* MarkCopied marks the copied lists of V as Verify marks a decoded one,
\* which only a defect does: the lists stay cut.
MarkCopied(V) ==
  /\ verified' = verified \cup V
  /\ short' = short \cup V
  /\ front' = (front \ V)
               \cup (IF gen = "open" THEN Owed(UNION {Cut[b] : b \in V}) \ V ELSE {})
  /\ run' = run \ (V \ parked)

\* CopyWrite writes b with the list a copy carries, Cut[b], which it holds.
\* A block whose list is marked takes it as a put from content does, and a
\* stored one keeps its list. While Open, a block not stored is stored with
\* the copied list unverified and enters the run and the front, and a parked
\* one leaves the park with its list, which holds its children again, and
\* returns to the front. While
\* Settled, a block not stored lands as while Open and reopens the
\* generation. While Sweeping, a block that reads absent is refused, and
\* so is a hole in every generation, written off or not.
CopyWrite(b) ==
  /\ txns < MaxTxns
  /\ b \notin off
  /\ \A k \in Cut[b] : Present(k)
  /\ CASE b \in verified /\ ~Present(b) ->
            Put(b)
       [] Present(b) ->
            /\ txns' = txns + 1
            /\ UNCHANGED <<gen, stored, verified, short, front, seeded, run, complete, parked, unmarked, root, fmt, owed, gained>>
            /\ UNCHANGED <<released, debt, off>>
       [] gen = "open" /\ b \notin stored ->
            /\ UNCHANGED off
            /\ stored' = stored \cup {b}
            /\ run' = run \cup {b}
            /\ front' = front \cup {b}
            /\ txns' = txns + 1
            /\ UNCHANGED <<gen, verified, short, seeded, complete, parked, unmarked, root, fmt, owed, gained>>
            /\ UNCHANGED <<released, debt>>
       [] gen = "settled" /\ b \notin stored ->
            /\ UNCHANGED off
            /\ stored' = stored \cup {b}
            /\ run' = run \cup {b}
            /\ front' = front \cup {b}
            /\ gen' = "open"
            /\ txns' = txns + 1
            /\ UNCHANGED <<verified, short, seeded, complete, parked, unmarked, root, fmt, owed, gained>>
            /\ UNCHANGED <<released, debt>>
       [] gen = "open" /\ b \in parked /\ b \notin verified ->
            /\ UNCHANGED off
            /\ parked' = parked \ {b}
            /\ front' = front \cup {b}
            /\ released' = released \ {b}
            /\ txns' = txns + 1
            /\ UNCHANGED <<gen, stored, verified, short, seeded, run, complete, unmarked, root, fmt, owed, gained, debt>>
       [] OTHER -> FALSE

Land == \E b \in Blocks : Put(b) \/ Move(b) \/ CopyWrite(b)

Seed ==
  /\ gen = "open"
  /\ ~seeded
  /\ seeded' = TRUE
  /\ front' = front \cup Owed({root})
  /\ UNCHANGED <<gen, stored, verified, short, run, complete, parked, released, debt, unmarked, root, fmt, owed, gained, txns>>
  /\ UNCHANGED off

\* Walk takes a front block off the front. It decodes only a block the root
\* reaches, as the walk reads through the root's tree: a present block, which
\* verifies its list, and an absent block that a peer's read resolves, which
\* gets its list from content without its bytes, while Open. A hidden block
\* leaves, since the run holds it and an unpark returns it, and a block
\* nothing holds may leave undecoded, present or not, as the free proof pops
\* it; any other block stays until the root reaches it or its holders go.
\* Nothing pops while a recount is owed, since a count may miss a holder,
\* and the recount returns only a stored child to the front, so an absent
\* child popped then would never return. The walk never meets a
\* written-off hole, which is never on the front; a peer's read that lists
\* one, as a repair's walk of the whole tree can, is Put.
Walk(b) ==
  /\ b \in front
  /\ \/ /\ b \in stored /\ ~Hidden(b) /\ b \in Reach({root})
        /\ Verify({b})
        /\ UNCHANGED <<stored, off>>
     \/ /\ gen = "open" /\ b \notin stored /\ b \notin Lost /\ b \in Reach({root})
        /\ stored' = stored \cup {b}
        /\ Verify({b})
        /\ UNCHANGED off
     \/ /\ ~owed
        /\ Hidden(b) \/ Count(b) = 0
        /\ front' = front \ {b}
        /\ UNCHANGED <<stored, verified, short, run, owed, gained, off>>
  /\ UNCHANGED <<gen, seeded, complete, parked, released, debt, unmarked, root, fmt, txns>>

BuildRun ==
  /\ gen = "open"
  /\ ~complete
  /\ complete' = TRUE
  /\ run' = run \cup Unverified(stored)
  /\ UNCHANGED <<gen, stored, verified, short, front, seeded, parked, released, debt, unmarked, root, fmt, owed, gained, txns>>
  /\ UNCHANGED off

\* Free deletes a block at zero holders, except that while Open it parks it.
\* A park is a free that keeps the bytes and the list: it releases the
\* block's holds as a free does, so its children may reach zero and park in
\* turn, and the block stays hidden until the sweep deletes it. Nothing is
\* freed or parked while a recount is owed, since a count may miss a holder:
\* a block an unpark left released, whose list the counts still lack, would
\* give its downs again, as the free gives a list's downs whatever the
\* records say.
Free(b) ==
  /\ ~owed
  /\ b \in stored
  /\ ~Hidden(b)
  /\ Count(b) = 0
  /\ IF gen = "open"
       THEN /\ parked' = parked \cup {b}
            /\ released' = released \cup {b}
            /\ run' = run \cup {b}
            /\ UNCHANGED stored
       ELSE /\ stored' = stored \ {b}
            /\ UNCHANGED <<parked, released, run>>
  /\ verified' = IF gen = "open" THEN verified ELSE verified \ {b}
  /\ short' = IF gen = "open" THEN short ELSE short \ {b}
  /\ debt' = [k \in Blocks |-> debt[k] + (IF b \in released /\ k \in List(b) THEN 1 ELSE 0)]
  /\ UNCHANGED <<gen, front, seeded, complete, unmarked, root, fmt, owed, gained, txns>>
  /\ UNCHANGED off

\* Unpark ends the park of a block that gained a holder. A park that kept
\* its holds leaves a list that holds; a released one stays released, since
\* the unpark gives its children no ups, and owes a recount, which Readopt
\* runs. An unverified one returns to the front, since the walk may have
\* popped it while parked.
Unpark(b) ==
  /\ b \in parked
  /\ Count(b) > 0
  /\ parked' = parked \ {b}
  /\ owed' = (owed \/ b \in released)
  /\ run' = IF b \in verified THEN run \ {b} ELSE run
  /\ front' = front \cup Owed({b})
  /\ UNCHANGED <<gen, stored, verified, short, seeded, complete, released, unmarked, root, fmt, txns, debt, gained>>
  /\ UNCHANGED off

Flip ==
  /\ gen = "open"
  /\ complete
  /\ seeded
  /\ front = {}
  /\ ~owed
  /\ \A b \in parked : Count(b) = 0
  /\ gen' = "sweeping"
  /\ unmarked' = run
  /\ parked' = {}
  /\ UNCHANGED <<stored, verified, short, front, seeded, run, complete, released, debt, root, fmt, owed, gained, txns>>
  /\ UNCHANGED off

\* Sweep deletes an unmarked block. A block its park released gives no
\* downs, since the park gave them; any other gives its downs as a free does.
Sweep(b) ==
  /\ b \in unmarked
  /\ stored' = stored \ {b}
  /\ verified' = verified \ {b}
  /\ short' = short \ {b}
  /\ unmarked' = unmarked \ {b}
  /\ released' = released \ {b}
  /\ debt' = debt
  /\ UNCHANGED <<gen, front, seeded, run, complete, parked, root, fmt, owed, gained, txns>>
  /\ UNCHANGED off

Settle ==
  /\ gen = "sweeping"
  /\ unmarked = {}
  /\ gen' = "settled"
  /\ run' = {}
  /\ UNCHANGED <<stored, verified, short, front, seeded, complete, parked, released, debt, unmarked, root, fmt, owed, gained, txns>>
  /\ UNCHANGED off

\* Recount is the adoption's recount: it drops every park first, taking a
\* verified parked block out of the run and returning an unverified one to
\* the front as Unpark does, and then counts every stored list, so no list
\* stays released. A block that is still at zero parks again. While Open the
\* walk may have popped a child of a released list while the park left it
\* at zero, and the recount holds it again, so the clear returns each owed
\* child of a released list to the front, absent ones too (unrunParked);
\* the count then returns each stored child of a list from content whose
\* own list is not (countChildren, adoption.hold). Its counts come from the
\* lists alone, so no down given beyond them survives, and no recount stays
\* owed.
Recount ==
  /\ parked' = {}
  /\ released' = {}
  /\ debt' = [k \in Blocks |-> 0]
  /\ owed' = FALSE
  /\ gained' = {}
  /\ run' = run \ (parked \cap verified)
  /\ front' = front \cup Owed(parked)
               \cup (IF gen = "open"
                     THEN Owed(UNION {List(p) : p \in released})
                          \cup Unverified(UNION {True[p] : p \in stored \cap (verified \ short)})
                     ELSE {})

\* Readopt is the recount an unpark owes: once the run is complete and the
\* roots seeded, the flip's attempt returns the Volume to unadopted instead
\* of flipping, whatever the front and the holes hold, and the adoption
\* recounts (settle.go flip, kindRewritten).
Readopt ==
  /\ gen = "open"
  /\ owed
  /\ complete
  /\ seeded
  /\ Recount
  /\ UNCHANGED <<gen, stored, verified, short, seeded, complete, unmarked, root, fmt, txns>>
  /\ UNCHANGED off

\* UpgradeFormat moves the store to format 10. It recounts and keeps the
\* generation, the run and the unmarked blocks.
UpgradeFormat ==
  /\ Upgrade
  /\ fmt = 9
  /\ fmt' = 10
  /\ Recount
  /\ UNCHANGED <<gen, stored, verified, short, seeded, complete, unmarked, root, txns>>
  /\ UNCHANGED off

\* WriteOff is the operator's command on a recorded hole: a block with no
\* list that the operator accepts no read will resolve. It takes the hole
\* off the front, whatever holds it, and changes no count and no list. It
\* acts once on a hole: a second run finds it written off and changes
\* nothing, so only Owed keeps a later push from returning it to the front.
WriteOff(b) ==
  /\ b \in WriteOffs
  /\ b \notin stored
  /\ b \notin verified
  /\ b \notin off
  /\ off' = off \cup {b}
  /\ front' = front \ {b}
  /\ UNCHANGED <<gen, stored, verified, short, seeded, run, complete, parked, released, debt, unmarked, root, fmt, owed, gained, txns>>

Next ==
  \/ Land \/ Seed \/ BuildRun \/ Flip \/ Settle \/ Readopt \/ UpgradeFormat
  \/ \E b \in Blocks : Walk(b) \/ Free(b) \/ Unpark(b) \/ Sweep(b) \/ WriteOff(b)

Spec ==
  /\ Init /\ [][Next]_vars
  /\ WF_vars(Seed) /\ WF_vars(BuildRun) /\ WF_vars(Flip) /\ WF_vars(Settle)
  /\ WF_vars(Readopt)
  /\ \A b \in Blocks : WF_vars(Walk(b)) /\ WF_vars(Unpark(b)) /\ WF_vars(Sweep(b))
  /\ \A b \in Blocks : WF_vars(Free(b))
  /\ \A b \in WriteOffs : WF_vars(WriteOff(b))

\* Every block the root holds through its content is stored, except the
\* block the Volume lacks.
LiveStored == Held({root}) \ {"h"} \subseteq stored

\* The root's block reads present.
HeldPresent == ~Hidden(root)

\* A hidden block the root reaches that a shown parent names, by a list from its
\* whole content that holds, keeps that holder, so an unpark shows it again;
\* only a released list's children, which its park gave their downs before an
\* unpark that owes their recount, and a rewritten list's gained children until
\* that recount, may lose it. Once the flip could run, no block the root reaches
\* reads hidden: the owed recount has given back every holder a released list
\* took.
Holding(b) ==
  {p \in Reach({root}) : Present(p) /\ p \in verified \ short /\ p \notin released
                         /\ <<p, b>> \notin gained /\ b \in True[p]}
FlipReady ==
  /\ gen = "open" /\ complete /\ seeded /\ front = {} /\ ~owed
  /\ \A b \in parked : Count(b) = 0
ReachedShown ==
  /\ \A b \in Held({root}) : Hidden(b) /\ Holding(b) # {} => Count(b) > 0
  /\ FlipReady => \A b \in Held({root}) : ~Hidden(b)

\* Once Settled every list was written from content and no settle record
\* remains, so the settle's code and records can go.
SelfRetiring ==
  gen = "settled" =>
    /\ stored \subseteq verified
    /\ front = {} /\ run = {} /\ parked = {} /\ released = {} /\ unmarked = {}

\* Once Settled again every block the root reaches is stored: the walk of a
\* reopened generation brought back what a copied list lacked.
SettledLive == gen = "settled" => LiveStored

\* The settle finishes once the writers stop.
EventuallySettled == <>(gen = "settled")
====
