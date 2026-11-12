---- MODULE ReplayHolds ----
\* What holds the pages of the Worlds a Space World's replay keeps: the
\* positions and the replay floor, the replay stage's epochs, the cursor save
\* that ends them, the background stores, the engine readers, a restart and
\* the Volume collector. Each World w has one page, kept in memory (mem[w])
\* or not, and in the Volume (vol[w]) absent, stored under the open epoch, or
\* stored and held only while a root names w (held).
\*
\* Callers, from source: Adapter.Apply through spaceReplay.run (Apply); the
\* signer's checkpoint through Adapter.Root and the store it returns (Root,
\* StoreLands); maybeSaveCursor and Adapter.Retry through saveCursor and its
\* sweep (Save); the served engine's readers, which also stand for the
\* installed and writer heads (Read, ReadEnds); NewAdapter with loadCursor and
\* restoredAt (Restart); and the Volume collector (Collect), whose grace any
\* interleaving here may outlast. The save's root move and the end of its
\* epochs are one action: the epochs end only after the move lands, and a
\* refused move keeps them, which holds more. Left out: the pending
\* candidates, which Admit holds under the pending root and the save holds
\* beside the served Worlds by the same path; refused and acknowledged
\* operations, which share the floor of the World before them; and the trim,
\* which releases a World only once the installed checkpoint covers it. The
\* base World, which the replay base root holds, is not a position.
EXTENDS Naturals, Sequences
CONSTANTS MaxWorlds
VARIABLES pos, floor, savedPos, next, mem, vol, held, readers, storing
vars == <<pos, floor, savedPos, next, mem, vol, held, readers, storing>>

Worlds == 1..(MaxWorlds - 1)

\* Pos(i) is the World after position i, from 0.
Pos(i) == pos[i + 1]
Indexes == 0..(Len(pos) - 1)

\* spaceReplay.unstored(i + 1): the World of position i left memory unstored,
\* and replay computes it again before it reads it.
Unstored(i) == i < floor
Readable(w) == mem[w] \/ vol[w] # "absent"
Min(a, b) == IF a < b THEN a ELSE b

\* spaceReplay.recompute: compute the positions below upTo again from the
\* base, which keeps their pages in memory.
Recompute(upTo) == [w \in Worlds |-> mem[w] \/ \E i \in Indexes : i < upTo /\ Pos(i) = w]

Init == /\ pos = <<>> /\ floor = 0 /\ savedPos = <<>> /\ next = 1
        /\ mem = [w \in Worlds |-> FALSE]
        /\ vol = [w \in Worlds |-> "absent"]
        /\ held = {} /\ readers = {} /\ storing = {}

\* Adapter.Apply at index k: truncate the positions to k; when the World
\* before k left memory, compute the positions below the floor again; then run
\* the operation on it. Its new page is kept, or stored under the open epoch
\* past the PendingStore's bound.
Apply(k, page) ==
  /\ next < MaxWorlds
  /\ LET kept == SubSeq(pos, 1, k)
         m == IF k # 0 /\ k <= floor
              THEN [w \in Worlds |-> mem[w] \/ \E i \in 1..k : kept[i] = w]
              ELSE mem
     IN /\ pos' = Append(kept, next)
        /\ floor' = IF k # 0 /\ k <= floor THEN 0 ELSE floor
        /\ mem' = [m EXCEPT ![next] = (page = "mem")]
  /\ vol' = [vol EXCEPT ![next] = IF page = "mem" THEN "absent" ELSE "epoch"]
  /\ next' = next + 1
  /\ UNCHANGED <<savedPos, held, readers, storing>>

\* Adapter.Root for the World after n positions: when it left memory, compute
\* every position below the floor again, then hand the World to the store the
\* signer runs in the background.
Root(n) ==
  /\ IF n <= floor
     THEN /\ mem' = Recompute(Min(floor, Len(pos)))
          /\ floor' = 0
     ELSE UNCHANGED <<mem, floor>>
  /\ storing' = storing \cup {Pos(n - 1)}
  /\ UNCHANGED <<pos, savedPos, next, vol, held, readers>>

\* The checkpoint's store lands: the checkpoint root holds the World, storing
\* its page from memory.
StoreLands(w) ==
  /\ storing' = storing \ {w}
  /\ held' = held \cup {w}
  /\ vol' = [vol EXCEPT ![w] = IF mem[w] \/ vol[w] # "absent" THEN "vol" ELSE "absent"]
  /\ UNCHANGED <<pos, floor, savedPos, next, mem, readers>>

\* An engine reader starts on the head World, and later ends.
Read == /\ Len(pos) > 0
        /\ readers' = readers \cup {Pos(Len(pos) - 1)}
        /\ UNCHANGED <<pos, floor, savedPos, next, mem, vol, held, storing>>
ReadEnds(w) == /\ readers' = readers \ {w}
               /\ UNCHANGED <<pos, floor, savedPos, next, mem, vol, held, storing>>

\* Adapter.saveCursor: the cursor root holds the last position's World, and,
\* when an ending epoch stored a page, the served Worlds beside it (gatherLive:
\* the engine readers' and the background stores'), and the floor moves up to
\* the saved World. Each held World's page is stored from memory, then the
\* epochs end. A save over the bound (full) moves the floor up as well and
\* sweeps the pages of every World that is neither a position above the floor
\* nor served.
Save(full) ==
  /\ Len(pos) > floor
  /\ LET last == Pos(Len(pos) - 1)
         stored == \E w \in Worlds : vol[w] = "epoch"
         served == readers \cup storing
         hold == readers \cup storing
         h == held \cup {last} \cup (IF stored THEN hold ELSE {})
         f == IF full \/ stored THEN Len(pos) - 1 ELSE floor
         live == {Pos(i) : i \in {j \in Indexes : j >= f}} \cup served
     IN /\ held' = h
        /\ vol' = [w \in Worlds |->
                     IF vol[w] = "epoch" \/ (w \in h /\ mem[w]) THEN "vol" ELSE vol[w]]
        /\ floor' = f
        /\ mem' = IF full THEN [w \in Worlds |-> mem[w] /\ w \in live] ELSE mem
  /\ savedPos' = pos
  /\ UNCHANGED <<pos, next, readers, storing>>

\* A restart: memory, readers and background stores are gone, the open
\* epoch ends with the stage, and replay restores the saved positions, whose
\* Worlds before the last it computes again (restoredAt).
Restart ==
  /\ pos' = savedPos
  /\ floor' = IF savedPos = <<>> THEN 0 ELSE Len(savedPos) - 1
  /\ mem' = [w \in Worlds |-> FALSE]
  /\ vol' = [w \in Worlds |-> IF vol[w] = "epoch" THEN "vol" ELSE vol[w]]
  /\ readers' = {} /\ storing' = {}
  /\ UNCHANGED <<savedPos, next, held>>

\* The Volume collector frees a stored page no root holds.
Collect(w) == /\ vol[w] = "vol" /\ w \notin held
              /\ vol' = [vol EXCEPT ![w] = "absent"]
              /\ UNCHANGED <<pos, floor, savedPos, next, mem, held, readers, storing>>

Next == \/ \E k \in 0..Len(pos), page \in {"mem", "epoch"} : Apply(k, page)
        \/ \E n \in 1..Len(pos) : Root(n)
        \/ \E w \in storing : StoreLands(w)
        \/ Read
        \/ \E w \in readers : ReadEnds(w)
        \/ \E full \in BOOLEAN : Save(full)
        \/ Restart
        \/ \E w \in Worlds : Collect(w)

Spec == Init /\ [][Next]_vars

TypeOK == /\ floor \in 0..MaxWorlds /\ next \in 1..MaxWorlds
          /\ held \subseteq Worlds /\ readers \subseteq Worlds
          /\ storing \subseteq Worlds
          /\ \A w \in Worlds : vol[w] \in {"absent", "epoch", "vol"}

\* Invariant A: every World the adapter may still read reads, from memory or
\* the Volume: each position replay counts as computed, each engine reader's
\* World and each World a background store is storing.
LiveWorldsRead == /\ \A i \in Indexes : ~Unstored(i) => Readable(Pos(i))
                  /\ \A w \in readers \cup storing : Readable(w)

\* Invariant D: every World a root holds is stored, so no stored root names
\* a hole.
RootsNameStoredWorlds == \A w \in held : vol[w] # "absent"
====
