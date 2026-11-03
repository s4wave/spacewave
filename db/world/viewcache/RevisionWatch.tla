---- MODULE RevisionWatch ----
\* cache-watch.go samples progress before a read and waits on that generation.
\* cache-head.go supplies catch-up and refresh; cache-invalidation.go advances.
\* Query values, dependency indexes, expiry and access floors are outside
\* this bounded notification model.
EXTENDS Naturals
CONSTANT SourceWait
VARIABLES world, cache, refresh, generation, pc, token, readAt, last, woke, bad, expired
vars == <<world, cache, refresh, generation, pc, token, readAt, last, woke, bad, expired>>
Init == /\ world = 0 /\ cache = 0 /\ refresh = 0 /\ generation = 0
        /\ pc = "subscribe" /\ token = 0 /\ readAt = 0 /\ last = 0
        /\ woke = FALSE /\ bad = FALSE /\ expired = FALSE
Publish == /\ world < 2 /\ world' = world + 1
           /\ UNCHANGED <<cache, refresh, generation, pc, token, readAt, last, woke, bad, expired>>
Advance == /\ cache < world /\ cache' = world /\ generation' = generation + 1
           /\ UNCHANGED <<world, refresh, pc, token, readAt, last, woke, bad, expired>>
Refresh == /\ refresh = 0 /\ refresh' = 1 /\ generation' = generation + 1
           /\ UNCHANGED <<world, cache, pc, token, readAt, last, woke, bad, expired>>
Subscribe == /\ pc = "subscribe" /\ token' = generation /\ pc' = "read"
             /\ UNCHANGED <<world, cache, refresh, generation, readAt, last, woke, bad, expired>>
Read == /\ pc = "read" /\ readAt' = cache /\ last' = generation /\ pc' = "wait"
        /\ UNCHANGED <<world, cache, refresh, generation, token, woke, bad, expired>>
\* A channel closed between subscribe and read may cause one extra read;
\* taking a fresh channel prevents a second wake without a cache change.
Wake == /\ pc = "wait"
        /\ IF SourceWait THEN world > readAt ELSE generation > token
        /\ bad' = (bad \/ (woke /\ generation = last /\ token = last))
        /\ woke' = TRUE /\ pc' = "subscribe"
        /\ UNCHANGED <<world, cache, refresh, generation, token, readAt, last, expired>>
\* One known deadline permits one reread even while the cache is unchanged.
Expire == /\ pc = "wait" /\ ~expired
          /\ expired' = TRUE /\ pc' = "subscribe" /\ woke' = FALSE
          /\ UNCHANGED <<world, cache, refresh, generation, token, readAt, last, bad>>
\* Parent cancellation terminates either kind of wait.
Cancel == /\ pc # "done" /\ pc' = "done"
          /\ UNCHANGED <<world, cache, refresh, generation, token, readAt, last, woke, bad, expired>>
Next == Expire \/ Cancel \/ Publish \/ Advance \/ Refresh \/ Subscribe \/ Read \/ Wake \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars
FairSpec == Spec /\ WF_vars(Advance) /\ WF_vars(Subscribe) /\ WF_vars(Read) /\ WF_vars(Wake)
NoSpin == ~bad
CacheCatchesUp == world > cache ~> world = cache
ChangedAnswerWakes == (pc = "wait" /\ generation > token) ~> (pc # "wait")
====
