---- MODULE ReplayCancellation ----
\* spaceReplay.applyTx and applyGraphSettingsStep run an operation against a
\* transport whose request lifetime may end while the adapter stays live. A
\* canceled dependency stops the pass with an error, never a refusal; the
\* BodyDriver applies the same operation again on the next state or after a
\* backoff. A semantic failure alone may place a rejection.
\* Three operations include one semantic rejection between two valid writes.
\* One transport restart covers cancellation both before and after a read starts.
\* Payload transfer and ordering are covered separately.
EXTENDS Naturals, FiniteSets
CONSTANT PropagateCancellation
VARIABLES phase, nextOp, transport, generation, readGeneration,
          applied, rejected, retired
vars == <<phase, nextOp, transport, generation, readGeneration,
          applied, rejected, retired>>
Valid == {1, 3}
Init == /\ phase = "ready" /\ nextOp = 1 /\ transport = "running"
        /\ generation = 0 /\ readGeneration = 0
        /\ applied = {} /\ rejected = {} /\ retired = FALSE
BeginRead == /\ phase = "ready" /\ nextOp <= 3 /\ ~retired
             /\ transport = "running"
             /\ phase' = "reading" /\ readGeneration' = generation
             /\ UNCHANGED <<nextOp, transport, generation, applied, rejected, retired>>
StopTransport == /\ transport = "running" /\ generation = 0
                 /\ transport' = "stopped"
                 /\ UNCHANGED <<phase, nextOp, generation, readGeneration,
                                applied, rejected, retired>>
RestartTransport == /\ transport = "stopped"
                    /\ transport' = "running" /\ generation' = 1
                    /\ UNCHANGED <<phase, nextOp, readGeneration, applied, rejected, retired>>
ReadSucceeded == /\ phase = "reading" /\ ~retired
                 /\ transport = "running" /\ readGeneration = generation
                 /\ phase' = "apply"
                 /\ UNCHANGED <<nextOp, transport, generation, readGeneration,
                                applied, rejected, retired>>
ReadCanceled == /\ phase = "reading" /\ ~retired
                /\ (transport = "stopped" \/ readGeneration # generation)
                /\ phase' = IF PropagateCancellation THEN "retry" ELSE "ready"
                /\ nextOp' = IF PropagateCancellation THEN nextOp ELSE nextOp + 1
                /\ rejected' = IF PropagateCancellation THEN rejected ELSE rejected \cup {nextOp}
                /\ UNCHANGED <<transport, generation, readGeneration, applied, retired>>
RetryReplay == /\ phase = "retry" /\ ~retired /\ transport = "running"
               /\ phase' = "ready"
               /\ UNCHANGED <<nextOp, transport, generation, readGeneration,
                              applied, rejected, retired>>
ApplyOperation == /\ phase = "apply" /\ ~retired
                  /\ applied' = IF nextOp \in Valid THEN applied \cup {nextOp} ELSE applied
                  /\ rejected' = IF nextOp \in Valid THEN rejected ELSE rejected \cup {nextOp}
                  /\ nextOp' = nextOp + 1 /\ phase' = "ready"
                  /\ UNCHANGED <<transport, generation, readGeneration, retired>>
RetireEngine == /\ ~retired /\ retired' = TRUE
                /\ UNCHANGED <<phase, nextOp, transport, generation, readGeneration,
                               applied, rejected>>
\* Done ends a run: every operation is placed, or the engine retired.
Done == (nextOp = 4 \/ retired) /\ UNCHANGED vars
Next == BeginRead \/ StopTransport \/ RestartTransport \/ ReadSucceeded
        \/ ReadCanceled \/ RetryReplay \/ ApplyOperation \/ RetireEngine
        \/ Done
Spec == Init /\ [][Next]_vars
FairSpec == Spec /\ WF_vars(BeginRead) /\ WF_vars(RestartTransport)
            /\ WF_vars(ReadSucceeded) /\ WF_vars(ReadCanceled)
            /\ WF_vars(RetryReplay) /\ WF_vars(ApplyOperation)
TypeOK == /\ phase \in {"ready", "reading", "apply", "retry"}
          /\ nextOp \in 1..4 /\ transport \in {"running", "stopped"}
          /\ generation \in 0..1 /\ readGeneration \in 0..1
          /\ applied \subseteq Valid /\ rejected \subseteq 1..3
          /\ retired \in BOOLEAN
CancellationIsLocal == rejected \cap Valid = {}
PlacedOnce == Cardinality(applied \cup rejected) = nextOp - 1
Finishes == <> (nextOp = 4 \/ retired)
====
