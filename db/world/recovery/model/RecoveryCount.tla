-------------------------- MODULE RecoveryCount --------------------------
EXTENDS Naturals, FiniteSets
CONSTANTS Items, Unsupported
Workers == {"first", "second"}
VARIABLES phase, resume, committed, pending, identityOK, source, owners, countedEver, unknown
vars == <<phase, resume, committed, pending, identityOK, source, owners, countedEver, unknown>>
(**************************************************************************
Count-only implementation. Each phase's bounded step prepares scratch rows,
result and cursor in one transaction. Commit publishes them atomically. A
kill before commit loses only pending work and releases the OS-owned lease;
resume acquires the lease before continuing. A second count cannot acquire
an owned scratch run. Ordinary failures pause while preserving its lease.
Counted observers cannot demote the durable terminal count. Source changes
invalidate every phase, including a completed report reopened for counting.
Typed classification, native SQLite durability and pager bounds need Go checks.
Unknown evidence stays in the durable result; completing work never clears it.
There is no export, disposition, source write, collector or reclaim action.
**************************************************************************)
Init == /\ phase = "inventory" /\ resume = "inventory"
        /\ committed = {} /\ pending = {} /\ identityOK = TRUE /\ source = Items
        /\ owners = {"first"} /\ countedEver = FALSE /\ unknown = {}
Prepare == /\ phase \in {"inventory", "interpret", "traverse"}
           /\ Cardinality(owners) = 1
           /\ pending = {} /\ committed # Items
           /\ \E i \in Items \ committed: pending' = {i}
           /\ UNCHANGED <<phase, resume, committed, identityOK, source, owners, countedEver, unknown>>
Commit == /\ pending # {} /\ identityOK /\ Cardinality(owners) = 1
          /\ committed' = committed \cup pending /\ pending' = {}
          /\ unknown' = unknown \cup (pending \cap Unsupported)
          /\ UNCHANGED <<phase, resume, identityOK, source, owners, countedEver>>
Advance == /\ committed = Items /\ pending = {} /\ identityOK
           /\ Cardinality(owners) = 1
           /\ phase \in {"inventory", "interpret", "traverse"}
           /\ phase' = CASE phase = "inventory" -> "interpret"
                         [] phase = "interpret" -> "traverse"
                         [] OTHER -> "counted"
           /\ committed' = IF phase = "traverse" THEN committed ELSE {}
           /\ countedEver' = (countedEver \/ phase = "traverse")
           /\ UNCHANGED <<resume, pending, identityOK, source, owners, unknown>>
Fault == /\ phase \in {"inventory", "interpret", "traverse"}
         /\ resume' = phase /\ phase' = "paused" /\ pending' = {}
         /\ UNCHANGED <<committed, identityOK, source, owners, countedEver, unknown>>
Crash == /\ phase \in {"inventory", "interpret", "traverse"}
         /\ resume' = phase /\ phase' = "paused" /\ pending' = {} /\ owners' = {}
         /\ UNCHANGED <<committed, identityOK, source, countedEver, unknown>>
Acquire == /\ owners = {}
           /\ \E worker \in Workers: owners' = {worker}
           /\ UNCHANGED <<phase, resume, committed, pending, identityOK, source, countedEver, unknown>>
Retry == /\ phase = "paused" /\ identityOK /\ Cardinality(owners) = 1 /\ phase' = resume
         /\ UNCHANGED <<resume, committed, pending, identityOK, source, owners, countedEver, unknown>>
Changed == /\ phase # "stale" /\ phase' = "stale" /\ identityOK' = FALSE
           /\ pending' = {} /\ UNCHANGED <<resume, committed, source, owners, countedEver, unknown>>
Done == /\ phase \in {"counted", "stale"} /\ UNCHANGED vars
Next == Prepare \/ Commit \/ Advance \/ Fault \/ Crash \/ Acquire \/ Retry \/ Changed \/ Done
Spec == Init /\ [][Next]_vars
TypeOK == /\ phase \in {"inventory", "interpret", "traverse", "paused", "counted", "stale"}
          /\ committed \subseteq Items /\ pending \subseteq Items /\ owners \subseteq Workers
          /\ unknown \subseteq Unsupported
SourceUnchanged == source = Items
CountComplete == phase = "counted" => committed = Items /\ pending = {} /\ identityOK
AtomicCursor == committed \cap pending = {}
StaleStops == ~identityOK => phase = "stale"
SingleOwner == Cardinality(owners) <= 1
UnknownReported == phase = "counted" => unknown = Unsupported
TerminalCount == countedEver => phase \in {"counted", "stale"}
=============================================================================
