---- MODULE StateWriters ----
\* SharedObject state intent precedes engine and provider writer locks.
\* W is a World candidate, D a peer import/configuration write, C an engine
\* control write and K a stable checkpoint. D/C drain the admitted prefix
\* while owning intent but neither writer. Intent excludes new admissions,
\* so a direct writer cannot starve behind continuously admitted writes.
\* W may run its callback under the engine writer first, but only tries intent
\* there: a busy intent releases the engine before waiting. It also releases
\* the engine writer when its exact SQL capacity is unavailable.
\* K takes intent before capacity, avoiding a claim/intent cycle with W.
\* A publication declined by the store retries with settled intent outside
\* both writers. State contents and durable failure are checked separately
\* by SQLPrimary and PublicationReservation. This model uses one SO/store.
EXTENDS Naturals, FiniteSets
CONSTANTS IntentBeforeEngine, DrainBeforeProvider, SettleBeforeEngine,
          IntentBeforeCapacity, WorldNeedsAlone
W == 1
D == 2
C == 3
K == 4
Actors == 1..4
VARIABLES pc, intent, engine, provider, pending, claims, large
vars == <<pc, intent, engine, provider, pending, claims, large>>
Init == /\ pc = [a \in Actors |-> "intent"]
        /\ intent = 0 /\ engine = 0 /\ provider = 0
        /\ pending \in 0..2 /\ claims = {} /\ large = FALSE
Available(a) ==
  IF a = W /\ WorldNeedsAlone THEN pending = 0 /\ claims = {}
  ELSE pending + Cardinality(claims) < 2 /\ ~large
       /\ ~(WorldNeedsAlone /\ W \in claims)
WorldCallback == /\ IntentBeforeEngine /\ pc[W] = "intent" /\ engine = 0
                 /\ engine' = W /\ pc' = [pc EXCEPT ![W] = "try-intent"]
                 /\ UNCHANGED <<intent, provider, pending, claims, large>>
TryIntent == /\ pc[W] = "try-intent"
             /\ intent' = IF intent = 0 THEN W ELSE intent
             /\ engine' = IF intent = 0 THEN W ELSE 0
             /\ pc' = [pc EXCEPT ![W] = IF intent = 0 THEN "budget" ELSE "intent-wait"]
             /\ UNCHANGED <<provider, pending, claims, large>>
BadEngine(a) == /\ a \in {W, C} /\ ~IntentBeforeEngine
                /\ pc[a] = "intent" /\ engine = 0
                /\ engine' = a /\ pc' = [pc EXCEPT ![a] = "intent-engine"]
                /\ UNCHANGED <<intent, provider, pending, claims, large>>
BadCheckpoint == /\ ~IntentBeforeCapacity /\ pc[K] = "intent" /\ Available(K)
                 /\ claims' = claims \cup {K}
                 /\ pc' = [pc EXCEPT ![K] = "intent-claim"]
                 /\ UNCHANGED <<intent, engine, provider, pending, large>>
TakeIntent(a) ==
  /\ intent = 0 /\ pc[a] \in {"intent", "intent-wait", "intent-engine", "intent-claim"}
  /\ (a \notin {W, C} \/ IntentBeforeEngine \/ pc[a] = "intent-engine")
  /\ (a # K \/ IntentBeforeCapacity \/ pc[a] = "intent-claim")
  /\ (a # C \/ SettleBeforeEngine \/ engine = 0)
  /\ intent' = a
  /\ pc' = [pc EXCEPT ![a] = IF a \in {D, C} THEN "drain"
                    ELSE IF a = K THEN IF K \in claims THEN "state" ELSE "budget"
                    ELSE IF engine = W THEN "budget" ELSE "engine"]
  /\ provider' = IF a \in {D, C} /\ ~DrainBeforeProvider THEN a ELSE provider
  /\ engine' = IF a = C /\ ~SettleBeforeEngine THEN C ELSE engine
  /\ UNCHANGED <<pending, claims, large>>
Drain(a) == /\ a \in {D, C} /\ pc[a] = "drain" /\ pending = 0
            /\ pc' = [pc EXCEPT ![a] = IF a = C /\ engine # C THEN "engine" ELSE "state"]
            /\ UNCHANGED <<intent, engine, provider, pending, claims, large>>
TakeEngine(a) == /\ a \in {W, C} /\ pc[a] \in {"engine", "relock"} /\ engine = 0
                 /\ engine' = a
                 /\ pc' = [pc EXCEPT ![a] = IF a = C \/ W \in claims THEN "state" ELSE "budget"]
                 /\ UNCHANGED <<intent, provider, pending, claims, large>>
TryCapacity(a) == /\ a \in {W, K} /\ pc[a] = "budget"
                  /\ pc' = [pc EXCEPT ![a] = IF Available(a) THEN "state" ELSE "capacity"]
                  /\ claims' = IF Available(a) THEN claims \cup {a} ELSE claims
                  /\ engine' = IF a = W /\ ~Available(a) THEN 0 ELSE engine
                  /\ UNCHANGED <<intent, provider, pending, large>>
WaitCapacity(a) == /\ a \in {W, K} /\ pc[a] = "capacity" /\ Available(a)
                   /\ claims' = claims \cup {a}
                   /\ pc' = [pc EXCEPT ![a] = IF a = W THEN "relock" ELSE "state"]
                   /\ UNCHANGED <<intent, engine, provider, pending, large>>
TakeProvider(a) == /\ pc[a] = "state" /\ provider \in {0, a}
                   /\ provider' = a /\ pc' = [pc EXCEPT ![a] = "write"]
                   /\ UNCHANGED <<intent, engine, pending, claims, large>>
Write(a) == /\ pc[a] = "write" /\ provider = a
            /\ pending' = IF a \in {W, K} THEN pending + 1 ELSE pending
            /\ claims' = claims \ {a}
            /\ large' = IF a = W /\ WorldNeedsAlone THEN TRUE ELSE large
            /\ provider' = 0 /\ pc' = [pc EXCEPT ![a] = "finish"]
            /\ UNCHANGED <<intent, engine>>
UnlockEngine(a) == /\ pc[a] = "finish" /\ engine = a
                   /\ engine' = 0
                   /\ UNCHANGED <<pc, intent, provider, pending, claims, large>>
Finish(a) == /\ pc[a] = "finish" /\ engine # a
             /\ intent' = 0 /\ pc' = [pc EXCEPT ![a] = "intent"]
             /\ UNCHANGED <<engine, provider, pending, claims, large>>
Fallback == /\ pc[W] = "write" /\ pending > 0
             /\ provider' = 0 /\ engine' = 0 /\ intent' = 0
             /\ claims' = claims \ {W} /\ pc' = [pc EXCEPT ![W] = "retry"]
             /\ UNCHANGED <<pending, large>>
Retry == /\ pc[W] = "retry" /\ intent = 0
          /\ intent' = W /\ pc' = [pc EXCEPT ![W] = "retry-drain"]
          /\ UNCHANGED <<engine, provider, pending, claims, large>>
RetryDrained == /\ pc[W] = "retry-drain" /\ pending = 0
                 /\ pc' = [pc EXCEPT ![W] = "engine"]
                 /\ UNCHANGED <<intent, engine, provider, pending, claims, large>>
Settle == /\ pending > 0 /\ pending' = pending - 1
          /\ large' = IF pending = 1 THEN FALSE ELSE large
          /\ UNCHANGED <<pc, intent, engine, provider, claims>>
Actor(a) == BadEngine(a) \/ TakeIntent(a) \/ Drain(a) \/ TakeEngine(a)
            \/ TryCapacity(a) \/ WaitCapacity(a) \/ TakeProvider(a)
            \/ Write(a) \/ UnlockEngine(a) \/ Finish(a)
Next == WorldCallback \/ TryIntent \/ (\E a \in Actors: Actor(a)) \/ BadCheckpoint \/ Fallback \/ Retry \/ RetryDrained \/ Settle \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars
FairSpec == Spec /\ WF_vars(TryIntent) /\ WF_vars(Settle) /\ SF_vars(Retry) /\ WF_vars(RetryDrained)
            /\ (\A a \in Actors: SF_vars(TakeIntent(a)) /\ SF_vars(TakeEngine(a)) /\ WF_vars(Actor(a)))
Bounded == pending + Cardinality(claims) <= 2
NoWriterReceiptWait ==
  /\ (\A a \in {D, C}: (pc[a] = "drain" /\ pending > 0) => (provider # a /\ engine # a))
  /\ (\A a \in Actors: pc[a] = "capacity" => (provider # a /\ engine # a))
  /\ (pc[W] = "retry-drain" => (provider # W /\ engine # W))
  /\ (\A a \in {W, C}: ~(engine = a /\ pc[a] = "intent-engine" /\ pending > 0
            /\ (IF intent = 0 THEN FALSE ELSE pc[intent] = "drain")))
NoCapacityCycle == ~(intent = W /\ pc[W] = "capacity" /\ pending = 0
                    /\ K \in claims /\ pc[K] = "intent-claim")
Finishes == \A a \in Actors: (pc[a] # "intent") ~> (pc[a] = "intent")
====
