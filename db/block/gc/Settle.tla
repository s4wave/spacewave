---- MODULE Settle ----
\* The settle of an old generation, which retires itself.
\*
\* A Volume whose child lists came from a store format before 7 may hold a
\* list that lacks a child, so a count drawn from it misses a holder. Under
\* hold until free a block's count is exact against the lists, as
\* Liveness.tla proves, so the settle's whole question is which lists to
\* trust. The generation moves Open, Sweeping, Settled and never back.
\*
\* While Open nothing is deleted: the free parks each block at zero, and a
\* parked block reads absent. Seeding pushes each unverified block a root
\* names onto the front; the walk decodes a front block, which verifies its
\* list, and pushes its unverified children. Land pushes the unverified
\* blocks it holds, and a put of a stored block while Open verifies its list.
\* The run names every unverified block and every parked one. A parked block
\* that gains a holder, as a recount after a list gained a child gives it,
\* leaves the park. The flip finds the run complete, the roots seeded, the
\* front empty and no parked block held; every block the run names is then
\* unreachable, and becomes unmarked. The sweep deletes each unmarked block,
\* and a put of one gives birth to it again from its content. Once none is
\* left the run goes and the generation is Settled, where every stored list
\* was written from content, so no settle record or rule remains.
\*
\* A format upgrade recounts and keeps the generation and the run, so an
\* upgrade that lands mid-Sweeping finishes its sweep first.
\*
\* Counts here are derived from the stored lists, which hold until free
\* makes exact; Liveness.tla models how the logs keep them. Callers found
\* from source: the settle slices (db/block/gc/settle.go), the free and its
\* park (collector.go, records.go), Land (land.go) and the format upgrade
\* (db/volume/common/kvtx/format.go).

EXTENDS Naturals, FiniteSets

CONSTANTS
  MaxTxns,  \* bounds the Land transactions, so the model is finite
  Upgrade   \* whether the format upgrade may run

\* r is the root's block; a's old list was cut and lacks b; g is an old,
\* unreachable block whose list was cut too; n is a block a writer may write.
Blocks == {"r", "a", "b", "g", "n"}
True == [r |-> {"a"}, a |-> {"b"}, b |-> {}, g |-> {"b"}, n |-> {"a"}]
Cut == [r |-> {"a"}, a |-> {}, b |-> {}, g |-> {}, n |-> {"a"}]

VARIABLES
  gen,       \* "open", "sweeping" or "settled"
  stored,    \* blocks with a child list
  verified,  \* stored blocks whose list was written from content
  front,     \* blocks the walk still has to decode
  seeded,    \* the roots' blocks were pushed onto the front
  run,       \* the unmarked run: unverified and parked blocks
  complete,  \* the run names every unverified block
  parked,    \* blocks the free parked while Open
  unmarked,  \* run blocks the flip unmarked and the sweep has not deleted
  root,      \* the block the root names
  fmt,       \* the store format
  txns       \* Land transactions so far
vars == <<gen, stored, verified, front, seeded, run, complete, parked, unmarked, root, fmt, txns>>

List(b) == IF b \in verified THEN True[b] ELSE Cut[b]

\* The holders of b: the root and each stored list that names it.
Count(b) ==
  (IF root = b THEN 1 ELSE 0) + Cardinality({p \in stored : b \in List(p)})

\* A hidden block reads absent, and a Land refuses a hold onto it.
Hidden(b) == b \in parked \/ b \in unmarked

Present(b) == b \in stored /\ ~Hidden(b)

Unverified(S) == {b \in S : b \in stored /\ b \notin verified}

RECURSIVE Reach(_)
Reach(S) ==
  LET next == S \cup UNION {True[b] : b \in S}
  IN IF next = S THEN S ELSE Reach(next)

Init ==
  /\ gen = "open"
  /\ stored = {"r", "a", "b", "g"}
  /\ verified = {}
  /\ front = {}
  /\ seeded = FALSE
  /\ run = {}
  /\ complete = FALSE
  /\ parked = {}
  /\ unmarked = {}
  /\ root = "r"
  /\ fmt = 9
  /\ txns = 0

\* Verify gives the blocks V lists from content, takes them off the front
\* and out of the run unless parked, and, while Open, pushes their unverified
\* children.
Verify(V) ==
  /\ verified' = verified \cup V
  /\ front' = (front \ V)
               \cup (IF gen = "open" THEN Unverified(UNION {True[b] : b \in V}) \ V ELSE {})
  /\ run' = run \ (V \ parked)

\* Put writes b from its content. A block without a list, or a hidden one,
\* is born again with every child present; a stored one is verified while
\* Open and otherwise keeps its list.
Put(b) ==
  /\ txns < MaxTxns
  /\ \A k \in True[b] : Present(k)
  /\ IF Present(b)
       THEN /\ gen = "open"
            /\ b \notin verified
            /\ Verify({b})
            /\ UNCHANGED <<stored, parked, unmarked>>
       ELSE /\ stored' = stored \cup {b}
            /\ verified' = verified \cup {b}
            /\ front' = (front \ {b})
                         \cup (IF gen = "open" THEN Unverified(True[b]) ELSE {})
            /\ parked' = parked \ {b}
            /\ unmarked' = unmarked \ {b}
            /\ run' = IF gen = "sweeping" THEN run ELSE run \ {b}
  /\ txns' = txns + 1
  /\ UNCHANGED <<gen, seeded, complete, root, fmt>>

\* Move moves the root onto a present block, pushing it while Open when its
\* list is unverified.
Move(t) ==
  /\ txns < MaxTxns
  /\ t # root
  /\ Present(t)
  /\ root' = t
  /\ front' = IF gen = "open" THEN front \cup Unverified({t}) ELSE front
  /\ txns' = txns + 1
  /\ UNCHANGED <<gen, stored, verified, seeded, run, complete, parked, unmarked, fmt>>

Land == \E b \in Blocks : Put(b) \/ Move(b)

Seed ==
  /\ gen = "open"
  /\ ~seeded
  /\ seeded' = TRUE
  /\ front' = front \cup Unverified({root})
  /\ UNCHANGED <<gen, stored, verified, run, complete, parked, unmarked, root, fmt, txns>>

Walk(b) ==
  /\ b \in front
  /\ IF b \in stored /\ ~Hidden(b)
       THEN Verify({b})
       ELSE /\ front' = front \ {b}
            /\ UNCHANGED <<verified, run>>
  /\ UNCHANGED <<gen, stored, seeded, complete, parked, unmarked, root, fmt, txns>>

BuildRun ==
  /\ gen = "open"
  /\ ~complete
  /\ complete' = TRUE
  /\ run' = run \cup Unverified(stored)
  /\ UNCHANGED <<gen, stored, verified, front, seeded, parked, unmarked, root, fmt, txns>>

\* Free deletes a block at zero holders, except that while Open it parks it.
Free(b) ==
  /\ b \in stored
  /\ ~Hidden(b)
  /\ Count(b) = 0
  /\ IF gen = "open"
       THEN /\ parked' = parked \cup {b}
            /\ run' = run \cup {b}
            /\ UNCHANGED stored
       ELSE /\ stored' = stored \ {b}
            /\ UNCHANGED <<parked, run>>
  /\ verified' = IF gen = "open" THEN verified ELSE verified \ {b}
  /\ UNCHANGED <<gen, front, seeded, complete, unmarked, root, fmt, txns>>

\* Unpark ends the park of a block that gained a holder. An unverified one
\* returns to the front, since the walk may have popped it while parked.
Unpark(b) ==
  /\ b \in parked
  /\ Count(b) > 0
  /\ parked' = parked \ {b}
  /\ run' = IF b \in verified THEN run \ {b} ELSE run
  /\ front' = front \cup Unverified({b})
  /\ UNCHANGED <<gen, stored, verified, seeded, complete, unmarked, root, fmt, txns>>

Flip ==
  /\ gen = "open"
  /\ complete
  /\ seeded
  /\ front = {}
  /\ \A b \in parked : Count(b) = 0
  /\ gen' = "sweeping"
  /\ unmarked' = run
  /\ parked' = {}
  /\ UNCHANGED <<stored, verified, front, seeded, run, complete, root, fmt, txns>>

Sweep(b) ==
  /\ b \in unmarked
  /\ stored' = stored \ {b}
  /\ verified' = verified \ {b}
  /\ unmarked' = unmarked \ {b}
  /\ UNCHANGED <<gen, front, seeded, run, complete, parked, root, fmt, txns>>

Settle ==
  /\ gen = "sweeping"
  /\ unmarked = {}
  /\ gen' = "settled"
  /\ run' = {}
  /\ UNCHANGED <<stored, verified, front, seeded, complete, parked, unmarked, root, fmt, txns>>

\* UpgradeFormat moves the store to format 10. It recounts, which the
\* derived counts already are, and keeps the generation and its records.
UpgradeFormat ==
  /\ Upgrade
  /\ fmt = 9
  /\ fmt' = 10
  /\ UNCHANGED <<gen, stored, verified, front, seeded, run, complete, parked, unmarked, root, txns>>

Next ==
  \/ Land \/ Seed \/ BuildRun \/ Flip \/ Settle \/ UpgradeFormat
  \/ \E b \in Blocks : Walk(b) \/ Free(b) \/ Unpark(b) \/ Sweep(b)

Spec ==
  /\ Init /\ [][Next]_vars
  /\ WF_vars(Seed) /\ WF_vars(BuildRun) /\ WF_vars(Flip) /\ WF_vars(Settle)
  /\ \A b \in Blocks : WF_vars(Walk(b)) /\ WF_vars(Unpark(b)) /\ WF_vars(Sweep(b))

\* Every block the root reaches through its content is stored.
LiveStored == Reach({root}) \subseteq stored

\* The root's block reads present.
HeldPresent == ~Hidden(root)

\* Once Settled every list was written from content and no settle record
\* remains, so the settle's code and records can go.
SelfRetiring ==
  gen = "settled" =>
    /\ stored \subseteq verified
    /\ front = {} /\ run = {} /\ parked = {} /\ unmarked = {}

\* The settle finishes once the writers stop.
EventuallySettled == <>(gen = "settled")
====
