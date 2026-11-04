-------------------------- MODULE ReadCursor --------------------------
EXTENDS Naturals, FiniteSets
\* Callers: NewReadCursor/Okra.NewTx bind immutable references; UnmarshalReadBlock
\* and Cursor.Unmarshal perform StartLoad/ReadBytes/StoreFill. readSlot builds
\* metadata outside the shard lock (StartAttach), then compares slot identity
\* and charges/evicts atomically under the lock (FinishAttach). AbortLoad and
\* AbortAttach represent cancellation, decode failure and GetBlockRefs failure.
\* Invalidate/Remove/ChangeScope model external freshness and borrowed authority.
\* Root publication does not retarget existing cursors; snapshot binding is
\* checked by ReadCursorRoots and the real World snapshot test.
\* Units are proportional byte charges: one decoded payload plus two units of
\* traversal metadata. Victim choice abstracts the queue, checked separately by
\* ReadCursorClock. Caller-held blocks during a read are outside the pool budget.
\* Encoded-hash validation remains Cursor.Unmarshal's existing tested contract.
CONSTANTS Scopes, Refs, TokenCount, Budget, ClearRetired, CheckAdmission, CheckSlot,
          CheckScope, CheckRemoval, CheckEpoch, CheckBudget, CheckEnabled
VARIABLES enabled, durable, generation, epoch, scope, moved, cache, blockKey, metadata,
          stampGen, stampEpoch, nextToken, links, fill, attach, bad
vars == <<enabled, durable, generation, epoch, scope, moved, cache, blockKey, metadata,
          stampGen, stampEpoch, nextToken, links, fill, attach, bad>>
Keys == Scopes \X Refs
Tokens == 1..TokenCount
None == <<2, CHOOSE r \in Refs : TRUE>>
IdleFill == [phase |-> "idle", key |-> CHOOSE k \in Keys : TRUE,
             gen |-> 0, rev |-> 0, committed |-> FALSE]
IdleAttach == [token |-> 0, key |-> CHOOSE k \in Keys : TRUE]
Live == {cache[k] : k \in Keys} \ {0}
Charge(tokens, meta) == Cardinality(tokens) + 2 * Cardinality({t \in tokens : meta[t]})
Retained == {t \in Tokens : blockKey[t] # None}
Init == /\ enabled = Scopes
        /\ durable \in SUBSET {k \in Keys : k[1] = 0}
        /\ generation = [r \in Refs |-> 0]
        /\ epoch = [s \in Scopes |-> 0]
        /\ scope = 0 /\ moved = FALSE
        /\ cache = [k \in Keys |-> 0]
        /\ blockKey = [t \in Tokens |-> None]
        /\ metadata = [t \in Tokens |-> FALSE]
        /\ stampGen = [t \in Tokens |-> 0]
        /\ stampEpoch = [t \in Tokens |-> 0]
        /\ nextToken = 1
        /\ links = [r \in Refs |-> 0]
        /\ fill = IdleFill /\ attach = IdleAttach /\ bad = FALSE
StartLoad(r, committed) ==
    /\ fill.phase = "idle"
    /\ (committed => <<scope, r>> \in durable)
    /\ fill' = [phase |-> "fetch", key |-> <<scope, r>>,
                gen |-> generation[r], rev |-> epoch[scope], committed |-> committed]
    /\ UNCHANGED <<enabled, durable, generation, epoch, scope, moved, cache, blockKey,
                   metadata, stampGen, stampEpoch, nextToken, links, attach, bad>>
ReadBytes == /\ fill.phase = "fetch"
             /\ fill' = [fill EXCEPT !.phase = "decoded"]
             /\ UNCHANGED <<enabled, durable, generation, epoch, scope, moved, cache,
                blockKey, metadata, stampGen, stampEpoch, nextToken, links, attach, bad>>
AbortLoad == /\ fill.phase # "idle" /\ fill' = IdleFill
             /\ UNCHANGED <<enabled, durable, generation, epoch, scope, moved, cache,
                blockKey, metadata, stampGen, stampEpoch, nextToken, links, attach, bad>>
Admissible == /\ (~CheckAdmission \/ fill.committed)
              /\ (~CheckEnabled \/ fill.key[1] \in enabled)
              /\ fill.gen = generation[fill.key[2]]
              /\ fill.rev = epoch[fill.key[1]]
StoreFill ==
    /\ fill.phase = "decoded" /\ Admissible /\ nextToken \in Tokens
    /\ \E victims \in SUBSET Live :
       /\ (cache[fill.key] = 0 \/ cache[fill.key] \in victims)
       /\ Charge(Live \ victims, metadata) + 1 <= Budget
       /\ cache' = [k \in Keys |-> IF k = fill.key THEN nextToken
                              ELSE IF cache[k] \in victims THEN 0 ELSE cache[k]]
       /\ blockKey' = [t \in Tokens |-> IF t = nextToken THEN fill.key
                         ELSE IF ClearRetired /\ t \in victims THEN None ELSE blockKey[t]]
       /\ metadata' = [t \in Tokens |-> IF t = nextToken THEN FALSE
                         ELSE IF ClearRetired /\ t \in victims THEN FALSE ELSE metadata[t]]
    /\ stampGen' = [stampGen EXCEPT ![nextToken] = fill.gen]
    /\ stampEpoch' = [stampEpoch EXCEPT ![nextToken] = fill.rev]
    /\ nextToken' = nextToken + 1 /\ fill' = IdleFill
    /\ UNCHANGED <<enabled, durable, generation, epoch, scope, moved, links, attach, bad>>
StartAttach(k) ==
    /\ attach.token = 0 /\ cache[k] # 0 /\ ~metadata[cache[k]]
    /\ attach' = [token |-> cache[k], key |-> k]
    /\ UNCHANGED <<enabled, durable, generation, epoch, scope, moved, cache, blockKey,
                   metadata, stampGen, stampEpoch, nextToken, links, fill, bad>>
AbortAttach == /\ attach.token # 0 /\ attach' = IdleAttach
               /\ UNCHANGED <<enabled, durable, generation, epoch, scope, moved, cache,
                  blockKey, metadata, stampGen, stampEpoch, nextToken, links, fill, bad>>
FinishAttach ==
    /\ attach.token # 0
    /\ (~CheckSlot \/ cache[attach.key] = attach.token)
    /\ \E victims \in SUBSET (Live \ {attach.token}) :
       /\ LET meta == [metadata EXCEPT ![attach.token] = TRUE]
          IN /\ (~CheckBudget \/ Charge(Live \ victims, meta) <= Budget)
             /\ metadata' = [t \in Tokens |-> IF t = attach.token THEN TRUE
                        ELSE IF ClearRetired /\ t \in victims THEN FALSE ELSE metadata[t]]
       /\ cache' = [k \in Keys |-> IF cache[k] \in victims THEN 0 ELSE cache[k]]
       /\ blockKey' = [t \in Tokens |-> IF t = attach.token THEN attach.key
                         ELSE IF ClearRetired /\ t \in victims THEN None ELSE blockKey[t]]
    /\ attach' = IdleAttach
    /\ UNCHANGED <<enabled, durable, generation, epoch, scope, moved, stampGen, stampEpoch,
                   nextToken, links, fill, bad>>
Evict(k) ==
    /\ cache[k] # 0
    /\ LET t == cache[k] IN
       /\ cache' = [cache EXCEPT ![k] = 0]
       /\ blockKey' = IF ClearRetired THEN [blockKey EXCEPT ![t] = None] ELSE blockKey
       /\ metadata' = IF ClearRetired THEN [metadata EXCEPT ![t] = FALSE] ELSE metadata
    /\ UNCHANGED <<enabled, durable, generation, epoch, scope, moved, stampGen, stampEpoch,
                   nextToken, links, fill, attach, bad>>
Remove(k) == /\ k \in durable /\ generation[k[2]] = 0
             /\ durable' = durable \ {k}
             /\ generation' = [generation EXCEPT ![k[2]] = 1]
             /\ UNCHANGED <<enabled, epoch, scope, moved, cache, blockKey, metadata,
                            stampGen, stampEpoch, nextToken, links, fill, attach, bad>>
Invalidate(s) == /\ epoch[s] = 0 /\ epoch' = [epoch EXCEPT ![s] = 1]
                 /\ UNCHANGED <<enabled, durable, generation, scope, moved, cache, blockKey,
                       metadata, stampGen, stampEpoch, nextToken, links, fill, attach, bad>>
Close(s) == /\ s \in enabled
            /\ enabled' = enabled \ {s}
            /\ epoch' = [epoch EXCEPT ![s] = @ + 1]
            /\ UNCHANGED <<durable, generation, scope, moved, cache, blockKey,
                 metadata, stampGen, stampEpoch, nextToken, links, fill, attach, bad>>

ChangeScope == /\ 1 \in Scopes /\ ~moved /\ scope' = 1 /\ moved' = TRUE
               /\ UNCHANGED <<enabled, durable, generation, epoch, cache, blockKey, metadata,
                              stampGen, stampEpoch, nextToken, links, fill, attach, bad>>
Bind(r) == /\ cache[<<scope, r>>] # 0 /\ metadata[cache[<<scope, r>>]]
           /\ links' = [links EXCEPT ![r] = cache[<<scope, r>>]]
           /\ UNCHANGED <<enabled, durable, generation, epoch, scope, moved, cache, blockKey,
                          metadata, stampGen, stampEpoch, nextToken, fill, attach, bad>>
Read(r) ==
    /\ links[r] # 0
    /\ LET t == links[r] IN
       /\ blockKey[t] # None /\ metadata[t]
       /\ (~CheckScope \/ blockKey[t] = <<scope, r>>)
       /\ (~CheckEnabled \/ scope \in enabled)
       /\ (~CheckRemoval \/ stampGen[t] = generation[r])
       /\ (~CheckEpoch \/ stampEpoch[t] = epoch[scope])
       /\ bad' = (bad \/ scope \notin enabled \/ <<scope, r>> \notin durable
                      \/ blockKey[t] # <<scope, r>> \/ stampEpoch[t] # epoch[scope])
    /\ UNCHANGED <<enabled, durable, generation, epoch, scope, moved, cache, blockKey,
                   metadata, stampGen, stampEpoch, nextToken, links, fill, attach>>
Next == (\E r \in Refs : StartLoad(r, TRUE) \/ StartLoad(r, FALSE) \/ Bind(r) \/ Read(r))
        \/ (\E k \in Keys : StartAttach(k) \/ Evict(k) \/ Remove(k))
        \/ (\E s \in Scopes : Invalidate(s) \/ Close(s))
        \/ ReadBytes \/ StoreFill \/ AbortLoad \/ FinishAttach \/ AbortAttach
        \/ ChangeScope \/ UNCHANGED vars
Bounded == Charge(Retained, metadata) <= Budget
RetiredEmpty == \A t \in Tokens : t \notin Live => blockKey[t] = None
StoredReads == ~bad
Spec == Init /\ [][Next]_vars
======================================================================
