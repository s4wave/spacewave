---- MODULE Transfer ----
\* A Space transfer: Transfer.Execute in transfer.go and the session routine
\* that runs it, StartTransfer in core/resource/session/transfer.go.
\*
\* The person picks a mode and the Spaces to move. Execute scans the source;
\* Merge and Migrate, which delete the source, refuse a pick that leaves a Space
\* out. It moves the picked Spaces one at a time: it copies the Space's World
\* under a stage, adds the Space to the target, holds the World under the
\* target's import root, checks the stage still held the copy, writes the
\* Space's state, then releases the stage and checkpoints the Space. Migrate
\* signs the target Spaces with the source's peer key, so it copies that key to
\* the target next. Merge and Migrate then delete each moved Space and the source
\* volume, which also holds the checkpoint, and the session routine deletes the
\* source session. The routine can stop at any step, by a crash or a failed
\* copy, and the person can start it again; a new run resumes from the
\* checkpoint.
\*
\* The target's collector frees a copied World at any time once neither the
\* stage nor the import root holds it. A stall past the stage's lease ends the
\* stage early. Adding a Space the target does not list releases every root its
\* bucket holds, as a create of an abandoned id does, so the hold follows the
\* add.

EXTENDS Naturals, FiniteSets

Spaces == {"a", "b"}
Modes == {"merge", "migrate", "mirror"}
\* MaxFaults bounds the crashes, failures and stage lapses in one behavior, so
\* the model is finite and a transfer that stops faulting can finish.
MaxFaults == 1

VARIABLES
  mode,     \* the requested mode
  picked,   \* the Spaces the person picked in the wizard
  pc,       \* the routine's step; "idle" when no routine runs
  entries,  \* the Spaces this run transfers, from its scan
  cur,      \* the Space this run is moving, or "none"
  step,     \* cur's next step
  staged,   \* the Spaces whose copied World this run's open stage holds
  blocks,   \* the Spaces whose World blocks the target stores
  listed,   \* the Spaces the target lists
  held,     \* the Spaces whose World the target's import root holds
  tstate,   \* the Spaces whose state the target stores, naming the World
  ckpt,     \* the checkpoint's completed Spaces, stored in the source volume
  src,      \* the Spaces the source volume holds
  vol,      \* the source volume exists
  session,  \* the source session exists
  key,      \* the target holds the source's peer key
  faults
vars == <<mode, picked, pc, entries, cur, step, staged, blocks, listed, held, tstate, ckpt, src, vol, session, key, faults>>
target == <<blocks, listed, held, tstate>>

Deletes == mode \in {"merge", "migrate"}
Ends == {"done", "refused"}

Init ==
  /\ mode \in Modes
  /\ picked \in SUBSET Spaces \ {{}}
  /\ pc = "idle"
  /\ entries = {}
  /\ cur = "none"
  /\ step = "none"
  /\ staged = {}
  /\ blocks = {}
  /\ listed = {}
  /\ held = {}
  /\ tstate = {}
  /\ ckpt = {}
  /\ src = Spaces
  /\ vol = TRUE
  /\ session = TRUE
  /\ key = FALSE
  /\ faults = 0

\* StartTransfer builds the source from the source session's volume, so a run
\* starts only while both exist. Execute reads the checkpoint, then scans.
Start ==
  /\ pc = "idle" /\ session /\ vol
  /\ ~(Deletes /\ src \ picked # {})
  /\ pc' = "spaces"
  /\ entries' = src \cap picked
  /\ UNCHANGED <<mode, picked, cur, step, staged, target, ckpt, src, vol, session, key, faults>>

\* Take the next Space; a Space the checkpoint holds is skipped.
Begin(s) ==
  /\ pc = "spaces" /\ cur = "none" /\ s \in entries \ ckpt
  /\ cur' = s /\ step' = "copy"
  /\ UNCHANGED <<mode, picked, pc, entries, staged, target, ckpt, src, vol, session, key, faults>>

\* Copy the Space's World under a new stage, which holds it from its put.
Copy ==
  /\ pc = "spaces" /\ step = "copy"
  /\ blocks' = blocks \cup {cur}
  /\ staged' = {cur}
  /\ step' = "add"
  /\ UNCHANGED <<mode, picked, pc, entries, cur, listed, held, tstate, ckpt, src, vol, session, key, faults>>

\* Add the Space to the target. A create of an id the target does not list
\* releases every root of its bucket; a listed id, as on a retry, is kept.
Add ==
  /\ pc = "spaces" /\ step = "add"
  /\ held' = IF cur \in listed THEN held ELSE held \ {cur}
  /\ listed' = listed \cup {cur}
  /\ step' = "hold"
  /\ UNCHANGED <<mode, picked, pc, entries, cur, staged, blocks, tstate, ckpt, src, vol, session, key, faults>>

\* Hold the World under the import root.
Hold ==
  /\ pc = "spaces" /\ step = "hold"
  /\ held' = held \cup {cur}
  /\ step' = "check"
  /\ UNCHANGED <<mode, picked, pc, entries, cur, staged, blocks, listed, tstate, ckpt, src, vol, session, key, faults>>

\* Check the stage still held the whole copy until the hold; a lapsed stage
\* may have lost it, and the run fails so a retry copies again.
Check ==
  /\ pc = "spaces" /\ step = "check"
  /\ IF cur \in blocks
       THEN /\ step' = "write"
            /\ UNCHANGED <<pc, entries, cur, staged>>
       ELSE /\ pc' = "idle" /\ entries' = {} /\ cur' = "none" /\ step' = "none" /\ staged' = {}
  /\ UNCHANGED <<mode, picked, target, ckpt, src, vol, session, key, faults>>

\* Write the Space's state, which names the World.
Write ==
  /\ pc = "spaces" /\ step = "write"
  /\ tstate' = tstate \cup {cur}
  /\ step' = "release"
  /\ UNCHANGED <<mode, picked, pc, entries, cur, staged, blocks, listed, held, ckpt, src, vol, session, key, faults>>

\* Release the stage and checkpoint the Space.
Release ==
  /\ pc = "spaces" /\ step = "release"
  /\ staged' = {}
  /\ ckpt' = ckpt \cup {cur}
  /\ cur' = "none" /\ step' = "none"
  /\ UNCHANGED <<mode, picked, pc, entries, target, src, vol, session, key, faults>>

SpacesDone ==
  /\ pc = "spaces" /\ cur = "none" /\ entries \subseteq ckpt
  /\ pc' = CASE mode = "migrate" -> "key" [] Deletes -> "cleanup" [] OTHER -> "finish"
  /\ UNCHANGED <<mode, picked, entries, cur, step, staged, target, ckpt, src, vol, session, key, faults>>

\* Merge and Migrate refuse to start when the pick leaves out a Space, since
\* deleting the volume would delete it.
Refuse ==
  /\ pc = "idle" /\ session /\ vol
  /\ Deletes /\ src \ picked # {}
  /\ pc' = "refused"
  /\ UNCHANGED <<mode, picked, entries, cur, step, staged, target, ckpt, src, vol, session, key, faults>>

\* Migrate gives the target the source's peer key while the source holds it.
CopyKey ==
  /\ pc = "key"
  /\ key' = TRUE
  /\ pc' = "cleanup"
  /\ UNCHANGED <<mode, picked, entries, cur, step, staged, target, ckpt, src, vol, session, faults>>

\* Cleanup deletes each scanned Space from the source, ignoring failures.
DeleteSpace(s) ==
  /\ pc = "cleanup" /\ s \in entries \cap src
  /\ src' = src \ {s}
  /\ UNCHANGED <<mode, picked, pc, entries, cur, step, staged, target, ckpt, vol, session, key, faults>>

\* Then it deletes the source volume, with every Space and the checkpoint in
\* it; a failure is logged and the transfer goes on.
DeleteVolume ==
  /\ pc = "cleanup"
  /\ \/ vol' = FALSE /\ src' = {} /\ ckpt' = {}
     \/ UNCHANGED <<vol, src, ckpt>>
  /\ pc' = "finish"
  /\ UNCHANGED <<mode, picked, entries, cur, step, staged, target, session, key, faults>>

\* Execute deletes the checkpoint and returns; the session routine then
\* deletes the source session in Merge and Migrate.
Finish ==
  /\ pc = "finish"
  /\ ckpt' = IF vol THEN {} ELSE ckpt
  /\ pc' = IF Deletes THEN "session" ELSE "done"
  /\ UNCHANGED <<mode, picked, entries, cur, step, staged, target, src, vol, session, key, faults>>

DeleteSession ==
  /\ pc = "session"
  /\ session' = FALSE
  /\ pc' = "done"
  /\ UNCHANGED <<mode, picked, entries, cur, step, staged, target, ckpt, src, vol, key, faults>>

\* The routine stops at any step, by a crash, a cancel or a failed copy,
\* losing what it held in memory and ending its stage.
Fault ==
  /\ pc \notin Ends \cup {"idle"} /\ faults < MaxFaults
  /\ pc' = "idle"
  /\ entries' = {}
  /\ cur' = "none" /\ step' = "none"
  /\ staged' = {}
  /\ faults' = faults + 1
  /\ UNCHANGED <<mode, picked, target, ckpt, src, vol, session, key>>

\* The routine stalls past its stage's lease, and the stage stops holding.
Lapse ==
  /\ staged # {} /\ faults < MaxFaults
  /\ staged' = {}
  /\ faults' = faults + 1
  /\ UNCHANGED <<mode, picked, pc, entries, cur, step, target, ckpt, src, vol, session, key>>

\* The target's collector frees a World nothing holds.
Free(s) ==
  /\ s \in blocks \ (staged \cup held)
  /\ blocks' = blocks \ {s}
  /\ UNCHANGED <<mode, picked, pc, entries, cur, step, staged, listed, held, tstate, ckpt, src, vol, session, key, faults>>

\* A run stopped after deleting the source volume and before deleting the
\* source session: no run can start without the volume, so that session stays
\* until the person removes it.
Stranded == pc = "idle" /\ session /\ ~vol

Done == (pc \in Ends \/ Stranded) /\ UNCHANGED vars

Progress ==
  \/ Start \/ Refuse \/ Copy \/ Add \/ Hold \/ Check \/ Write \/ Release
  \/ SpacesDone \/ DeleteVolume \/ Finish \/ CopyKey \/ DeleteSession
  \/ \E s \in Spaces : Begin(s) \/ DeleteSpace(s)
Next == Progress \/ Fault \/ Lapse \/ Done \/ \E s \in Spaces : Free(s)
Spec == Init /\ [][Next]_vars /\ WF_vars(Progress)

\* Every Space the person had is in the source, or on the target with its
\* state and the World it names.
NoSpaceLost == \A s \in Spaces : s \in src \/ (s \in tstate /\ s \in blocks)
\* The target never stores a state whose World nothing holds or it lost.
StateHeld == \A s \in tstate : s \in held /\ s \in blocks
\* Mirror copies and never deletes.
MirrorKeepsSource == mode = "mirror" => src = Spaces /\ vol /\ session
\* Migrate deletes the source volume only after the target holds its peer key.
MigrateKeepsKey == mode = "migrate" /\ ~vol => key
\* A transfer finishes, is refused, or is stranded.
Finishes == <>(pc \in Ends \/ Stranded)
====
