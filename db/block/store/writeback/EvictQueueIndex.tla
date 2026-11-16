-------------------------- MODULE EvictQueueIndex --------------------------
\* Three equal-size hashes and five epochs retain the real (epoch,hash)
\* multiplicity. The legacy A,H,B trace is reached by operations, not Init.
\* Rebuild may be interrupted after any committed entry and restart from zero.
\* The safety boundary is completion of repair, not an assumption excluding
\* duplicates. Mark/fill/clear run during repair too. Metadata actions never
\* mutate local or remote. Value-drop guards remain a separate eviction step.
\* Scenario isolates runtime restart/flip, staged activation and obsolete
\* eviction snapshots; each retains marker/clear/fill and the legacy trace.
\* Index requires retry: index-on/retry-off selects the complete legacy policy.
EXTENDS Integers, FiniteSets
CONSTANTS IndexOn, RetryOn, Scenario
Hashes == {1, 2, 3}
Epochs == {0, 1, 2, 3, 4}
Pairs == Epochs \X Hashes
VARIABLES accounted, entries, pointer, marked, local, remote, epoch, trace, cursor,
          complete, restarted, stage, fill, failed, enabled, flipped, snapshot, captured, retryActive, fence, markTx, markIndexed, markFence, repairPhase, repairEntry
vars == <<accounted, entries, pointer, marked, local, remote, epoch, trace, cursor,
          complete, restarted, stage, fill, failed, enabled, flipped, snapshot, captured, retryActive, fence, markTx, markIndexed, markFence, repairPhase, repairEntry>>
For(h) == {e \in entries : e[2] = h}
Indexed == IndexOn /\ retryActive /\ enabled
Remove(h) == entries \ {<<pointer[h],h>>}
Put(h) == IF Indexed THEN
             IF h \in marked THEN Remove(h) ELSE Remove(h) \cup {<<epoch,h>>}
          ELSE entries \cup {<<epoch,h>>}
Repaired == complete /\ Indexed
Init == /\ accounted = 0 /\ entries = {} /\ pointer = [h \in Hashes |-> -1]
        /\ marked = {} /\ local = {} /\ remote = {} /\ epoch = 0
        /\ trace = 0 /\ cursor = 0 /\ complete = FALSE /\ restarted = FALSE
        /\ stage = -1 /\ fill = "waiting" /\ failed = FALSE
        /\ enabled = FALSE /\ flipped = FALSE /\ snapshot = "waiting" /\ captured = <<0,2>> /\ retryActive = RetryOn /\ fence = 0 /\ markTx = "none" /\ markIndexed = FALSE /\ markFence = 0 /\ repairPhase = "read" /\ repairEntry = <<0,2>>
\* Exact old trace: upload A,H,B; evict only A; re-put/clear H in epoch one;
\* re-put H again; refuse old H then evict B and stop before newer H.
\* Refusal includes both legacy retry and the insufficient candidate that
\* dropped visited marked rows: even the latter leaves epoch-one H counted.
Legacy == /\ trace < 7 /\ trace' = trace + 1
          /\ CASE trace = 0 -> /\ entries' = {<<0,1>>,<<0,2>>,<<0,3>>}
                               /\ local' = Hashes /\ remote' = Hashes
                               /\ UNCHANGED <<marked,epoch,snapshot,captured,retryActive, fence, markTx, markIndexed, markFence, repairPhase, repairEntry>>
               [] trace = 1 -> /\ entries' = entries \ {<<0,1>>}
                               /\ local' = local \ {1} /\ epoch' = 1
                               /\ UNCHANGED <<marked,remote,snapshot,captured,retryActive, fence, markTx, markIndexed, markFence, repairPhase, repairEntry>>
               [] trace = 2 -> /\ marked' = {2}
                               /\ UNCHANGED <<entries,local,remote,epoch,snapshot,captured,retryActive, fence, markTx, markIndexed, markFence, repairPhase, repairEntry>>
               [] trace = 3 -> /\ entries' = entries \cup {<<1,2>>}
                               /\ marked' = {} /\ UNCHANGED <<local,remote,epoch,snapshot,captured,retryActive, fence, markTx, markIndexed, markFence, repairPhase, repairEntry>>
               [] trace = 4 -> /\ marked' = {2}
                               /\ UNCHANGED <<entries,local,remote,epoch,snapshot,captured,retryActive, fence, markTx, markIndexed, markFence, repairPhase, repairEntry>>
               [] trace = 5 -> /\ \E retain \in {FALSE, TRUE} :
                                    entries' = (entries \ {<<0,2>>}) \cup
                                               (IF retain THEN {<<2,2>>} ELSE {})
                               /\ epoch' = 2 /\ UNCHANGED <<marked,local,remote,snapshot,captured,retryActive, fence, markTx, markIndexed, markFence, repairPhase, repairEntry>>
               [] OTHER -> /\ entries' = entries \ {<<0,3>>}
                           /\ local' = local \ {3} /\ UNCHANGED <<marked,remote,epoch,snapshot,captured,retryActive, fence, markTx, markIndexed, markFence, repairPhase, repairEntry>>
          /\ UNCHANGED <<pointer,cursor,complete,restarted,stage,fill,failed,enabled,flipped,snapshot,captured,retryActive, fence, markTx, markIndexed, markFence, repairPhase, repairEntry>>
Enable == /\ trace = 7 /\ ~enabled /\ ~flipped /\ enabled' = TRUE /\ fence' = fence + 1 /\ epoch' = 3
          /\ cursor' = 0 /\ complete' = FALSE
          /\ UNCHANGED <<entries,pointer,marked,local,remote,trace,restarted,stage,fill,failed,flipped,snapshot,captured,retryActive, markTx, markIndexed, markFence, repairPhase, repairEntry>>
\* Each slice revisits one exact entry. Missing/old pointers are not evidence
\* of missing values. The committed byte delta is the cardinality difference.
ReadRepair == /\ Indexed /\ ~complete /\ repairPhase = "read" /\ cursor < 15
              /\ LET e == <<cursor \div 3, (cursor % 3) + 1>> IN
                   /\ e \in entries /\ repairEntry' = e /\ repairPhase' = "commit"
              /\ UNCHANGED <<entries,pointer,marked,local,remote,epoch,trace,cursor,complete,restarted,stage,fill,failed,enabled,flipped,snapshot,captured,retryActive,fence,markTx,markIndexed,markFence>>
SkipRepair == /\ Indexed /\ ~complete /\ repairPhase = "read" /\ cursor < 15
              /\ <<cursor \div 3, (cursor % 3) + 1>> \notin entries
              /\ cursor' = cursor + 1
              /\ UNCHANGED <<entries,pointer,marked,local,remote,epoch,trace,complete,restarted,stage,fill,failed,enabled,flipped,snapshot,captured,retryActive,fence,markTx,markIndexed,markFence,repairPhase,repairEntry>>
Rebuild == /\ trace = 7 /\ Indexed /\ ~complete /\ cursor < 15 /\ repairPhase = "commit"
           /\ LET e == repairEntry
                  h == e[2]
                  live == <<pointer[h],h>> \in entries
              IN /\ entries' = IF e \notin entries THEN entries
                               ELSE IF h \in marked \/ (live /\ pointer[h] # e[1])
                                    THEN entries \ {e} ELSE entries
                 /\ pointer' = IF e \in entries /\ (h \in marked \/ ~live)
                               THEN [pointer EXCEPT ![h] = e[1]] ELSE pointer
           /\ cursor' = cursor + 1 /\ repairPhase' = "read" /\ repairEntry' = <<0,2>>
           /\ UNCHANGED <<marked,local,remote,epoch,trace,complete,restarted,stage,fill,failed,enabled,flipped,snapshot,captured,retryActive, fence, markTx, markIndexed, markFence>>
Finish == /\ Indexed /\ cursor = 15 /\ ~complete /\ complete' = TRUE
          /\ UNCHANGED <<entries,pointer,marked,local,remote,epoch,trace,cursor,restarted,stage,fill,failed,enabled,flipped,snapshot,captured,retryActive, fence, markTx, markIndexed, markFence, repairPhase, repairEntry>>
Restart == /\ Scenario = "main" /\ enabled /\ ~restarted /\ restarted' = TRUE /\ fence' = fence + 1
           /\ cursor' = 0 /\ repairPhase' = "read" /\ repairEntry' = <<0,2>> /\ complete' = FALSE
           /\ UNCHANGED <<entries,pointer,marked,local,remote,epoch,trace,stage,fill,failed,enabled,flipped,snapshot,captured,retryActive, markTx, markIndexed, markFence>>
\* Toggle through legacy writes, invalidating both readiness and pointers.
Flip == /\ Scenario = "main" /\ enabled /\ ~flipped /\ flipped' = TRUE /\ enabled' = FALSE
        /\ complete' = FALSE /\ cursor' = 0 /\ repairPhase' = "read" /\ repairEntry' = <<0,2>>
        /\ entries' = entries \cup {<<0,2>>} /\ marked' = marked \cup {2}
        /\ UNCHANGED <<pointer,local,remote,epoch,trace,restarted,stage,fill,failed,snapshot,captured,retryActive, fence, markTx, markIndexed, markFence>>
FlipRetry == /\ Scenario = "main" /\ enabled /\ retryActive /\ ~flipped /\ flipped' = TRUE /\ enabled' = TRUE /\ retryActive' = FALSE
        /\ complete' = FALSE /\ cursor' = 0 /\ repairPhase' = "read" /\ repairEntry' = <<0,2>>
        /\ entries' = entries \cup {<<0,2>>} /\ marked' = marked \cup {2}
        /\ UNCHANGED <<pointer,local,remote,epoch,trace,restarted,stage,fill,failed,snapshot,captured,fence,markTx,markIndexed,markFence>>
Reenable == /\ flipped /\ (~enabled \/ retryActive # RetryOn) /\ enabled' = TRUE /\ retryActive' = RetryOn /\ fence' = fence + 1
            /\ UNCHANGED <<entries,pointer,marked,local,remote,epoch,trace,cursor,complete,restarted,stage,fill,failed,flipped,snapshot,captured,markTx,markIndexed,markFence, repairPhase, repairEntry>>
\* Clear the legacy marker normally, then create a fresh indexed marker.
Prime == /\ enabled /\ stage = -1 /\ stage' = 0 /\ marked' = marked \ {2}
         /\ entries' = IF Indexed THEN Remove(2) \cup {<<epoch,2>>}
                        ELSE entries \cup {<<epoch,2>>}
         /\ pointer' = IF Indexed THEN [pointer EXCEPT ![2] = epoch] ELSE pointer
         /\ UNCHANGED <<local,remote,epoch,trace,cursor,complete,restarted,fill,failed,enabled,flipped,snapshot,captured,retryActive, fence, markTx, markIndexed, markFence, repairPhase, repairEntry>>
AbsentRead == /\ 2 \in remote /\ stage = 1 /\ stage' = 4 /\ local' = local \ {2}
              /\ UNCHANGED <<entries,pointer,marked,remote,epoch,trace,cursor,complete,restarted,fill,failed,enabled,flipped,snapshot,captured,retryActive, fence, markTx, markIndexed, markFence, repairPhase, repairEntry>>

PrepareMarker == /\ Scenario = "fence" /\ trace = 7 /\ ~enabled /\ stage = -1 /\ stage' = 0
                 /\ marked' = marked \ {2} /\ entries' = entries \cup {<<epoch,2>>}
                 /\ UNCHANGED <<pointer,local,remote,epoch,trace,cursor,complete,restarted,fill,failed,enabled,flipped,snapshot,captured,retryActive,fence,markTx,markIndexed,markFence, repairPhase, repairEntry>>
StageMark == /\ (enabled \/ Scenario = "fence") /\ trace = 7 /\ stage = 0 /\ markTx = "none"
             /\ markTx' = "staged" /\ markIndexed' = Indexed /\ markFence' = fence
             /\ UNCHANGED <<entries,pointer,marked,local,remote,epoch,trace,cursor,complete,restarted,stage,fill,failed,enabled,flipped,snapshot,captured,retryActive,fence,repairPhase,repairEntry>>
RetryMark == /\ markTx = "staged" /\ markFence # fence /\ markTx' = "none" /\ markIndexed' = FALSE /\ markFence' = 0
             /\ UNCHANGED <<entries,pointer,marked,local,remote,epoch,trace,cursor,complete,restarted,stage,fill,failed,enabled,flipped,snapshot,captured,retryActive,fence, repairPhase, repairEntry>>
Mark == /\ enabled /\ stage = 0 /\ markTx = "staged" /\ markFence = fence
        /\ stage' = 1 /\ markTx' = "done" /\ markIndexed' = FALSE /\ markFence' = 0
        /\ marked' = marked \cup {2}
        /\ entries' = IF markIndexed THEN Remove(2) ELSE entries
        /\ pointer' = IF markIndexed THEN [pointer EXCEPT ![2] = IF @ = -1 THEN 0 ELSE @] ELSE pointer
        /\ UNCHANGED <<local,remote,epoch,trace,cursor,complete,restarted,fill,failed,enabled,flipped,snapshot,captured,retryActive,fence, repairPhase, repairEntry>>
AdvanceIndexedEpoch == /\ Scenario # "fence" /\ Repaired /\ epoch = 3 /\ epoch' = 4 /\ fence' = fence + 1
                       /\ UNCHANGED <<entries,pointer,marked,local,remote,trace,cursor,complete,restarted,stage,fill,failed,enabled,flipped,snapshot,captured,retryActive,markTx,markIndexed,markFence, repairPhase, repairEntry>>

FillReserve == /\ enabled /\ fill = "waiting" /\ fill' = "write"
               /\ entries' = Put(2)
               /\ pointer' = IF Indexed THEN [pointer EXCEPT ![2] = IF 2 \in marked THEN IF @ = -1 THEN 0 ELSE @ ELSE epoch] ELSE pointer
               /\ UNCHANGED <<marked,local,remote,epoch,trace,cursor,complete,restarted,stage,failed,enabled,flipped,snapshot,captured,retryActive, fence, markTx, markIndexed, markFence, repairPhase, repairEntry>>
FillWrite == /\ fill = "write" /\ fill' = "done" /\ local' = local \cup {2}
             /\ UNCHANGED <<entries,pointer,marked,remote,epoch,trace,cursor,complete,restarted,stage,failed,enabled,flipped,snapshot,captured,retryActive, fence, markTx, markIndexed, markFence, repairPhase, repairEntry>>
FillFail == /\ fill = "write" /\ ~failed /\ failed' = TRUE /\ fill' = "done"
            /\ UNCHANGED <<entries,pointer,marked,local,remote,epoch,trace,cursor,complete,restarted,stage,enabled,flipped,snapshot,captured,retryActive, fence, markTx, markIndexed, markFence, repairPhase, repairEntry>>
\* Normal and dropped clears share the retry-on unconditional reservation;
\* an absent local value does not need a foreground read and is safe surplus.
Clear == /\ enabled /\ stage \in {1,4} /\ stage' = 2 /\ marked' = marked \ {2}
         /\ entries' = IF stage # 4 \/ retryActive \/ pointer[2] # -1 THEN
                           IF Indexed THEN Remove(2) \cup {<<epoch,2>>}
                           ELSE entries \cup {<<epoch,2>>}
                        ELSE entries
         /\ pointer' = IF Indexed THEN [pointer EXCEPT ![2] = epoch] ELSE pointer
         /\ UNCHANGED <<local,remote,epoch,trace,cursor,complete,restarted,fill,failed,enabled,flipped,snapshot,captured,retryActive, fence, markTx, markIndexed, markFence, repairPhase, repairEntry>>
\* The marker/remote guards precede every value deletion.
Evict == /\ stage = 2 /\ fill = "done" /\ stage' = 3
         /\ local' = IF 2 \notin marked /\ 2 \in remote THEN local \ {2} ELSE local
         /\ UNCHANGED <<entries,pointer,marked,remote,epoch,trace,cursor,complete,restarted,fill,failed,enabled,flipped,snapshot,captured,retryActive, fence, markTx, markIndexed, markFence, repairPhase, repairEntry>>
\* A retained-entry snapshot may race a move by clear or fill. Cleanup may
\* delete only its exact old key and must preserve a newer canonical pointer.
Snapshot == /\ Scenario = "snapshot" /\ enabled /\ snapshot = "waiting" /\ entries # {}
            /\ \E e \in entries : captured' = e
            /\ snapshot' = "captured"
            /\ UNCHANGED <<entries,pointer,marked,local,remote,epoch,trace,cursor,complete,restarted,stage,fill,failed,enabled,flipped,retryActive, fence, markTx, markIndexed, markFence, repairPhase, repairEntry>>
CommitSnapshot == /\ snapshot = "captured" /\ snapshot' = "done" /\ captured' = <<0,2>>
                  /\ LET h == captured[2]
                     IN /\ entries' = IF captured \notin entries THEN entries
                                      ELSE IF h \notin marked
                                           THEN (entries \ {captured,<<pointer[h],h>>}) \cup {<<epoch,h>>}
                                           ELSE entries \ {captured}
                        /\ pointer' = IF captured \in entries /\ h \notin marked
                                      THEN [pointer EXCEPT ![h] = epoch] ELSE pointer
                  /\ UNCHANGED <<marked,local,remote,epoch,trace,cursor,complete,restarted,stage,fill,failed,enabled,flipped,retryActive, fence, markTx, markIndexed, markFence, repairPhase, repairEntry>>
LoseRemote == /\ Scenario = "main" /\ stage = 2 /\ enabled /\ 2 \in remote /\ 2 \in local
              /\ remote' = remote \ {2}
              /\ UNCHANGED <<entries,pointer,marked,local,epoch,trace,cursor,complete,restarted,stage,fill,failed,enabled,flipped,snapshot,captured,retryActive, fence, markTx, markIndexed, markFence, repairPhase, repairEntry>>

\* Failed transactions publish none of the entry, marker or byte mutations.
MetadataFail == /\ Scenario = "main" /\ enabled /\ ~failed /\ failed' = TRUE
                /\ stage \in {0,1,4}
                /\ UNCHANGED <<entries,pointer,marked,local,remote,epoch,trace,cursor,complete,restarted,stage,fill,enabled,flipped,snapshot,captured,retryActive, fence, markTx, markIndexed, markFence, repairPhase, repairEntry>>
FillQueueFail == /\ Scenario = "main" /\ enabled /\ fill = "waiting" /\ ~failed
                 /\ fill' = "done" /\ failed' = TRUE
                 /\ UNCHANGED <<entries,pointer,marked,local,remote,epoch,trace,cursor,complete,restarted,stage,enabled,flipped,snapshot,captured,retryActive, fence, markTx, markIndexed, markFence, repairPhase, repairEntry>>

\* All requested calls have returned; optional future switch/restart events
\* remain possible. This terminal state does not hide a stuck repair.
Done == /\ trace = 7 /\ stage = 3 /\ fill = "done" /\ (~Indexed \/ complete)
        /\ UNCHANGED <<entries,pointer,marked,local,remote,epoch,trace,cursor,complete,restarted,stage,fill,failed,enabled,flipped,snapshot,captured,retryActive, fence, markTx, markIndexed, markFence, repairPhase, repairEntry>>

RebuildDebit == LET e == repairEntry
                   h == e[2]
                   duplicate == <<pointer[h],h>> \in entries /\ pointer[h] # e[1]
               IN IF e \in entries /\ (h \in marked \/ duplicate) THEN -1 ELSE 0
SnapshotDebit == (IF captured \in entries THEN -1 ELSE 0)
                 + Cardinality(entries' \ (entries \ {captured}))
                 - Cardinality((entries \ {captured}) \ entries')

LedgerNext == accounted' = accounted + IF Rebuild THEN RebuildDebit ELSE IF CommitSnapshot THEN SnapshotDebit ELSE Cardinality(entries' \ entries) - Cardinality(entries \ entries')
Next == /\ (Legacy \/ Enable \/ ReadRepair \/ SkipRepair \/ Rebuild \/ Finish \/ Restart \/ Flip \/ FlipRetry \/ Reenable
             \/ Snapshot \/ CommitSnapshot \/ LoseRemote \/ Prime \/ PrepareMarker \/ StageMark \/ RetryMark \/ AdvanceIndexedEpoch \/ AbsentRead \/ Mark \/ FillReserve \/ FillWrite \/ FillFail \/ MetadataFail \/ FillQueueFail \/ Clear \/ Evict \/ Done)
        /\ LedgerNext
Spec == Init /\ [][Next]_vars /\ WF_vars(ReadRepair /\ LedgerNext) /\ WF_vars(SkipRepair /\ LedgerNext) /\ WF_vars(Rebuild /\ LedgerNext) /\ WF_vars(Finish /\ LedgerNext)
NoDuplicates == Repaired => \A h \in Hashes : Cardinality(For(h)) <= 1
NoMarkedOverlap == Repaired => \A h \in marked : For(h) = {}
PointerSound == Repaired => \A h \in Hashes :
                 h \notin marked =>
                   (For(h) = {} <=> pointer[h] = -1) /\
                   (pointer[h] # -1 => <<pointer[h],h>> \in entries)
NoStrandedValue == Repaired /\ RetryOn =>
                    \A h \in local \ marked : For(h) # {}
NoMissingCopyDropped == trace > 0 => Hashes \ remote \subseteq local
ExactBytes == accounted = Cardinality(entries)
RepairCompletes == Indexed ~> (~Indexed \/ complete)
LegacyOverlapAbsent == trace # 7 \/ Cardinality(For(2)) <= 1
=============================================================================
