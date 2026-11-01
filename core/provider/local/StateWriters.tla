---- MODULE StateWriters ----
\* World, direct, control and checkpoint writers share intent and SQL capacity.
\* Cooperative callers discard SQLite before waiting and rerun with capacity.
\* Controls try settled intent under the engine, and drain outside it without
\* retaining intent over a later engine wait. Checkpoints return either claim
\* before waiting for the other. Thus retry callbacks can still edit metadata.
\* Progress assumes receipts complete and successful claim pairs are scheduled.
EXTENDS Naturals, FiniteSets
CONSTANTS IntentBeforeEngine, DrainBeforeProvider, SettleBeforeEngine,
          IntentBeforeCapacity, WorldNeedsAlone, TryControls, ReleaseCheckpoint, Cooperative
W == 1
D == 2
C == 3
K == 4
Actors == 1..4
VARIABLE s
Init == s = [pc |-> [a \in Actors |-> IF a = W THEN "callback" ELSE IF a = C THEN "engine" ELSE "intent"],
             intent |-> 0, engine |-> 0, provider |-> 0, pending |-> 0,
             claims |-> {}, large |-> FALSE, sql |-> FALSE]
Available(a) == IF a = W /\ WorldNeedsAlone THEN s.pending = 0 /\ s.claims = {}
                ELSE s.pending + Cardinality(s.claims) < 2 /\ ~s.large /\ ~(WorldNeedsAlone /\ W \in s.claims)
Callback == /\ s.pc[W] = "callback" /\ s.engine = 0
            /\ s' = [s EXCEPT !.engine = W, !.sql = Cooperative, !.pc[W] = "body"]
Body == /\ s.pc[W] = "body" /\ s' = [s EXCEPT !.pc[W] = "try-state"]
TryWorld == /\ s.pc[W] = "try-state"
            /\ s' = [s EXCEPT !.intent = IF s.intent = 0 THEN W ELSE @,
                !.engine = IF s.intent = 0 \/ ~IntentBeforeEngine THEN W ELSE 0,
                !.sql = IF s.intent = 0 THEN @ ELSE FALSE,
                !.claims = IF s.intent = 0 THEN @ ELSE @ \ {W},
                !.pc[W] = IF s.intent = 0 THEN IF W \in s.claims THEN "state" ELSE "budget"
                          ELSE IF IntentBeforeEngine THEN "intent-wait" ELSE "intent-engine"]
WorldIntent == /\ s.pc[W] \in {"intent-wait", "intent-engine"} /\ s.intent = 0
               /\ s' = [s EXCEPT !.intent = W, !.pc[W] = "ready"]
WorldReady == /\ s.pc[W] = "ready"
              /\ s' = [s EXCEPT !.intent = IF Cooperative THEN 0 ELSE W,
                 !.pc[W] = IF Cooperative THEN "retry-capacity" ELSE IF s.engine = W THEN "budget" ELSE "relock"]
RelockWorld == /\ s.pc[W] = "relock" /\ s.engine = 0
               /\ s' = [s EXCEPT !.engine = W, !.pc[W] = IF W \in s.claims THEN "state" ELSE "budget"]
WorldBudget == /\ s.pc[W] = "budget"
               /\ s' = [s EXCEPT !.claims = IF Available(W) THEN @ \cup {W} ELSE @,
                 !.engine = IF Available(W) THEN @ ELSE 0, !.sql = IF Available(W) THEN @ ELSE FALSE,
                 !.intent = IF ~Available(W) /\ Cooperative THEN 0 ELSE @,
                 !.pc[W] = IF Available(W) THEN "state" ELSE IF Cooperative THEN "retry-capacity" ELSE "capacity"]
WorldCapacity == /\ s.pc[W] \in {"capacity", "retry-capacity"} /\ Available(W)
                 /\ s' = [s EXCEPT !.claims = @ \cup {W}, !.pc[W] = IF s.pc[W] = "retry-capacity" THEN "callback" ELSE "relock"]
DirectIntent(a) == /\ a \in {D, C} /\ s.pc[a] = "intent" /\ s.intent = 0
                   /\ (a # C \/ SettleBeforeEngine \/ s.engine = 0)
                   /\ s' = [s EXCEPT !.intent = a, !.pc[a] = "drain",
                      !.provider = IF DrainBeforeProvider THEN @ ELSE a,
                      !.engine = IF a = C /\ ~SettleBeforeEngine THEN C ELSE @]
Drain(a) == /\ a \in {D, C} /\ s.pc[a] = "drain" /\ s.pending = 0
            /\ s' = [s EXCEPT !.intent = IF a = C /\ TryControls THEN 0 ELSE @,
               !.engine = IF a = C /\ TryControls THEN 0 ELSE @, !.pc[a] = IF a = D THEN "state" ELSE "engine"]
ControlEngine == /\ s.pc[C] = "engine" /\ s.engine = 0
                 /\ s' = [s EXCEPT !.engine = C, !.pc[C] = IF s.intent = C THEN "state" ELSE "try-state"]
TryControl == /\ s.pc[C] = "try-state"
               /\ s' = [s EXCEPT !.intent = IF s.intent = 0 /\ s.pending = 0 THEN C ELSE @,
                  !.engine = IF s.intent = 0 /\ s.pending = 0 THEN C ELSE 0,
                  !.pc[C] = IF s.intent = 0 /\ s.pending = 0 THEN "state" ELSE "intent"]
CheckpointIntent == /\ s.pc[K] \in {"intent", "intent-claim"} /\ s.intent = 0
                    /\ (IntentBeforeCapacity \/ s.pc[K] = "intent-claim")
                    /\ s' = [s EXCEPT !.intent = K, !.pc[K] = IF K \in s.claims THEN "state" ELSE "budget"]
BadCheckpoint == /\ ~IntentBeforeCapacity /\ s.pc[K] = "intent" /\ Available(K)
                 /\ s' = [s EXCEPT !.claims = @ \cup {K}, !.pc[K] = "intent-claim"]
CheckpointBudget == /\ s.pc[K] = "budget"
                    /\ s' = [s EXCEPT !.claims = IF Available(K) THEN @ \cup {K} ELSE @,
                       !.intent = IF ~Available(K) /\ ReleaseCheckpoint THEN 0 ELSE @,
                       !.pc[K] = IF Available(K) THEN "state" ELSE "capacity"]
CheckpointCapacity == /\ s.pc[K] = "capacity" /\ Available(K)
                      /\ s' = [s EXCEPT !.claims = @ \cup {K}, !.pc[K] = IF s.intent = K THEN "state" ELSE "try-state"]
TryCheckpoint == /\ s.pc[K] = "try-state"
                 /\ s' = [s EXCEPT !.intent = IF s.intent = 0 THEN K ELSE @,
                    !.claims = IF s.intent = 0 THEN @ ELSE @ \ {K}, !.pc[K] = IF s.intent = 0 THEN "state" ELSE "intent"]
TakeProvider(a) == /\ s.pc[a] = "state" /\ s.provider \in {0, a}
                   /\ s' = [s EXCEPT !.provider = a, !.pc[a] = "write"]
Write(a) == /\ s.pc[a] = "write" /\ s.provider = a
            /\ s' = [s EXCEPT !.pending = IF a \in {W, K} THEN @ + 1 ELSE @,
               !.claims = @ \ {a}, !.large = IF a = W /\ WorldNeedsAlone THEN TRUE ELSE @,
               !.provider = 0, !.intent = 0, !.engine = IF s.engine = a THEN 0 ELSE @,
               !.sql = IF a = W THEN FALSE ELSE @,
               !.pc[a] = IF a = W THEN "callback" ELSE IF a = C THEN "engine" ELSE "intent"]
Settle == /\ s.pending > 0 /\ s' = [s EXCEPT !.pending = @ - 1, !.large = IF s.pending = 1 THEN FALSE ELSE @]
Actor(a) == DirectIntent(a) \/ Drain(a) \/ TakeProvider(a) \/ Write(a)
Next == Callback \/ Body \/ TryWorld \/ WorldIntent \/ WorldReady \/ RelockWorld \/ WorldBudget \/ WorldCapacity
        \/ ControlEngine \/ TryControl \/ CheckpointIntent \/ BadCheckpoint \/ CheckpointBudget \/ CheckpointCapacity \/ TryCheckpoint
        \/ (\E a \in Actors: Actor(a)) \/ Settle \/ UNCHANGED s
Spec == Init /\ [][Next]_s
Pair == (CheckpointBudget /\ Available(K)) \/ (TryCheckpoint /\ s.intent = 0)
FairSpec == Spec /\ WF_s(Settle) /\ WF_s(Body) /\ WF_s(TryWorld) /\ WF_s(WorldReady) /\ WF_s(WorldBudget)
            /\ WF_s(TryControl) /\ WF_s(CheckpointBudget) /\ WF_s(TryCheckpoint) /\ SF_s(Pair)
            /\ SF_s(Callback) /\ SF_s(WorldIntent) /\ SF_s(RelockWorld) /\ SF_s(WorldCapacity)
            /\ SF_s(ControlEngine) /\ SF_s(CheckpointIntent) /\ SF_s(CheckpointCapacity)
            /\ (\A a \in Actors: SF_s(Actor(a)))
Bounded == s.pending + Cardinality(s.claims) <= 2
NoWriterReceiptWait == /\ (\A a \in {D, C}: (s.pc[a] = "drain" /\ s.pending > 0) => (s.provider # a /\ s.engine # a))
                      /\ (\A a \in {W, C}: s.pc[a] = "intent-engine" => s.engine # a)
                      /\ (s.pc[W] \in {"capacity", "retry-capacity", "intent-wait"} => (s.engine # W /\ ~s.sql))
NoCapacityCycle == ~(s.intent = W /\ s.pc[W] = "capacity" /\ s.pending = 0 /\ K \in s.claims /\ s.pc[K] = "intent-claim")
NoCallbackCycle == ~(s.engine = W /\ s.pc[W] = "body" /\ ((s.intent = C /\ s.pc[C] = "engine")
                   \/ (s.intent = K /\ s.pc[K] = "capacity" /\ W \in s.claims)))
Finishes == (s.pc[K] # "intent") ~> (s.pc[K] = "intent")
====
