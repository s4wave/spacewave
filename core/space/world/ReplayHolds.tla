---- MODULE ReplayHolds ----
\* What holds the pages of the Worlds a Space World's replay keeps: the
\* positions and the replay floor, the replay stage's epochs, the cursor save
\* that ends them, the writers' candidates and the pending root, the install
\* and its trim, the background stores, the engine readers, a restart and the
\* Volume collector. Each World w has one page, kept in memory (mem[w]) or
\* not, and in the Volume (vol[w]) absent, stored under an epoch not yet
\* ended, or stored and held only while a root names w (held) or the base
\* root holds it (baseRoot). Replay computes from base, which an install
\* sets; the base root moves to it with the install's trim.
\*
\* Callers, from source: Adapter.Apply through spaceReplay.run, adopting a
\* candidate's World or computing a new one (Apply); a writer's turn through
\* Admit and holdAdmitted (Admit); the signer's checkpoint through
\* Adapter.Root and the store it returns (Root, StoreLands); maybeSaveCursor
\* and Adapter.Retry through saveCursor, which raises the floor, sweeps and
\* rotates the epochs as it starts (SaveStart), and its background
\* writeCursor, which lands (SaveLands) or fails after or before its root
\* move and keeps its epochs for the next save (SaveFails); Adapter.Install
\* with its sweep and trim plan, deciding none or all of the candidates
\* (Install), and the background runTrim
\* (Trim); the served engine's readers, which also stand for the installed
\* and writer heads (Read, ReadEnds); NewAdapter with loadCursor and
\* restoredAt (Restart); and the Volume collector (Collect), whose grace any
\* interleaving here may outlast.
\*
\* One held set stands for the replay cursor, pending and checkpoint roots,
\* which a trim treats alike; how a set is encoded, as a chain of blocks or
\* one rewritten block, is below this model. Left out: refused and
\* acknowledged operations, which share the floor of the World before them; a
\* refused root move, which moves nothing and holds more; a cursor restored
\* across a later install, which this model drops at the restart; and the
\* trim's gather racing the driver, which runs on the driver goroutine.
EXTENDS Naturals, Sequences, FiniteSets
CONSTANTS MaxWorlds
VARIABLES pos, floor, savedPos, savedBase, next, mem, vol, held, base, baseRoot,
          readers, storing, cands, ending, saveH, savePos, saveBase,
          trimRel, trimKeep
vars == <<pos, floor, savedPos, savedBase, next, mem, vol, held, base, baseRoot,
          readers, storing, cands, ending, saveH, savePos, saveBase,
          trimRel, trimKeep>>

Worlds == 1..(MaxWorlds - 1)

\* Pos(i) is the World after position i, from 0. World 0 is the genesis
\* base, which every device computes and nothing stores.
Pos(i) == pos[i + 1]
Indexes == 0..(Len(pos) - 1)
Positions == {Pos(i) : i \in Indexes}

\* spaceReplay.unstored(i + 1): the World of position i left memory unstored,
\* and replay computes it again from the base before it reads it.
Unstored(i) == i < floor
Readable(w) == mem[w] \/ vol[w] # "absent"
Min(a, b) == IF a < b THEN a ELSE b
Max(a, b) == IF a > b THEN a ELSE b

\* What background work keeps live through startBackground: the Worlds a
\* running save holds and the Worlds a pending trim holds.
Background == saveH \cup trimKeep

\* gatherLive's memory: the positions above the floor and the served Worlds,
\* the readers', the background stores' and the background work's.
Memory(f) == {Pos(i) : i \in {j \in Indexes : j >= f}} \cup readers \cup storing \cup Background

\* spaceReplay.recompute: compute the positions below upTo again from the
\* base, which keeps their pages in memory.
Recompute(upTo) == [w \in Worlds |-> mem[w] \/ \E i \in Indexes : i < upTo /\ Pos(i) = w]

Init == /\ pos = <<>> /\ floor = 0 /\ savedPos = <<>> /\ savedBase = 0
        /\ next = 1 /\ base = 0 /\ baseRoot = 0
        /\ mem = [w \in Worlds |-> FALSE]
        /\ vol = [w \in Worlds |-> "absent"]
        /\ held = {} /\ readers = {} /\ storing = {} /\ cands = {}
        /\ ending = {} /\ saveH = {} /\ savePos = <<>> /\ saveBase = 0
        /\ trimRel = {} /\ trimKeep = {}

\* Adapter.Apply at index k: truncate the positions to k; when the World
\* before k left memory, compute the positions below the floor again; then
\* run the operation on it. Its new World w's page is kept, or stored under
\* the open epoch past the PendingStore's bound; or the operation's writer
\* proposed it and Apply adopts the candidate's World.
ApplyTo(k, w, m0, v0, c0) ==
  /\ LET kept == SubSeq(pos, 1, k)
         m == IF k # 0 /\ k <= floor
              THEN [x \in Worlds |-> m0[x] \/ \E i \in 1..k : kept[i] = x]
              ELSE m0
     IN /\ pos' = Append(kept, w)
        /\ floor' = IF k # 0 /\ k <= floor THEN 0 ELSE floor
        /\ mem' = m
  /\ vol' = v0 /\ cands' = c0
  /\ UNCHANGED <<savedPos, savedBase, held, base, baseRoot, readers, storing,
                 ending, saveH, savePos, saveBase, trimRel, trimKeep>>

Apply(k, page) ==
  /\ next < MaxWorlds
  /\ next' = next + 1
  /\ ApplyTo(k, next, [mem EXCEPT ![next] = (page = "mem")],
             [vol EXCEPT ![next] = IF page = "mem" THEN "absent" ELSE "epoch"], cands)

\* The adopted admission leaves the pending list.
Adopt(k, c) ==
  /\ UNCHANGED next
  /\ ApplyTo(k, c, mem, vol, cands \ {c})

\* A writer's turn computes a candidate World on the head and Admit queues
\* it. When a stage stored a page or the PendingStore is full, holdAdmitted
\* flushes the candidate's pages and the pending root holds it.
Admit(page) ==
  /\ next < MaxWorlds
  /\ LET c == next
         stored == page = "epoch" \/ \E w \in Worlds : vol[w] = "epoch"
     IN /\ cands' = cands \cup {c}
        /\ mem' = [mem EXCEPT ![c] = (page = "mem")]
        /\ held' = IF stored THEN held \cup {c} ELSE held
        /\ vol' = [vol EXCEPT ![c] = IF stored THEN "vol" ELSE IF page = "mem" THEN "absent" ELSE "epoch"]
  /\ next' = next + 1
  /\ UNCHANGED <<pos, floor, savedPos, savedBase, base, baseRoot, readers,
                 storing, ending, saveH, savePos, saveBase, trimRel, trimKeep>>

\* Adapter.Root for the World after n positions: when it left memory, compute
\* every position below the floor again, then hand the World to the store the
\* signer runs in the background.
Root(n) ==
  /\ IF n <= floor
     THEN /\ mem' = Recompute(Min(floor, Len(pos)))
          /\ floor' = 0
     ELSE UNCHANGED <<mem, floor>>
  /\ storing' = storing \cup {Pos(n - 1)}
  /\ UNCHANGED <<pos, savedPos, savedBase, next, vol, held, base, baseRoot, readers,
                 cands, ending, saveH, savePos, saveBase, trimRel, trimKeep>>

\* The checkpoint's store lands: the checkpoint root holds the World, storing
\* its page from memory.
StoreLands(w) ==
  /\ storing' = storing \ {w}
  /\ held' = held \cup {w}
  /\ vol' = [vol EXCEPT ![w] = IF mem[w] \/ vol[w] # "absent" THEN "vol" ELSE "absent"]
  /\ UNCHANGED <<pos, floor, savedPos, savedBase, next, mem, base, baseRoot, readers,
                 cands, ending, saveH, savePos, saveBase, trimRel, trimKeep>>

\* An engine reader starts on the head World, and later ends.
Read == /\ Len(pos) > 0
        /\ readers' = readers \cup {Pos(Len(pos) - 1)}
        /\ UNCHANGED <<pos, floor, savedPos, savedBase, next, mem, vol, held,
                       base, baseRoot, storing, cands, ending, saveH, savePos, saveBase,
                       trimRel, trimKeep>>
ReadEnds(w) == /\ readers' = readers \ {w}
               /\ UNCHANGED <<pos, floor, savedPos, savedBase, next, mem, vol,
                              held, base, baseRoot, storing, cands, ending, saveH,
                              savePos, saveBase, trimRel, trimKeep>>

\* saveCursor starts: one save runs at a time. It encodes the positions, and
\* takes the epochs not yet ended, with any an earlier failed save kept
\* (rotate). When one of them stored a page, it holds the served Worlds, with
\* those a pending trim keeps through startBackground, and the candidates
\* beside the last position's World, and the floor moves up to
\* the saved World. A save over the bound (full) moves the floor up as well,
\* and maybeSaveCursor sweeps the pages of every World neither a position
\* above the floor nor live, before the save lands.
SaveStart(full) ==
  /\ saveH = {}
  /\ Len(pos) > floor
  /\ LET last == Pos(Len(pos) - 1)
         stored == \E w \in Worlds : vol[w] = "epoch"
         served == readers \cup storing \cup trimKeep
         hold == readers \cup storing \cup trimKeep
         h == {last} \cup (IF stored THEN hold \cup cands ELSE {})
         f == IF full \/ stored THEN Len(pos) - 1 ELSE floor
         live == {Pos(i) : i \in {j \in Indexes : j >= f}} \cup served \cup cands \cup h \cup trimKeep
     IN /\ saveH' = h
        /\ ending' = {w \in Worlds : vol[w] = "epoch"}
        /\ floor' = f
        /\ mem' = IF full THEN [w \in Worlds |-> mem[w] /\ w \in live] ELSE mem
  /\ savePos' = pos /\ saveBase' = base
  /\ UNCHANGED <<pos, savedPos, savedBase, next, vol, held, base, baseRoot, readers,
                 storing, cands, trimRel, trimKeep>>

\* writeCursor's root move stores the held Worlds' pages from memory and
\* moves the roots; a World neither kept nor stored refuses it. The epochs in
\* ends end with it.
SaveMove(ends) ==
  /\ held' = held \cup saveH
  /\ vol' = [w \in Worlds |->
              IF (w \in saveH /\ mem[w]) \/ (w \in ends /\ vol[w] = "epoch") THEN "vol" ELSE vol[w]]
SaveReadable == \A w \in saveH : Readable(w)

\* The save lands: the roots hold its Worlds, the record is written, and the
\* epochs it took end, freeing their pages to the collector.
SaveLands ==
  /\ saveH # {} /\ SaveReadable
  /\ SaveMove(ending)
  /\ savedPos' = savePos /\ savedBase' = saveBase
  /\ saveH' = {} /\ ending' = {}
  /\ UNCHANGED <<pos, floor, next, mem, base, baseRoot, readers, storing, cands,
                 savePos, saveBase, trimRel, trimKeep>>

\* The save fails, after its root move or before it. The floor stays where
\* the start raised it, and the epochs it took are kept for the next save.
SaveFails(moved) ==
  /\ saveH # {}
  /\ IF moved /\ SaveReadable THEN SaveMove({}) ELSE UNCHANGED <<held, vol>>
  /\ saveH' = {} /\ ending' = {}
  /\ UNCHANGED <<pos, floor, savedPos, savedBase, next, mem, base, baseRoot, readers,
                 storing, cands, savePos, saveBase, trimRel, trimKeep>>

\* Adapter.Install of a checkpoint covering n positions whose World the
\* checkpoint root holds: the positions after it stay, the floor moves down
\* with them, the writers' turns it decided drop their candidates, and the
\* sweep drops the pages of every World not live. The trim plan, gathered
\* now, holds the live Worlds and the base through startBackground until its
\* root move lands; a later install's plan replaces it. The plan may release
\* only what the histories held as it was gathered (trimRel), never a World a
\* later Admit or save adds.
Install(n) ==
  /\ Pos(n - 1) \in held /\ vol[Pos(n - 1)] # "absent"
  /\ \E decided \in {{}, cands} :
       LET p == SubSeq(pos, n + 1, Len(pos))
           f == Max(0, floor - n)
           c == cands \ decided
           live == {p[i] : i \in {j \in 1..Len(p) : j > f}} \cup readers \cup storing \cup saveH
       IN /\ pos' = p /\ floor' = f /\ cands' = c
          /\ base' = Pos(n - 1)
          /\ trimKeep' = live \cup c \cup {Pos(n - 1)}
          /\ trimRel' = {w \in held : w \notin {p[i] : i \in 1..Len(p)} \cup live \cup c} \cup ({Pos(n - 1)} \cap held)
          /\ mem' = [w \in Worlds |-> mem[w] /\ w \in live \cup c]
  /\ UNCHANGED <<savedPos, savedBase, next, vol, held, baseRoot, readers,
                 storing, ending, saveH, savePos, saveBase>>

\* runTrim: one root move releases every World the histories held when the
\* plan was gathered that the plan found neither live nor stored, and the
\* base, which the base root holds now, and holds the plan's live Worlds,
\* storing their pages. A World neither kept nor stored refuses the move,
\* which retries.
Trim ==
  /\ trimKeep # {}
  /\ \A w \in trimKeep : Readable(w)
  /\ held' = (held \ trimRel) \cup (trimKeep \ {base})
  /\ vol' = [w \in Worlds |-> IF w \in trimKeep /\ mem[w] THEN "vol" ELSE vol[w]]
  /\ baseRoot' = base
  /\ trimKeep' = {} /\ trimRel' = {}
  /\ UNCHANGED <<pos, floor, savedPos, savedBase, next, mem, base, readers,
                 storing, cands, ending, saveH, savePos, saveBase>>

\* A restart: memory, readers, writers' turns and background work are gone,
\* the epochs end with the stage, and replay restores the saved positions on
\* the base they were saved on, whose Worlds before the last it computes
\* again (restoredAt). loadCursor ignores a cursor whose head World the
\* cursor root no longer holds.
Restored == /\ savedPos # <<>> /\ savedBase = base
            /\ savedPos[Len(savedPos)] \in held
Restart ==
  /\ pos' = IF Restored THEN savedPos ELSE <<>>
  /\ floor' = IF Restored THEN Len(savedPos) - 1 ELSE 0
  /\ mem' = [w \in Worlds |-> FALSE]
  /\ vol' = [w \in Worlds |-> IF vol[w] = "epoch" THEN "vol" ELSE vol[w]]
  /\ readers' = {} /\ storing' = {} /\ cands' = {}
  /\ ending' = {} /\ saveH' = {} /\ trimRel' = {} /\ trimKeep' = {}
  /\ UNCHANGED <<savedPos, savedBase, next, held, base, baseRoot, savePos, saveBase>>

\* The Volume collector frees a stored page no root holds.
Collect(w) == /\ vol[w] = "vol" /\ w \notin held /\ w # baseRoot
              /\ vol' = [vol EXCEPT ![w] = "absent"]
              /\ UNCHANGED <<pos, floor, savedPos, savedBase, next, mem, held,
                             base, baseRoot, readers, storing, cands, ending, saveH,
                             savePos, saveBase, trimRel, trimKeep>>

Next == \/ \E k \in 0..Len(pos), page \in {"mem", "epoch"} : Apply(k, page)
        \/ \E k \in 0..Len(pos), c \in cands : Adopt(k, c)
        \/ \E page \in {"mem", "epoch"} : Admit(page)
        \/ \E n \in 1..Len(pos) : Root(n)
        \/ \E w \in storing : StoreLands(w)
        \/ Read
        \/ \E w \in readers : ReadEnds(w)
        \/ \E full \in BOOLEAN : SaveStart(full)
        \/ SaveLands
        \/ \E moved \in BOOLEAN : SaveFails(moved)
        \/ \E n \in 1..Len(pos) : Install(n)
        \/ Trim
        \/ Restart
        \/ \E w \in Worlds : Collect(w)

Spec == Init /\ [][Next]_vars

\* Bound the model to one engine reader or checkpoint store, and one
\* candidate, at a time, which every trace to a broken rule here needs at
\* most, so the check fits in 1 GiB.
Bound == Cardinality(readers \cup storing) <= 1 /\ Cardinality(cands) <= 1

TypeOK == /\ floor \in 0..MaxWorlds /\ next \in 1..MaxWorlds
          /\ held \subseteq Worlds /\ readers \subseteq Worlds
          /\ storing \subseteq Worlds /\ cands \subseteq Worlds
          /\ base \in 0..MaxWorlds /\ baseRoot \in 0..MaxWorlds
          /\ \A w \in Worlds : vol[w] \in {"absent", "epoch", "vol"}

\* Invariant A: every World the adapter may still read reads, from memory or
\* the Volume: each position replay counts as computed, the base it
\* computes the rest from, each engine reader's World, each World a
\* background store, save or trim holds, and each writer's candidate.
LiveWorldsRead == /\ \A i \in Indexes : ~Unstored(i) => Readable(Pos(i))
                  /\ base = 0 \/ Readable(base)
                  /\ \A w \in readers \cup storing \cup Background \cup cands : Readable(w)

\* Invariant D: every World a root holds is stored, so no stored root names
\* a hole.
RootsNameStoredWorlds == \A w \in held \cup ({baseRoot} \ {0}) : vol[w] # "absent"
====
