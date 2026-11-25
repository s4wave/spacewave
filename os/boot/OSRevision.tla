----------------------------- MODULE OSRevision -----------------------------
EXTENDS Naturals, TLC
\* Moving an installed Device between system revisions. The image store holds
\* four durable named roots: accepted, the revision the Device falls back to;
\* trial, a staged revision to boot once; attempted, the trial a boot has
\* already tried; and rolled-back, the last trial that failed. Each revision
\* root is published with its own authorization, and each move between
\* revisions is one atomic publish of the named roots.
\* The console, the store's one writer, stages the revision the Device's
\* os/system node serves in a child beside its own read-only reader: the child
\* copies the revision's blocks, publishes it as the trial, and the console
\* reboots. It never restages a revision that rolled
\* back; that waits for the person to place another. A boot marks the trial
\* attempted before serving it, so a trial that panics, hangs past the
\* console's deadline or loses power boots back to the accepted revision. A
\* trial is accepted once its daemon listens, which needs no network.
\* Collection keeps every block the accepted and trial roots reach.
\* A power cut may end any boot; only the named roots and stored blocks are
\* durable.
\* Publishing a trial frees rolled-back, so the roots alone tell what the
\* last update did, as the Device's update state reports it: a trial pending,
\* or a trial that rolled back, or neither, once the last trial was accepted.
\* outcome records that last result for the check; it is not stored.
\* Bounds: four revisions (A installed; B good; C fails its health; D signed by
\* a key the Device did not pin), each placed at most once, MaxCuts cuts.
CONSTANT MaxCuts
VARIABLES phase, serving, roots, present, placed, everPlaced, trialServes,
          cuts, outcome
vars == <<phase, serving, roots, present, placed, everPlaced, trialServes,
          cuts, outcome>>

Revs == {"A", "B", "C", "D"}
None == "none"
\* Good revisions reach health; signed ones carry a pinned signer's
\* authorization.
Good == {"A", "B"}
Signed == {"A", "B", "C"}

\* roots is the store's durable named roots; one assignment is one publish.
Accepted == roots.accepted
Trial == roots.trial

Init == /\ phase = "off" /\ serving = None
        /\ roots = [accepted |-> "A", trial |-> None, attempted |-> None,
                    rolledBack |-> None]
        /\ present = {"A"} /\ placed = None /\ everPlaced = {}
        /\ trialServes = [r \in Revs |-> 0] /\ cuts = 0
        /\ outcome = None

Boot == /\ phase = "off" /\ phase' = "selecting"
        /\ UNCHANGED <<serving, roots, present, placed, everPlaced,
                       trialServes, cuts, outcome>>

\* A trial the pinned signers authorized, not yet tried, is marked attempted
\* durably before anything serves it.
MarkAttempted == /\ phase = "selecting" /\ Trial # None
                 /\ roots.attempted # Trial /\ Trial \in Signed
                 /\ roots' = [roots EXCEPT !.attempted = Trial]
                 /\ phase' = "trying"
                 /\ UNCHANGED <<serving, present, placed, everPlaced,
                                trialServes, cuts, outcome>>

ServeTrial == /\ phase = "trying"
              /\ serving' = Trial /\ phase' = "serving"
              /\ trialServes' = [trialServes EXCEPT ![Trial] = @ + 1]
              /\ UNCHANGED <<roots, present, placed, everPlaced, cuts, outcome>>

\* A trial already tried, or one no pinned signer authorized, is dropped and
\* recorded as rolled back in one publish; the boot then serves the accepted
\* revision.
Rollback == /\ phase = "selecting" /\ Trial # None
            /\ (roots.attempted = Trial \/ Trial \notin Signed)
            /\ roots' = [roots EXCEPT !.trial = None, !.attempted = None,
                                      !.rolledBack = Trial]
            /\ outcome' = "rolledBack"
            /\ UNCHANGED <<phase, serving, present, placed, everPlaced,
                           trialServes, cuts>>

ServeAccepted == /\ phase = "selecting" /\ Trial = None
                 /\ Accepted \in Signed
                 /\ serving' = Accepted /\ phase' = "serving"
                 /\ UNCHANGED <<roots, present, placed, everPlaced,
                                trialServes, cuts, outcome>>

\* The daemon listens once its revision works.
Healthy == /\ phase = "serving" /\ serving \in Good
           /\ phase' = "healthy"
           /\ UNCHANGED <<serving, roots, present, placed, everPlaced,
                          trialServes, cuts, outcome>>

\* The console's deadline reboots a trial that never got healthy.
Deadline == /\ phase = "serving" /\ serving = Trial
            /\ phase' = "off" /\ serving' = None
            /\ UNCHANGED <<roots, present, placed, everPlaced, trialServes,
                           cuts, outcome>>

Accept == /\ phase = "healthy" /\ Trial # None /\ serving = Trial
          /\ roots' = [roots EXCEPT !.accepted = Trial, !.trial = None,
                                    !.attempted = None]
          /\ outcome' = "accepted"
          /\ UNCHANGED <<phase, serving, present, placed, everPlaced,
                         trialServes, cuts>>

\* The person places a new revision on the Device's os/system node.
Place(r) == /\ r \notin everPlaced /\ r # "A"
            /\ placed' = r /\ everPlaced' = everPlaced \cup {r}
            /\ UNCHANGED <<phase, serving, roots, present, trialServes, cuts, outcome>>

\* Staging runs on the accepted revision while no trial is pending, for a
\* placed revision that is neither running nor rolled back.
CanStage == /\ phase = "healthy" /\ serving = Accepted /\ Trial = None
            /\ placed \notin {None, Accepted, roots.rolledBack}

\* Copy the placed revision's blocks; a repeat copies nothing new.
Copy == /\ CanStage /\ present' = present \cup {placed}
        /\ UNCHANGED <<phase, serving, roots, placed, everPlaced, trialServes,
                       cuts, outcome>>

\* Publish the placed revision as the trial once its blocks are all present,
\* freeing the record of the last trial that rolled back.
Publish == /\ CanStage /\ placed \in present
           /\ roots' = [roots EXCEPT !.trial = placed, !.attempted = None,
                                     !.rolledBack = None]
           /\ outcome' = None
           /\ UNCHANGED <<phase, serving, present, placed, everPlaced,
                          trialServes, cuts>>

RequestReboot == /\ phase = "healthy" /\ serving = Accepted /\ Trial # None
                 /\ phase' = "off" /\ serving' = None
                 /\ UNCHANGED <<roots, present, placed, everPlaced,
                                trialServes, cuts, outcome>>

\* Collection releases every block neither revision root reaches.
Collect == /\ phase \in {"serving", "healthy"}
           /\ present' = present \cap ({Accepted, Trial} \ {None})
           /\ UNCHANGED <<phase, serving, roots, placed, everPlaced,
                          trialServes, cuts, outcome>>

PowerCut == /\ phase # "off" /\ cuts < MaxCuts
            /\ phase' = "off" /\ serving' = None /\ cuts' = cuts + 1
            /\ UNCHANGED <<roots, present, placed, everPlaced, trialServes, outcome>>

Next == \/ Boot \/ MarkAttempted \/ ServeTrial \/ Rollback \/ ServeAccepted
        \/ Healthy \/ Deadline \/ Accept \/ Copy \/ Publish \/ RequestReboot
        \/ Collect \/ PowerCut
        \/ \E r \in Revs : Place(r)

Fairness == /\ WF_vars(Boot) /\ WF_vars(MarkAttempted) /\ WF_vars(ServeTrial)
            /\ WF_vars(Rollback) /\ WF_vars(ServeAccepted) /\ WF_vars(Healthy)
            /\ WF_vars(Deadline) /\ WF_vars(Accept) /\ WF_vars(RequestReboot)

Spec == Init /\ [][Next]_vars /\ Fairness

TypeOK == /\ phase \in {"off", "selecting", "trying", "serving", "healthy"}
          /\ serving \in Revs \cup {None}
          /\ Accepted \in Revs
          /\ {Trial, roots.attempted, roots.rolledBack} \subseteq Revs \cup {None}
          /\ present \subseteq Revs /\ placed \in Revs \cup {None}
          /\ outcome \in {None, "accepted", "rolledBack"}

\* Every selected revision boots from blocks the store holds.
SelectedPresent == /\ Accepted \in present
                   /\ Trial # None => Trial \in present

\* Only an authorized revision is ever served.
ServesSigned == serving # None => serving \in Signed

\* The fallback is always a revision that reached health.
AcceptedGood == Accepted \in Good

\* A trial boots at most once, so a bad trial cannot loop.
TrialOnce == \A r \in Revs : trialServes[r] <= 1

\* With no trial pending, rolled-back is set exactly when the last trial
\* rolled back, so the Device's update state reads from the roots alone.
RootsTellOutcome == Trial = None => (roots.rolledBack # None <=> outcome = "rolledBack")

\* A pending trial always settles back to a healthy Device with none pending.
TrialSettles == (Trial # None) ~> (Trial = None /\ phase = "healthy")
=============================================================================
