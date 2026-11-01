---- MODULE RootPublication ----
EXTENDS Naturals
(* volume.go NewVolume supplies writeHeadState to kvtx/block storeTx.Commit.
   Source stages: release claim, commit KV tree, write block graph, rmtx identity
   check, commitFn, setRootRefLocked. The last three hold one lock but callback
   persistence and reopening can fail independently; model them separately.
   SetRootRef may invalidate a writer during block I/O. Model its accepted
   reference as already backed by blocks. A failed callback leaves the durable
   head unchanged. Reopening after successful callback may fail: live and saved
   roots need not match. Restart loads saved root. The no-state-store caller
   has no persistence claim and is excluded; InitVolume's callback only returns
   its new reference. No backend fsync or GC assumptions are proved here.
   One local commit and one external root update suffice for stale-write race. *)
CONSTANTS CheckIdentity, BlocksBeforeHead
VARIABLES phase, valid, blocks, saved, live, external, accepted, locked
vars == <<phase, valid, blocks, saved, live, external, accepted, locked>>
Init == /\ phase = "writing" /\ valid = TRUE /\ blocks = {0, 2}
        /\ saved = 0 /\ live = 0 /\ external = FALSE
        /\ accepted = FALSE /\ locked = FALSE
WriteBlocks == /\ phase = "writing" /\ phase' = "check"
               /\ blocks' = blocks \cup {1}
               /\ UNCHANGED <<valid, saved, live, external, accepted, locked>>
SetRootRef == /\ ~external /\ ~locked
              /\ external' = TRUE /\ valid' = FALSE /\ live' = 2
              /\ UNCHANGED <<phase, blocks, saved, accepted, locked>>
CheckWriter == /\ (phase = "check" \/ (~BlocksBeforeHead /\ phase = "writing"))
               /\ (CheckIdentity => valid)
               /\ phase' = "callback" /\ locked' = TRUE
               /\ UNCHANGED <<valid, blocks, saved, live, external, accepted>>
WriteHeadState == /\ phase = "callback"
                  /\ saved' = 1 /\ phase' = "reopen"
                  /\ accepted' = valid
                  /\ UNCHANGED <<valid, blocks, live, external, locked>>
Reopen == /\ phase = "reopen" /\ phase' = "done" /\ locked' = FALSE
          /\ live' = 1 /\ UNCHANGED <<valid, blocks, saved, external, accepted>>
Fail == /\ phase # "done" /\ phase' = "done" /\ locked' = FALSE
        /\ UNCHANGED <<valid, blocks, saved, live, external, accepted>>
Restart == /\ phase = "done" /\ live' = saved
           /\ UNCHANGED <<phase, valid, blocks, saved, external, accepted, locked>>
Next == WriteBlocks \/ SetRootRef \/ CheckWriter \/ WriteHeadState \/ Reopen \/ Fail \/ Restart \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars
SavedRootHasBlocks == saved \in blocks
LiveRootHasBlocks == live \in blocks
RejectStaleWriter == saved = 1 => accepted
====
