------------------------- MODULE P2PSyncLifecycle -------------------------
\* The lifecycle of P2P sync generations on one ProviderAccount. A generation
\* moves Starting -> Running -> Stopping -> Stopped, and only its own run
\* goroutine releases its resources. A caller joins a Starting generation or
\* creates a replacement that holds the current one as its lower source, read
\* through while the replacement starts. A replacement that runs releases that
\* hold and retires its predecessor. A failed one that is still current
\* restores a live retained predecessor, and otherwise retires it. Callers leave; the last
\* owner, counting the successor's hold, stops a generation.
\* Three generations and one extra joiner bound the model. The SO list, the
\* controllers and the block reads are outside it; the release waits other than
\* the retire wait are atomic here.
EXTENDS Naturals, FiniteSets
CONSTANT N
VARIABLES cur, nextGen, joins, phase, callers, lower, prev, runner, released,
          dropped, failed
vars == <<cur, nextGen, joins, phase, callers, lower, prev, runner, released,
          dropped, failed>>
Gens == 1..N
Live == {"starting", "running"}

Holds(g, lw) == Cardinality({h \in Gens : lw[h] = g})
Owners(g, cl, lw) == cl[g] + Holds(g, lw)
Stop(ph, g) == IF ph[g] \in Live THEN [ph EXCEPT ![g] = "stopping"] ELSE ph
Detach(c, g) == IF c = g THEN 0 ELSE c

Init == /\ cur = 0 /\ nextGen = 1 /\ joins = 0
        /\ phase = [g \in Gens |-> "none"] /\ callers = [g \in Gens |-> 0]
        /\ lower = [g \in Gens |-> 0] /\ prev = [g \in Gens |-> 0]
        /\ runner = [g \in Gens |-> "none"] /\ released = [g \in Gens |-> 0]
        /\ dropped = FALSE /\ failed = {}

\* startP2PSync on a Starting current generation takes one more ownership.
Join == /\ cur # 0 /\ phase[cur] = "starting" /\ joins < 1
        /\ callers' = [callers EXCEPT ![cur] = @ + 1] /\ joins' = joins + 1
        /\ UNCHANGED <<cur, nextGen, phase, lower, prev, runner, released, dropped, failed>>

\* startP2PSync on another transport, or with no live current generation,
\* creates a replacement that holds the current one as lower.
Create == /\ nextGen <= N
          /\ LET n == nextGen IN
             /\ phase' = [phase EXCEPT ![n] = "starting"]
             /\ callers' = [callers EXCEPT ![n] = 1]
             /\ lower' = [lower EXCEPT ![n] = IF cur # 0 /\ phase[cur] \in Live THEN cur ELSE 0]
             /\ prev' = [prev EXCEPT ![n] = cur]
             /\ runner' = [runner EXCEPT ![n] = "start"]
             /\ cur' = n /\ nextGen' = n + 1
          /\ UNCHANGED <<joins, released, dropped, failed>>

\* A caller's context ends; the last owner stops and detaches the generation.
Leave(g) == /\ callers[g] > 0
            /\ callers' = [callers EXCEPT ![g] = @ - 1]
            /\ IF Owners(g, callers', lower) = 0 /\ phase[g] \in Live
                 THEN phase' = Stop(phase, g) /\ cur' = Detach(cur, g)
                 ELSE UNCHANGED <<phase, cur>>
            /\ UNCHANGED <<nextGen, joins, lower, prev, runner, released, dropped, failed>>

\* StopP2PSync detaches and stops the current generation.
AccountStop == /\ cur # 0 /\ phase' = Stop(phase, cur) /\ cur' = 0
               /\ UNCHANGED <<nextGen, joins, callers, lower, prev, runner, released, dropped, failed>>

\* finishStart(nil): enter Running and take lowerSource in one step.
FinishOK(g) == /\ runner[g] = "start" /\ phase[g] = "starting"
               /\ LET L == lower[g]
                      lw == [lower EXCEPT ![g] = 0]
                      orphan == L # 0 /\ Owners(L, callers, lw) = 0
                      ph == [phase EXCEPT ![g] = "running"]
                  IN /\ lower' = lw
                     /\ phase' = IF orphan THEN Stop(ph, L) ELSE ph
                     /\ cur' = IF orphan THEN Detach(cur, L) ELSE cur
               /\ runner' = [runner EXCEPT ![g] = "retire"]
               /\ UNCHANGED <<nextGen, joins, callers, prev, released, dropped, failed>>

\* Startup failed or was canceled. While the generation is still current, it
\* restores a retained predecessor that has not begun stopping and otherwise
\* retires the predecessor; then it cleans up.
FailPath(g) == LET P == prev[g]
                   restore == cur = g /\ P # 0 /\ lower[g] = P /\ phase[P] \in Live
                   retireP == P # 0 /\ cur = g /\ ~restore
                   ph == Stop(phase, g)
                   c == IF restore THEN P ELSE Detach(cur, g)
               IN /\ cur' = IF retireP THEN Detach(c, P) ELSE c
                  /\ phase' = IF retireP THEN Stop(ph, P) ELSE ph
                  /\ dropped' = (dropped \/ (restore /\ (cur' # P \/ phase'[P] \notin Live)))
                  /\ runner' = [runner EXCEPT ![g] = "clean"]
                  /\ UNCHANGED <<nextGen, joins, callers, lower, prev, released>>
Fail(g) == /\ runner[g] = "start" /\ phase[g] = "starting" /\ FailPath(g)
           /\ failed' = failed \cup {g}
Canceled(g) == /\ runner[g] = "start" /\ phase[g] = "stopping" /\ FailPath(g)
               /\ UNCHANGED failed

\* retireP2PSyncState(previous) after a successful start: detach, stop, wait.
Retire(g) == /\ runner[g] = "retire"
             /\ phase' = IF prev[g] # 0 THEN Stop(phase, prev[g]) ELSE phase
             /\ cur' = Detach(cur, prev[g])
             /\ runner' = [runner EXCEPT ![g] = "waitprev"]
             /\ UNCHANGED <<nextGen, joins, callers, lower, prev, released, dropped, failed>>
WaitPrev(g) == /\ runner[g] = "waitprev" /\ IF prev[g] = 0 THEN TRUE ELSE phase[prev[g]] = "stopped"
               /\ runner' = [runner EXCEPT ![g] = "follow"]
               /\ UNCHANGED <<cur, nextGen, joins, phase, callers, lower, prev, released, dropped, failed>>

\* The follow loop ends when the generation's context ends.
FollowEnds(g) == /\ runner[g] = "follow" /\ phase[g] = "stopping"
                 /\ runner' = [runner EXCEPT ![g] = "clean"]
                 /\ UNCHANGED <<cur, nextGen, joins, phase, callers, lower, prev, released, dropped, failed>>

\* finishP2PSyncState: release resources once, release the lower hold, Stopped.
Cleanup(g) == /\ runner[g] = "clean" /\ phase[g] = "stopping"
              /\ LET L == lower[g]
                     lw == [lower EXCEPT ![g] = 0]
                     orphan == L # 0 /\ Owners(L, callers, lw) = 0
                     ph == [phase EXCEPT ![g] = "stopped"]
                 IN /\ lower' = lw
                    /\ phase' = IF orphan THEN Stop(ph, L) ELSE ph
                    /\ cur' = IF orphan THEN Detach(cur, L) ELSE cur
              /\ released' = [released EXCEPT ![g] = @ + 1]
              /\ runner' = [runner EXCEPT ![g] = "done"]
              /\ UNCHANGED <<nextGen, joins, callers, prev, dropped, failed>>

Done == /\ \A g \in Gens : runner[g] \in {"none", "done"} /\ nextGen > N
        /\ UNCHANGED vars
Next == Join \/ Create \/ AccountStop \/ Done
        \/ \E g \in Gens : Leave(g) \/ FinishOK(g) \/ Fail(g) \/ Canceled(g)
                           \/ Retire(g) \/ WaitPrev(g) \/ FollowEnds(g) \/ Cleanup(g)
Spec == Init /\ [][Next]_vars
        /\ \A g \in Gens : WF_vars(FinishOK(g)) /\ WF_vars(Canceled(g)) /\ WF_vars(Retire(g))
                           /\ WF_vars(WaitPrev(g)) /\ WF_vars(FollowEnds(g)) /\ WF_vars(Cleanup(g))

ReleasedOnce == \A g \in Gens : released[g] <= 1
\* A Starting generation's reads fall back to its lower source, so that source
\* stays live until the hold is released, unless its own startup failed.
ReadsServed == \A g \in Gens : phase[g] = "starting" /\ lower[g] # 0
                                => phase[lower[g]] \in Live \/ lower[g] \in failed
RunningHasNoLower == \A g \in Gens : phase[g] = "running" => lower[g] = 0
CurrentLive == cur # 0 => phase[cur] \in Live
LiveHasOwner == \A g \in Gens : phase[g] \in Live => Owners(g, callers, lower) > 0
\* A failed replacement that restores its predecessor leaves it current and
\* live (dropped records the step that does not).
KeepsPredecessor == ~dropped
AwaitStartEnds == \A g \in Gens : phase[g] = "starting" ~> phase[g] # "starting"
StopCompletes == \A g \in Gens : phase[g] = "stopping" ~> phase[g] = "stopped"
=============================================================================
