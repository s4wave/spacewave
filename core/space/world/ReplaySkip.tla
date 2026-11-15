---- MODULE ReplaySkip ----
\* An owner's skip across two devices. The owner has no handler for the
\* plugin op p, so its replay waits there; the member gains a handler at
\* any time, so it may apply p before the skip, wait at p when the skip
\* arrives, or reach p after it.
\*
\* The skip is a config entry, signed by an owner, that names the op by its
\* hash. Each device reads the config from the same snapshot as the order,
\* so it decides the op by the config it holds: an op the config skips is
\* refused as skipped, and an op it had applied and the config now skips
\* cuts its kept prefix, as an order change does, so replay re-runs from
\* there and refuses it.
\*
\* The order above the checkpoint may change: a late op can sort before p.
\* The checkpoint covers the owner's kept prefix and names the config it was
\* signed under; a device installs it only once it holds that config.
EXTENDS Naturals, Sequences

Devices == {"owner", "member"}
Ops == {"a", "p", "b", "late"}
Outs == {"applied", "skipped"}

VARIABLES order, done, cfgs, seen, registered, ck, reordered
vars == <<order, done, cfgs, seen, registered, ck, reordered>>

\* Skipped is the set of ops the first v config entries skip, by hash.
Skipped(v) == {cfgs[j].op : j \in 1..v}

\* Keep is the prefix of s replay keeps under config v: the ops that match
\* the order, up to the first applied op the config now skips.
Ok(s, v, k) == \A i \in 1..k : /\ i <= Len(order) /\ s[i].op = order[i]
                               /\ ~(s[i].out = "applied" /\ s[i].op \in Skipped(v))
Keep(s, v) == CHOOSE k \in 0..Len(s) : Ok(s, v, k) /\ (k = Len(s) \/ ~Ok(s, v, k + 1))

\* Can is whether device d has a handler for op o.
Can(d, o) == o # "p" \/ (d = "member" /\ registered)

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

Init == /\ order = <<"a", "p", "b">>
        /\ done = [d \in Devices |-> <<>>]
        /\ cfgs = <<>> /\ seen = [d \in Devices |-> 0]
        /\ registered = FALSE
        /\ ck = [outs |-> <<>>, cfg |-> 0]
        /\ reordered = FALSE

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
    /\ UNCHANGED <<order, cfgs, seen, registered, ck, reordered>>

\* Skip is the owner's config change naming the op its replay waits at.
\* The entry records the position too, which only the index mutant reads.
Skip == /\ Waiting("owner")
        /\ cfgs' = Append(cfgs, [op |-> Next0("owner"), at |-> Kept("owner") + 1])
        /\ seen' = [seen EXCEPT !["owner"] = Len(cfgs) + 1]
        /\ UNCHANGED <<order, done, registered, ck, reordered>>

\* The member receives the latest config with the next snapshot.
SeeConfig(d) == /\ d # "owner" /\ seen[d] < Len(cfgs)
                /\ seen' = [seen EXCEPT ![d] = Len(cfgs)]
                /\ UNCHANGED <<order, done, cfgs, registered, ck, reordered>>

\* The member's plugin registers its handler.
Register == /\ ~registered /\ registered' = TRUE
            /\ UNCHANGED <<order, done, cfgs, seen, ck, reordered>>

\* The owner checkpoints its kept prefix under the config it holds.
Checkpoint == /\ Kept("owner") > Len(ck.outs)
              /\ ck' = [outs |-> SubSeq(done["owner"], 1, Kept("owner")), cfg |-> seen["owner"]]
              /\ UNCHANGED <<order, done, cfgs, seen, registered, reordered>>

\* A late op sorts into the order above the checkpoint.
Reorder == /\ ~reordered
           /\ \E i \in (Len(ck.outs) + 1)..(Len(order) + 1) :
                order' = SubSeq(order, 1, i - 1) \o <<"late">> \o SubSeq(order, i, Len(order))
           /\ reordered' = TRUE
           /\ UNCHANGED <<done, cfgs, seen, registered, ck>>

\* The member installs the checkpoint once it holds the checkpoint's config.
Install(d) == /\ d # "owner" /\ seen[d] >= ck.cfg
              /\ ~(Len(done[d]) >= Len(ck.outs) /\ SubSeq(done[d], 1, Len(ck.outs)) = ck.outs)
              /\ done' = [done EXCEPT ![d] = ck.outs]
              /\ UNCHANGED <<order, cfgs, seen, registered, ck, reordered>>

Next == \/ \E d \in Devices : Step(d) \/ SeeConfig(d) \/ Install(d)
        \/ Skip \/ Register \/ Checkpoint \/ Reorder

Spec == Init /\ [][Next]_vars
FairSpec == /\ Spec /\ WF_vars(Skip) /\ WF_vars(Register)
            /\ \A d \in Devices : WF_vars(Step(d)) /\ WF_vars(SeeConfig(d))

Records == [op : Ops, out : Outs]
TypeOK == /\ order \in Seq(Ops) /\ done \in [Devices -> Seq(Records)]
          /\ cfgs \in Seq([op : Ops, at : Nat]) /\ seen \in [Devices -> 0..Len(cfgs)]
          /\ ck.outs \in Seq(Records) /\ ck.cfg \in 0..Len(cfgs)

\* Only the op the owner waited at is ever refused as skipped, on any
\* device, a reordered log included.
SkipHitsOnlyStalledOp == \A d \in Devices : \A i \in 1..Len(done[d]) :
                           done[d][i].out = "skipped" => done[d][i].op = "p"

\* A device refuses an op as skipped only under a config it holds.
NoMismatch == \A d \in Devices : \A i \in 1..Len(done[d]) :
                done[d][i].out = "skipped" => done[d][i].op \in Skipped(seen[d])

\* A checkpoint that covers a skipped op names a config holding the skip.
CheckpointHoldsSkips == \A i \in 1..Len(ck.outs) :
                          ck.outs[i].out = "skipped" => ck.outs[i].op \in Skipped(ck.cfg)

\* Two settled devices hold the same outcomes, the one that applied p
\* before the skip included.
Agree == \A d1, d2 \in Devices : Settled(d1) /\ Settled(d2) => done[d1] = done[d2]

\* The owner's skip ends the stall: every device settles.
StallExits == <>[](\A d \in Devices : Settled(d))
====
