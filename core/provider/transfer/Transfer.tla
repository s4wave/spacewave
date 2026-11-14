---- MODULE Transfer ----
\* A Space transfer: Transfer.Execute in transfer.go and the session routine
\* that runs it, StartTransfer in core/resource/session/transfer.go.
\*
\* The person picks a mode and the Spaces to move. Execute scans the source;
\* Merge and Migrate, which delete the source, refuse a pick that leaves a Space
\* out. It decides each picked Space from what the target holds, then moves it
\* or records it as not moved, and continues with the next. Migrate signs the
\* target Spaces with the source's peer key, so it copies that key to the
\* target next. Merge and Migrate then delete each moved Space and the source
\* volume, and the session routine deletes the source session. The routine can
\* stop at any step, by a crash or a failed copy, and the person can start it
\* again, with any mode and pick; nothing a run keeps on the source decides
\* anything, so every run asks the target about every picked Space.
\*
\* The target keeps, under each Space it lists for a transfer, the Space's
\* origin: ADDED when the transfer listed it with an empty genesis, WRITTEN
\* with the source head it wrote. The source head names the replayed World; a
\* target state is unedited while its World is still the one the origin names.
\* A delete on the target removes the origin with the Space. Per Space:
\*
\*   not listed                                 add, copy, hold, check, write
\*   ADDED, target state the empty genesis      copy, hold, check, write
\*   WRITTEN at the source's current head       moved, no write
\*   WRITTEN, source moved, target unedited     copy the newer source over it
\*   anything else                              not moved: no write
\*
\* A Space the source holds as a member of someone else's shared object
\* ("shared") is never copied, since a copy would start a new object without
\* its other members. When the target is a member of the same object, the
\* Space is moved with no write, but only from a local source: a local delete
\* removes this account's copy and nothing of the object, while a cloud delete
\* deletes the object for every member. Otherwise it is not moved.
\*
\* The write is a compare and set on the target state the decision saw, so a
\* write on the target during the copy makes the Space not moved. A run with
\* any Space not moved ends with that list and cleans up nothing. Cleanup
\* checks each Space again just before deleting it; a Space whose source moved
\* since its copy is moved again first.
\*
\* The person may write to the source at any time except during cleanup, and
\* may write to, delete or create a Space on the target at any time. The code
\* checks the source head just before each delete, so a source write between
\* that check and the delete is the one window the model leaves out.
\*
\* The target's collector frees a copied World at any time once neither the
\* stage nor the import root holds it. A stall past the stage's lease ends the
\* stage early. Adding a Space the target does not list releases every root its
\* bucket holds, as a create of an abandoned id does, so the hold follows the
\* add.

EXTENDS Naturals, FiniteSets

Spaces == {"a", "b"}
Modes == {"merge", "migrate", "mirror"}
\* MaxRuns bounds the runs the person starts after a run ends.
MaxRuns == 1
\* MaxEdits bounds the writes to each source Space.
MaxEdits == 1
\* MaxTouches bounds the person's writes, deletes and creates on the target.
MaxTouches == 2
\* MaxFaults bounds the crashes, failures and stage lapses in one behavior, so
\* the model is finite and a transfer that stops faulting can finish.
MaxFaults == 1

\* A target state: none (not listed), the empty genesis a transfer lists, a
\* state written from source version v and edited e times since, or a state
\* the transfer did not write.
None == [k |-> "none", v |-> 0, e |-> 0]
Empty == [k |-> "empty", v |-> 0, e |-> 0]
Src(v) == [k |-> "src", v |-> v, e |-> 0]
Other == [k |-> "other", v |-> 0, e |-> 0]
\* The target is a member of the same shared object as the source.
Member == [k |-> "member", v |-> 0, e |-> 0]
\* An origin: none, ADDED, or WRITTEN at source head h.
NoOrigin == [ph |-> "none", h |-> 0]
Added == [ph |-> "added", h |-> 0]
Written(h) == [ph |-> "written", h |-> h]

VARIABLES
  mode,     \* the requested mode
  picked,   \* the Spaces the person picked in the wizard
  pc,       \* the routine's step; "idle" when no routine runs
  entries,  \* the Spaces this run transfers, from its scan
  cur,      \* the Space this run is moving, or "none"
  step,     \* cur's next step
  expect,   \* the target state cur's write expects to replace
  cv,       \* the source version this run copied for cur
  moved,    \* the Spaces this run moved
  notMoved, \* the Spaces this run left not moved
  staged,   \* the Spaces whose copied World this run's open stage holds
  blocks,   \* the Spaces whose World blocks the target stores
  held,     \* the Spaces whose World the target holds
  tc,       \* each Space's target state
  origin,   \* each Space's origin on the target
  src,      \* the Spaces the source volume holds
  ver,      \* each source Space's head, raised by a source write
  delOk,    \* each deleted source Space was on the target from its head
  lost,     \* a write replaced an edited or foreign target state
  touched,  \* the Spaces the person wrote, deleted or created on the target
  touches,
  runs,
  vol,      \* the source volume exists
  session,  \* the source session exists
  key,      \* the target holds the source's peer key
  faults,
  shared,   \* the source Spaces held as a member of another's shared object
  cloudSrc, \* the source is a cloud account
  alive,    \* the shared objects a second member still reads
  keyLost   \* a Migrate deleted the source volume before the target held its key
run == <<pc, entries, cur, step, expect, cv, moved, notMoved, staged>>
target == <<blocks, held, tc, origin, lost, touched, touches>>
source == <<src, ver, delOk, vol, session, key, shared, cloudSrc, alive, keyLost>>
vars == <<mode, picked, run, target, source, runs, faults>>

Deletes == mode \in {"merge", "migrate"}
Ends == {"done", "notmoved", "refused"}
Listed(s) == tc[s].k # "none"

Init ==
  /\ mode \in Modes
  /\ picked \in SUBSET Spaces \ {{}}
  /\ pc = "idle"
  /\ entries = {}
  /\ cur = "none"
  /\ step = "none"
  /\ expect = None
  /\ cv = 0
  /\ moved = {}
  /\ notMoved = {}
  /\ staged = {}
  \* The target may already list a Space the transfer did not write.
  /\ shared \in SUBSET Spaces
  /\ cloudSrc \in BOOLEAN
  /\ alive = shared
  /\ tc \in [Spaces -> {None, Other, Member}]
  /\ blocks = {s \in Spaces : Listed(s)}
  /\ held = blocks
  /\ origin = [s \in Spaces |-> NoOrigin]
  /\ lost = FALSE
  /\ touched = [s \in Spaces |-> Listed(s)]
  /\ touches = 0
  /\ runs = 0
  /\ src = Spaces
  /\ ver = [s \in Spaces |-> 0]
  /\ delOk = [s \in Spaces |-> TRUE]
  /\ vol = TRUE
  /\ session = TRUE
  /\ key = FALSE
  /\ keyLost = FALSE
  /\ faults = 0

\* Clear this run's memory and go to pc.
Stop(to) ==
  /\ pc' = to /\ entries' = {} /\ cur' = "none" /\ step' = "none"
  /\ expect' = None /\ cv' = 0 /\ moved' = {} /\ notMoved' = {} /\ staged' = {}

\* StartTransfer builds the source from the source session's volume. The model
\* starts a run only while both exist; see Stranded for the code's case.
Start ==
  /\ pc = "idle" /\ session /\ vol
  /\ ~(Deletes /\ src \ picked # {})
  /\ pc' = "spaces"
  /\ entries' = src \cap picked
  /\ UNCHANGED <<mode, picked, cur, step, expect, cv, moved, notMoved, staged, target, source, runs, faults>>

\* Merge and Migrate refuse to start when the pick leaves out a Space, since
\* deleting the volume would delete it.
Refuse ==
  /\ pc = "idle" /\ session /\ vol
  /\ Deletes /\ src \ picked # {}
  /\ Stop("refused")
  /\ UNCHANGED <<mode, picked, target, source, runs, faults>>

\* Take the next Space this run has not decided.
Begin(s) ==
  /\ pc = "spaces" /\ cur = "none" /\ s \in entries \ (moved \cup notMoved)
  /\ cur' = s /\ step' = "decide"
  /\ UNCHANGED <<mode, picked, pc, entries, expect, cv, moved, notMoved, staged, target, source, runs, faults>>

\* Decide the Space from its target state, its origin and the source head.
\* Listing an id the target does not list releases every root of its bucket.
Decide ==
  /\ pc = "spaces" /\ step = "decide"
  /\ LET t == tc[cur]
         o == origin[cur]
     IN CASE cur \in shared ->
               /\ IF t = Member /\ ~cloudSrc
                    THEN moved' = moved \cup {cur} /\ UNCHANGED notMoved
                    ELSE notMoved' = notMoved \cup {cur} /\ UNCHANGED moved
               /\ cur' = "none" /\ step' = "none"
               /\ UNCHANGED <<tc, origin, held, expect>>
          [] ~Listed(cur) ->
               /\ tc' = [tc EXCEPT ![cur] = Empty]
               /\ origin' = [origin EXCEPT ![cur] = Added]
               /\ held' = held \ {cur}
               /\ expect' = Empty /\ step' = "copy"
               /\ UNCHANGED <<cur, moved, notMoved>>
          [] o = Added /\ t = Empty ->
               /\ expect' = t /\ step' = "copy"
               /\ UNCHANGED <<tc, origin, held, cur, moved, notMoved>>
          [] o.ph = "written" /\ o.h = ver[cur] ->
               /\ moved' = moved \cup {cur}
               /\ cur' = "none" /\ step' = "none"
               /\ UNCHANGED <<tc, origin, held, expect, notMoved>>
          [] o.ph = "written" /\ t = Src(o.h) ->
               /\ expect' = t /\ step' = "copy"
               /\ UNCHANGED <<tc, origin, held, cur, moved, notMoved>>
          [] OTHER ->
               /\ notMoved' = notMoved \cup {cur}
               /\ cur' = "none" /\ step' = "none"
               /\ UNCHANGED <<tc, origin, held, expect, moved>>
  /\ UNCHANGED <<mode, picked, pc, entries, cv, staged, blocks, lost, touched, touches, source, runs, faults>>

\* Copy the Space's World at the source head under a new stage, which holds it
\* from its put.
Copy ==
  /\ pc = "spaces" /\ step = "copy"
  /\ blocks' = blocks \cup {cur}
  /\ staged' = {cur}
  /\ cv' = ver[cur]
  /\ step' = "hold"
  /\ UNCHANGED <<mode, picked, pc, entries, cur, expect, moved, notMoved, held, tc, origin, lost, touched, touches, source, runs, faults>>

\* Hold the World under the import root.
Hold ==
  /\ pc = "spaces" /\ step = "hold"
  /\ held' = held \cup {cur}
  /\ step' = "check"
  /\ UNCHANGED <<mode, picked, pc, entries, cur, expect, cv, moved, notMoved, staged, blocks, tc, origin, lost, touched, touches, source, runs, faults>>

\* Check the stage still held the whole copy until the hold; a lapsed stage
\* may have lost it, and the run fails so a retry copies again.
Check ==
  /\ pc = "spaces" /\ step = "check"
  /\ IF cur \in blocks
       THEN /\ step' = "write"
            /\ UNCHANGED <<pc, entries, cur, expect, cv, moved, notMoved, staged>>
       ELSE Stop("idle")
  /\ UNCHANGED <<mode, picked, target, source, runs, faults>>

\* Write the Space's state and its WRITTEN origin, only while the target state
\* is the one the decision saw; otherwise the Space is not moved.
Write ==
  /\ pc = "spaces" /\ step = "write"
  /\ IF tc[cur] = expect
       THEN /\ lost' = (lost \/ tc[cur].e > 0 \/ tc[cur].k = "other")
            /\ tc' = [tc EXCEPT ![cur] = Src(cv)]
            /\ origin' = [origin EXCEPT ![cur] = Written(cv)]
            /\ moved' = moved \cup {cur}
            /\ UNCHANGED notMoved
       ELSE /\ notMoved' = notMoved \cup {cur}
            /\ UNCHANGED <<lost, tc, origin, moved>>
  /\ step' = "release"
  /\ UNCHANGED <<mode, picked, pc, entries, cur, expect, cv, staged, blocks, held, touched, touches, source, runs, faults>>

\* Release the stage.
Release ==
  /\ pc = "spaces" /\ step = "release"
  /\ staged' = {}
  /\ cur' = "none" /\ step' = "none"
  /\ UNCHANGED <<mode, picked, pc, entries, expect, cv, moved, notMoved, target, source, runs, faults>>

\* Every Space is decided: a run with any Space not moved ends there, with
\* nothing cleaned up.
SpacesDone ==
  /\ pc = "spaces" /\ cur = "none" /\ entries \subseteq moved \cup notMoved
  /\ IF notMoved # {}
       THEN /\ pc' = "notmoved"
            /\ UNCHANGED <<entries, cur, step, expect, cv, moved, notMoved, staged>>
       ELSE /\ pc' = CASE mode = "migrate" -> "key" [] Deletes -> "cleanup" [] OTHER -> "done"
            /\ UNCHANGED <<entries, cur, step, expect, cv, moved, notMoved, staged>>
  /\ UNCHANGED <<mode, picked, target, source, runs, faults>>

\* Migrate gives the target the source's peer key while the source holds it.
CopyKey ==
  /\ pc = "key"
  /\ key' = TRUE
  /\ pc' = "cleanup"
  /\ UNCHANGED <<mode, picked, entries, cur, step, expect, cv, moved, notMoved, staged, target, src, ver, delOk, vol, session, runs, faults, shared, cloudSrc, alive, keyLost>>

\* The target holds the Space as written from its current source head, or is
\* a member of the shared object a local source holds.
AtHead(s) ==
  IF s \in shared THEN tc[s] = Member /\ ~cloudSrc ELSE origin[s] = Written(ver[s])

\* Cleanup checks each Space just before deleting it from the source.
DeleteSpace(s) ==
  /\ pc = "cleanup" /\ s \in entries \cap src /\ AtHead(s)
  /\ src' = src \ {s}
  /\ delOk' = [delOk EXCEPT ![s] =
       IF s \in shared THEN tc[s] = Member ELSE tc[s].k = "src" /\ tc[s].v = ver[s]]
  /\ alive' = IF s \in shared /\ cloudSrc THEN alive \ {s} ELSE alive
  /\ UNCHANGED <<mode, picked, run, target, ver, vol, session, key, runs, faults, shared, cloudSrc, keyLost>>

\* A Space whose source moved since its copy goes back to be decided again.
Recheck(s) ==
  /\ pc = "cleanup" /\ s \in entries \cap src /\ ~AtHead(s)
  /\ pc' = "spaces"
  /\ moved' = moved \ {s}
  /\ UNCHANGED <<mode, picked, entries, cur, step, expect, cv, notMoved, staged, target, source, runs, faults>>

\* Then it deletes the source volume, after every Space in it.
DeleteVolume ==
  /\ pc = "cleanup" /\ entries \cap src = {}
  /\ vol' = FALSE /\ src' = {}
  /\ alive' = IF cloudSrc THEN alive \ (src \cap shared) ELSE alive
  /\ keyLost' = (mode = "migrate" /\ ~key)
  /\ pc' = "session"
  /\ UNCHANGED <<mode, picked, entries, cur, step, expect, cv, moved, notMoved, staged, target, ver, delOk, session, key, runs, faults, shared, cloudSrc>>

DeleteSession ==
  /\ pc = "session"
  /\ session' = FALSE
  /\ pc' = "done"
  /\ UNCHANGED <<mode, picked, entries, cur, step, expect, cv, moved, notMoved, staged, target, src, ver, delOk, vol, key, runs, faults, shared, cloudSrc, alive, keyLost>>

\* The routine stops at any step, by a crash, a cancel or a failed copy,
\* losing what it held in memory and ending its stage. The person may start
\* again with another mode and pick.
Fault ==
  /\ pc \notin Ends \cup {"idle"} /\ faults < MaxFaults
  /\ Stop("idle")
  /\ mode' \in Modes
  /\ picked' \in SUBSET Spaces \ {{}}
  /\ faults' = faults + 1
  /\ UNCHANGED <<target, source, runs>>

\* The routine stalls past its stage's lease, and the stage stops holding.
Lapse ==
  /\ staged # {} /\ faults < MaxFaults
  /\ staged' = {}
  /\ faults' = faults + 1
  /\ UNCHANGED <<mode, picked, pc, entries, cur, step, expect, cv, moved, notMoved, target, source, runs>>

\* The target's collector frees a World nothing holds.
Free(s) ==
  /\ s \in blocks \ (staged \cup held)
  /\ blocks' = blocks \ {s}
  /\ UNCHANGED <<mode, picked, run, held, tc, origin, lost, touched, touches, source, runs, faults>>

\* The person writes to a source Space, except during cleanup.
SourceWrite(s) ==
  /\ s \in src /\ ver[s] < MaxEdits /\ pc # "cleanup"
  /\ ver' = [ver EXCEPT ![s] = @ + 1]
  /\ UNCHANGED <<mode, picked, run, target, src, delOk, vol, session, key, runs, faults, shared, cloudSrc, alive, keyLost>>

\* The person writes to a Space the target lists; the target holds its World.
\* A Space the transfer added and has not written is arriving: the target
\* refuses to mount it, so nothing else writes it.
TargetWrite(s) ==
  /\ Listed(s) /\ touches < MaxTouches /\ origin[s] # Added
  /\ tc' = [tc EXCEPT ![s] = CASE @.k = "src" -> [@ EXCEPT !.e = @ + 1]
                                 [] @.k = "member" -> @
                                 [] OTHER -> [Other EXCEPT !.e = 1]]
  /\ held' = held \cup {s} /\ blocks' = blocks \cup {s}
  /\ touched' = [touched EXCEPT ![s] = TRUE] /\ touches' = touches + 1
  /\ UNCHANGED <<mode, picked, run, origin, lost, source, runs, faults>>

\* The person deletes a Space on the target, its origin with it.
TargetDelete(s) ==
  /\ Listed(s) /\ touches < MaxTouches
  /\ tc' = [tc EXCEPT ![s] = None]
  /\ origin' = [origin EXCEPT ![s] = NoOrigin]
  /\ held' = held \ {s}
  /\ touched' = [touched EXCEPT ![s] = TRUE] /\ touches' = touches + 1
  /\ UNCHANGED <<mode, picked, run, blocks, lost, source, runs, faults>>

\* The person creates a Space on the target with an id the target does not
\* list.
TargetCreate(s) ==
  /\ ~Listed(s) /\ touches < MaxTouches
  /\ tc' = [tc EXCEPT ![s] = Other]
  /\ held' = held \cup {s} /\ blocks' = blocks \cup {s}
  /\ touched' = [touched EXCEPT ![s] = TRUE] /\ touches' = touches + 1
  /\ UNCHANGED <<mode, picked, run, origin, lost, source, runs, faults>>

\* After a run ends, the person starts another from the same source to the
\* same target, with any mode and pick.
NewRun ==
  /\ pc \in Ends /\ session /\ vol /\ runs < MaxRuns
  /\ Stop("idle")
  /\ mode' \in Modes
  /\ picked' \in SUBSET Spaces \ {{}}
  /\ runs' = runs + 1
  /\ UNCHANGED <<target, source, faults>>

\* A run stopped after deleting the source volume and before deleting the
\* source session. The model starts no run from it. The code does not refuse
\* one: the local provider reopens the deleted volume empty, with a new peer
\* key, so a retry moves nothing, and a Migrate retry copies that new key over
\* the one the target's Spaces are signed with. That gap is outside this model.
Stranded == pc = "idle" /\ session /\ ~vol

Done == (pc \in Ends \/ Stranded) /\ UNCHANGED vars

Progress ==
  \/ Start \/ Refuse \/ Decide \/ Copy \/ Hold \/ Check \/ Write \/ Release
  \/ SpacesDone \/ CopyKey \/ DeleteVolume \/ DeleteSession
  \/ \E s \in Spaces : Begin(s) \/ DeleteSpace(s) \/ Recheck(s)
Next ==
  \/ Progress \/ Fault \/ Lapse \/ Done \/ NewRun
  \/ \E s \in Spaces :
       Free(s) \/ SourceWrite(s) \/ TargetWrite(s) \/ TargetDelete(s) \/ TargetCreate(s)
Spec == Init /\ [][Next]_vars /\ WF_vars(Progress)

\* No write replaces a target state the person edited or the transfer did not
\* write.
NoEditOverwritten == ~lost
\* Cleanup deletes a source Space only when the target's state of it was
\* written from that source Space at the head it had when deleted.
CleanupAtHead == \A s \in Spaces \ src : delOk[s]
\* An arriving Space holds the empty genesis until the transfer writes it.
ArrivingUntouched == \A s \in Spaces : origin[s] = Added => tc[s] = Empty
\* A Space is left not moved only when the person wrote, deleted or created it
\* on the target, or it is a shared object the transfer must not copy, never
\* for a reason the transfer could resolve itself.
NotMovedJustified == \A s \in notMoved : touched[s] \/ s \in shared
\* A second member of a shared object the source holds keeps reading it.
MembersKeepShared == alive = shared
\* No Space the source holds as a member is ever written on the target.
SharedNotCopied == \A s \in shared : tc[s].k # "src" 
\* A run that ends with a Space not moved keeps that Space and the source
\* volume. Cleanup may already have deleted Spaces it checked at their head.
NotMovedKeepsSource == pc = "notmoved" => notMoved \subseteq src /\ vol
\* The target never stores a written state whose World nothing holds or it
\* lost.
StateHeld == \A s \in Spaces : tc[s].k = "src" => s \in held /\ s \in blocks
\* Migrate deletes the source volume only after the target holds its peer key.
MigrateKeepsKey == ~keyLost
\* Every run ends: done, not moved, refused, or stranded.
Finishes == [](pc \notin Ends => <>(pc \in Ends \/ Stranded))
====
