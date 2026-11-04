---- MODULE WriterStall ----
\* How long the writer lock is held, as WriterLock.tla's grants hand it on.
\* WriterLock.tla proves that every waiter is granted eventually; this spec
\* proves how long it waits, measured in reads of the database file done
\* under the lock (ticks). A read from memory costs no tick, a cold index
\* page read costs one, a batched prefetch round one, and writing the
\* values and the record one.
\*
\* A big commit of BigKeys and a small commit of SmallKey contend with the
\* checkpoint, which goes ahead of queued writers (lockFirst) and retires
\* the overlay into the tree. The index pages of the commit keys are warm in
\* the shared page cache, of CacheCap pages, or cold. With Pressure, readers
\* outside the lock evict warm pages at any time. With FalsePositive, the
\* overlay's filter answers "maybe" for keys the overlay does not hold, as a
\* filter far past the overlay size it was built for does.
\*
\* Mode chooses how commit resolves its keys against the base state:
\*   "filter":  under the lock, prefetch the keys the filter rules out of
\*              the overlay in one round, then look each key up, reading a
\*              cold page for each key the prefetch skipped or that was
\*              evicted since.
\*   "overlay": as "filter", but the prefetch skips only keys the overlay
\*              holds.
\*   "outside": read every key before taking the lock, keeping the result
\*              in the commit; under the lock resolve again only the keys
\*              another commit changed since, from the overlay, the cache or
\*              the file.
\*   "batch":   under the lock, resolve every key the overlay does not
\*              hold in one batched descent of the tree that answers each
\*              key from the pages it read, so neither the filter nor an
\*              eviction can cost a second read. The descent reads one round
\*              per tree level; a level is one tick here.
\*
\* The system starts as production did after a restart: the cache cold,
\* the big commit holding the lock and the checkpoint and the small commit
\* queued behind it. Wait counters saturate past their bound so the state
\* space stays finite. Not modeled: cancellation (WriterLock.tla), crash
\* and recovery (Durability.tla), and CPU time, which stretches every tick
\* alike when the collector runs.
EXTENDS Naturals, Sequences, FiniteSets
CONSTANTS BigKeys, SmallKey, CacheCap, Pressure, FalsePositive, Mode,
          CkptBound, WriterBound
Writers == {"big", "small"}
Actors == Writers \cup {"checkpoint"}
Keys == BigKeys \cup {SmallKey}
KeysOf(w) == IF w = "big" THEN BigKeys ELSE {SmallKey}
Bound(a) == IF a = "checkpoint" THEN CkptBound ELSE WriterBound
\* Under-lock phases a holder passes through, in order.
Phases == {"prefetch", "lookup", "resolve", "write", "save"}

VARIABLES owner, first, rest, pc, warm, overlay, maybe, dirty, waited
vars == <<owner, first, rest, pc, warm, overlay, maybe, dirty, waited>>

\* The first phase under the lock, and what a writer does before it.
FirstPhase(a) ==
 IF a = "checkpoint" THEN "save" ELSE IF Mode = "outside" THEN "resolve" ELSE "prefetch"

\* Charge cost ticks to every waiter, saturating one past its bound.
Charge(cost) ==
 waited' = [a \in Actors |-> IF pc[a] = "waiting"
                             THEN IF waited[a] + cost > Bound(a) THEN Bound(a) + 1 ELSE waited[a] + cost
                             ELSE waited[a]]

\* The cache after pages enter it, keeping at most CacheCap, any of them.
Fill(pages) ==
 {S \in SUBSET (warm \cup pages) : Cardinality(S) = IF Cardinality(warm \cup pages) > CacheCap
                                                    THEN CacheCap ELSE Cardinality(warm \cup pages)}

Init ==
 /\ owner = "big" /\ first = <<"checkpoint">> /\ rest = <<"small">>
 /\ pc = [a \in Actors |-> IF a = "big" THEN FirstPhase("big") ELSE "waiting"]
 /\ warm = {} /\ overlay = {} /\ dirty = [w \in Writers |-> {}]
 /\ maybe \in IF FalsePositive THEN SUBSET Keys ELSE {{}}
 /\ waited = [a \in Actors |-> 0]

\* A writer reads its keys outside the lock; nobody waits for that.
Prepare(w) ==
 /\ pc[w] = "idle" /\ Mode = "outside"
 /\ pc' = [pc EXCEPT ![w] = "ready"] /\ dirty' = [dirty EXCEPT ![w] = {}]
 /\ UNCHANGED <<owner, first, rest, warm, overlay, maybe, waited>>

Enter(a) ==
 /\ IF a \in Writers /\ Mode = "outside" THEN pc[a] = "ready" ELSE pc[a] = "idle"
 /\ waited' = [waited EXCEPT ![a] = 0]
 /\ IF owner = "none"
    THEN /\ owner' = a /\ pc' = [pc EXCEPT ![a] = FirstPhase(a)]
         /\ UNCHANGED <<first, rest>>
    ELSE /\ owner' = owner /\ pc' = [pc EXCEPT ![a] = "waiting"]
         /\ IF a = "checkpoint"
            THEN first' = Append(first, a) /\ rest' = rest
            ELSE rest' = Append(rest, a) /\ first' = first
 /\ UNCHANGED <<warm, overlay, maybe, dirty>>

\* Under the lock: one prefetch round over the keys the overlay seems not
\* to answer. The batched descent resolves the keys in that round.
Prefetch(w) ==
 /\ owner = w /\ pc[w] = "prefetch"
 /\ LET skip == IF Mode = "filter" THEN overlay \cup maybe ELSE overlay
        cold == (KeysOf(w) \ skip) \ warm
    IN /\ warm' \in Fill(cold)
       /\ Charge(IF cold = {} THEN 0 ELSE 1)
 /\ pc' = [pc EXCEPT ![w] = IF Mode = "batch" THEN "write" ELSE "lookup"]
 /\ UNCHANGED <<owner, first, rest, overlay, maybe, dirty>>

\* Under the lock: a cold page for each key neither overlay nor cache holds.
ReadCold(w, ks) ==
 LET cold == (ks \ overlay) \ warm
 IN /\ warm' \in Fill(cold)
    /\ Charge(Cardinality(cold))

Lookup(w) ==
 /\ owner = w /\ pc[w] = "lookup"
 /\ ReadCold(w, KeysOf(w))
 /\ pc' = [pc EXCEPT ![w] = "write"]
 /\ UNCHANGED <<owner, first, rest, overlay, maybe, dirty>>

Resolve(w) ==
 /\ owner = w /\ pc[w] = "resolve"
 /\ ReadCold(w, KeysOf(w) \cap dirty[w])
 /\ pc' = [pc EXCEPT ![w] = "write"]
 /\ UNCHANGED <<owner, first, rest, overlay, maybe, dirty>>

\* Hand the lock to the first queued checkpoint, else the first writer.
Handoff(a) ==
 LET q == IF Len(first) > 0 THEN first ELSE rest IN
 IF Len(q) = 0
 THEN /\ owner' = "none" /\ pc' = [pc EXCEPT ![a] = "idle"]
      /\ UNCHANGED <<first, rest>>
 ELSE /\ owner' = Head(q)
      /\ pc' = [pc EXCEPT ![a] = "idle", ![Head(q)] = FirstPhase(Head(q))]
      /\ IF Len(first) > 0
         THEN first' = Tail(first) /\ rest' = rest
         ELSE rest' = Tail(rest) /\ first' = first

\* Write the values and the record, mark the keys changed for every writer
\* that read before this commit, and release.
Write(w) ==
 /\ owner = w /\ pc[w] = "write"
 /\ overlay' = overlay \cup KeysOf(w)
 /\ dirty' = [x \in Writers |-> IF x # w /\ pc[x] \in {"ready", "waiting"}
                               THEN dirty[x] \cup KeysOf(w) ELSE dirty[x]]
 /\ Charge(1)
 /\ Handoff(w)
 /\ UNCHANGED <<warm, maybe>>

\* The checkpoint's step under the lock: reserve, save and retire the
\* overlay. Its new tree pages replace the cached ones, cold. The next
\* filter starts empty and may fill again.
Save ==
 /\ owner = "checkpoint" /\ pc["checkpoint"] = "save"
 /\ warm' = warm \ overlay /\ overlay' = {}
 /\ maybe' \in IF FalsePositive THEN SUBSET Keys ELSE {{}}
 /\ Charge(1)
 /\ Handoff("checkpoint")
 /\ UNCHANGED dirty

\* Readers outside the lock push a page out of the cache.
Evict(k) ==
 /\ Pressure /\ k \in warm
 /\ warm' = warm \ {k}
 /\ UNCHANGED <<owner, first, rest, pc, overlay, maybe, dirty, waited>>

Next ==
 \/ Save \/ \E k \in Keys: Evict(k)
 \/ \E a \in Actors: Enter(a)
 \/ \E w \in Writers: Prepare(w) \/ Prefetch(w) \/ Lookup(w) \/ Resolve(w) \/ Write(w)

Safety ==
 /\ owner \in Actors \cup {"none"}
 /\ \A a \in Actors: (pc[a] \in Phases) <=> owner = a
 /\ Len(first) + Len(rest) = Cardinality({a \in Actors : pc[a] = "waiting"})
 /\ Cardinality(warm) <= CacheCap

\* The checkpoint is granted within CkptBound ticks of queueing, so it makes
\* progress while writers keep arriving.
CheckpointWait == waited["checkpoint"] <= CkptBound

\* Every queued writer is granted within WriterBound ticks.
WriterWait == \A w \in Writers: waited[w] <= WriterBound

\* Every waiter is granted eventually. It holds in every mode: fairness alone
\* cannot tell a stall from progress.
Progress == \A a \in Actors: (pc[a] = "waiting") ~> (pc[a] \in Phases)

Spec == Init /\ [][Next]_vars
 /\ WF_vars(Save)
 /\ \A w \in Writers: WF_vars(Prefetch(w)) /\ WF_vars(Lookup(w)) /\ WF_vars(Resolve(w)) /\ WF_vars(Write(w))
====
