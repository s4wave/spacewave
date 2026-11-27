---- MODULE SOPeerHost ----
EXTENDS Naturals
(* sobject.go and p2p-sync.go: each tracker run of a Space publishes a peer
   host, its verified import contract, once the cloud host has registered for
   events and before the mount is ready. A Space's direct worker acquires only
   its own Space's peer host and runs SOSync against it until that run ends.
   Every import goes through the host's peer-import lock: the host resolves
   initial authority, verifies the peer state, persists it, and only then
   publishes it to watchers. The lock refuses a closed host. A run that ends
   withdraws its peer host; its host closes before the run leaves its run
   slot, and the next run takes the slot only after that, so it loads every
   write of the run before it. A worker notices the end of its run
   asynchronously, so it can still hold the ended peer host. *)
CONSTANTS Spaces, MaxRun, MaxVer
\* phase: the Space's current run: idle, loading (slot taken, cache loaded,
\*   host built), started (events registered, peer host published) or ready
\*   (initial authority resolved and membership recovered: the mount).
\* run: the current run's number, or the last one after it ended.
\* open: the runs whose hosts are not yet closed.
\* published: the run whose peer host is published, or 0.
\* authority: whether the newest run's host has resolved initial authority.
\* worker: the run whose peer host the Space's worker holds, or 0.
\* durable, visible: the cache's and the watchers' state versions.
\* badVisible: watchers saw an unverified peer state.
\* unauthorized: an import was accepted before initial authority.
\* late: an ended run wrote after the next run loaded the cache.
VARIABLES phase, run, open, published, authority, worker,
          durable, visible, badVisible, unauthorized, late
vars == <<phase, run, open, published, authority, worker,
          durable, visible, badVisible, unauthorized, late>>

Phases == {"idle", "loading", "started", "ready"}
Runs == 1..MaxRun

TypeOK ==
  /\ phase \in [Spaces -> Phases]
  /\ run \in [Spaces -> 0..MaxRun]
  /\ open \in [Spaces -> SUBSET Runs]
  /\ published \in [Spaces -> 0..MaxRun]
  /\ authority \in [Spaces -> BOOLEAN]
  /\ worker \in [Spaces -> 0..MaxRun]
  /\ durable \in [Spaces -> 0..MaxVer]
  /\ visible \in [Spaces -> 0..MaxVer]
  /\ badVisible \in BOOLEAN /\ unauthorized \in BOOLEAN /\ late \in BOOLEAN

Init ==
  /\ phase = [s \in Spaces |-> "idle"]
  /\ run = [s \in Spaces |-> 0]
  /\ open = [s \in Spaces |-> {}]
  /\ published = [s \in Spaces |-> 0]
  /\ authority = [s \in Spaces |-> FALSE]
  /\ worker = [s \in Spaces |-> 0]
  /\ durable = [s \in Spaces |-> 0]
  /\ visible = [s \in Spaces |-> 0]
  /\ badVisible = FALSE /\ unauthorized = FALSE /\ late = FALSE

\* A run takes the run slot once every earlier host has closed, loads the
\* verified cache and builds its host.
Start(s) ==
  /\ phase[s] = "idle" /\ run[s] < MaxRun
  /\ open[s] = {}
  /\ phase' = [phase EXCEPT ![s] = "loading"]
  /\ run' = [run EXCEPT ![s] = run[s] + 1]
  /\ open' = [open EXCEPT ![s] = open[s] \cup {run[s] + 1}]
  /\ visible' = [visible EXCEPT ![s] = durable[s]]
  /\ authority' = [authority EXCEPT ![s] = FALSE]
  /\ UNCHANGED <<published, worker, durable, badVisible, unauthorized, late>>

\* The host registers for events, then publishes its peer host.
Subscribe(s) ==
  /\ phase[s] = "loading"
  /\ phase' = [phase EXCEPT ![s] = "started"]
  /\ published' = [published EXCEPT ![s] = run[s]]
  /\ UNCHANGED <<run, open, authority, worker, durable, visible,
                 badVisible, unauthorized, late>>

\* The host resolves initial authority from the cache or the cloud. The cloud
\* state the host verified is durable before it is visible.
Authority(s) ==
  /\ phase[s] \in {"started", "ready"} /\ ~authority[s]
  /\ authority' = [authority EXCEPT ![s] = TRUE]
  /\ UNCHANGED <<phase, run, open, published, worker, durable, visible,
                 badVisible, unauthorized, late>>

\* Membership recovery finishes and the mount becomes ready.
Ready(s) ==
  /\ phase[s] = "started" /\ authority[s]
  /\ phase' = [phase EXCEPT ![s] = "ready"]
  /\ UNCHANGED <<run, open, published, authority, worker, durable, visible,
                 badVisible, unauthorized, late>>

\* The worker acquires its own Space's published peer host.
Acquire(s) ==
  /\ worker[s] = 0 /\ published[s] # 0
  /\ worker' = [worker EXCEPT ![s] = published[s]]
  /\ UNCHANGED <<phase, run, open, published, authority, durable, visible,
                 badVisible, unauthorized, late>>

\* The worker sees its run end and drops the peer host.
Release(s) ==
  /\ worker[s] # 0 /\ published[s] # worker[s]
  /\ worker' = [worker EXCEPT ![s] = 0]
  /\ UNCHANGED <<phase, run, open, published, authority, durable, visible,
                 badVisible, unauthorized, late>>

\* A peer offers a state, valid or not, to the worker's host. The peer-import
\* lock refuses a closed host and waits for the host to resolve authority.
\* The host rejects an unverified state. A verified state is persisted, and
\* published only if the persist succeeded.
Import(s, valid, persisted) ==
  LET r == worker[s]
      accept == valid /\ persisted
      show == accept
  IN
  /\ r # 0 /\ r \in open[s]
  /\ authority[s]
  /\ durable[s] < MaxVer
  /\ late' = (late \/ r < run[s])
  /\ unauthorized' = (unauthorized \/ ~authority[s])
  /\ badVisible' = (badVisible \/ (show /\ ~valid))
  /\ durable' = [durable EXCEPT ![s] = IF accept THEN durable[s] + 1 ELSE durable[s]]
  /\ visible' = [visible EXCEPT ![s] = IF show THEN durable[s] + 1 ELSE visible[s]]
  /\ UNCHANGED <<phase, run, open, published, authority, worker>>

\* Revocation, a failure or a release ends the run: its context is canceled
\* and its peer host withdrawn. Its host stays open until Close.
End(s) ==
  /\ phase[s] # "idle"
  /\ phase' = [phase EXCEPT ![s] = "idle"]
  /\ published' = [published EXCEPT ![s] = 0]
  /\ UNCHANGED <<run, open, authority, worker, durable, visible,
                 badVisible, unauthorized, late>>

\* An ended run's host closes, after the import holding its lock.
Close(s, r) ==
  /\ r \in open[s] /\ (r # run[s] \/ phase[s] = "idle")
  /\ open' = [open EXCEPT ![s] = open[s] \ {r}]
  /\ UNCHANGED <<phase, run, published, authority, worker, durable, visible,
                 badVisible, unauthorized, late>>

Next ==
  \E s \in Spaces:
    \/ Start(s) \/ Subscribe(s) \/ Authority(s) \/ Ready(s)
    \/ Acquire(s) \/ Release(s) \/ End(s)
    \/ \E r \in Runs: Close(s, r)
    \/ \E valid, persisted \in BOOLEAN: Import(s, valid, persisted)

\* Watchers see only states the host verified.
Verified == ~badVisible
\* Watchers see a state only once it is durable.
DurableBeforeVisible == \A s \in Spaces: visible[s] <= durable[s]
\* No import is accepted before the run resolves initial authority.
AuthorityFirst == ~unauthorized
\* An ended run never writes after the next run loaded the cache.
EndedRunSilent == ~late
\* A published peer host can be acquired whatever the state of any mount,
\* this Space's or another's.
Independent ==
  \A s \in Spaces: (worker[s] = 0 /\ published[s] # 0) => ENABLED Acquire(s)
\* A peer host is published only by a running, subscribed run.
PublishedLive ==
  \A s \in Spaces: published[s] # 0 =>
    /\ published[s] = run[s] /\ phase[s] \in {"started", "ready"}

Spec == Init /\ [][Next]_vars
====
