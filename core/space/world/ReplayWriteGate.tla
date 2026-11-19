---- MODULE ReplayWriteGate ----
\* The adapter's write gate on one device. Replay is opening, caught up, or
\* waiting at an operation it cannot apply yet: for a handler, for data a gap
\* names, or to retry after a step fails outside the operation's apply. The
\* driver retries such a step after 250 ms, doubling the wait up to 30 s, with
\* no end: a step that keeps failing keeps replay in that wait.
\* The driver moves the replay state, and a writer checks it and admits,
\* under the one lock, the adapter's bcast, so no move falls between a
\* writer's check and its admission.
\*
\* A write has two outcomes: admitted while caught up, or refused with
\* REPLAY_WAITS while replay waits. A write that meets the opening waits in
\* the gate, on the broadcast the state's moves fire, bounded by its caller's
\* context: it is admitted once replay is caught up, or refused once replay
\* enters a named wait. A refused or cancelled writer may try again; the
\* gate itself queues nothing.
\*
\* Ops: a writer's admitted op is its own; Peers are another peer's ops. The
\* peer op plugin needs a handler, which registers when HandlerArrives and
\* which replay sees only when lookup is enabled; flaky's first step fails
\* outside its apply, such as on a store read; gap waits until data it names
\* arrives; bad's apply fails with an error of unknown kind, which refuses it
\* as any failed apply is refused. The seed is a plugin op submitted by the
\* plugin that handles it, so its writer begins only once its handler
\* resolves. The nested writer begins once the seed's op is admitted, as a
\* seed op's own plugin writes on a one-device Space. newer is an op a newer
\* build wrote, whose body this build cannot read: replay waits at it for a
\* build that reads it, which only a restart brings, and only when Upgrades.
\*
\* Restart ends the process once: writers in flight are cancelled, the
\* handler must register again, and replay opens again over every op the log
\* holds, on a build that reads newer when Upgrades. Until the seed's plugin
\* loads, replay waits at the seed's own op.
\*
\* With OwnerSkips, an owner may end a named wait by skipping the op replay
\* waits at: a config entry, signed by the owner, that names the op by its
\* hash. Every member's replay then refuses that op as skipped. The config
\* change is a new state, so a waiting driver steps at once. A restart
\* keeps the skip, since the config holds it.
\*
\* Replay states while it holds an op it has not applied, the arrival
\* window, are not this spec's: a write admitted then is re-applied on the
\* new head and decided there.
EXTENDS Naturals, Sequences, FiniteSets
CONSTANTS Peers, HandlerArrives, LookupDisabled, OwnerSkips, Upgrades

Writers == {"seed", "nested"}
Ops == Writers \cup Peers
Holders == {"free"} \cup Writers
WStates == {"idle", "begun", "waiting", "checked", "admitted", "refused", "cancelled"}
\* Waits are the named waits, where a write is refused at once.
Waits == {"waitHandler", "waitGap", "waitRetry", "waitUpgrade"}

\* log is the order of every op, as this device holds it; upgraded is whether
\* the running build reads newer.
VARIABLES replay, todo, arrived, registered, failed, filled, holder, wstate,
          applied, refused, badAdmit, badRefuse, restarted, skipped, log, upgraded
vars == <<replay, todo, arrived, registered, failed, filled, holder, wstate,
          applied, refused, badAdmit, badRefuse, restarted, skipped, log, upgraded>>

\* Writable is whether a caller may try a write again: replay is caught up.
Writable == replay = "caughtUp"

Init == /\ replay = "opening" /\ todo = <<>> /\ arrived = {}
        /\ registered = FALSE /\ failed = FALSE /\ filled = FALSE
        /\ holder = "free"
        /\ wstate = [w \in Writers |-> "idle"]
        /\ applied = {} /\ refused = {} /\ badAdmit = FALSE /\ badRefuse = FALSE
        /\ restarted = FALSE /\ skipped = {} /\ log = <<>> /\ upgraded = FALSE

\* A peer's op reaches this device and joins the order to apply.
Arrive(p) == /\ p \in Peers /\ p \notin arrived
             /\ arrived' = arrived \cup {p} /\ todo' = Append(todo, p)
             /\ log' = Append(log, p)
             /\ UNCHANGED <<replay, registered, failed, filled, holder, wstate,
                            applied, refused, badAdmit, badRefuse, restarted, skipped, upgraded>>

\* The plugin that handles the plugin and seed ops registers on the bus.
Register == /\ HandlerArrives /\ ~registered /\ registered' = TRUE
            /\ UNCHANGED <<replay, todo, arrived, failed, filled, holder, wstate,
                           applied, refused, badAdmit, badRefuse, restarted, skipped, log, upgraded>>

\* Seen is whether the wait's watch resolves the handler.
Seen == registered /\ ~LookupDisabled

\* The data the gap op names arrives.
Fill == /\ "gap" \in arrived /\ ~filled /\ filled' = TRUE
        /\ UNCHANGED <<replay, todo, arrived, registered, failed, holder, wstate,
                       applied, refused, badAdmit, badRefuse, restarted, skipped, log, upgraded>>

\* Step is one driver step under the lock. It applies the next op, refuses
\* one whose apply fails with an error of unknown kind, or stops at it and
\* waits; with nothing left it is caught up. Opening and caught up step at
\* once; a handler wait steps when the watch sees the handler, a gap when its
\* data arrives, a failed step after its backoff. An upgrade wait steps on a
\* new state alone, which finds it the same until the owner skips its op; a
\* restart on a build that reads it opens replay again.
StepReady == \/ replay \in {"opening", "caughtUp"}
             \/ replay = "waitHandler" /\ Seen
             \/ replay = "waitGap" /\ filled
             \/ replay = "waitRetry"
             \/ replay \in Waits /\ Head(todo) \in skipped
\* Next1 is the state after the head op leaves the order: caught up when
\* none is left, else opening again after a wait.
Next1 == IF Tail(todo) = <<>> THEN "caughtUp"
         ELSE IF replay = "caughtUp" THEN "caughtUp" ELSE "opening"
Step == /\ holder = "free" /\ StepReady
        /\ IF todo = <<>>
           THEN /\ replay' = "caughtUp"
                /\ UNCHANGED <<todo, failed, applied, refused>>
           ELSE LET op == Head(todo) IN
                IF op \in skipped
                THEN /\ refused' = refused \cup {op} /\ todo' = Tail(todo)
                     /\ replay' = Next1
                     /\ UNCHANGED <<failed, applied>>
                ELSE
                CASE op \in {"plugin", "seed"} /\ ~Seen ->
                       /\ replay' = "waitHandler"
                       /\ UNCHANGED <<todo, failed, applied, refused>>
                  [] op = "gap" /\ ~filled ->
                       /\ replay' = "waitGap"
                       /\ UNCHANGED <<todo, failed, applied, refused>>
                  [] op = "newer" /\ ~upgraded ->
                       /\ replay' = "waitUpgrade"
                       /\ UNCHANGED <<todo, failed, applied, refused>>
                  [] op = "flaky" /\ ~failed ->
                       /\ replay' = "waitRetry" /\ failed' = TRUE
                       /\ UNCHANGED <<todo, applied, refused>>
                  [] op = "bad" ->
                       /\ refused' = refused \cup {op} /\ todo' = Tail(todo)
                       /\ replay' = Next1
                       /\ UNCHANGED <<failed, applied>>
                  [] OTHER ->
                       /\ applied' = applied \cup {op} /\ todo' = Tail(todo)
                       /\ replay' = Next1
                       /\ UNCHANGED <<failed, refused>>
        /\ UNCHANGED <<arrived, registered, filled, holder, wstate, badAdmit,
                       badRefuse, restarted, skipped, log, upgraded>>

\* Begin reads the gate without the lock: a named wait refuses the write at
\* once. The seed begins once its handler resolves, and the nested write
\* while the seed's op is admitted.
Begin(w) == /\ wstate[w] = "idle"
            /\ w = "seed" => Seen
            /\ w = "nested" => wstate["seed"] = "admitted"
            /\ wstate' = [wstate EXCEPT ![w] = IF replay \in Waits THEN "refused" ELSE "begun"]
            /\ UNCHANGED <<replay, todo, arrived, registered, failed, filled, holder,
                           applied, refused, badAdmit, badRefuse, restarted, skipped, log, upgraded>>

\* Check takes the lock and reads the gate again: caught up holds the lock to
\* admit, opening releases it and waits on the broadcast, a named wait
\* refuses.
Check(w) == /\ wstate[w] = "begun" /\ holder = "free"
            /\ CASE replay = "caughtUp" ->
                      /\ holder' = w /\ wstate' = [wstate EXCEPT ![w] = "checked"]
                 [] replay = "opening" ->
                      /\ UNCHANGED holder /\ wstate' = [wstate EXCEPT ![w] = "waiting"]
                 [] OTHER ->
                      /\ UNCHANGED holder /\ wstate' = [wstate EXCEPT ![w] = "refused"]
            /\ badRefuse' = (badRefuse \/ (wstate'[w] = "refused" /\ replay \notin Waits))
            /\ UNCHANGED <<replay, todo, arrived, registered, failed, filled,
                           applied, refused, badAdmit, restarted, skipped, log, upgraded>>

\* Wake runs once the broadcast fires on a move out of opening: the writer
\* takes the lock and admits while caught up, or is refused in a named wait.
Wake(w) == /\ wstate[w] = "waiting" /\ replay # "opening" /\ holder = "free"
           /\ IF replay = "caughtUp"
              THEN /\ holder' = w /\ wstate' = [wstate EXCEPT ![w] = "checked"]
              ELSE /\ UNCHANGED holder /\ wstate' = [wstate EXCEPT ![w] = "refused"]
           /\ badRefuse' = (badRefuse \/ (wstate'[w] = "refused" /\ replay \notin Waits))
           /\ UNCHANGED <<replay, todo, arrived, registered, failed, filled,
                          applied, refused, badAdmit, restarted, skipped, log, upgraded>>

\* The caller's context ends a write waiting in the opening.
Cancel(w) == /\ wstate[w] = "waiting"
             /\ wstate' = [wstate EXCEPT ![w] = "cancelled"]
             /\ UNCHANGED <<replay, todo, arrived, registered, failed, filled, holder,
                            applied, refused, badAdmit, badRefuse, restarted, skipped, log, upgraded>>

\* Admit queues the write's op in the same hold of the lock and releases it.
Admit(w) == /\ wstate[w] = "checked" /\ holder = w
            /\ wstate' = [wstate EXCEPT ![w] = "admitted"]
            /\ todo' = Append(todo, w) /\ log' = Append(log, w)
            /\ badAdmit' = (badAdmit \/ replay # "caughtUp")
            /\ holder' = "free"
            /\ UNCHANGED <<replay, arrived, registered, failed, filled, applied,
                           refused, badRefuse, restarted, skipped, upgraded>>

\* A refused or cancelled caller tries again once replay is caught up.
Retry(w) == /\ wstate[w] \in {"refused", "cancelled"} /\ Writable
            /\ wstate' = [wstate EXCEPT ![w] = "idle"]
            /\ UNCHANGED <<replay, todo, arrived, registered, failed, filled, holder,
                           applied, refused, badAdmit, badRefuse, restarted, skipped, log, upgraded>>

\* Restart ends the process once, outside any hold of the lock: writes in
\* flight are cancelled, the handler registers again, and replay opens again
\* over every op the log holds.
Restart == /\ ~restarted /\ holder = "free"
           /\ restarted' = TRUE /\ replay' = "opening" /\ registered' = FALSE
           /\ upgraded' = (upgraded \/ Upgrades)
           /\ todo' = SelectSeq(log, LAMBDA o : o \in applied \cup refused) \o todo
           /\ applied' = {} /\ refused' = {}
           /\ wstate' = [w \in Writers |->
                           IF wstate[w] \in {"begun", "waiting"} THEN "cancelled" ELSE wstate[w]]
           /\ UNCHANGED <<arrived, failed, filled, holder, badAdmit, badRefuse, skipped, log>>

\* Skip is the owner's config change that skips the op replay waits at, by
\* its hash. Only a named wait offers it: the owner sees why replay stopped.
Skip == /\ OwnerSkips /\ replay \in Waits /\ Head(todo) \notin skipped
        /\ skipped' = skipped \cup {Head(todo)}
        /\ UNCHANGED <<replay, todo, arrived, registered, failed, filled, holder, wstate,
                       applied, refused, badAdmit, badRefuse, restarted, log, upgraded>>

Next == \/ \E p \in Peers : Arrive(p)
        \/ Register \/ Fill \/ Step \/ Restart \/ Skip
        \/ \E w \in Writers : Begin(w) \/ Check(w) \/ Wake(w) \/ Cancel(w)
                              \/ Admit(w) \/ Retry(w)

Spec == Init /\ [][Next]_vars
FairSpec ==
    /\ Spec
    /\ WF_vars(Step) /\ WF_vars(Register) /\ WF_vars(Fill) /\ WF_vars(Skip)
    /\ \A p \in Peers : WF_vars(Arrive(p))
    /\ \A w \in Writers : WF_vars(Begin(w))
    /\ \A w \in Writers : WF_vars(Check(w))
    /\ \A w \in Writers : WF_vars(Wake(w))
    /\ \A w \in Writers : WF_vars(Admit(w))
    /\ \A w \in Writers : WF_vars(Retry(w))

TypeOK == /\ replay \in {"opening", "caughtUp"} \cup Waits
          /\ holder \in Holders /\ wstate \in [Writers -> WStates]
          /\ arrived \subseteq Peers /\ applied \subseteq Ops /\ refused \subseteq Ops
          /\ restarted \in BOOLEAN /\ skipped \subseteq Ops
          /\ log \in Seq(Ops) /\ upgraded \in BOOLEAN

\* No write is admitted unless replay is caught up.
NoWriteWhileWaiting == ~badAdmit

\* A write is refused only while replay is in a named wait: one waiting in the
\* opening leaves it admitted after caught up, refused on a named wait, or
\* cancelled by its caller.
RefusedOnlyInWait == ~badRefuse

\* An op leaves the order only applied, or refused when its apply fails with
\* an error of unknown kind or the owner skipped it: a step that fails
\* outside the apply waits and keeps it, and a restart re-runs it.
OpsKept == /\ refused \subseteq {"bad"} \cup skipped
           /\ \A op \in arrived \cup {w \in Writers : wstate[w] = "admitted"} :
                op \in applied \cup refused \/ \E i \in 1..Len(todo) : todo[i] = op

\* Replay waits to retry only at a step that failed outside an apply: an
\* apply's own error of unknown kind refuses its op instead, as on a build
\* before the wait. In the apply only a gap waits: a missing handler, an
\* unavailable plugin, a missing plugin build, an unavailable or released
\* block, or a cancelled or expired context.
RetryOnlyOutsideApply == replay = "waitRetry" => Head(todo) = "flaky"

\* On a one-device Space, replay never waits once it opened, except at its
\* own seed op after a restart, until the seed's plugin registers again.
OneDeviceNeverWaits ==
    Peers = {} => \/ replay \in {"opening", "caughtUp"}
                  \/ restarted /\ replay = "waitHandler" /\ Head(todo) = "seed"

\* Within a run, replay never waits for a handler at an op of its own
\* device; after a restart it may wait at the seed's op until its plugin
\* registers. It waits at a gap only for the gap's data, never as a missing
\* handler, and for a build only at the op it cannot read.
OwnOpNeverWaits == /\ replay = "waitHandler" =>
                        \/ Head(todo) = "plugin"
                        \/ restarted /\ Head(todo) = "seed"
                   /\ replay = "waitGap" => Head(todo) = "gap"
                   /\ replay = "waitUpgrade" => Head(todo) = "newer"

\* The opening always ends, in caught up or a named wait.
OpeningExits == replay = "opening" ~> replay # "opening"

\* A write waiting in the opening always leaves the wait.
WaitingWriteDecided == \A w \in Writers : wstate[w] = "waiting" ~> wstate[w] # "waiting"

\* Each named wait ends once its cause can resolve: a handler wait when a
\* handler registers that lookup sees, a gap when its data arrives, a retry
\* after its backoff, an upgrade once a restart brings a build that reads it.
EveryWaitExits ==
    /\ (HandlerArrives /\ ~LookupDisabled) => (replay = "waitHandler" ~> replay # "waitHandler")
    /\ replay = "waitGap" ~> replay # "waitGap"
    /\ replay = "waitRetry" ~> replay # "waitRetry"
    /\ Upgrades => (replay = "waitUpgrade" ~> replay # "waitUpgrade")

\* A one-device Space always ends caught up, a restart included.
OneDeviceCatchesUp == (Peers = {} /\ HandlerArrives /\ ~LookupDisabled) => <>[](replay = "caughtUp")

\* Replay ends a handler wait once the handler registers and lookup sees it.
HandlerEndsWait == (HandlerArrives /\ ~LookupDisabled) => <>[](replay = "caughtUp")

\* A write is admitted when replay does not wait for good, the nested write
\* of the seed's own plugin op included.
WritesAdmitted == (HandlerArrives /\ ~LookupDisabled) =>
                    \A w \in Writers :
                      <>[](replay = "caughtUp") => <>(wstate[w] = "admitted")

\* With no handler, or lookup disabled, and no owner to skip, a plugin op
\* ends replay in the handler wait for good: the Space stays read only.
NoHandlerStaysReadOnly == ("plugin" \in Peers /\ ~(HandlerArrives /\ ~LookupDisabled) /\ ~OwnerSkips)
                            => <>[](replay = "waitHandler")

\* A skipped op is never applied, a restart included: the config keeps the
\* skip, so a re-run refuses it again even once a handler registers.
SkippedNeverApplied == skipped \cap applied = {}

\* An owner's skip ends every stall: replay catches up even when no handler
\* ever registers.
StallExits == OwnerSkips => <>[](replay = "caughtUp")

\* Replay never passes an op it cannot read: until a build reads it or the
\* owner skips it, no op ordered after newer is applied or refused, so every
\* later op is decided on the World after newer.
NeverPastUnreadable ==
    \A i \in 1..Len(log) :
      (log[i] = "newer" /\ ~upgraded /\ "newer" \notin skipped) =>
        \A j \in (i + 1)..Len(log) : log[j] \notin applied \cup refused

\* With no build that reads newer and no owner to skip it, replay ends in the
\* upgrade wait for good: the Space stays read only.
NoUpgradeStaysReadOnly == ("newer" \in Peers /\ ~Upgrades /\ ~OwnerSkips)
                            => <>[](replay = "waitUpgrade")

\* UpgradeSpec is FairSpec with a restart that must come, as a person
\* installing a newer build restarts the process.
UpgradeSpec == FairSpec /\ WF_vars(Restart)
====
