---- MODULE ReplaySkip ----
\* An owner's skips across two devices. The owner has no handler for the
\* plugin ops p and q, so its replay waits at each; the member gains a
\* handler at any time, so it may apply an op before its skip, wait at it
\* when the skip arrives, or reach it after.
\*
\* A skip is a config entry, signed by an owner, that names the op by its
\* hash. Each device reads the config from the same snapshot as the order,
\* so it decides the op by the config it holds: an op the config skips is
\* refused as skipped, and an op it had applied and the config now skips
\* cuts its kept prefix, as an order change does, so replay re-runs from
\* there and refuses it.
\*
\* The config holds at most MaxLive skips. A config change prunes the skips
\* the held checkpoint covers, and the state that carries a config carries
\* the checkpoint it was pruned against, so no device holds a config without
\* a skip and a prefix below that skip's op.
\* The spec models the owner's builder and no verifier: only Skip writes a
\* config, so no config drops an uncovered skip.
\*
\* The order above the checkpoint may change: a late op can sort before p.
\* The checkpoint covers the owner's kept prefix and names the config it was
\* signed under; a device installs it only once it holds that config.
EXTENDS Naturals, Sequences, FiniteSets

CONSTANT MaxLive

\* SecondOwner makes the member an owner too, which may sign a checkpoint
\* under the config it holds while the owner's skip has not reached it.
CONSTANT SecondOwner

Devices == {"owner", "member"}
Plugin == {"p", "q"}
Ops == {"a", "b", "late"} \cup Plugin
Outs == {"applied", "skipped"}

VARIABLES order, done, cfgs, seen, registered, ck, reordered, ck2
vars == <<order, done, cfgs, seen, registered, ck, reordered, ck2>>

\* Skipped is the set of ops config v skips, by hash; config 0 is genesis.
Skipped(v) == IF v = 0 THEN {} ELSE {e.op : e \in cfgs[v].skips}

\* EverSkipped is every op a config up to v skipped, pruned ones included.
EverSkipped(v) == UNION {Skipped(j) : j \in 0..v}

\* Base is the checkpoint prefix config v was pruned against.
Base(v) == IF v = 0 THEN <<>> ELSE cfgs[v].base

\* Covered is the set of ops the checkpoint covers.
Covered == {ck.outs[i].op : i \in 1..Len(ck.outs)}

\* HasPrefix is whether s starts with t.
HasPrefix(s, t) == Len(s) >= Len(t) /\ SubSeq(s, 1, Len(t)) = t

\* Keep is the prefix of s replay keeps under config v: the ops that match
\* the order, up to the first applied op the config now skips. The
\* checkpoint's prefix is final once s extends it: its ops left the set.
Ok(s, v, k) == \A i \in 1..k : /\ i <= Len(order) /\ s[i].op = order[i]
                               /\ \/ i <= Len(ck.outs) /\ HasPrefix(s, ck.outs)
                                  \/ ~(s[i].out = "applied" /\ s[i].op \in Skipped(v))
Keep(s, v) == CHOOSE k \in 0..Len(s) : Ok(s, v, k) /\ (k = Len(s) \/ ~Ok(s, v, k + 1))

\* Can is whether device d has a handler for op o.
Can(d, o) == o \notin Plugin \/ (d = "member" /\ registered)

\* Next0 is the op after device d's kept prefix.
Kept(d) == Keep(done[d], seen[d])
Next0(d) == order[Kept(d) + 1]

\* Waiting is whether device d's replay waits at an op it cannot apply.
Waiting(d) == /\ Kept(d) = Len(done[d]) /\ Len(done[d]) < Len(order)
              /\ Next0(d) \notin Skipped(seen[d]) /\ ~Can(d, Next0(d))

\* Settled is whether device d holds the latest config and has decided
\* every op in the order.
Settled(d) == /\ seen[d] = Len(cfgs) /\ Kept(d) = Len(done[d])
              /\ Len(done[d]) = Len(order)

Init == /\ order = <<"a", "p", "b", "q">>
        /\ done = [d \in Devices |-> <<>>]
        /\ cfgs = <<>> /\ seen = [d \in Devices |-> 0]
        /\ registered = FALSE
        /\ ck = [outs |-> <<>>, cfg |-> 0]
        /\ reordered = FALSE
        /\ ck2 = [outs |-> <<>>, cfg |-> 0]

\* Step is one driver step: cut back to the kept prefix, or decide the next
\* op, refused as skipped or applied.
Step(d) ==
    /\ IF Kept(d) < Len(done[d])
       THEN done' = [done EXCEPT ![d] = SubSeq(done[d], 1, Kept(d))]
       ELSE /\ Len(done[d]) < Len(order)
            /\ LET o == Next0(d) IN
               IF o \in Skipped(seen[d])
               THEN done' = [done EXCEPT ![d] = Append(done[d], [op |-> o, out |-> "skipped"])]
               ELSE /\ Can(d, o)
                    /\ done' = [done EXCEPT ![d] = Append(done[d], [op |-> o, out |-> "applied"])]
    /\ UNCHANGED <<order, cfgs, seen, registered, ck, reordered, ck2>>

\* Live is the set of skip entries the next config change keeps: those the
\* checkpoint does not cover.
Live == IF cfgs = <<>> THEN {} ELSE {e \in cfgs[Len(cfgs)].skips : e.op \notin Covered}

\* Skip is the owner's config change naming the op its replay waits at, while
\* fewer than MaxLive skips are live. The entry records the position too,
\* which only the position mutant reads.
Skip == /\ Waiting("owner")
        /\ Cardinality(Live) < MaxLive
        /\ cfgs' = Append(cfgs, [skips |-> Live \cup {[op |-> Next0("owner"), at |-> Kept("owner") + 1]},
                                 base |-> ck.outs])
        /\ seen' = [seen EXCEPT !["owner"] = Len(cfgs) + 1]
        /\ UNCHANGED <<order, done, registered, ck, reordered, ck2>>

\* The member receives the latest config with the next snapshot, which also
\* carries the checkpoint the config was pruned against.
SeeConfig(d) == /\ d # "owner" /\ seen[d] < Len(cfgs)
                /\ seen' = [seen EXCEPT ![d] = Len(cfgs)]
                /\ done' = IF HasPrefix(done[d], Base(Len(cfgs))) THEN done
                           ELSE [done EXCEPT ![d] = Base(Len(cfgs))]
                /\ UNCHANGED <<order, cfgs, registered, ck, reordered, ck2>>

\* The member's plugin registers its handler.
Register == /\ ~registered /\ registered' = TRUE
            /\ UNCHANGED <<order, done, cfgs, seen, ck, reordered, ck2>>

\* The owner checkpoints its kept prefix under the config it holds.
Checkpoint == /\ Kept("owner") > Len(ck.outs)
              /\ ck' = [outs |-> SubSeq(done["owner"], 1, Kept("owner")), cfg |-> seen["owner"]]
              /\ UNCHANGED <<order, done, cfgs, seen, registered, reordered, ck2>>

\* A late op sorts into the order above the checkpoint.
Reorder == /\ ~reordered
           /\ \E i \in (Len(ck.outs) + 1)..(Len(order) + 1) :
                order' = SubSeq(order, 1, i - 1) \o <<"late">> \o SubSeq(order, i, Len(order))
           /\ reordered' = TRUE
           /\ UNCHANGED <<done, cfgs, seen, registered, ck, ck2>>

\* The member installs the checkpoint once it holds the checkpoint's config.
Install(d) == /\ d # "owner" /\ seen[d] >= ck.cfg
              /\ ~HasPrefix(done[d], ck.outs)
              /\ done' = [done EXCEPT ![d] = ck.outs]
              /\ UNCHANGED <<order, cfgs, seen, registered, ck, reordered, ck2>>

\* The second owner checkpoints its kept prefix under the config it holds,
\* which may not have the owner's latest skip yet.
StaleCheckpoint == /\ SecondOwner /\ ck2.outs = <<>>
                   /\ Kept("member") > Len(ck.outs) /\ HasPrefix(done["member"], ck.outs)
                   /\ ck2' = [outs |-> SubSeq(done["member"], 1, Kept("member")), cfg |-> seen["member"]]
                   /\ UNCHANGED <<order, done, cfgs, seen, registered, ck, reordered>>

\* The owner adopts the second owner's checkpoint. Under owner control an
\* owner's signature authorizes it, whatever config it names, as long as
\* that config is one the owner holds.
AdoptStale == /\ ck2.outs # <<>> /\ seen["owner"] >= ck2.cfg
              /\ Len(ck2.outs) > Len(ck.outs) /\ HasPrefix(ck2.outs, ck.outs)
              /\ ck' = ck2 /\ ck2' = [outs |-> <<>>, cfg |-> 0]
              /\ done' = [done EXCEPT !["owner"] = ck2.outs]
              /\ UNCHANGED <<order, cfgs, seen, registered, reordered>>

Next == \/ \E d \in Devices : Step(d) \/ SeeConfig(d) \/ Install(d)
        \/ Skip \/ Register \/ Checkpoint \/ Reorder
        \/ StaleCheckpoint \/ AdoptStale

Spec == Init /\ [][Next]_vars
FairSpec == /\ Spec /\ WF_vars(Skip) /\ WF_vars(Register) /\ WF_vars(Checkpoint)
            /\ \A d \in Devices : WF_vars(Step(d)) /\ WF_vars(SeeConfig(d))

Records == [op : Ops, out : Outs]
TypeOK == /\ order \in Seq(Ops) /\ done \in [Devices -> Seq(Records)]
          /\ \A v \in 1..Len(cfgs) : cfgs[v].skips \subseteq [op : Ops, at : Nat]
          /\ seen \in [Devices -> 0..Len(cfgs)]
          /\ ck.outs \in Seq(Records) /\ ck.cfg \in 0..Len(cfgs)
          /\ ck2.outs \in Seq(Records) /\ ck2.cfg \in 0..Len(cfgs)

\* Only an op an owner waited at is ever refused as skipped, on any device,
\* a reordered log included.
SkipHitsOnlyStalledOp == \A d \in Devices : \A i \in 1..Len(done[d]) :
                           done[d][i].out = "skipped" => done[d][i].op \in Plugin

\* A device refuses an op as skipped only under a config it holds, or one
\* before it that skipped the op before a checkpoint covered it.
NoMismatch == \A d \in Devices : \A i \in 1..Len(done[d]) :
                done[d][i].out = "skipped" => done[d][i].op \in EverSkipped(seen[d])

\* A checkpoint that covers a skipped op names a config that skipped it, or
\* one after it that pruned the skip.
CheckpointHoldsSkips == \A i \in 1..Len(ck.outs) :
                          ck.outs[i].out = "skipped" => ck.outs[i].op \in EverSkipped(ck.cfg)

\* No device keeps as applied an op the config it holds skips: a written
\* skip is never overridden.
SkipHonored == \A d \in Devices : \A i \in 1..Kept(d) :
                 ~(done[d][i].out = "applied" /\ done[d][i].op \in Skipped(seen[d]))

\* The config holds at most MaxLive skips.
LiveCapped == \A v \in 1..Len(cfgs) : Cardinality(cfgs[v].skips) <= MaxLive

\* Two settled devices hold the same outcomes, the one that applied an op
\* before its skip included.
Agree == \A d1, d2 \in Devices : Settled(d1) /\ Settled(d2) => done[d1] = done[d2]

\* The owner's skips end every stall, at the cap too: every device settles.
StallExits == <>[](\A d \in Devices : Settled(d))
====
